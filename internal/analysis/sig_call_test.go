package analysis

import (
	"strings"
	"testing"

	ppi "github.com/skaji/go-ppi"
)

func TestSigCallDiagnosticsSimple(t *testing.T) {
	src := `
# :SIG((any, int) -> void)
sub foo {
}
foo(1, 2);
foo(1);
foo(1, 2, 3);
foo(@args);
# :SIG(any -> void)
sub bar {
}
bar();
`
	doc := ppi.NewDocument(src)
	doc.ParseWithDiagnostics()
	diags := SigCallDiagnostics(doc)
	if len(diags) != 3 {
		t.Fatalf("expected 3 diags, got %d", len(diags))
	}
	msgs := []string{diags[0].Message, diags[1].Message, diags[2].Message}
	if !contains(msgs, "expected 2 args") {
		t.Fatalf("expected mismatch message, got %v", msgs)
	}
	if !contains(msgs, "expected 1 args") {
		t.Fatalf("expected bar mismatch message, got %v", msgs)
	}
}

func contains(list []string, needle string) bool {
	for _, item := range list {
		if strings.Contains(item, needle) {
			return true
		}
	}
	return false
}
