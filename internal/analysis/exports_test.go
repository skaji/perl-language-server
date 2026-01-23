package analysis

import (
	"testing"

	ppi "github.com/skaji/go-ppi"
)

func TestExportedSymbols(t *testing.T) {
	src := "our @EXPORT = qw($FOO @BAR %BAZ);"
	doc := ppi.NewDocument(src)
	doc.ParseWithDiagnostics()
	exports := ExportedSymbols(doc)
	if len(exports) != 3 {
		t.Fatalf("expected 3 exports, got %d", len(exports))
	}
	for _, name := range []string{"$FOO", "@BAR", "%BAZ"} {
		if _, ok := exports[name]; !ok {
			t.Fatalf("missing export %s", name)
		}
	}
}

func TestExportedSymbolsPackageExport(t *testing.T) {
	src := "@Config::EXPORT = qw(%Config);"
	doc := ppi.NewDocument(src)
	doc.ParseWithDiagnostics()
	exports := ExportedSymbols(doc)
	if len(exports) != 1 {
		t.Fatalf("expected 1 export, got %d", len(exports))
	}
	if _, ok := exports["%Config"]; !ok {
		t.Fatalf("missing export %%Config")
	}
}
