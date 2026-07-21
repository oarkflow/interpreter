# 32 — Database

Source: `plugins/database` (optional package, wraps
`github.com/oarkflow/squealx`; linked only into `cmd/interpreter`, including
its `--playground` mode). Virtual module name: `"database"`. Requires the
`db` capability under a restrictive security policy (doc 45).

```spl
import "database";
```

## Connecting

```spl
let db, err = db_connect("sqlite", ":memory:");
```

Supported drivers: `postgres`/`postgresql`, `mysql`, `sqlite`/`sqlite3`.
Connecting pings the database and registers an auto-close cleanup hook on
the environment.

## Queries

```spl
let _, cerr = db_exec(db, "CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT, qty INTEGER)");

// Positional params (?):
let _, e1 = db_exec(db, "INSERT INTO items(name, qty) VALUES(?, ?)", ["apples", 3]);

// Named params (:name):
let _, e2 = db_exec(db, "INSERT INTO items(name, qty) VALUES(:name, :qty)", {"name": "pears", "qty": 4});

let rows, qerr = db_query(db, "SELECT name, qty FROM items ORDER BY qty ASC", null, "array");
print rows;
// [{name:"apples",qty:3}, {name:"pears",qty:4}, {name:"committed",qty:7}]
```

- `db_query` returns rows as an `ARRAY of HASH` (format `"array"`, default)
  or a formatted `STRING` table (format `"table"`).
- `db_exec` returns `{rows_affected, last_insert_id}`.
- Both accept an optional trailing `timeout_ms` to bound a single slow
  query without cancelling the rest of the script.
- `HASH` params require named (`:name`) placeholders; `ARRAY` params are
  positional (`?`); mixing styles in one call is rejected.

## Transactions

```spl
let tx, tx_err = db_begin(db);
let _, _ = db_exec(tx, "INSERT INTO items(name, qty) VALUES(:name, :qty)", {"name": "committed", "qty": 7});
let ok, commit_err = db_commit(tx);
// or: db_rollback(tx);
```

## Introspection & cleanup

```spl
print db_tables(db); // ["items"]
db_close(db);
```

## Fluent query builder

```spl
let rows, err = query(db, "items")
    .where("qty", ">", 3)
    .order_by("qty DESC")
    .limit(2)
    .exec();
```

```spl
let qb = query(db, "items").where("qty", ">", 3).order_by("qty DESC").limit(2);
print qb.sql();
// SELECT * FROM items WHERE qty > ? ORDER BY qty DESC LIMIT 2
```

### Query builder methods

| Method | Description |
|---|---|
| `.from(table)` | set/validate table name |
| `.select(...cols)` | SELECT columns |
| `.where(col, val)` / `.where(col, op, val)` | AND-ed condition (`op` ∈ `=,!=,<>,<,<=,>,>=,like,not like,in,not in,is,is not`) |
| `.where_raw(sql)` | raw WHERE fragment — never pass unsanitized external input |
| `.where_in(col, values)` | `col IN (...)` |
| `.where_between(col, min, max)` | `BETWEEN` |
| `.where_like(col, pattern)` | `LIKE` |
| `.where_null(col)` / `.where_not_null(col)` | null checks |
| `.where_filter(hash)` | AND-ed equality conditions from a hash |
| `.order_by(...cols)` | e.g. `"age DESC"` |
| `.limit(n)` / `.offset(n)` | pagination |
| `.join(clause[, type])` | raw join clause — never pass unsanitized input |
| `.group_by(...cols)` | grouping |
| `.match(pattern)` / `.where_match(pattern)` | SPL match-style pattern → WHERE clauses |
| `.decode(pattern)` / `.decode_match(pattern)` | executes + destructures matching rows |
| `.exec()` | run now, returns `[rows, err]` |
| `.lazy()` | returns a `LazyDBQuery`, forced on first access |
| `.sql()` | returns the built SQL text without executing |

```spl
let matched = query(db, "items").where_match("{kind: \"fruit\", qty: > 1}").decode_match();
let lazyRows = lazy_query(db, "items"); // auto-forces to [rows, err] on first use
```

## Error handling pattern

Every DB call follows the `[value, error]` tuple convention (doc 03) — check
`err != null` before trusting the result, or wrap in `try/catch` if you'd
rather branch on exceptions.

## Capability requirement

`db_connect` (and everything that depends on it) requires the `db`
capability. It's granted by the `data-processing` preset and by
`cmd/interpreter --playground`'s extra capability grant, but denied under
the plain `untrusted` preset — see doc 45.
