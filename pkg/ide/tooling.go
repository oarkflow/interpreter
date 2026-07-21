package ide

import (
	"sync"

	"github.com/oarkflow/interpreter/pkg/tooling"
)

// ToolingService caches one tooling.WorkspaceIndex per project so the
// browser editor's completion/hover/diagnostics requests get real,
// import-aware, multi-file results - the same engine cmd/spltool's LSP
// server (used by the VS Code extension) already relies on, just exposed
// over HTTP instead of JSON-RPC.
type ToolingService struct {
	mu      sync.Mutex
	indexes map[string]*tooling.WorkspaceIndex
}

func NewToolingService() *ToolingService {
	return &ToolingService{indexes: make(map[string]*tooling.WorkspaceIndex)}
}

func (s *ToolingService) indexFor(projectID, projectDir string) *tooling.WorkspaceIndex {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, ok := s.indexes[projectID]
	if !ok {
		idx = tooling.NewWorkspaceIndex(projectDir)
		s.indexes[projectID] = idx
	}
	return idx
}

// Touch refreshes a project's index for one file's current (possibly
// unsaved) contents - called on every file write and on every completion/
// hover/diagnostics request that carries an in-editor buffer.
func (s *ToolingService) Touch(projectID, projectDir, path, src string) {
	s.indexFor(projectID, projectDir).Update(path, src)
}

// Forget removes a file from a project's index (e.g. after a delete/rename).
func (s *ToolingService) Forget(projectID, projectDir, path string) {
	s.indexFor(projectID, projectDir).Remove(path)
}

// Drop discards a project's entire tooling index, e.g. when the project
// itself is deleted.
func (s *ToolingService) Drop(projectID string) {
	s.mu.Lock()
	delete(s.indexes, projectID)
	s.mu.Unlock()
}

func (s *ToolingService) Completions(projectID, projectDir, path, src, prefix string) []tooling.CompletionItem {
	idx := s.indexFor(projectID, projectDir)
	idx.Update(path, src)
	return tooling.WorkspaceCompletionItems(idx, path, src, prefix)
}

func (s *ToolingService) Hover(projectID, projectDir, path, src string, line, col int) string {
	idx := s.indexFor(projectID, projectDir)
	idx.Update(path, src)
	return tooling.HoverMarkdown(idx, path, src, line, col)
}

func (s *ToolingService) Diagnostics(projectID, projectDir, path, src string) []tooling.Diagnostic {
	idx := s.indexFor(projectID, projectDir)
	idx.Update(path, src)
	return tooling.CheckSource(path, src).Diagnostics
}
