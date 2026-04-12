package interpreter

import (
	"io"
	"sync"
)

type TemplateRuntime interface {
	Render(tmpl string, data map[string]any) (string, error)
	RenderFile(path string, data map[string]any) (string, error)
	RenderSSR(tmpl string, data map[string]any) (string, error)
	RenderSSRFile(path string, data map[string]any) (string, error)
	RenderStream(w io.Writer, tmpl string, data map[string]any) error
	RenderStreamFile(w io.Writer, path string, data map[string]any) error
	InvalidateCaches()
}

var templateRuntimeRegistry struct {
	mu      sync.RWMutex
	factory func(baseDir string) TemplateRuntime
}

func RegisterTemplateRuntimeFactory(factory func(baseDir string) TemplateRuntime) {
	templateRuntimeRegistry.mu.Lock()
	defer templateRuntimeRegistry.mu.Unlock()
	templateRuntimeRegistry.factory = factory
}

func newTemplateRuntime(baseDir string) TemplateRuntime {
	templateRuntimeRegistry.mu.RLock()
	defer templateRuntimeRegistry.mu.RUnlock()
	if templateRuntimeRegistry.factory == nil {
		return nil
	}
	return templateRuntimeRegistry.factory(baseDir)
}

var hotReloadHooks struct {
	mu    sync.RWMutex
	hooks []func(path string)
}

func RegisterHotReloadHook(hook func(path string)) {
	hotReloadHooks.mu.Lock()
	defer hotReloadHooks.mu.Unlock()
	hotReloadHooks.hooks = append(hotReloadHooks.hooks, hook)
}

func dispatchHotReloadHooks(path string) {
	hotReloadHooks.mu.RLock()
	hooks := append([]func(path string){}, hotReloadHooks.hooks...)
	hotReloadHooks.mu.RUnlock()
	for _, hook := range hooks {
		hook(path)
	}
}
