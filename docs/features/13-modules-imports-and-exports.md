# 13 — Modules: Imports & Exports

Source: `pkg/parser/parser.go` (`parseImportStatement`, `parseExportStatement`),
`pkg/eval` module resolution/caching.

## Import forms

```spl
import "path/to/mod.spl";                    // side-effect only
import "path/to/mod.spl" as mod;              // namespaced
import {a, b} from "path/to/mod.spl";         // named, unqualified
import {a as c} from "path/to/mod.spl";       // named + rename
import * as mod from "path/to/mod.spl";       // namespace-all
```

Given `mathlib.spl`:

```spl
export const name = "mathlib";
export let answer = 42;
export function add(a, b) { return a + b; }
```

```spl
import "./mathlib.spl" as math;
print math.answer;    // 42
print math.add(2, 3); // 5

import {answer, add} from "./mathlib.spl";
print answer;          // 42
print add(10, 20);     // 30

import * as m2 from "./mathlib.spl";
print m2.name;         // mathlib
```

## Exports

```spl
export let value = 42;
export const name = "math";
export function add(a, b) { return a + b; }
```

`export` is only valid at the top level of a file that's actually being
imported as a module (it requires `env.ModuleContext != nil`); using it in
the entry script itself is a no-op/error context.

## Module resolution & caching behavior

- Relative import paths (`"./mod.spl"`, `"../lib/mod.spl"`) resolve relative
  to the **importing file's own directory**, not the process's cwd.
- The module cache is keyed by resolved path; re-importing the same path
  returns the cached module's exports without re-executing it.
- Cache entries are invalidated automatically if the file's modification
  time changes on disk (useful for REPL `:reload` / hot-reload workflows).
- Circular imports (A imports B imports A) are detected and rejected with a
  clear error rather than infinite-looping.
- `SPL_MODULE_PATH` adds additional module lookup directories, searched when
  a bare (non-relative, non-package-alias) import path doesn't resolve
  directly.

## Virtual / built-in standard modules

Beyond filesystem modules, the interpreter registers a set of **virtual std
modules** that don't correspond to a file on disk:

```spl
import "std/core" as core;
core.sprintf("value=%d", 42);

import "database";                    // global names AND a virtual module
let db, err = db_connect("sqlite", ":memory:");

import "database" as database;
let db2, err2 = database.db_connect("sqlite", ":memory:");
```

Always-available aliases: `std/core`, `std/fs`, `std/render`, `std/test`,
`std/config`, plus short forms `core`, `fs`, `render`, `test`, `config`, and
many more (`math`, `string`, `array`, `hash`, `time`, `json`, `csv`, `crypto`,
`path`, `random`, each with a `std/`-prefixed alias too).

Optional groups become virtual modules **only when their Go package is
linked into the running binary** (e.g. via `cmd/interpreter`, which
blank-imports every `plugins/*` package): `database`, `images`,
`integrations`, `cryptoextra`, `securetoken`, `yaml`, `config/yaml`, `xql`,
`naturaldate`, `wuid`, `money`, `phone`, `ip`, `shamir`, `metadata`, and the
`tools/*` family (`tools/files`, `tools/archive`, `tools/images`,
`tools/office`, `tools/secrets`, `tools/media`, `tools/system`,
`tools/network`). If a script imports one of these under a binary that
doesn't link the backing package (e.g. a custom embedding host that only
imports the root module), calling one of its functions fails with an
actionable error naming the missing module rather than a bare "identifier
not found".

## Import security

Under a restrictive security policy (doc 44), import behavior can be
constrained: `--allow-import-path`/`--deny-import-path`,
`--allow-import-package`/`--deny-import-package`, `--deny-dynamic-imports`
(rejects any `import` whose path isn't a string literal), and
`SPL_IMPORT_PATH_ALLOW`/`DENY`, `SPL_IMPORT_PACKAGE_ALLOW`/`DENY`,
`SPL_IMPORT_DENY_DYNAMIC` env vars. Import depth/count are also bounded via
`--max-import-depth`/`--max-import-count` to defend against pathological or
malicious import graphs.

## Bare package-style imports

For dependency-manifest-driven imports (`import "mathlib/math.spl" as math;`
resolving through a `spl.mod` dependency alias rather than a relative path),
see doc 14 (Package Manifests).
