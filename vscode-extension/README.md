# SPL Language Support

First-party VS Code support for `.spl` files.

## Features

- Syntax highlighting and language configuration for SPL.
- Live parser/static diagnostics from `spltool lsp --stdio`.
- Completion, hover, go to definition, references, document symbols, workspace symbols, and formatting.
- Purpose-oriented hover and completion docs for builtin functions, standard modules such as `std/math`, and daily tool modules such as `tools/files`, `tools/images`, and `tools/media`.
- File finder snippets and highlighting for `file_search` / `file_finder`, including glob, literal, filename regex, path regex, and content regex filters.
- Query-builder highlighting and snippets for common filters such as `where_in`, `where_between`, `where_like`, `where_null`, `where_not_null`, and `where_filter`.
- Native OS adapter support for `import "native/os"` / `import "native/os" as os`, including syntax highlighting, snippets, hover/completion through the language server, and policy-aware evaluation settings.
- Manual safe evaluation commands:
  - `SPL: Run Current File`
  - `SPL: Evaluate Selection`
  - `SPL: Insert Native OS Example`
- Daily tools commands backed by `spltool`:
  - `SPL: Tools FFmpeg Status`
  - `SPL: Tools Install FFmpeg`
  - `SPL: Tools Preview Bulk Rename`

Evaluation defaults to the untrusted SPL profile with timeout and output limits.
To evaluate native OS scripts from VS Code, set `spl.evaluation.profile` to
`native`, add explicit `spl.evaluation.allowedExecCommands` entries such as
`go`, `echo`, or `git`, and keep `spl.evaluation.allowedNativeModules` limited
to `native/os` unless you intentionally add more native modules. Native
evaluation still runs through policy gates and direct executable invocation.
Daily tool file operations are preview-first by default. Use the command output
to inspect planned changes before running matching `spltool ... --apply`
commands. Media conversion uses ffmpeg; the install command runs
`spltool media install-ffmpeg --apply` with the package manager detected by
the interpreter tooling.
For file discovery, use `file_finder("testdata").files().regex("^examples_.*\\.spl$").content_regex("print\\s+").exec()` or the equivalent `file_search` hash options.

## Development

```bash
npm install
npm run compile
```

Open this folder in the VS Code Extension Development Host. By default the extension launches:

```bash
go run ./cmd/spltool lsp --stdio
```

Set `spl.toolPath` to use a prebuilt `spltool` binary instead.

If your scripts use any plugin builtin (the HTTP server/router, `xql`,
`images`, `database`, `money`, `phone`, `ip`, `wuid`, `naturaldate`, `pdf`,
extra `crypto` helpers, `yaml`, `template`, `secretr`, `integrations`,
`metadata`, `shamir`, ...), build `cmd/spltool-full` instead - it's the
same tool with the full plugin surface linked in, so completion/hover/
diagnostics recognize those builtins too (the plain `spltool` above only
knows the root module's builtins):

```bash
go build -o spltool-full ./cmd/spltool-full
```

and point `spl.toolPath` at the resulting binary.
