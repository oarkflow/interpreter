# SPL Language Support

First-party VS Code support for `.spl` files.

## Features

- Syntax highlighting and language configuration for SPL.
- Live parser/static diagnostics from `spltool lsp --stdio`.
- Completion, hover, go to definition, references, document symbols, workspace symbols, and formatting.
- Purpose-oriented hover and completion docs for builtin functions, standard modules such as `std/math`, and daily tool modules such as `tools/files`, `tools/images`, and `tools/media`.
- Manual safe evaluation commands:
  - `SPL: Run Current File`
  - `SPL: Evaluate Selection`
- Daily tools commands backed by `spltool`:
  - `SPL: Tools FFmpeg Status`
  - `SPL: Tools Install FFmpeg`
  - `SPL: Tools Preview Bulk Rename`

Evaluation is forced through the untrusted SPL profile with timeout and output limits.
Daily tool file operations are preview-first by default. Use the command output
to inspect planned changes before running matching `spltool ... --apply`
commands. Media conversion uses ffmpeg; the install command runs
`spltool media install-ffmpeg --apply` with the package manager detected by
the interpreter tooling.

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
