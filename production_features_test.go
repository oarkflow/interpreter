package interpreter

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestExecWithOptionsRuntimeErrorIncludesStack(t *testing.T) {
	_, err := ExecWithOptions(`
let boom = function() {
  unknown_identifier;
};
let wrapper = function() {
  boom();
};
wrapper();
`, nil, ExecOptions{})
	if err == nil {
		t.Fatalf("expected runtime error")
	}

	var execErr *ExecError
	if !errors.As(err, &execErr) {
		t.Fatalf("expected ExecError, got %T", err)
	}
	if len(execErr.Stack) < 2 {
		t.Fatalf("expected at least two stack frames, got %d", len(execErr.Stack))
	}
	if execErr.Stack[0].Function != "boom" {
		t.Fatalf("expected first frame to be boom, got %#v", execErr.Stack[0])
	}
	if execErr.Stack[1].Function != "wrapper" {
		t.Fatalf("expected second frame to be wrapper, got %#v", execErr.Stack[1])
	}
	if len(execErr.Diagnostics) == 0 || !strings.Contains(execErr.Diagnostics[0], "Stack trace:") {
		t.Fatalf("expected stack trace diagnostics, got %#v", execErr.Diagnostics)
	}
}

func TestExecBuiltinRespectsContextCancellation(t *testing.T) {
	t.Setenv("SPL_DISABLE_EXEC", "0")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := ExecWithOptions(`exec("tail", "-f", "/dev/null")`, nil, ExecOptions{
		Context:  ctx,
		Security: &SecurityPolicy{AllowedExecCommands: []string{"tail"}},
	})
	if err == nil {
		t.Fatalf("expected cancellation error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "cancel") {
		t.Fatalf("expected cancellation message, got %v", err)
	}
}

func TestDatabaseBuiltinsSupportParamsAndTransactions(t *testing.T) {
	res, err := ExecWithOptions(`
let db, err = db_connect("sqlite", ":memory:");
if (err != null) { throw err; }

let _, createErr = db_exec(db, "CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT, qty INTEGER)");
if (createErr != null) { throw createErr; }

let _, insertErr1 = db_exec(db, "INSERT INTO items(name, qty) VALUES(?, ?)", ["apples", 3]);
if (insertErr1 != null) { throw insertErr1; }

let _, insertErr2 = db_exec(db, "INSERT INTO items(name, qty) VALUES(:name, :qty)", {"name": "pears", "qty": 4});
if (insertErr2 != null) { throw insertErr2; }

let tx, txErr = db_begin(db);
if (txErr != null) { throw txErr; }

let _, insertErr3 = db_exec(tx, "INSERT INTO items(name, qty) VALUES(?, ?)", ["rolled", 99]);
if (insertErr3 != null) { throw insertErr3; }

let rolledBack, rollbackErr = db_rollback(tx);
if (rollbackErr != null) { throw rollbackErr; }
if (!rolledBack) { throw "expected rollback"; }

let committedTx, beginErr2 = db_begin(db);
if (beginErr2 != null) { throw beginErr2; }

let _, insertErr4 = db_exec(committedTx, "INSERT INTO items(name, qty) VALUES(:name, :qty)", {"name": "committed", "qty": 7});
if (insertErr4 != null) { throw insertErr4; }

let committed, commitErr = db_commit(committedTx);
if (commitErr != null) { throw commitErr; }
if (!committed) { throw "expected commit"; }

let rows, queryErr = db_query(db, "SELECT name, qty FROM items ORDER BY qty ASC", null, "array");
if (queryErr != null) { throw queryErr; }

let countRows, countErr = db_query(db, "SELECT COUNT(*) AS total FROM items WHERE qty >= ?", [3], "array");
if (countErr != null) { throw countErr; }

db_close(db);
{"first": rows[0].name, "second": rows[1].name, "third": rows[2].name, "count": countRows[0].total};
`, nil, ExecOptions{})
	if err != nil {
		t.Fatalf("ExecWithOptions failed: %v", err)
	}

	result, ok := res.(*Hash)
	if !ok {
		t.Fatalf("expected HASH result, got %T", res)
	}

	expectString := func(key, want string) {
		t.Helper()
		val, ok := hashGet(result, key)
		if !ok {
			t.Fatalf("missing key %q", key)
		}
		str, ok := val.(*String)
		if !ok || str.Value != want {
			t.Fatalf("unexpected %s: got %T (%v), want %q", key, val, val, want)
		}
	}
	expectInt := func(key string, want int64) {
		t.Helper()
		val, ok := hashGet(result, key)
		if !ok {
			t.Fatalf("missing key %q", key)
		}
		num, ok := val.(*Integer)
		if !ok || num.Value != want {
			t.Fatalf("unexpected %s: got %T (%v), want %d", key, val, val, want)
		}
	}

	expectString("first", "apples")
	expectString("second", "pears")
	expectString("third", "committed")
	expectInt("count", 3)
}
