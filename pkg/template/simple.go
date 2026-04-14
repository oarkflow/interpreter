package template

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// SimpleTemplateRuntime implements a basic template engine for SPL
type SimpleTemplateRuntime struct {
	baseDir string
}

// NewSimpleTemplateRuntime creates a new simple template runtime
func NewSimpleTemplateRuntime(baseDir string) TemplateRuntime {
	return &SimpleTemplateRuntime{baseDir: baseDir}
}

func (r *SimpleTemplateRuntime) Render(tmpl string, data map[string]any) (string, error) {
	return r.renderTemplate(tmpl, data)
}

func (r *SimpleTemplateRuntime) RenderFile(path string, data map[string]any) (string, error) {
	fullPath := filepath.Join(r.baseDir, path)
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to read template file %s: %w", path, err)
	}
	return r.renderTemplate(string(content), data)
}

func (r *SimpleTemplateRuntime) RenderSSR(tmpl string, data map[string]any) (string, error) {
	return r.renderTemplate(tmpl, data)
}

func (r *SimpleTemplateRuntime) RenderSSRFile(path string, data map[string]any) (string, error) {
	return r.RenderFile(path, data)
}

func (r *SimpleTemplateRuntime) RenderStream(w io.Writer, tmpl string, data map[string]any) error {
	result, err := r.renderTemplate(tmpl, data)
	if err != nil {
		return err
	}
	_, err = w.Write([]byte(result))
	return err
}

func (r *SimpleTemplateRuntime) RenderStreamFile(w io.Writer, path string, data map[string]any) error {
	result, err := r.RenderFile(path, data)
	if err != nil {
		return err
	}
	_, err = w.Write([]byte(result))
	return err
}

func (r *SimpleTemplateRuntime) InvalidateCaches() {
	// No caching implemented
}

func (r *SimpleTemplateRuntime) renderTemplate(tmpl string, data map[string]any) (string, error) {
	result := tmpl

	// Simple variable replacement: {{variable}}
	re := regexp.MustCompile(`\{\{(\w+)\}\}`)
	result = re.ReplaceAllStringFunc(result, func(match string) string {
		varName := strings.Trim(match, "{}")
		if val, ok := data[varName]; ok {
			return fmt.Sprintf("%v", val)
		}
		return match // Keep original if not found
	})

	return result, nil
}
