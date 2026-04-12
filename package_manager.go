package interpreter

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

func defaultModuleName(dir string) string {
	base := filepath.Base(dir)
	if base == "" || base == "." || base == string(os.PathSeparator) {
		return "spl-app"
	}
	return base
}

func discoverProjectRoot(start string) string {
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

func findNearestModuleFile(startDir, fileName string) (string, error) {
	dir := discoverProjectRoot(startDir)
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

func readModuleManifestFromFile(path string) (*SPLModuleManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var manifest SPLModuleManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("invalid %s: %w", SPLManifestFileName, err)
	}
	if manifest.Module == "" {
		manifest.Module = defaultModuleName(filepath.Dir(path))
	}
	if manifest.Dependencies == nil {
		manifest.Dependencies = map[string]string{}
	}
	return &manifest, nil
}

func readModuleLockFromFile(path string) (*SPLModuleLock, error) {
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

func writeModuleManifest(projectDir string, manifest *SPLModuleManifest) error {
	if manifest == nil {
		return fmt.Errorf("manifest is nil")
	}
	if manifest.Module == "" {
		manifest.Module = defaultModuleName(projectDir)
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

func writeModuleLock(projectDir string, lock *SPLModuleLock) error {
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

func dependencyChecksum(path string) (string, error) {
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

func syncModuleLock(projectDir string) (*SPLModuleLock, error) {
	projectDir = discoverProjectRoot(projectDir)
	manifestPath := filepath.Join(projectDir, SPLManifestFileName)
	manifest, err := readModuleManifestFromFile(manifestPath)
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
		resolved, err := sanitizePath(filepath.Join(projectDir, source))
		if err != nil {
			return nil, fmt.Errorf("dependency %q: %w", alias, err)
		}
		if _, err := os.Stat(resolved); err != nil {
			return nil, fmt.Errorf("dependency %q: %w", alias, err)
		}
		checksum, err := dependencyChecksum(resolved)
		if err != nil {
			return nil, fmt.Errorf("dependency %q checksum failed: %w", alias, err)
		}
		lock.Dependencies[alias] = SPLLockedDependency{
			Source:       source,
			ResolvedPath: resolved,
			Checksum:     checksum,
		}
	}

	if err := writeModuleLock(projectDir, lock); err != nil {
		return nil, err
	}
	return lock, nil
}

func resolveManifestImport(importPath string, env *Environment) (string, bool, error) {
	if importPath == "" || strings.HasPrefix(importPath, ".") || filepath.IsAbs(importPath) {
		return "", false, nil
	}

	startDir := ""
	if env != nil {
		startDir = env.moduleDir
		if startDir == "" {
			startDir = env.sourcePath
		}
	}
	lockPath, err := findNearestModuleFile(startDir, SPLLockFileName)
	if err != nil {
		return "", false, err
	}
	manifestPath, err := findNearestModuleFile(startDir, SPLManifestFileName)
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
		lock, err := readModuleLockFromFile(lockPath)
		if err != nil {
			return "", false, err
		}
		if dep, ok := lock.Dependencies[alias]; ok {
			root = dep.ResolvedPath
		}
	}
	if root == "" && manifestPath != "" {
		manifest, err := readModuleManifestFromFile(manifestPath)
		if err != nil {
			return "", false, err
		}
		if source, ok := manifest.Dependencies[alias]; ok {
			root, err = sanitizePath(filepath.Join(filepath.Dir(manifestPath), source))
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
	resolved, err := sanitizePath(target)
	if err != nil {
		return "", true, err
	}
	return resolved, true, nil
}

func InitModuleManifest(projectDir, moduleName string) (*SPLModuleManifest, error) {
	projectDir = discoverProjectRoot(projectDir)
	if moduleName == "" {
		moduleName = defaultModuleName(projectDir)
	}
	manifest := &SPLModuleManifest{
		Module:       moduleName,
		Dependencies: map[string]string{},
	}
	if err := writeModuleManifest(projectDir, manifest); err != nil {
		return nil, err
	}
	return manifest, nil
}

func SyncModuleLock(projectDir string) (*SPLModuleLock, error) {
	return syncModuleLock(projectDir)
}
