# SPL Language Support

First-party VS Code support for `.spl` files.

## Features

- Syntax highlighting and language configuration for SPL.
- Live parser/static diagnostics from `spltool lsp --stdio`.
- Completion, hover, go to definition, references, document symbols, workspace symbols, and formatting.
- Manual safe evaluation commands:
  - `SPL: Run Current File`
  - `SPL: Evaluate Selection`

Evaluation is forced through the untrusted SPL profile with timeout and output limits.

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
