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

func TestHoverSigCases(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{
			name: "proto arg",
			src:  "# :SIG(App::cpm::CLI -> void)\nsub foo ($app) {\n    $app;\n}\n",
		},
		{
			name: "shift",
			src:  "# :SIG(App::cpm::CLI -> void)\nsub foo {\n    my $app = shift;\n    $app;\n}\n",
		},
		{
			name: "paren shift",
			src:  "# :SIG(App::cpm::CLI -> void)\nsub foo {\n    my ($app) = shift;\n    $app;\n}\n",
		},
		{
			name: "sig before my",
			src:  "sub foo {\n    # SIG(App::cpm::CLI)\n    my $app = shift;\n    $app;\n}\n",
		},
		{
			name: "sig before my list",
			src:  "sub foo {\n    # SIG(App::cpm::CLI)\n    my ($app) = @_;\n    $app;\n}\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			idx := newDocumentStore()
			d := idx.set("file:///test.pl", tc.src, nil)
			offset := findIndex(tc.src, "$app;")
			if offset < 0 {
				t.Fatalf("expected $app in source")
			}
			sig := hoverVarSigType(d, offset, "$app", nil)
			if sig != "App::cpm::CLI" {
				t.Fatalf("expected App::cpm::CLI, got %q", sig)
			}
		})
	}
}

func TestHoverSigReturnFromCall(t *testing.T) {
	src := "# :SIG(any -> App::cpm::CLI)\nsub bar {\n}\n\nmy $x = bar(undef);\nmy $y = __PACKAGE__->bar();\nmy $z = __PACKAGE__->bar;\n$x;\n$y;\n$z;\n"
	idx := newDocumentStore()
	d := idx.set("file:///test.pl", src, nil)
	cases := []struct {
		name   string
		needle string
	}{
		{name: "x", needle: "$x;"},
		{name: "y", needle: "$y;"},
		{name: "z", needle: "$z;"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			offset := findIndex(src, tc.needle)
			if offset < 0 {
				t.Fatalf("expected %s in source", tc.needle)
			}
			sig := hoverVarSigType(d, offset, "$"+tc.name, nil)
			if sig != "App::cpm::CLI" {
				t.Fatalf("expected App::cpm::CLI, got %q", sig)
			}
		})
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
