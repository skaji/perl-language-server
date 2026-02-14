package lsp

import (
	"testing"
)

func TestPerlCompileDiagnostics(t *testing.T) {
	src := "use strict;\nuse Foo;\nmy $x = ;\n"
	out := "Can't locate Foo.pm in @INC (you may need to install the Foo module) (@INC contains: /tmp/lib) at test.pl line 2.\nsyntax error at test.pl line 3, near \";\"\ntest.pl had compilation errors.\n"

	diags := perlCompileDiagnostics(src, "/tmp/test.pl", out)
	if len(diags) != 2 {
		t.Fatalf("expected 2 diagnostics, got %d", len(diags))
	}
	if got := diags[0].Message; got != "Can't locate Foo.pm in @INC (you may need to install the Foo module) (@INC contains: /tmp/lib)" {
		t.Fatalf("unexpected first message: %q", got)
	}
	if got := diags[1].Message; got != "syntax error" {
		t.Fatalf("unexpected second message: %q", got)
	}
	if diags[0].Range.Start.Line != 1 {
		t.Fatalf("expected first diagnostic line 2, got %d", diags[0].Range.Start.Line+1)
	}
	if diags[1].Range.Start.Line != 2 {
		t.Fatalf("expected second diagnostic line 3, got %d", diags[1].Range.Start.Line+1)
	}
}

func TestPerlCompileDiagnosticsIgnoresOtherFiles(t *testing.T) {
	src := "use strict;\n"
	out := "syntax error at other.pl line 1.\n"
	diags := perlCompileDiagnostics(src, "/tmp/test.pl", out)
	if len(diags) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d", len(diags))
	}
}
