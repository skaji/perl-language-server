# perl-language-server

Perl Language Server implemented in Go.

## Status

- textDocument/hover (basic)
- textDocument/publishDiagnostics (basic structural diagnostics from go-ppi)
- document sync (full text)

## Requirements

- Go 1.25+

## Build

```sh
go mod download
make build
```

## Run (stdio)

```sh
./perl-language-server
```

The server uses stdio for JSON-RPC.

## Logging

- `DEBUG=1` enables debug logs
- `LOG_FILE=/path/to/log` writes logs to that file (otherwise stderr)

Example:

```sh
DEBUG=1 LOG_FILE=/tmp/perl-lsp.log ./perl-language-server
```

## Vim (vim-lsp) example

```vim
" vim-lsp + asyncomplete/lsp or similar client
if executable('perl-language-server')
  augroup perl_lsp
    autocmd!
    autocmd User lsp_setup call lsp#register_server({
          \ 'name': 'perl-language-server',
          \ 'cmd': {server_info->['perl-language-server']},
          \ 'whitelist': ['perl'],
          \ })
  augroup END
endif
```

## Development

Keep `make build`, `make test`, and `make lint` passing.

## License

TBD
