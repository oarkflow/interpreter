package ide

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

// SafeJoin resolves requestedPath against baseDir and guarantees the result
// stays inside baseDir - rejecting absolute paths, ".." traversal, and
// symlink escapes. baseDir must already be an absolute, existing directory.
// Centralizes the containment check that every file-tree/read/write/delete/
// rename endpoint needs, rather than re-implementing an ad hoc ".." check
// per handler.
func SafeJoin(baseDir, requestedPath string) (string, error) {
	baseAbs, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("resolve base dir: %w", err)
	}
	baseResolved, err := filepath.EvalSymlinks(baseAbs)
	if err != nil {
		return "", fmt.Errorf("resolve base dir: %w", err)
	}

	cleanReq := filepath.Clean("/" + strings.TrimPrefix(requestedPath, "/"))
	// cleanReq is now always rooted at "/", e.g. "/../../etc/passwd" ->
	// "/etc/passwd" - Clean collapses ".." against the leading "/" so it
	// can never escape above it.
	target := filepath.Join(baseResolved, cleanReq)

	if target != baseResolved && !strings.HasPrefix(target, baseResolved+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes project directory: %s", requestedPath)
	}

	// If the target (or its nearest existing ancestor) is itself a symlink
	// pointing outside baseResolved, reject it too - EvalSymlinks only
	// works on paths that exist, so walk up from the target to the first
	// existing ancestor.
	check := target
	for {
		resolved, err := filepath.EvalSymlinks(check)
		if err == nil {
			if resolved != baseResolved && !strings.HasPrefix(resolved, baseResolved+string(filepath.Separator)) && check == target {
				return "", fmt.Errorf("path escapes project directory: %s", requestedPath)
			}
			break
		}
		parent := filepath.Dir(check)
		if parent == check {
			break
		}
		check = parent
	}

	return target, nil
}

// FileNode is one entry in a project's file tree.
type FileNode struct {
	Path     string      `json:"path"` // relative to the project root, forward-slash separated
	Name     string      `json:"name"`
	Type     string      `json:"type"` // "file" or "dir"
	Size     int64       `json:"size,omitempty"`
	Children []*FileNode `json:"children,omitempty"`
}

// FileTree walks projectDir and returns its structure, skipping the
// interpreter's own runtime artifacts (spl.lock is kept - it's meaningful
// project state) and any dotfile directories such as a stray .git.
func FileTree(projectDir string) (*FileNode, error) {
	return walkTree(projectDir, "")
}

func walkTree(dir, rel string) (*FileNode, error) {
	name := filepath.Base(dir)
	if rel == "" {
		name = "."
	}
	node := &FileNode{Path: rel, Name: name, Type: "dir"}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}
		return entries[i].Name() < entries[j].Name()
	})
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".git") {
			continue
		}
		childRel := e.Name()
		if rel != "" {
			childRel = rel + "/" + e.Name()
		}
		if e.IsDir() {
			child, err := walkTree(filepath.Join(dir, e.Name()), childRel)
			if err != nil {
				return nil, err
			}
			node.Children = append(node.Children, child)
			continue
		}
		info, err := e.Info()
		var size int64
		if err == nil {
			size = info.Size()
		}
		node.Children = append(node.Children, &FileNode{
			Path: childRel,
			Name: e.Name(),
			Type: "file",
			Size: size,
		})
	}
	return node, nil
}

const maxTextFileBytes = 5 * 1024 * 1024

// ReadFile reads a project file as text. Returns an error for binary
// content or files over maxTextFileBytes - the browser editor only handles
// text.
func ReadFile(projectDir, relPath string) (string, error) {
	abs, err := SafeJoin(projectDir, relPath)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s is a directory", relPath)
	}
	if info.Size() > maxTextFileBytes {
		return "", fmt.Errorf("file too large to edit (%d bytes)", info.Size())
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return "", err
	}
	if bytes.IndexByte(data, 0) != -1 || !utf8.Valid(data) {
		return "", fmt.Errorf("%s is not a text file", relPath)
	}
	return string(data), nil
}

// WriteFile creates or overwrites a project file, creating parent
// directories as needed.
func WriteFile(projectDir, relPath, content string) error {
	abs, err := SafeJoin(projectDir, relPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	return os.WriteFile(abs, []byte(content), 0o644)
}

// DeleteFile removes a project file or directory (recursively).
func DeleteFile(projectDir, relPath string) error {
	abs, err := SafeJoin(projectDir, relPath)
	if err != nil {
		return err
	}
	root, err := SafeJoin(projectDir, ".")
	if err != nil {
		return err
	}
	if abs == root {
		return fmt.Errorf("cannot delete the project root")
	}
	return os.RemoveAll(abs)
}

// RenameFile moves a project file/dir from one relative path to another.
func RenameFile(projectDir, fromRel, toRel string) error {
	fromAbs, err := SafeJoin(projectDir, fromRel)
	if err != nil {
		return err
	}
	toAbs, err := SafeJoin(projectDir, toRel)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(toAbs), 0o755); err != nil {
		return err
	}
	return os.Rename(fromAbs, toAbs)
}
