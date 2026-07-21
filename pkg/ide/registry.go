// Package ide implements the project/file/process-management backend for
// the browser playground's "Projects" mode: scaffolding, editing, and
// running multi-file SPL applications shaped like examples/app, as real
// directories on disk. It is imported by cmd/interpreter's --playground
// mode, parameterized by a RunnerConfig supplying the interpreter command
// and scaffold kind.
package ide

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Project is one IDE-managed SPL application, scaffolded under a
// Registry's workspace root.
type Project struct {
	ID           string    `json:"id"`
	Slug         string    `json:"slug"`
	Name         string    `json:"name"`
	ScaffoldKind string    `json:"scaffold_kind"`
	Dir          string    `json:"dir"` // relative to the workspace root
	Port         int       `json:"port"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type projectIndex struct {
	Projects []*Project `json:"projects"`
}

// Registry tracks project metadata in a single JSON index file at
// <workspaceRoot>/.spl-ide/projects.json. Actual project files live at
// <workspaceRoot>/<project.Dir>/...
type Registry struct {
	mu       sync.RWMutex
	root     string
	indexDir string
	indexPath string
	projects map[string]*Project
	order    []string
}

// NewRegistry loads (or creates) the project index under workspaceRoot,
// which is created if it does not already exist.
func NewRegistry(workspaceRoot string) (*Registry, error) {
	abs, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace root: %w", err)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, fmt.Errorf("create workspace root: %w", err)
	}
	indexDir := filepath.Join(abs, ".spl-ide")
	if err := os.MkdirAll(indexDir, 0o755); err != nil {
		return nil, fmt.Errorf("create index dir: %w", err)
	}
	r := &Registry{
		root:      abs,
		indexDir:  indexDir,
		indexPath: filepath.Join(indexDir, "projects.json"),
		projects:  make(map[string]*Project),
	}
	if err := r.load(); err != nil {
		return nil, err
	}
	return r, nil
}

// Root returns the absolute workspace root directory.
func (r *Registry) Root() string {
	return r.root
}

func (r *Registry) load() error {
	data, err := os.ReadFile(r.indexPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read project index: %w", err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil
	}
	var idx projectIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return fmt.Errorf("parse project index: %w", err)
	}
	for _, p := range idx.Projects {
		if p == nil || p.ID == "" {
			continue
		}
		r.projects[p.ID] = p
		r.order = append(r.order, p.ID)
	}
	return nil
}

// saveLocked persists the index atomically (write to a temp file, then
// rename) so a crash mid-write can't leave a torn/corrupt index. Caller
// must hold r.mu (read or write lock doesn't matter for the read side, but
// mutating callers must hold the write lock).
func (r *Registry) saveLocked() error {
	idx := projectIndex{Projects: make([]*Project, 0, len(r.order))}
	for _, id := range r.order {
		if p, ok := r.projects[id]; ok {
			idx.Projects = append(idx.Projects, p)
		}
	}
	payload, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	tmp := r.indexPath + ".tmp"
	if err := os.WriteFile(tmp, payload, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, r.indexPath)
}

// List returns all projects in creation order.
func (r *Registry) List() []*Project {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Project, 0, len(r.order))
	for _, id := range r.order {
		if p, ok := r.projects[id]; ok {
			cp := *p
			out = append(out, &cp)
		}
	}
	return out
}

// Get returns a copy of the project with the given id.
func (r *Registry) Get(id string) (*Project, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.projects[id]
	if !ok {
		return nil, false
	}
	cp := *p
	return &cp, true
}

var slugInvalid = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = slugInvalid.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "project"
	}
	return s
}

func newID() string {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

// Create registers a new project with a unique slug/dir and calls
// scaffold(absoluteProjectDir) to populate its files. If scaffold returns an
// error, the partially-created directory and registry entry are removed.
func (r *Registry) Create(name, scaffoldKind string, scaffold func(dir string) error) (*Project, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	base := slugify(name)
	slug := base
	dir := base
	for i := 2; ; i++ {
		if _, taken := r.dirTakenLocked(dir); !taken {
			break
		}
		slug = fmt.Sprintf("%s-%d", base, i)
		dir = slug
	}

	projectDir := filepath.Join(r.root, dir)
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		return nil, fmt.Errorf("create project dir: %w", err)
	}
	if err := scaffold(projectDir); err != nil {
		_ = os.RemoveAll(projectDir)
		return nil, err
	}

	now := time.Now().UTC()
	p := &Project{
		ID:           newID(),
		Slug:         slug,
		Name:         strings.TrimSpace(name),
		ScaffoldKind: scaffoldKind,
		Dir:          dir,
		Port:         0,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if p.Name == "" {
		p.Name = slug
	}
	r.projects[p.ID] = p
	r.order = append(r.order, p.ID)
	if err := r.saveLocked(); err != nil {
		delete(r.projects, p.ID)
		r.order = r.order[:len(r.order)-1]
		_ = os.RemoveAll(projectDir)
		return nil, err
	}
	cp := *p
	return &cp, nil
}

func (r *Registry) dirTakenLocked(dir string) (*Project, bool) {
	for _, p := range r.projects {
		if p.Dir == dir {
			return p, true
		}
	}
	return nil, false
}

// Delete removes a project's registry entry and its on-disk directory.
func (r *Registry) Delete(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.projects[id]
	if !ok {
		return fmt.Errorf("project %q not found", id)
	}
	dir := filepath.Join(r.root, p.Dir)
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("remove project dir: %w", err)
	}
	delete(r.projects, id)
	for i, oid := range r.order {
		if oid == id {
			r.order = append(r.order[:i], r.order[i+1:]...)
			break
		}
	}
	return r.saveLocked()
}

// UpdatePort persists the last-assigned port for a project.
func (r *Registry) UpdatePort(id string, port int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.projects[id]
	if !ok {
		return fmt.Errorf("project %q not found", id)
	}
	p.Port = port
	p.UpdatedAt = time.Now().UTC()
	return r.saveLocked()
}

// ProjectDir returns the absolute directory for a project.
func (r *Registry) ProjectDir(p *Project) string {
	return filepath.Join(r.root, p.Dir)
}
