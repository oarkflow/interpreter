package interpreter

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ── File Watcher ───────────────────────────────────────────────────

type FileWatcher struct {
	mu       sync.RWMutex
	watches  map[string]*WatchEntry
	nextID   int
	running  bool
	stopCh   chan struct{}
	hotMu    sync.RWMutex
	hotFiles map[string]time.Time
}

type WatchEntry struct {
	ID       string
	Path     string
	Pattern  string // glob pattern
	Handler  Object
	Env      *Environment
	Debounce time.Duration
	lastMod  map[string]time.Time
	lastFire time.Time
}

var globalWatcher = &FileWatcher{
	watches:  make(map[string]*WatchEntry),
	stopCh:   make(chan struct{}),
	hotFiles: make(map[string]time.Time),
}

func (fw *FileWatcher) start() {
	fw.mu.Lock()
	if fw.running {
		fw.mu.Unlock()
		return
	}
	fw.running = true
	fw.mu.Unlock()
	go fw.loop()
}

func (fw *FileWatcher) loop() {
	ticker := time.NewTicker(500 * time.Millisecond) // poll every 500ms
	defer ticker.Stop()
	for {
		select {
		case <-fw.stopCh:
			return
		case <-ticker.C:
			fw.check()
		}
	}
}

func (fw *FileWatcher) check() {
	fw.mu.RLock()
	entries := make([]*WatchEntry, 0, len(fw.watches))
	for _, e := range fw.watches {
		entries = append(entries, e)
	}
	fw.mu.RUnlock()

	now := time.Now()
	for _, entry := range entries {
		if now.Sub(entry.lastFire) < entry.Debounce {
			continue
		}
		changed := fw.detectChanges(entry)
		if len(changed) > 0 {
			entry.lastFire = now
			go fw.fireHandler(entry, changed)
		}
	}
}

func (fw *FileWatcher) detectChanges(entry *WatchEntry) []string {
	var changed []string
	var files []string

	if entry.Pattern != "" {
		matches, err := filepath.Glob(filepath.Join(entry.Path, entry.Pattern))
		if err == nil {
			files = matches
		}
	} else {
		// Watch the single file or directory
		info, err := os.Stat(entry.Path)
		if err != nil {
			return nil
		}
		if info.IsDir() {
			dirEntries, err := os.ReadDir(entry.Path)
			if err != nil {
				return nil
			}
			for _, de := range dirEntries {
				if !de.IsDir() {
					files = append(files, filepath.Join(entry.Path, de.Name()))
				}
			}
		} else {
			files = []string{entry.Path}
		}
	}

	for _, f := range files {
		info, err := os.Stat(f)
		if err != nil {
			continue
		}
		modTime := info.ModTime()
		if prev, ok := entry.lastMod[f]; ok {
			if modTime.After(prev) {
				changed = append(changed, f)
			}
		}
		entry.lastMod[f] = modTime
		fw.hotMu.Lock()
		fw.hotFiles[f] = modTime
		fw.hotMu.Unlock()
	}

	return changed
}

func (fw *FileWatcher) fireHandler(entry *WatchEntry, changedFiles []string) {
	elems := make([]Object, len(changedFiles))
	for i, f := range changedFiles {
		elems[i] = &String{Value: f}
	}
	filesArg := &Array{Elements: elems}

	switch fn := entry.Handler.(type) {
	case *Function:
		args := []Object{filesArg}
		extEnv := extendFunctionEnv(fn, args, entry.Env, nil)
		Eval(fn.Body, extEnv)
	case *Builtin:
		if fn.FnWithEnv != nil {
			fn.FnWithEnv(entry.Env, filesArg)
		} else {
			fn.Fn(filesArg)
		}
	}
}

func (fw *FileWatcher) addWatch(entry *WatchEntry) string {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	fw.nextID++
	entry.ID = fmt.Sprintf("watch_%d", fw.nextID)
	entry.lastMod = make(map[string]time.Time)
	// Initialize mod times so we don't fire on first check
	fw.initModTimes(entry)
	fw.watches[entry.ID] = entry
	return entry.ID
}

func (fw *FileWatcher) initModTimes(entry *WatchEntry) {
	var files []string
	if entry.Pattern != "" {
		matches, err := filepath.Glob(filepath.Join(entry.Path, entry.Pattern))
		if err == nil {
			files = matches
		}
	} else {
		info, err := os.Stat(entry.Path)
		if err != nil {
			return
		}
		if info.IsDir() {
			dirEntries, err := os.ReadDir(entry.Path)
			if err != nil {
				return
			}
			for _, de := range dirEntries {
				if !de.IsDir() {
					files = append(files, filepath.Join(entry.Path, de.Name()))
				}
			}
		} else {
			files = []string{entry.Path}
		}
	}
	for _, f := range files {
		info, err := os.Stat(f)
		if err == nil {
			entry.lastMod[f] = info.ModTime()
		}
	}
}

func (fw *FileWatcher) removeWatch(id string) bool {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	if _, ok := fw.watches[id]; ok {
		delete(fw.watches, id)
		return true
	}
	return false
}

// ── Module Cache Invalidation ──────────────────────────────────────

var moduleCache = struct {
	mu    sync.RWMutex
	cache map[string]*Environment
}{
	cache: make(map[string]*Environment),
}

func invalidateModuleCache(path string) {
	moduleCache.mu.Lock()
	defer moduleCache.mu.Unlock()
	// Invalidate exact match and any path that contains this
	for key := range moduleCache.cache {
		if key == path || strings.HasSuffix(key, "/"+filepath.Base(path)) {
			delete(moduleCache.cache, key)
		}
	}
}

// ── Builtins ───────────────────────────────────────────────────────

func init() {
	registerBuiltins(map[string]*Builtin{
		"watch":      {FnWithEnv: builtinWatch},
		"unwatch":    {Fn: builtinUnwatch},
		"hot_reload": {FnWithEnv: builtinHotReload},
	})
}

// watch(path, handler) or watch(path, pattern, handler) or watch(path, options)
func builtinWatch(env *Environment, args ...Object) Object {
	if len(args) < 2 {
		return newError("watch() requires at least (path, handler)")
	}
	pathStr, ok := args[0].(*String)
	if !ok {
		return newError("watch() first argument must be a string path")
	}

	entry := &WatchEntry{
		Path:     pathStr.Value,
		Env:      env,
		Debounce: 200 * time.Millisecond,
	}

	switch v := args[1].(type) {
	case *Function:
		entry.Handler = v
	case *Builtin:
		entry.Handler = v
	case *String:
		// pattern, then handler
		entry.Pattern = v.Value
		if len(args) >= 3 {
			entry.Handler = args[2]
		} else {
			return newError("watch() with pattern requires a handler")
		}
	case *Hash:
		// Options hash
		for _, pair := range v.Pairs {
			key := pair.Key.Inspect()
			switch key {
			case "pattern":
				if s, ok := pair.Value.(*String); ok {
					entry.Pattern = s.Value
				}
			case "handler":
				entry.Handler = pair.Value
			case "debounce":
				if ms, ok := pair.Value.(*Integer); ok {
					entry.Debounce = time.Duration(ms.Value) * time.Millisecond
				}
			}
		}
		if entry.Handler == nil {
			return newError("watch() options must include a handler")
		}
	default:
		return newError("watch() second argument must be a function, pattern, or options hash")
	}

	globalWatcher.start()
	id := globalWatcher.addWatch(entry)
	return &String{Value: id}
}

// unwatch(watch_id)
func builtinUnwatch(args ...Object) Object {
	if len(args) < 1 {
		return newError("unwatch() requires a watch ID")
	}
	id, ok := args[0].(*String)
	if !ok {
		return newError("unwatch() argument must be a string")
	}
	if globalWatcher.removeWatch(id.Value) {
		return TRUE
	}
	return FALSE
}

// hot_reload(path) — watches a file and re-evaluates it on change
func builtinHotReload(env *Environment, args ...Object) Object {
	if len(args) < 1 {
		return newError("hot_reload() requires a file path")
	}
	pathStr, ok := args[0].(*String)
	if !ok {
		return newError("hot_reload() argument must be a string path")
	}

	absPath, err := filepath.Abs(pathStr.Value)
	if err != nil {
		return newError("hot_reload() invalid path: %s", err)
	}

	reloadHandler := &Builtin{
		FnWithEnv: func(e *Environment, a ...Object) Object {
			invalidateModuleCache(absPath)
			data, err := os.ReadFile(absPath)
			if err != nil {
				fmt.Printf("[hot_reload] error reading %s: %s\n", absPath, err)
				return NULL
			}
			fmt.Printf("[hot_reload] reloading %s\n", absPath)
			// Re-parse and eval
			l := NewLexer(string(data))
			p := NewParser(l)
			program := p.ParseProgram()
			if len(p.Errors()) > 0 {
				fmt.Printf("[hot_reload] parse errors: %v\n", p.Errors())
				return NULL
			}
			result := Eval(program, env)
			if isError(result) {
				fmt.Printf("[hot_reload] eval error: %s\n", result.Inspect())
			}
			dispatchHotReloadHooks(absPath)
			return NULL
		},
	}

	entry := &WatchEntry{
		Path:     filepath.Dir(absPath),
		Pattern:  filepath.Base(absPath),
		Handler:  reloadHandler,
		Env:      env,
		Debounce: 300 * time.Millisecond,
	}

	globalWatcher.start()
	id := globalWatcher.addWatch(entry)
	return &String{Value: id}
}
