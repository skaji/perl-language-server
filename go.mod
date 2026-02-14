module github.com/skaji/perl-language-server

go 1.26.0

require (
	github.com/skaji/go-ppi v0.0.2
	github.com/tliron/glsp v0.2.2
)

require (
	github.com/aymanbagabas/go-osc52/v2 v2.0.1 // indirect
	github.com/gorilla/websocket v1.5.4-0.20250319132907-e064f32e3674 // indirect
	github.com/iancoleman/strcase v0.3.0 // indirect
	github.com/lucasb-eyer/go-colorful v1.3.0 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/muesli/termenv v0.16.0 // indirect
	github.com/petermattis/goid v0.0.0-20260113132338-7c7de50cc741 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/rivo/uniseg v0.4.7 // indirect
	github.com/sasha-s/go-deadlock v0.3.6 // indirect
	github.com/segmentio/ksuid v1.0.4 // indirect
	github.com/sourcegraph/jsonrpc2 v0.2.1 // indirect
	github.com/tliron/commonlog v0.2.21 // indirect
	github.com/tliron/go-kutil v0.4.0 // indirect
	golang.org/x/crypto v0.48.0 // indirect
	golang.org/x/net v0.50.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
	golang.org/x/term v0.40.0 // indirect
)

replace github.com/tliron/glsp => github.com/skaji/glsp v0.0.0-20260107192625-dd9435c01989

replace github.com/skaji/go-ppi => ./go-ppi
