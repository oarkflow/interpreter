package eval

import (
	"fmt"
	"strings"
	"sync"

	"github.com/oarkflow/interpreter/pkg/ast"
	"github.com/oarkflow/interpreter/pkg/object"
)

type EmbeddedLanguageContext struct {
	Tag  string
	Code string
	Env  *object.Environment
}

type EmbeddedLanguageHandler func(EmbeddedLanguageContext) object.Object

var embeddedLanguages = struct {
	mu       sync.RWMutex
	handlers map[string]EmbeddedLanguageHandler
}{
	handlers: map[string]EmbeddedLanguageHandler{},
}

func RegisterEmbeddedLanguage(tag string, handler EmbeddedLanguageHandler) error {
	tag = strings.TrimSpace(strings.ToLower(tag))
	if tag == "" {
		return fmt.Errorf("embedded language tag cannot be empty")
	}
	if handler == nil {
		return fmt.Errorf("embedded language handler for %q cannot be nil", tag)
	}
	embeddedLanguages.mu.Lock()
	defer embeddedLanguages.mu.Unlock()
	embeddedLanguages.handlers[tag] = handler
	return nil
}

func EmbeddedLanguageTags() []string {
	embeddedLanguages.mu.RLock()
	defer embeddedLanguages.mu.RUnlock()
	tags := make([]string, 0, len(embeddedLanguages.handlers))
	for tag := range embeddedLanguages.handlers {
		tags = append(tags, tag)
	}
	return tags
}

func evalTaggedBlockLiteral(node *ast.TaggedBlockLiteral, env *object.Environment) object.Object {
	tag := strings.TrimSpace(strings.ToLower(node.Tag))
	embeddedLanguages.mu.RLock()
	handler, ok := embeddedLanguages.handlers[tag]
	embeddedLanguages.mu.RUnlock()
	if !ok {
		return object.NewError("embedded language %q is not registered; import its plugin module first", node.Tag)
	}
	if env != nil && env.RuntimeLimits != nil && env.RuntimeLimits.MaxStringBytes > 0 && int64(len(node.Code)) > env.RuntimeLimits.MaxStringBytes {
		return object.NewError("embedded block size limit exceeded (%d bytes)", env.RuntimeLimits.MaxStringBytes)
	}
	return handler(EmbeddedLanguageContext{Tag: tag, Code: node.Code, Env: env})
}
