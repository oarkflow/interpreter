package interpreter

import "testing"

func TestQueryBuilderPatternMatchAndDecode(t *testing.T) {
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
	matched, _ := hashGet(h, "matched")
	if matched.(*String).Value != "apples" {
		t.Fatalf("unexpected match result: %s", matched.Inspect())
	}
	decoded, _ := hashGet(h, "decoded")
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
