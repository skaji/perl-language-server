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

func TestStrictVarsSpecialArrayIndex(t *testing.T) {
	src := "use strict; $INC[-1];"
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

func TestStrictVarsHashSize(t *testing.T) {
	src := "use strict; my $x; $#{$x};"
	doc := parseDoc(src)
	diags := StrictVarDiagnostics(doc)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diag, got %d", len(diags))
	}
}

func TestStrictVarsHashSizeScalar(t *testing.T) {
	src := "use strict; my $headers; $#$headers;"
	doc := parseDoc(src)
	diags := StrictVarDiagnostics(doc)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diag, got %d", len(diags))
	}
}

func TestStrictVarsTypeglobDeref(t *testing.T) {
	src := "use strict; my $self; ${*$self}{+__PACKAGE__};"
	doc := parseDoc(src)
	diags := StrictVarDiagnostics(doc)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diag, got %d", len(diags))
	}
}

func TestStrictVarsDoubleDeref(t *testing.T) {
	src := "use strict; my $x; $$x;"
	doc := parseDoc(src)
	diags := StrictVarDiagnostics(doc)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diag, got %d", len(diags))
	}
}

func TestStrictVarsDoubleDerefSpecial(t *testing.T) {
	src := "use strict; $$_[0];"
	doc := parseDoc(src)
	diags := StrictVarDiagnostics(doc)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diag, got %d", len(diags))
	}
}

func TestStrictVarsPostDerefSigils(t *testing.T) {
	src := "use strict; my $x; $x->@*; $x->%*;"
	doc := parseDoc(src)
	diags := StrictVarDiagnostics(doc)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diag, got %d", len(diags))
	}
}

func TestStrictVarsConfigSlice(t *testing.T) {
	src := "use strict; use Config; $Config{foo}; @Config{'foo','bar'};"
	doc := parseDoc(src)
	extra := map[string]struct{}{"%Config": {}}
	diags := StrictVarDiagnosticsWithExtra(doc, extra)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diag, got %d", len(diags))
	}
}

func TestStrictVarsTodoWithTestMore(t *testing.T) {
	src := "use strict; use Test::More; $TODO = 'todo';"
	doc := parseDoc(src)
	extra := map[string]struct{}{"$TODO": {}}
	diags := StrictVarDiagnosticsWithExtra(doc, extra)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diag, got %d", len(diags))
	}
}

func TestStrictVarsExtraAllowlist(t *testing.T) {
	src := "use strict; $FOO = 1;"
	doc := parseDoc(src)
	extra := map[string]struct{}{"$FOO": {}}
	diags := StrictVarDiagnosticsWithExtra(doc, extra)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diag, got %d", len(diags))
	}
}

func TestStrictVarsEnvSlice(t *testing.T) {
	src := "use strict; @ENV{'foo','bar'};"
	doc := parseDoc(src)
	diags := StrictVarDiagnostics(doc)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diag, got %d", len(diags))
	}
}

func TestStrictVarsAmpersandSub(t *testing.T) {
	src := "use strict; &foo;"
	doc := parseDoc(src)
	diags := StrictVarDiagnostics(doc)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diag, got %d", len(diags))
	}
}

func TestStrictVarsModuloOperator(t *testing.T) {
	src := "use strict; my @copy; my $n; $n = @copy % 2;"
	doc := parseDoc(src)
	diags := StrictVarDiagnostics(doc)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diag, got %d", len(diags))
	}
}

func TestStrictVarsArrayLengthToken(t *testing.T) {
	src := "use strict; my @script_name; $#script_name;"
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
