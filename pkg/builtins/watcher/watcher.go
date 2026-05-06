package watcher

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/lexer"
	"github.com/oarkflow/interpreter/pkg/object"
	"github.com/oarkflow/interpreter/pkg/parser"
	"github.com/oarkflow/interpreter/pkg/security"
)

// DispatchHotReloadHooksFn is called after a file is hot-reloaded.
// Set this from the main package if hot-reload hooks are used.
var DispatchHotReloadHooksFn func(path string)

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
	Handler  object.Object
	Env      *object.Environment
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
	ticker := time.NewTicker(500 * time.Millisecond)
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
	elems := make([]object.Object, len(changedFiles))
	for i, f := range changedFiles {
		elems[i] = &object.String{Value: f}
	}
	filesArg := &object.Array{Elements: elems}

	switch fn := entry.Handler.(type) {
	case *object.Function:
		if object.ExtendFunctionEnvFn != nil && object.EvalFn != nil {
			args := []object.Object{filesArg}
			extEnv := object.ExtendFunctionEnvFn(fn, args, entry.Env)
			object.EvalFn(fn.Body, extEnv)
		}
	case *object.Builtin:
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
	cache map[string]*object.Environment
}{
	cache: make(map[string]*object.Environment),
}

func invalidateModuleCache(path string) {
	moduleCache.mu.Lock()
	defer moduleCache.mu.Unlock()
	for key := range moduleCache.cache {
		if key == path || strings.HasSuffix(key, "/"+filepath.Base(path)) {
			delete(moduleCache.cache, key)
		}
	}
}

// ── Builtins ───────────────────────────────────────────────────────

func init() {
	eval.RegisterBuiltins(map[string]*object.Builtin{
		"watch":      {FnWithEnv: builtinWatch},
		"unwatch":    {Fn: builtinUnwatch},
		"hot_reload": {FnWithEnv: builtinHotReload},
	})
}

// watch(path, handler) or watch(path, pattern, handler) or watch(path, options)
func builtinWatch(env *object.Environment, args ...object.Object) object.Object {
	if err := security.CheckCapabilityAllowed(security.CapabilityWatch); err != nil {
		return object.NewError("%s", err)
	}
	if len(args) < 2 {
		return object.NewError("watch() requires at least (path, handler)")
	}
	pathStr, ok := args[0].(*object.String)
	if !ok {
		return object.NewError("watch() first argument must be a string path")
	}

	entry := &WatchEntry{
		Path:     pathStr.Value,
		Env:      env,
		Debounce: 200 * time.Millisecond,
	}

	switch v := args[1].(type) {
	case *object.Function:
		entry.Handler = v
	case *object.Builtin:
		entry.Handler = v
	case *object.String:
		entry.Pattern = v.Value
		if len(args) >= 3 {
			entry.Handler = args[2]
		} else {
			return object.NewError("watch() with pattern requires a handler")
		}
	case *object.Hash:
		for _, pair := range v.Pairs {
			key := pair.Key.Inspect()
			switch key {
			case "pattern":
				if s, ok := pair.Value.(*object.String); ok {
					entry.Pattern = s.Value
				}
			case "handler":
				entry.Handler = pair.Value
			case "debounce":
				if ms, ok := pair.Value.(*object.Integer); ok {
					entry.Debounce = time.Duration(ms.Value) * time.Millisecond
				}
			}
		}
		if entry.Handler == nil {
			return object.NewError("watch() options must include a handler")
		}
	default:
		return object.NewError("watch() second argument must be a function, pattern, or options hash")
	}

	globalWatcher.start()
	id := globalWatcher.addWatch(entry)
	if env != nil {
		env.RegisterCleanup(func() {
			globalWatcher.removeWatch(id)
		})
	}
	return &object.String{Value: id}
}

// unwatch(watch_id)
func builtinUnwatch(args ...object.Object) object.Object {
	if len(args) < 1 {
		return object.NewError("unwatch() requires a watch ID")
	}
	id, ok := args[0].(*object.String)
	if !ok {
		return object.NewError("unwatch() argument must be a string")
	}
	if globalWatcher.removeWatch(id.Value) {
		return object.TRUE
	}
	return object.FALSE
}

// hot_reload(path) — watches a file and re-evaluates it on change
func builtinHotReload(env *object.Environment, args ...object.Object) object.Object {
	if err := security.CheckCapabilityAllowed(security.CapabilityWatch); err != nil {
		return object.NewError("%s", err)
	}
	if len(args) < 1 {
		return object.NewError("hot_reload() requires a file path")
	}
	pathStr, ok := args[0].(*object.String)
	if !ok {
		return object.NewError("hot_reload() argument must be a string path")
	}

	absPath, err := filepath.Abs(pathStr.Value)
	if err != nil {
		return object.NewError("hot_reload() invalid path: %s", err)
	}

	reloadHandler := &object.Builtin{
		FnWithEnv: func(e *object.Environment, a ...object.Object) object.Object {
			invalidateModuleCache(absPath)
			data, readErr := os.ReadFile(absPath)
			if readErr != nil {
				fmt.Printf("[hot_reload] error reading %s: %s\n", absPath, readErr)
				return object.NULL
			}
			fmt.Printf("[hot_reload] reloading %s\n", absPath)
			l := lexer.NewLexer(string(data))
			p := parser.NewParser(l)
			program := p.ParseProgram()
			if len(p.Errors()) > 0 {
				fmt.Printf("[hot_reload] parse errors: %v\n", p.Errors())
				return object.NULL
			}
			if object.EvalFn == nil {
				fmt.Printf("[hot_reload] eval function not available\n")
				return object.NULL
			}
			result := object.EvalFn(program, env)
			if object.IsError(result) {
				fmt.Printf("[hot_reload] eval error: %s\n", result.Inspect())
			}
			if DispatchHotReloadHooksFn != nil {
				DispatchHotReloadHooksFn(absPath)
			}
			return object.NULL
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
	if env != nil {
		env.RegisterCleanup(func() {
			globalWatcher.removeWatch(id)
		})
	}
	return &object.String{Value: id}
}
