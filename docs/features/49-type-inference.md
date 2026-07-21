# 49 — CSV/JSON Type Inference (`metadata`)

Source: `plugins/metadata`. Registers virtual module `"metadata"`. This is a
direct Go port of the pure inference functions from
`github.com/oarkflow/metadata`'s `field.go`, **reimplemented rather than
imported**: that module's other files pull in a full SQL-datasource
connector layer (mysql/postgres/mssql/sqlite via `squealx`) that both
overlaps with `plugins/database` and, at the versions this repo currently
pins, fails to build (a `squealx` subpackage those connector files import
was removed upstream). Porting just the ~100 lines of inference logic
avoids both problems — no capability is required for any of its builtins.

Useful for profiling an unfamiliar CSV/JSON data source ("is this column
really numeric, or does it have stray text rows?") before writing a
schema/import pipeline by hand.

## `infer_csv_types(csv_text)`

```spl
let [types, err] = infer_csv_types("id,name,active,joined\n1,Ada,true,2020-01-15\n2,Grace,false,2021-06-30\n");
if (err != null) { throw err; }
print types;
// {active: "bool", id: "int", joined: "time.Time", name: "string"}
```

Takes raw CSV text (e.g. from `file_text(file_load(path))`, or a string you
already have) and returns a `HASH` mapping each header column to its
inferred type, merging types across every data row. Possible values:
`"int"`, `"int64"`, `"float64"`, `"bool"`, `"time.Time"`, `"string"`, or
`"empty"` for an all-blank column. Numbers with thousands separators (e.g.
`"1,234"`) are recognized as numeric. A column is only inferred as
`"time.Time"` when **every** non-empty value in it parses as a date/time;
otherwise mixed content merges toward `"string"`.

## `infer_json_types(value)`

```spl
let [types, err] = infer_json_types([
    {"id": 1, "score": 9.5},
    {"id": 2, "score": 10}
]);
if (err != null) { throw err; }
print types;
// {id: "int", score: "float64"}
```

Takes an already-decoded JSON value — a `HASH`, or an `ARRAY` of `HASH`
rows, the same shape `read_json`/`json_decode` already return — and infers
a per-field type map, merging types across every row/array element. Beyond
the CSV vocabulary, JSON values also produce `"null"`, `"[]<type>"` for a
homogeneous array field (e.g. `"[]int"`), `"[]any"` for a mixed-type array,
and `"map[string]any"` for a nested object field. An `int`/`float64` mix in
the same field merges to `"float64"` (matching `infer_csv_types`'
`int`+`float64` → `float64` merge rule).

## `infer_value_type(value)`

```spl
print infer_value_type(42);            // "int"
print infer_value_type("2026-01-01");  // "time.Time"
print infer_value_type("not a date");  // "string"
```

Infers the type of a single value directly, using the same type vocabulary
as `infer_json_types`. Useful for a quick one-off check without building an
array/hash just to profile it.

## Capability notes

None of `infer_csv_types`, `infer_json_types`, or `infer_value_type` check
any capability — verified running under `--profile untrusted` with no
`--allow-cap` flags. They operate purely on the string/value already
passed in; reading the source file (if any) is the caller's concern via
`read_file`/`file_load`, not something these builtins do themselves.
