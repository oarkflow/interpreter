# Runnable SPL examples

Run an example from the repository root:

```sh
go run ./cmd/interpreter examples/all_in_one.spl
```

Validate syntax and static diagnostics without executing:

```sh
go run ./cmd/spltool check examples/all_in_one.spl
```

## Language example

`all_in_one.spl` is the single canonical, self-contained tour. It covers core
syntax, collections, strings, crypto and time helpers, control flow, error
handling, assertions, pattern matching, async functions, classes, interfaces,
generators, channels, permissions, observability, immutable values, ADTs, init
hooks, and test blocks. Optional host integrations are documented behind a
disabled guard so the example runs with the lightweight interpreter.

## PDF example

`pdf_all_in_one.spl` tours `builtins/pdf` (generation, page operations,
watermarking/stamping, encryption, extraction, search, and images-to-PDF),
writing real files under `examples/pdf_demo/` (gitignored scratch output) so
every builtin is exercised against an actual PDF instead of a mock.

PDF builtins are only linked into `cmd/interpreter-full` (a separate Go
module - see the root README's "Quick Start" for why heavier optional
dependencies stay out of the lightweight `cmd/interpreter`), which itself
can't be `go run`/`go build` directly from the repository root since it's
excluded from the root module. Build it once, then run scripts against that
binary from the repo root so relative paths resolve as documented above:

```sh
cd cmd/interpreter-full && go build -o ../../bin/interpreter-full . && cd ../..
./bin/interpreter-full examples/pdf_all_in_one.spl
```
