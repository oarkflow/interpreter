// Package template links github.com/oarkflow/spl, a full templating
// engine (loops, conditionals, layouts/@extends, includes, components -
// see its source for the complete directive set) that self-registers as
// the interpreter's TemplateRuntime via RegisterTemplateRuntimeFactory in
// its own init(), replacing the lightweight built-in
// pkg/template.SimpleTemplateRuntime ({{var}} substitution only).
//
// This package has no API of its own - importing it for its side effect
// (the blank import below) is the entire integration, following the same
// plugin pattern as builtins/database, builtins/images, etc: a separate
// Go module (see go.mod) so the root module doesn't carry this dependency
// for embedders who don't need richer templates, blank-imported by hosts
// that do (e.g. cmd/interpreter, examples/app).
package template

import (
	_ "github.com/oarkflow/spl"
)
