// Command builtindocs generates docs/reference/builtins.md: a single
// Markdown reference page enumerating every SPL builtin function.
//
// It lives in the cmd/spltool-full module (not the root
// github.com/oarkflow/interpreter module) because it needs
// eval.PluginBuiltins fully populated to document plugin builtins (pdf,
// database, rules, ...), and those are only registered when
// github.com/oarkflow/interpreter/plugins is linked in — exactly what
// cmd/spltool-full already blank-imports for the same reason (see that
// package's doc comment). The root module deliberately does not depend on
// plugins, so this generator could not live there and still see the full
// builtin surface.
//
// Usage (from the repo root):
//
//	cd cmd/spltool-full && go run ./cmd/builtindocs -out ../../docs/reference/builtins.md
//
// or simply `make builtins-doc` / `make builtins-doc-check` from the repo
// root, which wrap the above.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	interp "github.com/oarkflow/interpreter"
	"github.com/oarkflow/interpreter/pkg/eval"
	_ "github.com/oarkflow/interpreter/plugins"
)

const (
	generatorCmd    = "make builtins-doc"
	undocumentedMsg = "(undocumented)"
)

func main() {
	out := flag.String("out", "docs/reference/builtins.md", "output path for the generated Markdown reference")
	flag.Parse()

	md, stats := render()

	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "builtindocs: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, []byte(md), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "builtindocs: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr,
		"builtindocs: wrote %s (global=%d, plugin_modules=%d, plugin_builtins=%d, total_builtins=%d, undocumented=%d)\n",
		*out, stats.globalCount, stats.moduleCount, stats.pluginCount, stats.globalCount+stats.pluginCount, stats.undocumented,
	)
}

type stats struct {
	globalCount  int
	pluginCount  int
	moduleCount  int
	undocumented int
}

// moduleGroup collects every std-module name that resolves to the exact
// same set of plugin builtins (e.g. "std/core"/"core", or "yaml"/"config/yaml")
// under one canonical heading, so the doc doesn't repeat the same builtin
// list once per alias name.
type moduleGroup struct {
	canonical string
	aliases   []string
	prefix    string
	names     []string // plugin-only builtin names, sorted
}

func render() (string, stats) {
	var st stats
	var b strings.Builder

	b.WriteString("# SPL Builtin Function Reference\n\n")
	b.WriteString("> This file is generated — do not hand-edit; regenerate with `" + generatorCmd + "`\n")
	b.WriteString("> (generator: `cmd/spltool-full/cmd/builtindocs`).\n\n")
	b.WriteString(
		"Two kinds of builtins exist. **Global builtins** resolve as bare identifiers in any\n" +
			"script, no import required. **Plugin builtins** are only reachable after importing\n" +
			"their owning std module (`import \"pdf\" as pdf;`), and are called through that\n" +
			"module namespace (`pdf.to_docx(...)`).\n\n",
	)

	// ---- Global builtins ------------------------------------------------
	globalNames := make([]string, 0, len(eval.Builtins))
	for name := range eval.Builtins {
		globalNames = append(globalNames, name)
	}
	sort.Strings(globalNames)
	st.globalCount = len(globalNames)

	b.WriteString("## Global builtins\n\n")
	fmt.Fprintf(&b, "%d global builtins (no import required).\n\n", st.globalCount)
	for _, name := range globalNames {
		desc, documented := eval.BuiltinHelpDescriptions[name]
		if !documented || strings.TrimSpace(desc) == "" {
			desc = undocumentedMsg
			st.undocumented++
		}
		fmt.Fprintf(&b, "- **`%s`** — %s\n", name, desc)
	}
	b.WriteString("\n")

	// ---- Plugin builtins, grouped by owning std module -----------------
	groups := groupPluginModules()
	st.moduleCount = len(groups)

	sort.Slice(groups, func(i, j int) bool { return groups[i].canonical < groups[j].canonical })

	// Track which plugin builtins got placed in at least one module group,
	// so we can call out any stragglers (registered in eval.PluginBuiltins
	// but not exposed via any RegisterStdBuiltinModule* call) instead of
	// silently dropping them from the doc.
	placed := make(map[string]bool)

	b.WriteString("## Plugin builtins by module\n\n")
	fmt.Fprintf(&b, "%d plugin modules, %d plugin builtins total. Each module is reachable via\n"+
		"`import \"<module>\" as <alias>;` and its builtins are called as `<alias>.<member>(...)`.\n\n",
		st.moduleCount, len(eval.PluginBuiltins))

	for _, g := range groups {
		fmt.Fprintf(&b, "### %s\n\n", g.canonical)
		if len(g.aliases) > 0 {
			sort.Strings(g.aliases)
			fmt.Fprintf(&b, "_Also importable as: %s_\n\n", strings.Join(quoteAll(g.aliases), ", "))
		}
		keyCounts := stripKeyCounts(g.names, g.prefix)
		for _, name := range g.names {
			placed[name] = true
			desc, documented := eval.BuiltinHelpDescriptions[name]
			if !documented || strings.TrimSpace(desc) == "" {
				desc = undocumentedMsg
				st.undocumented++
			}
			qualified := qualifiedForm(g.canonical, g.prefix, name, keyCounts)
			fmt.Fprintf(&b, "- **`%s`** (`%s`) — %s\n", name, qualified, desc)
		}
		b.WriteString("\n")
	}

	// ---- Stragglers: plugin builtins not exposed via any std module ----
	var strays []string
	for name := range eval.PluginBuiltins {
		if !placed[name] {
			strays = append(strays, name)
		}
	}
	st.pluginCount = len(placed) + len(strays)
	if len(strays) > 0 {
		sort.Strings(strays)
		b.WriteString("## Plugin builtins not exposed via any std module\n\n")
		b.WriteString(
			"These are registered in `eval.PluginBuiltins` but are not backing any\n" +
				"`RegisterStdBuiltinModule`/`RegisterStdBuiltinModuleWithPrefix` registration in\n" +
				"`presets_plugins.go`, so no import currently reaches them.\n\n",
		)
		for _, name := range strays {
			desc, documented := eval.BuiltinHelpDescriptions[name]
			if !documented || strings.TrimSpace(desc) == "" {
				desc = undocumentedMsg
				st.undocumented++
			}
			fmt.Fprintf(&b, "- **`%s`** — %s\n", name, desc)
		}
		b.WriteString("\n")
	}

	return b.String(), st
}

// groupPluginModules reads interp.StdBuiltinModules(), keeps only the
// builtin names that are actually plugin-only (present in
// eval.PluginBuiltins — modules like "core"/"fs"/"math" wrap already-global
// builtins under an extra namespace and carry no plugin-only members, so
// they are dropped here entirely), and merges module names that expose the
// exact same plugin builtin set (aliases such as "std/core"/"core" or
// "yaml"/"config/yaml") under one canonical heading.
func groupPluginModules() []moduleGroup {
	mods := interp.StdBuiltinModules()
	bySig := map[string]*moduleGroup{}

	for name, info := range mods {
		var pluginNames []string
		for _, n := range info.BuiltinNames {
			if _, ok := eval.PluginBuiltins[n]; ok {
				pluginNames = append(pluginNames, n)
			}
		}
		if len(pluginNames) == 0 {
			continue
		}
		sort.Strings(pluginNames)
		sig := info.Prefix + "|" + strings.Join(pluginNames, ",")
		g, ok := bySig[sig]
		if !ok {
			g = &moduleGroup{prefix: info.Prefix, names: pluginNames}
			bySig[sig] = g
		}
		g.aliases = append(g.aliases, name)
	}

	groups := make([]moduleGroup, 0, len(bySig))
	for _, g := range bySig {
		sort.Strings(g.aliases)
		canonical := pickCanonical(g.aliases)
		var aliases []string
		for _, a := range g.aliases {
			if a != canonical {
				aliases = append(aliases, a)
			}
		}
		groups = append(groups, moduleGroup{
			canonical: canonical,
			aliases:   aliases,
			prefix:    g.prefix,
			names:     g.names,
		})
	}
	return groups
}

// pickCanonical prefers the shortest name without a "std/" prefix (e.g.
// "core" over "std/core"), falling back to the shortest name overall, then
// alphabetical order to break ties deterministically.
func pickCanonical(names []string) string {
	best := names[0]
	for _, n := range names[1:] {
		if canonicalLess(n, best) {
			best = n
		}
	}
	return best
}

func canonicalLess(a, b string) bool {
	aStd, bStd := strings.HasPrefix(a, "std/"), strings.HasPrefix(b, "std/")
	if aStd != bStd {
		return !aStd // non-"std/" wins
	}
	if len(a) != len(b) {
		return len(a) < len(b)
	}
	return a < b
}

// stripKeyCounts mirrors the collision handling in LookupStdModule: a
// stripped key must be unique within the module, otherwise every builtin
// involved in the collision keeps its full name.
func stripKeyCounts(names []string, prefix string) map[string]int {
	counts := make(map[string]int, len(names))
	for _, n := range names {
		key := n
		if prefix != "" && strings.HasPrefix(n, prefix) {
			key = strings.TrimPrefix(n, prefix)
		}
		counts[key]++
	}
	return counts
}

func qualifiedForm(module, prefix, name string, keyCounts map[string]int) string {
	key := name
	if prefix != "" && strings.HasPrefix(name, prefix) {
		if stripped := strings.TrimPrefix(name, prefix); keyCounts[stripped] == 1 {
			key = stripped
		}
	}
	return module + "." + key
}

func quoteAll(names []string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = "`" + n + "`"
	}
	return out
}
