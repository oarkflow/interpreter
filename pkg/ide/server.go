package ide

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// AuthChecker reports whether an incoming request is authenticated.
// pkg/playgroundserver passes in its own existing authManager-backed check
// so pkg/ide doesn't need to depend on that package-specific type. A nil
// AuthChecker means "no auth required" (e.g. PLAYGROUND_AUTH_SECRET unset).
type AuthChecker func(r *http.Request) bool

// Server wires together the Registry, ProcessManager, and ToolingService
// behind the /api/projects/* HTTP surface used by cmd/interpreter's
// --playground mode.
type Server struct {
	Registry  *Registry
	Processes *ProcessManager
	Tooling   *ToolingService
	Cfg       RunnerConfig
	Auth      AuthChecker
}

// NewServer creates a Server rooted at workspaceRoot.
func NewServer(cfg RunnerConfig, workspaceRoot string, auth AuthChecker) (*Server, error) {
	reg, err := NewRegistry(workspaceRoot)
	if err != nil {
		return nil, err
	}
	return &Server{
		Registry:  reg,
		Processes: NewProcessManager(cfg, reg),
		Tooling:   NewToolingService(),
		Cfg:       cfg,
		Auth:      auth,
	}, nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (s *Server) authorized(r *http.Request) bool {
	if s.Auth == nil {
		return true
	}
	return s.Auth(r)
}

func (s *Server) wrap(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.authorized(r) {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return
		}
		h(w, r)
	}
}

func (s *Server) projectOr404(w http.ResponseWriter, r *http.Request) (*Project, bool) {
	id := r.PathValue("id")
	p, ok := s.Registry.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "project not found")
		return nil, false
	}
	return p, true
}

// Routes registers every /api/projects/... endpoint onto mux, using Go's
// stdlib method+wildcard ServeMux patterns (Go 1.22+).
func (s *Server) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/projects", s.wrap(s.handleListProjects))
	mux.HandleFunc("POST /api/projects", s.wrap(s.handleCreateProject))
	mux.HandleFunc("DELETE /api/projects/{id}", s.wrap(s.handleDeleteProject))

	mux.HandleFunc("GET /api/projects/{id}/files", s.wrap(s.handleFileTree))
	mux.HandleFunc("GET /api/projects/{id}/files/{path...}", s.wrap(s.handleReadFile))
	mux.HandleFunc("PUT /api/projects/{id}/files/{path...}", s.wrap(s.handleWriteFile))
	mux.HandleFunc("DELETE /api/projects/{id}/files/{path...}", s.wrap(s.handleDeleteFile))
	mux.HandleFunc("POST /api/projects/{id}/files/rename", s.wrap(s.handleRenameFile))

	mux.HandleFunc("POST /api/projects/{id}/run/start", s.wrap(s.handleRunStart))
	mux.HandleFunc("POST /api/projects/{id}/run/stop", s.wrap(s.handleRunStop))
	mux.HandleFunc("POST /api/projects/{id}/run/restart", s.wrap(s.handleRunRestart))
	mux.HandleFunc("GET /api/projects/{id}/run/status", s.wrap(s.handleRunStatus))

	mux.HandleFunc("GET /api/projects/{id}/logs/stream", s.wrap(s.handleLogStream))

	mux.HandleFunc("POST /api/projects/{id}/tooling/completions", s.wrap(s.handleToolingCompletions))
	mux.HandleFunc("POST /api/projects/{id}/tooling/hover", s.wrap(s.handleToolingHover))
	mux.HandleFunc("POST /api/projects/{id}/tooling/diagnostics", s.wrap(s.handleToolingDiagnostics))
}

// ---------------------------------------------------------------------------
// Project CRUD
// ---------------------------------------------------------------------------

type projectView struct {
	ID           string       `json:"id"`
	Slug         string       `json:"slug"`
	Name         string       `json:"name"`
	ScaffoldKind string       `json:"scaffold_kind"`
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
	Status       ProcessStatus `json:"status"`
}

func (s *Server) view(p *Project) projectView {
	return projectView{
		ID:           p.ID,
		Slug:         p.Slug,
		Name:         p.Name,
		ScaffoldKind: p.ScaffoldKind,
		CreatedAt:    p.CreatedAt,
		UpdatedAt:    p.UpdatedAt,
		Status:       s.Processes.Status(p.ID),
	}
}

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	projects := s.Registry.List()
	views := make([]projectView, 0, len(projects))
	for _, p := range projects {
		views = append(views, s.view(p))
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": views})
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name         string `json:"name"`
		ScaffoldKind string `json:"scaffold_kind"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	kind := strings.TrimSpace(req.ScaffoldKind)
	if kind == "" {
		kind = s.Cfg.ScaffoldKind
	}
	if !ValidScaffoldKind(kind) {
		writeError(w, http.StatusBadRequest, "invalid scaffold_kind")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	project, err := s.Registry.Create(name, kind, func(dir string) error {
		return Scaffold(kind, dir, name)
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"project": s.view(project)})
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	p, ok := s.projectOr404(w, r)
	if !ok {
		return
	}
	_, _ = s.Processes.Stop(p.ID)
	if err := s.Registry.Delete(p.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.Tooling.Drop(p.ID)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---------------------------------------------------------------------------
// Files
// ---------------------------------------------------------------------------

func (s *Server) handleFileTree(w http.ResponseWriter, r *http.Request) {
	p, ok := s.projectOr404(w, r)
	if !ok {
		return
	}
	tree, err := FileTree(s.Registry.ProjectDir(p))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tree": tree})
}

func (s *Server) handleReadFile(w http.ResponseWriter, r *http.Request) {
	p, ok := s.projectOr404(w, r)
	if !ok {
		return
	}
	path := r.PathValue("path")
	content, err := ReadFile(s.Registry.ProjectDir(p), path)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": path, "content": content, "size": len(content)})
}

func (s *Server) handleWriteFile(w http.ResponseWriter, r *http.Request) {
	p, ok := s.projectOr404(w, r)
	if !ok {
		return
	}
	path := r.PathValue("path")
	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	dir := s.Registry.ProjectDir(p)
	if err := WriteFile(dir, path, req.Content); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.Tooling.Touch(p.ID, dir, path, req.Content)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleDeleteFile(w http.ResponseWriter, r *http.Request) {
	p, ok := s.projectOr404(w, r)
	if !ok {
		return
	}
	path := r.PathValue("path")
	dir := s.Registry.ProjectDir(p)
	if err := DeleteFile(dir, path); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.Tooling.Forget(p.ID, dir, path)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleRenameFile(w http.ResponseWriter, r *http.Request) {
	p, ok := s.projectOr404(w, r)
	if !ok {
		return
	}
	var req struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	dir := s.Registry.ProjectDir(p)
	if err := RenameFile(dir, req.From, req.To); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.Tooling.Forget(p.ID, dir, req.From)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---------------------------------------------------------------------------
// Run lifecycle
// ---------------------------------------------------------------------------

func (s *Server) handleRunStart(w http.ResponseWriter, r *http.Request) {
	p, ok := s.projectOr404(w, r)
	if !ok {
		return
	}
	status, err := s.Processes.Start(p)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleRunStop(w http.ResponseWriter, r *http.Request) {
	p, ok := s.projectOr404(w, r)
	if !ok {
		return
	}
	status, err := s.Processes.Stop(p.ID)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleRunRestart(w http.ResponseWriter, r *http.Request) {
	p, ok := s.projectOr404(w, r)
	if !ok {
		return
	}
	status, err := s.Processes.Restart(p)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleRunStatus(w http.ResponseWriter, r *http.Request) {
	p, ok := s.projectOr404(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, s.Processes.Status(p.ID))
}

// ---------------------------------------------------------------------------
// Log streaming (SSE)
// ---------------------------------------------------------------------------

func (s *Server) handleLogStream(w http.ResponseWriter, r *http.Request) {
	p, ok := s.projectOr404(w, r)
	if !ok {
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	backlog, ch, cancel := s.Processes.Subscribe(p.ID)
	defer cancel()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	for _, line := range backlog {
		writeSSELog(w, line)
	}
	flusher.Flush()

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case line := <-ch:
			writeSSELog(w, line)
			flusher.Flush()
		case <-heartbeat.C:
			_, _ = w.Write([]byte(": ping\n\n"))
			flusher.Flush()
		}
	}
}

func writeSSELog(w http.ResponseWriter, line LogLine) {
	payload, err := json.Marshal(line)
	if err != nil {
		return
	}
	_, _ = w.Write([]byte("event: log\ndata: "))
	_, _ = w.Write(payload)
	_, _ = w.Write([]byte("\n\n"))
}

// ---------------------------------------------------------------------------
// Tooling (completions/hover/diagnostics)
// ---------------------------------------------------------------------------

type toolingRequest struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Prefix  string `json:"prefix"`
	Line    int    `json:"line"`
	Col     int    `json:"col"`
}

// resolveBuffer returns the request's unsaved buffer content if provided,
// otherwise reads the file from disk - so completions/hover/diagnostics
// work both while actively editing and for never-yet-touched files.
func (s *Server) resolveBuffer(dir string, req toolingRequest) string {
	if req.Content != "" {
		return req.Content
	}
	content, _ := ReadFile(dir, req.Path)
	return content
}

func decodeToolingRequest(r *http.Request) (toolingRequest, error) {
	var req toolingRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	return req, err
}

func (s *Server) handleToolingCompletions(w http.ResponseWriter, r *http.Request) {
	p, ok := s.projectOr404(w, r)
	if !ok {
		return
	}
	req, err := decodeToolingRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	dir := s.Registry.ProjectDir(p)
	src := s.resolveBuffer(dir, req)
	items := s.Tooling.Completions(p.ID, dir, req.Path, src, req.Prefix)
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handleToolingHover(w http.ResponseWriter, r *http.Request) {
	p, ok := s.projectOr404(w, r)
	if !ok {
		return
	}
	req, err := decodeToolingRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	dir := s.Registry.ProjectDir(p)
	src := s.resolveBuffer(dir, req)
	markdown := s.Tooling.Hover(p.ID, dir, req.Path, src, req.Line, req.Col)
	writeJSON(w, http.StatusOK, map[string]string{"markdown": markdown})
}

func (s *Server) handleToolingDiagnostics(w http.ResponseWriter, r *http.Request) {
	p, ok := s.projectOr404(w, r)
	if !ok {
		return
	}
	req, err := decodeToolingRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	dir := s.Registry.ProjectDir(p)
	src := s.resolveBuffer(dir, req)
	diags := s.Tooling.Diagnostics(p.ID, dir, req.Path, src)
	writeJSON(w, http.StatusOK, map[string]any{"diagnostics": diags})
}
