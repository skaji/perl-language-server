## Purpose

Build a Perl Language Server in Go.

- Parsing/analysis: github.com/skaji/go-ppi
- LSP framework: github.com/skaji/glsp (fork of github.com/tliron/glsp)
- Logging: log/slog (DEBUG enables debug logs, LOG_FILE writes logs to a file)

## Development Flow

Start small and iterate:

1. Implement a minimal LSP server that initializes and responds to basic requests.
2. Wire in go-ppi to parse documents on open/change.
3. Add language features (hover/definition/completion/etc.) once parsing/analysis is stable.
4. Keep `make build`, `make test`, and `make lint` passing at all times.

## Notes and Gotchas

Dependencies:

- Use github.com/skaji/glsp (fork) instead of github.com/tliron/glsp.
- Parsing/analysis should go through go-ppi, not ad-hoc parsing.
- Keep implementation under internal/lsp; keep cmd/perl-language-server/main.go minimal.

## Implemented So Far

- Minimal LSP server over stdio (`initialize`, `initialized`, `shutdown`, `setTrace`).
- Document lifecycle with parse cache (`textDocument/didOpen`, `didChange`, `didClose`).
- Parsing/analysis wired through go-ppi on open/change.
- Diagnostics:
  - go-ppi structural diagnostics
  - strict vars diagnostics
  - `:SIG(...)` validation diagnostics
  - signature call diagnostics
- Language features:
  - `textDocument/hover`
  - `textDocument/definition`
  - `textDocument/typeDefinition`
  - `textDocument/completion` (symbols, keywords/builtins, method completion from inferred receiver type)
- Workspace indexing for `.pm` files to resolve package/sub definitions across workspace/lib paths.

## Still To Implement

- Prioritize and stabilize next LSP feature set (e.g. references, document symbols, rename, signature help).
- Improve incremental update strategy (currently full-text sync) and performance on large files/workspaces.
- Expand cross-file/type inference accuracy and edge-case handling.
- Add more end-to-end LSP integration tests (including workspace and `use lib` resolution scenarios).
- Keep docs (`README.md`) aligned with implemented feature set.
