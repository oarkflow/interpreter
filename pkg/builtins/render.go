package builtins

import (
	"strings"

	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/object"
)

func init() {
	eval.RegisterBuiltins(map[string]*object.Builtin{
		"file": {
			Fn: func(args ...object.Object) object.Object {
				return newRenderArtifact("file", args...)
			},
		},
		"image": {
			Fn: func(args ...object.Object) object.Object {
				return newRenderArtifact("image", args...)
			},
		},
		"render": {
			Fn: func(args ...object.Object) object.Object {
				if len(args) < 1 || len(args) > 2 {
					return object.NewError("wrong number of arguments. got=%d, want=1 or 2", len(args))
				}
				opts, errObj := renderOpts(args)
				if errObj != nil {
					return errObj
				}
				if art, ok := args[0].(*object.RenderArtifact); ok {
					cloned := *art
					applyRenderOpts(&cloned, opts)
					return &cloned
				}
				if art, errObj := convertToRenderArtifact(args[0], opts); errObj == nil {
					return art
				}
				if s, ok := args[0].(*object.String); ok {
					kind := "file"
					mimeType := ""
					sourceTyp := detectRenderSourceType(s.Value)
					if looksLikeHTML(s.Value) {
						kind = "html"
						mimeType = "text/html"
						sourceTyp = "data"
					}
					art := &object.RenderArtifact{
						Kind:      kind,
						Source:    s.Value,
						SourceTyp: sourceTyp,
						MIME:      mimeType,
					}
					applyRenderOpts(art, opts)
					return art
				}
				art := &object.RenderArtifact{
					Kind:      "text",
					Source:    args[0].Inspect(),
					SourceTyp: "data",
					MIME:      "text/plain",
				}
				applyRenderOpts(art, opts)
				return art
			},
		},
	})
}

func newRenderArtifact(kind string, args ...object.Object) object.Object {
	if len(args) < 1 || len(args) > 2 {
		return object.NewError("wrong number of arguments. got=%d, want=1 or 2", len(args))
	}
	source, errObj := asString(args[0], "source")
	if errObj != nil {
		return errObj
	}
	opts, errObj := renderOpts(args)
	if errObj != nil {
		return errObj
	}
	art := &object.RenderArtifact{
		Kind:      kind,
		Source:    source,
		SourceTyp: detectRenderSourceType(source),
	}
	applyRenderOpts(art, opts)
	return art
}

func renderOpts(args []object.Object) (map[string]object.Object, object.Object) {
	if len(args) < 2 {
		return nil, nil
	}
	h, ok := args[1].(*object.Hash)
	if !ok {
		return nil, object.NewError("argument `opts` must be HASH, got %s", args[1].Type())
	}
	out := make(map[string]object.Object, len(h.Pairs))
	for _, pair := range h.Pairs {
		out[strings.TrimSpace(pair.Key.Inspect())] = pair.Value
	}
	return out, nil
}

func applyRenderOpts(art *object.RenderArtifact, opts map[string]object.Object) {
	if art == nil || len(opts) == 0 {
		return
	}
	if v := optString(opts, "kind"); v != "" {
		art.Kind = strings.ToLower(v)
	}
	if v := optString(opts, "source_type"); v != "" {
		art.SourceTyp = strings.ToLower(v)
	}
	if v := optString(opts, "mime"); v != "" {
		art.MIME = v
	}
	if v := optString(opts, "name"); v != "" {
		art.Name = v
	}
	if v := optString(opts, "alt"); v != "" {
		art.Alt = v
	}
	if v := optString(opts, "mode"); v != "" {
		art.Mode = strings.ToLower(v)
	}
	if v, ok := optInt(opts, "width"); ok {
		art.Width = v
	}
	if v, ok := optInt(opts, "height"); ok {
		art.Height = v
	}
	if v, ok := optInt(opts, "max_bytes"); ok {
		art.MaxBytes = v
	}
}

func optString(opts map[string]object.Object, key string) string {
	if opts == nil {
		return ""
	}
	val, ok := opts[key]
	if !ok || val == nil {
		return ""
	}
	switch v := val.(type) {
	case *object.String:
		return strings.TrimSpace(v.Value)
	case *object.Secret:
		return strings.TrimSpace(v.Value)
	default:
		return strings.TrimSpace(v.Inspect())
	}
}

func optInt(opts map[string]object.Object, key string) (int64, bool) {
	if opts == nil {
		return 0, false
	}
	val, ok := opts[key]
	if !ok || val == nil {
		return 0, false
	}
	if i, ok := val.(*object.Integer); ok {
		return i.Value, true
	}
	return 0, false
}

func detectRenderSourceType(source string) string {
	raw := strings.TrimSpace(source)
	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "data:") {
		return "data"
	}
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return "url"
	}
	return "path"
}

func looksLikeHTML(source string) bool {
	trimmed := strings.TrimSpace(source)
	lower := strings.ToLower(trimmed)
	return strings.HasPrefix(lower, "<!doctype html") ||
		strings.HasPrefix(lower, "<html") ||
		(strings.Contains(lower, "<") && strings.Contains(lower, "</") && strings.Contains(lower, ">"))
}
