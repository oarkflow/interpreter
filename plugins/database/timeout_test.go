package database

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/oarkflow/interpreter/pkg/object"
)

// slowJoinQuery forces SQLite to do enough work (a self cross-join with
// modulo filters, no usable index) that a 1ms timeout reliably expires
// before the query completes, while still completing quickly under a
// generous timeout. This avoids relying on a SLEEP()-style function, which
// SQLite doesn't have.
const slowJoinQuery = `
	SELECT count(*) AS total FROM t a, t b WHERE a.x % 7 = 0 AND b.x % 11 = 0
`

// newTimeoutTestDB opens a file-backed (not in-memory) SQLite database and
// seeds it with enough rows that slowJoinQuery takes a few milliseconds.
// A file-backed DB is used because squealx/sqlite's connection pool opens a
// fresh in-memory database per connection, which makes ":memory:" DBs
// unsuitable for reliably reproducing slow-query timing in tests.
func newTimeoutTestDB(t *testing.T) *object.DB {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "timeout-test-*.db")
	if err != nil {
		t.Fatalf("failed to create temp db file: %v", err)
	}
	f.Close()

	env := object.NewEnvironment()
	res := builtinDBConnect(env, &object.String{Value: "sqlite"}, &object.String{Value: f.Name()})
	tuple, ok := res.(*object.Array)
	if !ok || len(tuple.Elements) != 2 {
		t.Fatalf("db_connect returned unexpected result: %#v", res)
	}
	if tuple.Elements[1] != object.NULL {
		t.Fatalf("db_connect failed: %s", tuple.Elements[1].Inspect())
	}
	dbObj, ok := tuple.Elements[0].(*object.DB)
	if !ok {
		t.Fatalf("db_connect did not return a DB object: %#v", tuple.Elements[0])
	}

	mustExec := func(query string, args ...object.Object) {
		t.Helper()
		callArgs := append([]object.Object{dbObj, &object.String{Value: query}}, args...)
		res := builtinDBExec(env, callArgs...)
		tuple, ok := res.(*object.Array)
		if !ok || len(tuple.Elements) != 2 {
			t.Fatalf("db_exec returned unexpected result: %#v", res)
		}
		if tuple.Elements[1] != object.NULL {
			t.Fatalf("db_exec(%q) failed: %s", query, tuple.Elements[1].Inspect())
		}
	}

	mustExec("CREATE TABLE t (x INTEGER)")
	elements := make([]object.Object, 0, 4000)
	for i := 0; i < 4000; i++ {
		elements = append(elements, &object.Integer{Value: int64(i)})
	}
	// Insert in a single statement per row is slow to set up individually in
	// a loop of 4000 db_exec calls; instead build a batched INSERT using a
	// recursive CTE, which SQLite supports natively and is fast to seed.
	mustExec("WITH RECURSIVE seq(x) AS (SELECT 0 UNION ALL SELECT x+1 FROM seq WHERE x < 3999) INSERT INTO t(x) SELECT x FROM seq")

	t.Cleanup(func() {
		builtinDBClose(dbObj)
	})

	return dbObj
}

func TestDBQueryTimeoutMsExpires(t *testing.T) {
	env := object.NewEnvironment()
	db := newTimeoutTestDB(t)

	res := builtinDBQuery(env, db, &object.String{Value: slowJoinQuery}, object.NULL, &object.String{Value: "array"}, &object.Integer{Value: 1})
	tuple, ok := res.(*object.Array)
	if !ok || len(tuple.Elements) != 2 {
		t.Fatalf("db_query returned unexpected result: %#v", res)
	}
	if tuple.Elements[1] == object.NULL {
		t.Fatalf("expected db_query to time out, but it succeeded: %#v", tuple.Elements[0])
	}
	errMsg := tuple.Elements[1].Inspect()
	if !strings.Contains(errMsg, "query timed out after") {
		t.Fatalf("expected distinguishable timeout error, got: %s", errMsg)
	}
}

func TestDBExecTimeoutMsExpires(t *testing.T) {
	env := object.NewEnvironment()
	db := newTimeoutTestDB(t)

	// UPDATE driven by the same expensive predicate as slowJoinQuery so it
	// takes long enough to blow through a 1ms budget.
	res := builtinDBExec(env, db, &object.String{Value: `
		UPDATE t SET x = x WHERE x IN (SELECT a.x FROM t a, t b WHERE a.x % 7 = 0 AND b.x % 11 = 0)
	`}, object.NULL, &object.Integer{Value: 1})
	tuple, ok := res.(*object.Array)
	if !ok || len(tuple.Elements) != 2 {
		t.Fatalf("db_exec returned unexpected result: %#v", res)
	}
	if tuple.Elements[1] == object.NULL {
		t.Fatalf("expected db_exec to time out, but it succeeded: %#v", tuple.Elements[0])
	}
	errMsg := tuple.Elements[1].Inspect()
	if !strings.Contains(errMsg, "query timed out after") {
		t.Fatalf("expected distinguishable timeout error, got: %s", errMsg)
	}
}

func TestDBQueryTimeoutMsGenerousSucceeds(t *testing.T) {
	env := object.NewEnvironment()
	db := newTimeoutTestDB(t)

	res := builtinDBQuery(env, db, &object.String{Value: slowJoinQuery}, object.NULL, &object.String{Value: "array"}, &object.Integer{Value: 30000})
	tuple, ok := res.(*object.Array)
	if !ok || len(tuple.Elements) != 2 {
		t.Fatalf("db_query returned unexpected result: %#v", res)
	}
	if tuple.Elements[1] != object.NULL {
		t.Fatalf("expected db_query with generous timeout to succeed, got error: %s", tuple.Elements[1].Inspect())
	}
}

// TestDBQueryAndExecWithoutTimeoutArgUnaffected is the most important
// regression check: existing call patterns documented in the README that
// never pass timeout_ms must behave exactly as before (whole-script
// context, no per-call bound), for both db_query and db_exec, across every
// previously-supported arity.
func TestDBQueryAndExecWithoutTimeoutArgUnaffected(t *testing.T) {
	env := object.NewEnvironment()
	db := newTimeoutTestDB(t)

	// db_exec(db, query) -- 2 args
	res := builtinDBExec(env, db, &object.String{Value: "INSERT INTO t(x) VALUES(99999)"})
	tuple := res.(*object.Array)
	if tuple.Elements[1] != object.NULL {
		t.Fatalf("db_exec(db, query) failed: %s", tuple.Elements[1].Inspect())
	}

	// db_exec(db, query, params) -- 3 args, positional
	res = builtinDBExec(env, db, &object.String{Value: "INSERT INTO t(x) VALUES(?)"}, &object.Array{Elements: []object.Object{&object.Integer{Value: 100000}}})
	tuple = res.(*object.Array)
	if tuple.Elements[1] != object.NULL {
		t.Fatalf("db_exec(db, query, params) failed: %s", tuple.Elements[1].Inspect())
	}

	// db_exec(db, query, named_params) -- 3 args, named
	res = builtinDBExec(env, db, &object.String{Value: "INSERT INTO t(x) VALUES(:x)"}, hashOf(map[string]object.Object{"x": &object.Integer{Value: 100001}}))
	tuple = res.(*object.Array)
	if tuple.Elements[1] != object.NULL {
		t.Fatalf("db_exec(db, query, named_params) failed: %s", tuple.Elements[1].Inspect())
	}

	// db_query(db, query) -- 2 args
	res = builtinDBQuery(env, db, &object.String{Value: "SELECT count(*) AS total FROM t"})
	tuple = res.(*object.Array)
	if tuple.Elements[1] != object.NULL {
		t.Fatalf("db_query(db, query) failed: %s", tuple.Elements[1].Inspect())
	}

	// db_query(db, query, format) -- 3 args, format shorthand (no params)
	res = builtinDBQuery(env, db, &object.String{Value: "SELECT count(*) AS total FROM t"}, &object.String{Value: "table"})
	tuple = res.(*object.Array)
	if tuple.Elements[1] != object.NULL {
		t.Fatalf("db_query(db, query, format) failed: %s", tuple.Elements[1].Inspect())
	}
	if _, ok := tuple.Elements[0].(*object.String); !ok {
		t.Fatalf("expected table-formatted STRING result, got %T", tuple.Elements[0])
	}

	// db_query(db, query, params, format) -- 4 args, exactly as documented
	// in README.md.
	res = builtinDBQuery(env, db,
		&object.String{Value: "SELECT count(*) AS total FROM t WHERE x >= ?"},
		&object.Array{Elements: []object.Object{&object.Integer{Value: 0}}},
		&object.String{Value: "array"},
	)
	tuple = res.(*object.Array)
	if tuple.Elements[1] != object.NULL {
		t.Fatalf("db_query(db, query, params, format) failed: %s", tuple.Elements[1].Inspect())
	}
	arr, ok := tuple.Elements[0].(*object.Array)
	if !ok || len(arr.Elements) != 1 {
		t.Fatalf("expected single-row array result, got %#v", tuple.Elements[0])
	}
}

func hashOf(m map[string]object.Object) *object.Hash {
	pairs := make(map[object.HashKey]object.HashPair, len(m))
	for k, v := range m {
		key := &object.String{Value: k}
		pairs[key.HashKey()] = object.HashPair{Key: key, Value: v}
	}
	return &object.Hash{Pairs: pairs}
}

// TestDBQueryInvalidTimeoutArg confirms bad timeout_ms values are rejected
// with a clear error rather than silently ignored or panicking.
func TestDBQueryInvalidTimeoutArg(t *testing.T) {
	env := object.NewEnvironment()
	db := newTimeoutTestDB(t)

	res := builtinDBQuery(env, db, &object.String{Value: "SELECT 1"}, object.NULL, &object.String{Value: "array"}, &object.String{Value: "not-an-int"})
	tuple := res.(*object.Array)
	if tuple.Elements[1] == object.NULL {
		t.Fatalf("expected error for non-integer timeout_ms, got success")
	}
	if !strings.Contains(tuple.Elements[1].Inspect(), "timeout_ms must be INTEGER") {
		t.Fatalf("expected timeout_ms type error, got: %s", tuple.Elements[1].Inspect())
	}

	res = builtinDBExec(env, db, &object.String{Value: "SELECT 1"}, object.NULL, &object.Integer{Value: 0})
	tuple = res.(*object.Array)
	if tuple.Elements[1] == object.NULL {
		t.Fatalf("expected error for zero timeout_ms, got success")
	}
	if !strings.Contains(tuple.Elements[1].Inspect(), "timeout_ms must be > 0") {
		t.Fatalf("expected timeout_ms > 0 error, got: %s", tuple.Elements[1].Inspect())
	}
}

// TestRuntimeContextWithTimeoutHonorsExistingDeadline is a lower-level unit
// check that runtimeContextWithTimeout actually derives a bounded child
// context via context.WithTimeout rather than replacing the runtime context
// outright.
func TestRuntimeContextWithTimeoutHonorsExistingDeadline(t *testing.T) {
	env := object.NewEnvironment()
	ctx, cancel := runtimeContextWithTimeout(env, 0)
	defer cancel()
	if ctx != context.Background() {
		t.Fatalf("expected zero timeout to reuse the runtime context unchanged")
	}

	ctx2, cancel2 := runtimeContextWithTimeout(env, 1)
	defer cancel2()
	if _, ok := ctx2.Deadline(); !ok {
		t.Fatalf("expected a deadline to be set when timeout > 0")
	}
}
