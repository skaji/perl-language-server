package analysis

import (
	"testing"

	ppi "github.com/skaji/go-ppi"
)

func TestStrictVarsUseStrict(t *testing.T) {
	src := "use strict; my $x = 1; $x; $y;"
	doc := parseDoc(src)
	diags := StrictVarDiagnostics(doc)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diag, got %d", len(diags))
	}
	if diags[0].Message == "" || diags[0].Offset == 0 {
		t.Fatalf("unexpected diag: %+v", diags[0])
	}
}

func TestStrictVarsUseVersion(t *testing.T) {
	src := "use v5.12; my $x = 1; $x; $y;"
	doc := parseDoc(src)
	diags := StrictVarDiagnostics(doc)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diag, got %d", len(diags))
	}
}

func TestStrictVarsNoStrict(t *testing.T) {
	src := "use strict; no strict; $y;"
	doc := parseDoc(src)
	diags := StrictVarDiagnostics(doc)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diag, got %d", len(diags))
	}
}

func TestStrictVarsBlockScope(t *testing.T) {
	src := "use strict; { no strict; $y; } $z;"
	doc := parseDoc(src)
	diags := StrictVarDiagnostics(doc)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diag, got %d", len(diags))
	}
}

func TestStrictVarsSpecials(t *testing.T) {
	src := "use strict; $^X; $];"
	doc := parseDoc(src)
	diags := StrictVarDiagnostics(doc)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diag, got %d", len(diags))
	}
}

func TestStrictVarsArrayHashScalarAccess(t *testing.T) {
	src := "use strict; my @f; my %g; $f[0]; $g{a};"
	doc := parseDoc(src)
	diags := StrictVarDiagnostics(doc)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diag, got %d", len(diags))
	}
}

func TestStrictVarsArrayHashMismatch(t *testing.T) {
	src := "use strict; my @f; my %g; $f{a}; $g[0];"
	doc := parseDoc(src)
	diags := StrictVarDiagnostics(doc)
	if len(diags) != 2 {
		t.Fatalf("expected 2 diag, got %d", len(diags))
	}
}

func TestStrictVarsDerefSigils(t *testing.T) {
	src := "use strict; my $f; @$f; %$f; @{$f}; %{$f};"
	doc := parseDoc(src)
	diags := StrictVarDiagnostics(doc)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diag, got %d", len(diags))
	}
}

func parseDoc(src string) *ppi.Document {
	doc := ppi.NewDocument(src)
	doc.ParseWithDiagnostics()
	return doc
}
