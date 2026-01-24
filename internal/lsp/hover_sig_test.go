package lsp

import (
	"testing"

	ppi "github.com/skaji/go-ppi"
)

func TestHoverSigFromFunctionArg(t *testing.T) {
	src := "# :SIG(App::cpm::CLI -> void)\nsub foo ($app) {\n    $app;\n}\n"
	doc := ppi.NewDocument(src)
	doc.ParseWithDiagnostics()
	idx := newDocumentStore()
	d := idx.set("file:///test.pl", src, nil)
	offset := findIndex(src, "$app;")
	if offset < 0 {
		t.Fatalf("expected $app in source")
	}
	sig := hoverVarSigType(d, offset, "$app", nil)
	if sig != "App::cpm::CLI" {
		t.Fatalf("expected App::cpm::CLI, got %q", sig)
	}
}

func findIndex(src, needle string) int {
	for i := 0; i+len(needle) <= len(src); i++ {
		if src[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
