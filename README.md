# perl-language-server

WIP

Perl Language Server implemented in Go.

## Features

- Document sync: full text (`didOpen`, `didChange`, `didClose`, `didSave`)
- Hover: `textDocument/hover`
- Definition: `textDocument/definition`
- Type definition: `textDocument/typeDefinition`
- Completion: `textDocument/completion`
- Diagnostics:
  - structural diagnostics from go-ppi
  - strict vars diagnostics
  - `:SIG(...)` validation diagnostics
  - signature call diagnostics
  - `perl -c` diagnostics on open/save
- Workspace index for cross-file resolution is built asynchronously.

## Requirements

- Go 1.26+
- Perl (`perl` command available in `PATH`)

## Build

```sh
git clone --depth=1 https://github.com/skaji/go-ppi.git
go mod download
make build
```

## Run (stdio)

```sh
./perl-language-server
```

The server uses stdio for JSON-RPC.

Show binary version:

```sh
./perl-language-server --version
```

## Logging

- `DEBUG=1` enables debug logs
- `LOG_FILE=/path/to/log` writes logs to that file (otherwise stderr)
- Output format is JSON (`slog` JSON handler)

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

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
