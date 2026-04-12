package pkgmgr

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	SPLManifestFileName = "spl.mod"
	SPLLockFileName     = "spl.lock"
)

// SanitizePathFn is a function variable that must be set by the host package
// to provide path sanitisation. It defaults to filepath.Abs.
var SanitizePathFn func(userPath string) (string, error) = func(userPath string) (string, error) {
	return filepath.Abs(userPath)
}

type SPLModuleManifest struct {
	Module       string            `json:"module"`
	Dependencies map[string]string `json:"dependencies,omitempty"`
}

type SPLLockedDependency struct {
	Source       string `json:"source"`
	ResolvedPath string `json:"resolved_path"`
	Checksum     string `json:"checksum"`
}

type SPLModuleLock struct {
	Module       string                         `json:"module"`
	Dependencies map[string]SPLLockedDependency `json:"dependencies,omitempty"`
}

func DefaultModuleName(dir string) string {
	base := filepath.Base(dir)
	if base == "" || base == "." || base == string(os.PathSeparator) {
		return "spl-app"
	}
	return base
}

func DiscoverProjectRoot(start string) string {
	if start == "" {
		if cwd, err := os.Getwd(); err == nil {
			start = cwd
		}
	}
	abs, err := filepath.Abs(start)
	if err != nil {
		return start
	}
	if info, err := os.Stat(abs); err == nil && !info.IsDir() {
		abs = filepath.Dir(abs)
	}
	return abs
}

func FindNearestModuleFile(startDir, fileName string) (string, error) {
	dir := DiscoverProjectRoot(startDir)
	for {
		candidate := filepath.Join(dir, fileName)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil
		}
		dir = parent
	}
}

func ReadModuleManifestFromFile(path string) (*SPLModuleManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var manifest SPLModuleManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("invalid %s: %w", SPLManifestFileName, err)
	}
	if manifest.Module == "" {
		manifest.Module = DefaultModuleName(filepath.Dir(path))
	}
	if manifest.Dependencies == nil {
		manifest.Dependencies = map[string]string{}
	}
	return &manifest, nil
}

func ReadModuleLockFromFile(path string) (*SPLModuleLock, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var lock SPLModuleLock
	if err := json.Unmarshal(data, &lock); err != nil {
		return nil, fmt.Errorf("invalid %s: %w", SPLLockFileName, err)
	}
	if lock.Dependencies == nil {
		lock.Dependencies = map[string]SPLLockedDependency{}
	}
	return &lock, nil
}

func WriteModuleManifest(projectDir string, manifest *SPLModuleManifest) error {
	if manifest == nil {
		return fmt.Errorf("manifest is nil")
	}
	if manifest.Module == "" {
		manifest.Module = DefaultModuleName(projectDir)
	}
	if manifest.Dependencies == nil {
		manifest.Dependencies = map[string]string{}
	}
	payload, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	return os.WriteFile(filepath.Join(projectDir, SPLManifestFileName), payload, 0o600)
}

func WriteModuleLock(projectDir string, lock *SPLModuleLock) error {
	if lock == nil {
		return fmt.Errorf("lock is nil")
	}
	if lock.Dependencies == nil {
		lock.Dependencies = map[string]SPLLockedDependency{}
	}
	payload, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	return os.WriteFile(filepath.Join(projectDir, SPLLockFileName), payload, 0o600)
}

func DependencyChecksum(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	hasher := sha256.New()
	if !info.IsDir() {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		hasher.Write([]byte(filepath.Base(path)))
		hasher.Write(data)
		return hex.EncodeToString(hasher.Sum(nil)), nil
	}

	var entries []string
	if err := filepath.WalkDir(path, func(current string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		entries = append(entries, current)
		return nil
	}); err != nil {
		return "", err
	}
	sort.Strings(entries)
	for _, entry := range entries {
		rel, err := filepath.Rel(path, entry)
		if err != nil {
			return "", err
		}
		data, err := os.ReadFile(entry)
		if err != nil {
			return "", err
		}
		hasher.Write([]byte(filepath.ToSlash(rel)))
		hasher.Write(data)
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func SyncModuleLock(projectDir string) (*SPLModuleLock, error) {
	projectDir = DiscoverProjectRoot(projectDir)
	manifestPath := filepath.Join(projectDir, SPLManifestFileName)
	manifest, err := ReadModuleManifestFromFile(manifestPath)
	if err != nil {
		return nil, err
	}

	lock := &SPLModuleLock{
		Module:       manifest.Module,
		Dependencies: make(map[string]SPLLockedDependency, len(manifest.Dependencies)),
	}

	aliases := make([]string, 0, len(manifest.Dependencies))
	for alias := range manifest.Dependencies {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)

	for _, alias := range aliases {
		source := strings.TrimSpace(manifest.Dependencies[alias])
		if alias == "" {
			return nil, fmt.Errorf("dependency aliases must not be empty")
		}
		if source == "" {
			return nil, fmt.Errorf("dependency %q has an empty source path", alias)
		}
		resolved, err := SanitizePathFn(filepath.Join(projectDir, source))
		if err != nil {
			return nil, fmt.Errorf("dependency %q: %w", alias, err)
		}
		if _, err := os.Stat(resolved); err != nil {
			return nil, fmt.Errorf("dependency %q: %w", alias, err)
		}
		checksum, err := DependencyChecksum(resolved)
		if err != nil {
			return nil, fmt.Errorf("dependency %q checksum failed: %w", alias, err)
		}
		lock.Dependencies[alias] = SPLLockedDependency{
			Source:       source,
			ResolvedPath: resolved,
			Checksum:     checksum,
		}
	}

	if err := WriteModuleLock(projectDir, lock); err != nil {
		return nil, err
	}
	return lock, nil
}

// ResolveManifestImport resolves an import path using the module manifest/lock.
// moduleDir and sourcePath are used to locate the nearest manifest.
// Returns (resolvedPath, matched, error).
func ResolveManifestImport(importPath, moduleDir, sourcePath string) (string, bool, error) {
	if importPath == "" || strings.HasPrefix(importPath, ".") || filepath.IsAbs(importPath) {
		return "", false, nil
	}

	startDir := moduleDir
	if startDir == "" {
		startDir = sourcePath
	}
	lockPath, err := FindNearestModuleFile(startDir, SPLLockFileName)
	if err != nil {
		return "", false, err
	}
	manifestPath, err := FindNearestModuleFile(startDir, SPLManifestFileName)
	if err != nil {
		return "", false, err
	}
	if lockPath == "" && manifestPath == "" {
		return "", false, nil
	}

	alias := importPath
	rest := ""
	if idx := strings.IndexRune(importPath, '/'); idx >= 0 {
		alias = importPath[:idx]
		rest = importPath[idx+1:]
	}

	var root string
	if lockPath != "" {
		lock, err := ReadModuleLockFromFile(lockPath)
		if err != nil {
			return "", false, err
		}
		if dep, ok := lock.Dependencies[alias]; ok {
			root = dep.ResolvedPath
		}
	}
	if root == "" && manifestPath != "" {
		manifest, err := ReadModuleManifestFromFile(manifestPath)
		if err != nil {
			return "", false, err
		}
		if source, ok := manifest.Dependencies[alias]; ok {
			root, err = SanitizePathFn(filepath.Join(filepath.Dir(manifestPath), source))
			if err != nil {
				return "", false, err
			}
		}
	}
	if root == "" {
		return "", false, nil
	}

	target := root
	if rest != "" {
		target = filepath.Join(root, rest)
	}
	resolved, err := SanitizePathFn(target)
	if err != nil {
		return "", true, err
	}
	return resolved, true, nil
}

// InitModuleManifest creates a new spl.mod manifest in the given project
// directory and writes it to disk.
func InitModuleManifest(projectDir, moduleName string) (*SPLModuleManifest, error) {
	projectDir = DiscoverProjectRoot(projectDir)
	if moduleName == "" {
		moduleName = DefaultModuleName(projectDir)
	}
	manifest := &SPLModuleManifest{
		Module:       moduleName,
		Dependencies: map[string]string{},
	}
	if err := WriteModuleManifest(projectDir, manifest); err != nil {
		return nil, err
	}
	return manifest, nil
}
