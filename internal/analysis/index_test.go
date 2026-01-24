package analysis

import (
	"testing"

	ppi "github.com/skaji/go-ppi"
)

func TestVariablesAtOrder(t *testing.T) {
	src := "sub foo { my $x = 1; my $y = 2; }"
	doc := ppi.NewDocument(src)
	doc.ParseWithDiagnostics()
	idx := IndexDocument(doc)
	if idx == nil {
		t.Fatalf("expected index")
	}

	beforeX := offsetOf(t, src, "my $x") + 1
	vars := idx.VariablesAt(beforeX)
	if containsVar(vars, "$x") {
		t.Fatalf("did not expect $x before declaration")
	}

	afterX := offsetOf(t, src, "my $x") + len("my $x")
	vars = idx.VariablesAt(afterX)
	if !containsVar(vars, "$x") {
		t.Fatalf("expected $x after declaration")
	}

	beforeY := offsetOf(t, src, "my $y") + 1
	vars = idx.VariablesAt(beforeY)
	if containsVar(vars, "$y") {
		t.Fatalf("did not expect $y before declaration")
	}
	if !containsVar(vars, "$x") {
		t.Fatalf("expected $x before $y declaration")
	}
}

func TestVariablesAtListAndHash(t *testing.T) {
	src := "sub foo { my @a = (1); my %h = (a => 1); @a; %h; }"
	doc := ppi.NewDocument(src)
	doc.ParseWithDiagnostics()
	idx := IndexDocument(doc)
	if idx == nil {
		t.Fatalf("expected index")
	}

	beforeA := offsetOf(t, src, "my @a") + 1
	vars := idx.VariablesAt(beforeA)
	if containsVar(vars, "@a") {
		t.Fatalf("did not expect @a before declaration")
	}

	afterA := offsetOf(t, src, "my @a") + len("my @a")
	vars = idx.VariablesAt(afterA)
	if !containsVar(vars, "@a") {
		t.Fatalf("expected @a after declaration")
	}

	beforeH := offsetOf(t, src, "my %h") + 1
	vars = idx.VariablesAt(beforeH)
	if containsVar(vars, "%h") {
		t.Fatalf("did not expect %%h before declaration")
	}

	afterH := offsetOf(t, src, "my %h") + len("my %h")
	vars = idx.VariablesAt(afterH)
	if !containsVar(vars, "%h") {
		t.Fatalf("expected %%h after declaration")
	}
}

func TestVariablesAtShadowing(t *testing.T) {
	src := "my $x = 1; sub foo { my $x = 2; $x }"
	doc := ppi.NewDocument(src)
	doc.ParseWithDiagnostics()
	idx := IndexDocument(doc)
	if idx == nil {
		t.Fatalf("expected index")
	}

	inside := offsetOf(t, src, "$x }") + 1
	vars := idx.VariablesAt(inside)
	sym, ok := findVar(vars, "$x")
	if !ok {
		t.Fatalf("expected $x inside sub")
	}
	if sym.Storage != "my" {
		t.Fatalf("expected my $x inside sub, got %q", sym.Storage)
	}
}

func TestVariablesAtOurInDocumentScope(t *testing.T) {
	src := "our $g = 1; sub foo { $g }"
	doc := ppi.NewDocument(src)
	doc.ParseWithDiagnostics()
	idx := IndexDocument(doc)
	if idx == nil {
		t.Fatalf("expected index")
	}

	inside := offsetOf(t, src, "$g }") + 1
	vars := idx.VariablesAt(inside)
	if !containsVar(vars, "$g") {
		t.Fatalf("expected our $g to be visible inside sub")
	}
}

func TestVariablesAtUseVars(t *testing.T) {
	src := "use vars qw($g $h); sub foo { $g; $h }"
	doc := ppi.NewDocument(src)
	doc.ParseWithDiagnostics()
	idx := IndexDocument(doc)
	if idx == nil {
		t.Fatalf("expected index")
	}

	inside := offsetOf(t, src, "$h }") + 1
	vars := idx.VariablesAt(inside)
	if !containsVar(vars, "$g") || !containsVar(vars, "$h") {
		t.Fatalf("expected use vars to declare $g and $h")
	}
}

func TestVariablesAtSignatureVars(t *testing.T) {
	src := "use experimental 'signatures'; sub foo ($self, $opt, @rest) { $self; $opt; @rest }"
	doc := ppi.NewDocument(src)
	doc.ParseWithDiagnostics()
	idx := IndexDocument(doc)
	if idx == nil {
		t.Fatalf("expected index")
	}
	inside := offsetOf(t, src, "@rest }") + 1
	vars := idx.VariablesAt(inside)
	if !containsVar(vars, "$self") || !containsVar(vars, "$opt") || !containsVar(vars, "@rest") {
		t.Fatalf("expected signature vars to be visible")
	}
}

func TestVarDefinitionAt(t *testing.T) {
	src := "my $x = 1; $x = 2;"
	doc := ppi.NewDocument(src)
	doc.ParseWithDiagnostics()
	idx := IndexDocument(doc)
	if idx == nil {
		t.Fatalf("expected index")
	}
	useOffset := offsetOf(t, src, "$x = 2")
	def, ok := idx.VarDefinitionAt("$x", useOffset)
	if !ok {
		t.Fatalf("expected var definition")
	}
	defOffset := offsetOf(t, src, "my $x")
	if def.Start != defOffset+len("my ") {
		t.Fatalf("expected definition at %d, got %d", defOffset, def.Start)
	}
}

func TestVariablesAtAnonSignatureVars(t *testing.T) {
	src := "my $cb = sub ($self, $opt) { $self; $opt };"
	doc := ppi.NewDocument(src)
	doc.ParseWithDiagnostics()
	idx := IndexDocument(doc)
	if idx == nil {
		t.Fatalf("expected index")
	}
	inside := offsetOf(t, src, "$opt };") + 1
	vars := idx.VariablesAt(inside)
	if !containsVar(vars, "$self") || !containsVar(vars, "$opt") {
		t.Fatalf("expected anon signature vars to be visible")
	}
}

func TestReceiverNamesFromSignature(t *testing.T) {
	src := "sub foo ($self) { $self->bar }"
	doc := ppi.NewDocument(src)
	doc.ParseWithDiagnostics()
	idx := IndexDocument(doc)
	if idx == nil {
		t.Fatalf("expected index")
	}
	inside := offsetOf(t, src, "$self->") + 1
	receivers := idx.ReceiverNamesAt(inside)
	if receivers == nil {
		t.Fatalf("expected receiver names")
	}
	if _, ok := receivers["$self"]; !ok {
		t.Fatalf("expected $self to be receiver")
	}
}

func TestReceiverNamesFromShift(t *testing.T) {
	src := "sub foo { my $self = shift; $self->bar }"
	doc := ppi.NewDocument(src)
	doc.ParseWithDiagnostics()
	idx := IndexDocument(doc)
	if idx == nil {
		t.Fatalf("expected index")
	}
	inside := offsetOf(t, src, "$self->") + 1
	receivers := idx.ReceiverNamesAt(inside)
	if receivers == nil {
		t.Fatalf("expected receiver names")
	}
	if _, ok := receivers["$self"]; !ok {
		t.Fatalf("expected $self to be receiver")
	}
}

func TestReceiverNamesFromArray(t *testing.T) {
	src := "sub foo { my ($self) = @_; $self->bar }"
	doc := ppi.NewDocument(src)
	doc.ParseWithDiagnostics()
	idx := IndexDocument(doc)
	if idx == nil {
		t.Fatalf("expected index")
	}
	inside := offsetOf(t, src, "$self->") + 1
	receivers := idx.ReceiverNamesAt(inside)
	if receivers == nil {
		t.Fatalf("expected receiver names")
	}
	if _, ok := receivers["$self"]; !ok {
		t.Fatalf("expected $self to be receiver")
	}
}

func TestReceiverNamesFromArrayWithExtraArgs(t *testing.T) {
	src := "sub foo { my ($self, $argv) = @_; $self->bar }"
	doc := ppi.NewDocument(src)
	doc.ParseWithDiagnostics()
	idx := IndexDocument(doc)
	if idx == nil {
		t.Fatalf("expected index")
	}
	inside := offsetOf(t, src, "$self->") + 1
	receivers := idx.ReceiverNamesAt(inside)
	if receivers == nil {
		t.Fatalf("expected receiver names")
	}
	if _, ok := receivers["$self"]; !ok {
		t.Fatalf("expected $self to be receiver")
	}
}

func TestReceiverNamesFromSubscript(t *testing.T) {
	src := "sub foo { my $self = $_[0]; $self->bar }"
	doc := ppi.NewDocument(src)
	doc.ParseWithDiagnostics()
	idx := IndexDocument(doc)
	if idx == nil {
		t.Fatalf("expected index")
	}
	inside := offsetOf(t, src, "$self->") + 1
	receivers := idx.ReceiverNamesAt(inside)
	if receivers == nil {
		t.Fatalf("expected receiver names")
	}
	if _, ok := receivers["$self"]; !ok {
		t.Fatalf("expected $self to be receiver")
	}
}

func offsetOf(t *testing.T, src, needle string) int {
	t.Helper()
	idx := -1
	if needle != "" {
		idx = findIndex(src, needle)
	}
	if idx < 0 {
		t.Fatalf("needle %q not found", needle)
	}
	return idx
}

func findIndex(src, needle string) int {
	for i := 0; i+len(needle) <= len(src); i++ {
		if src[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

func containsVar(vars []Symbol, name string) bool {
	_, ok := findVar(vars, name)
	return ok
}

func findVar(vars []Symbol, name string) (Symbol, bool) {
	for _, sym := range vars {
		if sym.Kind == SymbolVar && sym.Name == name {
			return sym, true
		}
	}
	return Symbol{}, false
}
