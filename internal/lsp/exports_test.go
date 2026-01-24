package lsp

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	ppi "github.com/skaji/go-ppi"
	"github.com/skaji/perl-language-server/internal/analysis"
)

func TestExportedStrictVarsNoImportList(t *testing.T) {
	dir := filepath.Join("testdata", "exports")
	path := filepath.Join(dir, "main_no_import.pl")
	doc := parseFile(t, path)
	srv := newTestServer()

	extra := srv.exportedStrictVars(doc, path)
	if len(extra) == 0 {
		t.Fatalf("expected extra exports")
	}

	diags := analysis.StrictVarDiagnosticsWithExtra(doc, extra)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diags, got %d", len(diags))
	}
}

func TestExportedStrictVarsExplicitImportList(t *testing.T) {
	dir := filepath.Join("testdata", "exports")
	path := filepath.Join(dir, "main_explicit.pl")
	doc := parseFile(t, path)
	srv := newTestServer()

	extra := srv.exportedStrictVars(doc, path)
	if len(extra) == 0 {
		t.Fatalf("expected extra exports")
	}
	if _, ok := extra["$FOO"]; !ok {
		t.Fatalf("expected $FOO to be exported")
	}
	if _, ok := extra["$BAR"]; ok {
		t.Fatalf("did not expect $BAR to be exported")
	}

	diags := analysis.StrictVarDiagnosticsWithExtra(doc, extra)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diag, got %d", len(diags))
	}
	if diags[0].Message == "" {
		t.Fatalf("unexpected diag: %+v", diags[0])
	}
}

func TestExportedStrictVarsExplicitQuoted(t *testing.T) {
	dir := filepath.Join("testdata", "exports")
	path := filepath.Join(dir, "main_explicit_quoted.pl")
	doc := parseFile(t, path)
	srv := newTestServer()

	extra := srv.exportedStrictVars(doc, path)
	if _, ok := extra["$FOO"]; !ok {
		t.Fatalf("expected $FOO to be exported")
	}
	diags := analysis.StrictVarDiagnosticsWithExtra(doc, extra)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diags, got %d", len(diags))
	}
}

func TestExportedStrictVarsExplicitNoSigil(t *testing.T) {
	dir := filepath.Join("testdata", "exports")
	path := filepath.Join(dir, "main_explicit_nosigil.pl")
	doc := parseFile(t, path)
	srv := newTestServer()

	extra := srv.exportedStrictVars(doc, path)
	if len(extra) != 0 {
		t.Fatalf("expected no exports, got %v", extra)
	}
	diags := analysis.StrictVarDiagnosticsWithExtra(doc, extra)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diag, got %d", len(diags))
	}
}

func TestExportedStrictVarsExplicitDefaultTag(t *testing.T) {
	dir := filepath.Join("testdata", "exports")
	path := filepath.Join(dir, "main_explicit_default.pl")
	doc := parseFile(t, path)
	srv := newTestServer()

	extra := srv.exportedStrictVars(doc, path)
	if _, ok := extra["$FOO"]; !ok {
		t.Fatalf("expected $FOO to be exported")
	}
	diags := analysis.StrictVarDiagnosticsWithExtra(doc, extra)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diags, got %d", len(diags))
	}
}

func parseFile(t *testing.T, path string) *ppi.Document {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("abs path: %v", err)
	}
	src, err := os.ReadFile(abs)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	doc := ppi.NewDocument(string(src))
	doc.ParseWithDiagnostics()
	return doc
}

func newTestServer() *Server {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := NewServer(logger)
	srv.incRoots = []string{filepath.Join(string(filepath.Separator), "nonexistent")}
	return srv
}
