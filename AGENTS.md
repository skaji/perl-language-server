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

Current LSP skeleton with hover using go-ppi AST.

## Still To Implement

- Decide initial LSP feature set (hover/definition/completion/etc.).
- Minimal LSP server skeleton using skaji/glsp.
- Document lifecycle: open/change/close with parse cache.
- go-ppi integration for parsing/analysis.
