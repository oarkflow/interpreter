# 38 — XQL Data Pipeline

Source: `plugins/xql` (optional package, wraps `github.com/oarkflow/xql`; linked
only into `cmd/interpreter`). Registers virtual module `"xql"` and an
**embedded language tag** `xql` (tagged block literals, doc 02).

```spl
import "xql";
```

XQL is a small pipeline query DSL (`source |> stage |> stage`) for
filtering/projecting/limiting in-memory data and, via connected
integrations, external HTTP/REST/GraphQL/webhook/database/etc. sources.

## Tagged-block form (recommended — auto-registers scope variables)

```spl
let items = [{"id": 1, "n": "a"}, {"id": 2, "n": "b"}];

let result, err = xql```
items
|> keep id, n
|> take 1
```;
print result; // [{id: 1, n: "a"}]
print err;    // null
```

**Verified**: any SPL array-of-hashes variable in scope (`items` above) is
automatically available as a named source inside the tagged block, with no
explicit registration step.

## `xql_run(queryString)` — string form (does *not* auto-register scope)

```spl
let items = [{"id": 1, "n": "a"}];
let result, err = xql_run("items |> keep id, n |> take 1");
print err; // "schema not found for source \"items\"" — items is NOT auto-visible here
```

Unlike the tagged-block literal, calling `xql_run` with a plain string does
**not** automatically see in-scope SPL variables as sources — connect a
source explicitly first with `xql_connect` if you need to query from
`xql_run`, or prefer the `` xql`...` `` tagged-block form when querying a
local in-scope value.

## `xql_connect` — registering external integrations

```spl
xql_connect("alias", "http", {"base_url": "https://example.com"}[, "source_name"]);
print xql_list_integrations(); // ["alias", ...]
```

Built-in provider types: `http.json`, `rest`, `graphql`, `webhook`, `hl7`,
`google_api`, `github_api`, `facebook_api`, `slack_api`, `database`,
`webcrawler`, `smtp`, `smpp`, `email_http`, `sms_http`, `temp_file`, plus
catalog-only stubs `ftp`, `sftp`, `kafka`, `mqtt`, `grpc`, `soap`,
`customtcp`, `push`, `voip`. Any `http`/`https` URI is auto-connectable.

## Querying a remote HTTP/REST API

```spl
let result, err = xql_run(`
call https://example.com/api {
  method: "GET"
}
|> keep userId, id, title, body
|> take 5
`);
```

## Pipeline stage vocabulary

Common stages seen in examples: `keep <cols>` (projection), `take <n>`
(limit). Consult `github.com/oarkflow/xql` directly for the complete stage
grammar (filter/sort/join/aggregate stages) — this package is a thin SPL
binding over that engine rather than reimplementing its query language.

## When to reach for XQL vs. plain array methods

- Use ordinary array/collection methods (`filter`/`map`/`reduce`, doc
  15/21) for straightforward in-language data manipulation.
- Reach for XQL when you want a **declarative pipeline syntax** over data
  that may come from an external integration (HTTP/DB/etc.) uniformly, or
  when embedding query text as a distinct "language block" (`` xql`...` ``)
  improves readability for non-trivial multi-stage queries.
