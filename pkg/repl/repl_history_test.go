package repl

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/oarkflow/interpreter/pkg/ast"
	"github.com/oarkflow/interpreter/pkg/object"
)

func TestParseHistoryData(t *testing.T) {
	data := []byte("first\n\n second \r\nthird\n")
	got := ParseHistoryData(data)
	want := []string{"first", " second ", "third"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected history parse result: got=%#v want=%#v", got, want)
	}
}

func TestHistoryEntriesToPersist(t *testing.T) {
	history := []string{"loaded", "", "   ", "new-one", "new-two"}
	got := HistoryEntriesToPersist(history, 1)
	want := []string{"new-one", "new-two"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected entries to persist: got=%#v want=%#v", got, want)
	}
}

func TestLoadAndAppendHistoryEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.txt")

	loaded, err := LoadHistoryEntries(path)
	if err != nil {
		t.Fatalf("load missing history returned error: %v", err)
	}
	if len(loaded) != 0 {
		t.Fatalf("expected empty history for missing file, got %#v", loaded)
	}

	if err := AppendHistoryEntries(path, []string{"cmd1", "", "cmd2"}); err != nil {
		t.Fatalf("append failed: %v", err)
	}
	if err := AppendHistoryEntries(path, []string{"cmd3"}); err != nil {
		t.Fatalf("second append failed: %v", err)
	}

	got, err := LoadHistoryEntries(path)
	if err != nil {
		t.Fatalf("load after append failed: %v", err)
	}
	want := []string{"cmd1", "cmd2", "cmd3"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected loaded history: got=%#v want=%#v", got, want)
	}
}

func TestAppendHistoryEntriesError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "readonly")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	if err := AppendHistoryEntries(path, []string{"cmd"}); err == nil {
		t.Fatalf("expected append error when path is a directory")
	}
}

func TestNavigationWordBoundaries(t *testing.T) {
	buf := []rune("orders.filter(function")
	if got := previousWordBoundary(buf, len(buf)); got != len("orders.filter(") {
		t.Fatalf("unexpected previous word boundary: %d", got)
	}
	if got := previousWordBoundary(buf, len("orders.filter")); got != 0 {
		t.Fatalf("expected dot-qualified word to move to start, got %d", got)
	}
	if got := nextWordBoundary(buf, 0); got != len("orders.filter") {
		t.Fatalf("unexpected next word boundary: %d", got)
	}
}

func TestFuzzyCompletionsFindCommands(t *testing.T) {
	got := FindCompletions("pal", ReplCandidates())
	found := false
	for _, item := range got {
		if item == ":palette" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected :palette completion, got %#v", got)
	}
}

func TestSuggestionLinesShowVisibleMenu(t *testing.T) {
	editor := &ReplEditor{}
	ctx := ReplCompletionContext{Prefix: ":pa", Ok: true}
	lines := editor.SuggestionLines(ctx, ReplCandidates(), 100)
	if len(lines) == 0 {
		t.Fatalf("expected visible suggestion lines")
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "suggestions:") || !strings.Contains(joined, ":palette") {
		t.Fatalf("unexpected suggestion lines: %#v", lines)
	}
}

func TestTabAcceptsVisiblePrefixSuggestion(t *testing.T) {
	editor := &ReplEditor{Candidates: []string{"print", "printf"}}
	buf, cursor := editor.applyCompletion([]rune("pri"), len("pri"), ">> ")
	if got := string(buf); got != "print" {
		t.Fatalf("expected tab to accept visible suggestion, got %q cursor=%d", got, cursor)
	}
	if cursor != len("print") {
		t.Fatalf("unexpected cursor after completion: %d", cursor)
	}
}

func TestTabCompletesCommandSuggestion(t *testing.T) {
	editor := &ReplEditor{Candidates: ReplCandidates()}
	buf, _ := editor.applyCompletion([]rune(":pa"), len(":pa"), ">> ")
	if got := string(buf); got != ":palette" {
		t.Fatalf("expected command completion, got %q", got)
	}
}

func TestReplCallTipStaysVisibleInsideArguments(t *testing.T) {
	env := object.NewEnvironment()
	env.Set("makeLabel", &object.Function{
		Name: "makeLabel",
		Parameters: []*ast.Identifier{
			{Name: "name"},
			{Name: "count"},
		},
		ParamTypes: []string{"string", "integer"},
		ReturnType: "string",
	})

	line := `makeLabel("Ada", `
	tip := ReplCallTip(line, len([]rune(line)), env)
	if !strings.Contains(tip, "makeLabel(name: string, count: integer) -> string") {
		t.Fatalf("expected function signature in call tip, got %q", tip)
	}
	if !strings.Contains(tip, "active: count: integer") {
		t.Fatalf("expected active parameter in call tip, got %q", tip)
	}
}

func TestReplCallTipIgnoresNestedCommas(t *testing.T) {
	env := object.NewEnvironment()
	env.Set("outer", &object.Function{
		Name: "outer",
		Parameters: []*ast.Identifier{
			{Name: "first"},
			{Name: "second"},
		},
		ParamTypes: []string{"array", "integer"},
	})

	line := `outer([1, 2, 3], `
	tip := ReplCallTip(line, len([]rune(line)), env)
	if !strings.Contains(tip, "active: second: integer") {
		t.Fatalf("expected nested commas to be ignored, got %q", tip)
	}
}

func TestSuggestionLinesIncludeFunctionSignature(t *testing.T) {
	env := object.NewEnvironment()
	env.Set("makeLabel", &object.Function{
		Name:       "makeLabel",
		Parameters: []*ast.Identifier{{Name: "name"}},
		ParamTypes: []string{"string"},
		ReturnType: "string",
	})
	editor := &ReplEditor{Env: env, Candidates: []string{"makeLabel"}}
	ctx := ReplCompletionContext{Prefix: "make", Ok: true}
	lines := editor.SuggestionLines(ctx, []string{"makeLabel"}, 120)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "makeLabel(name: string) -> string") {
		t.Fatalf("expected function signature in suggestions, got %#v", lines)
	}
}

func TestToolsBuiltinsAppearInPaletteAndSuggestions(t *testing.T) {
	oldNames := BuiltinNames
	oldHelp := BuiltinHelpTextFn
	BuiltinNames = func() map[string]struct{} {
		return map[string]struct{}{
			"bulk_rename":   {},
			"file_finder":   {},
			"ffmpeg_status": {},
		}
	}
	BuiltinHelpTextFn = func(name string) string {
		switch name {
		case "bulk_rename":
			return "bulk_rename(dir[, opts]) previews or applies bulk file renames"
		case "file_finder":
			return "file_finder(root) creates a chainable filesystem finder with glob, regex, content, sort, and limit filters"
		case "ffmpeg_status":
			return "ffmpeg_status() reports ffmpeg/ffprobe availability"
		default:
			return ""
		}
	}
	defer func() {
		BuiltinNames = oldNames
		BuiltinHelpTextFn = oldHelp
	}()

	palette := ReplPalette("ffmpeg", nil, nil)
	if !strings.Contains(palette, "ffmpeg_status") {
		t.Fatalf("expected ffmpeg_status in palette, got %q", palette)
	}

	editor := &ReplEditor{Candidates: ReplCandidatesForEnv(nil)}
	ctx := ReplCompletionContext{Prefix: "bulk", Ok: true}
	lines := editor.SuggestionLines(ctx, editor.Candidates, 120)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "bulk_rename") || !strings.Contains(joined, "previews or applies") {
		t.Fatalf("expected tool builtin suggestion details, got %#v", lines)
	}

	ctx = ReplCompletionContext{Prefix: "file_find", Ok: true}
	lines = editor.SuggestionLines(ctx, editor.Candidates, 120)
	joined = strings.Join(lines, "\n")
	if !strings.Contains(joined, "file_finder") || !strings.Contains(joined, "chainable filesystem finder") {
		t.Fatalf("expected file_finder suggestion details, got %#v", lines)
	}
}

func TestBuiltinCallTipShowsParameter(t *testing.T) {
	oldHasBuiltin := HasBuiltinFn
	oldHelp := BuiltinHelpTextFn
	HasBuiltinFn = func(name string) bool { return name == "read_json" }
	BuiltinHelpTextFn = func(name string) string {
		if name == "read_json" {
			return "read_json(path[, opts]) loads JSON from disk"
		}
		return ""
	}
	defer func() {
		HasBuiltinFn = oldHasBuiltin
		BuiltinHelpTextFn = oldHelp
	}()

	line := `read_json(`
	tip := ReplCallTip(line, len([]rune(line)), nil)
	if !strings.Contains(tip, "read_json(path[, opts])") || !strings.Contains(tip, "active: path") {
		t.Fatalf("expected builtin signature and parameter, got %q", tip)
	}

	line = `read_json("data.json", `
	tip = ReplCallTip(line, len([]rune(line)), nil)
	if !strings.Contains(tip, "active: opts optional") {
		t.Fatalf("expected optional builtin parameter, got %q", tip)
	}
}
