package interpreter_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/oarkflow/interpreter"
	_ "github.com/oarkflow/interpreter/pkg/builtins/scheduler"
	_ "github.com/oarkflow/interpreter/pkg/builtins/tools"
	"github.com/oarkflow/interpreter/pkg/eval"
)

func TestQueryBuilderPatternMatchAndDecode(t *testing.T) {
	if !eval.HasBuiltin("db_connect") {
		t.Skip("database builtins are optional")
	}
	res, err := ExecWithOptions(`
let db, err = db_connect("sqlite", ":memory:");
if (err != null) { throw err; }
let _, createErr = db_exec(db, "CREATE TABLE items (name TEXT, qty INTEGER, kind TEXT)");
if (createErr != null) { throw createErr; }
let _, _ = db_exec(db, "INSERT INTO items(name, qty, kind) VALUES(?, ?, ?)", ["apples", 3, "fruit"]);
let _, _ = db_exec(db, "INSERT INTO items(name, qty, kind) VALUES(?, ?, ?)", ["desk", 8, "furniture"]);
let qb = query(db, "items").where_match("{kind: \"fruit\", qty: > 1}");
let rows, qerr = qb.exec();
if (qerr != null) { throw qerr; }
let decoded = query(db, "items").select("name", "qty").decode_match("{name: item, qty: total}");
{"matched": rows[0].name, "decoded": decoded[1].total};
`, nil, ExecOptions{})
	if err != nil {
		t.Fatal(err)
	}
	h, ok := res.(*Hash)
	if !ok {
		t.Fatalf("expected hash result, got %T", res)
	}
	matched, _ := HashGet(h, "matched")
	if matched.(*String).Value != "apples" {
		t.Fatalf("unexpected match result: %s", matched.Inspect())
	}
	decoded, _ := HashGet(h, "decoded")
	switch v := decoded.(type) {
	case *Integer:
		if v.Value != 8 {
			t.Fatalf("unexpected decoded qty: %s", decoded.Inspect())
		}
	case *String:
		if v.Value != "8" {
			t.Fatalf("unexpected decoded qty: %s", decoded.Inspect())
		}
	default:
		t.Fatalf("unexpected decoded type: %T", decoded)
	}
}

func TestQueryBuilderCommonFilters(t *testing.T) {
	if !eval.HasBuiltin("db_connect") {
		t.Skip("database builtins are optional")
	}
	res, err := ExecWithOptions(`
let db, err = db_connect("sqlite", ":memory:");
if (err != null) { throw err; }
let _, createErr = db_exec(db, "CREATE TABLE items (name TEXT, qty INTEGER, kind TEXT, tag TEXT)");
if (createErr != null) { throw createErr; }
let _, _ = db_exec(db, "INSERT INTO items(name, qty, kind, tag) VALUES(?, ?, ?, ?)", ["apples", 3, "fruit", null]);
let _, _ = db_exec(db, "INSERT INTO items(name, qty, kind, tag) VALUES(?, ?, ?, ?)", ["desk", 8, "furniture", "home"]);
let _, _ = db_exec(db, "INSERT INTO items(name, qty, kind, tag) VALUES(?, ?, ?, ?)", ["bananas", 6, "fruit", "yellow"]);

let q = query(db, "items")
	.where_in("kind", ["fruit"])
	.where_between("qty", 3, 6)
	.where_like("name", "%a%")
	.where_filter({"kind": "fruit", "ignored": null})
	.where_not_null("name")
	.where_null("tag")
	.order_by("qty ASC");
let rows, qerr = q.exec();
if (qerr != null) { throw qerr; }
{"sql": q.sql(), "count": len(rows), "first": rows[0].name};
`, nil, ExecOptions{})
	if err != nil {
		t.Fatal(err)
	}
	h, ok := res.(*Hash)
	if !ok {
		t.Fatalf("expected hash result, got %T", res)
	}
	count, _ := HashGet(h, "count")
	if count.(*Integer).Value != 1 {
		t.Fatalf("unexpected filtered count: %s", count.Inspect())
	}
	first, _ := HashGet(h, "first")
	if first.(*String).Value != "apples" {
		t.Fatalf("unexpected first row: %s", first.Inspect())
	}
	sql, _ := HashGet(h, "sql")
	for _, want := range []string{"IN (?)", "BETWEEN ? AND ?", "LIKE ?", "IS NULL", "IS NOT NULL"} {
		if !strings.Contains(sql.(*String).Value, want) {
			t.Fatalf("expected SQL to contain %q, got %q", want, sql.(*String).Value)
		}
	}
}

func TestFileSearchAndFinderBuiltins(t *testing.T) {
	dir, err := os.MkdirTemp(".", "finder-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	alpha := filepath.Join(dir, "alpha.txt")
	beta := filepath.Join(dir, "beta.log")
	if err := os.WriteFile(alpha, []byte("hello finder"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(beta, []byte("log data"), 0o600); err != nil {
		t.Fatal(err)
	}
	script := fmt.Sprintf(`
let quick = file_search(%q, {"ext": "txt", "content": "finder"});
let chained = file_finder(%q).files().regex("^alpha.*\\.txt$").content_regex("find(er)").sort("name").limit(1).exec();
{"quick": len(quick), "chained": chained[0].name};
`, dir, dir)
	res, err := ExecWithOptions(script, nil, ExecOptions{})
	if err != nil {
		t.Fatal(err)
	}
	h, ok := res.(*Hash)
	if !ok {
		t.Fatalf("expected hash result, got %T", res)
	}
	quick, _ := HashGet(h, "quick")
	if quick.(*Integer).Value != 1 {
		t.Fatalf("unexpected quick result count: %s", quick.Inspect())
	}
	chained, _ := HashGet(h, "chained")
	if chained.(*String).Value != "alpha.txt" {
		t.Fatalf("unexpected chained result: %s", chained.Inspect())
	}
}

func TestSchedulerRestoreAndTimezone(t *testing.T) {
	dir := t.TempDir()
	file := dir + "/jobs.json"
	res, err := ExecWithOptions(`
schedule_timezone("UTC");
let id = schedule_interval("5s", "heartbeat", function() { 1; });
schedule_persist("`+file+`");
schedule_cancel(id);
schedule_restore("`+file+`");
let jobs = schedule_list();
len(jobs);
`, nil, ExecOptions{})
	if err != nil {
		t.Fatal(err)
	}
	count, ok := res.(*Integer)
	if !ok || count.Value < 1 {
		t.Fatalf("expected restored jobs, got %T (%v)", res, res)
	}
}

func TestBackgroundNamedFunctionRecursion(t *testing.T) {
	res, err := ExecWithOptions(`
let future = background(function fib(n) {
	if (n < 2) {
		return n;
	}
	return fib(n - 1) + fib(n - 2);
}, 8);

await future;
`, nil, ExecOptions{})
	if err != nil {
		t.Fatal(err)
	}
	value, ok := res.(*Integer)
	if !ok || value.Value != 21 {
		t.Fatalf("expected fibonacci result 21, got %T (%v)", res, res)
	}
}
