package lsp

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/skaji/perl-language-server/internal/analysis"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

func TestCompletionMethodUsesSigClass(t *testing.T) {
	s, tmp := newServerWithModule(t)

	src := "# :SIG(App::cpm::CLI -> void)\nsub foo ($app) {\n    $app->\n}\n"
	uri := protocol.DocumentUri("file://" + filepath.ToSlash(filepath.Join(tmp, "test.pl")))
	doc := s.docs.set(string(uri), src, nil)
	if doc == nil {
		t.Fatalf("expected document")
	}

	offset := findIndex(src, "$app->")
	if offset < 0 {
		t.Fatalf("expected $app-> in source")
	}
	offset += len("$app->")
	pos := positionFromOffset(src, offset)

	params := &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri},
			Position:     pos,
		},
	}
	got, err := s.completion(nil, params)
	if err != nil {
		t.Fatalf("completion error: %v", err)
	}
	list, ok := got.(protocol.CompletionList)
	if !ok {
		t.Fatalf("expected CompletionList, got %T", got)
	}
	if !hasCompletionLabel(list.Items, "bar") {
		t.Fatalf("expected bar completion, got %v", completionLabels(list.Items))
	}
}

func TestCompletionMethodSigCases(t *testing.T) {
	s, tmp := newServerWithModule(t)
	cases := []struct {
		name string
		src  string
	}{
		{
			name: "proto arg",
			src:  "# :SIG(App::cpm::CLI -> void)\nsub foo ($app) {\n    $app->\n}\n",
		},
		{
			name: "shift",
			src:  "# :SIG(App::cpm::CLI -> void)\nsub foo {\n    my $app = shift;\n    $app->\n}\n",
		},
		{
			name: "paren shift",
			src:  "# :SIG(App::cpm::CLI -> void)\nsub foo {\n    my ($app) = shift;\n    $app->\n}\n",
		},
		{
			name: "sig before my",
			src:  "sub foo {\n    # SIG(App::cpm::CLI)\n    my $app = shift;\n    $app->\n}\n",
		},
		{
			name: "sig before my list",
			src:  "sub foo {\n    # SIG(App::cpm::CLI)\n    my ($app) = @_;\n    $app->\n}\n",
		},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			uri := protocol.DocumentUri("file://" + filepath.ToSlash(filepath.Join(tmp, "case"+strconv.Itoa(i)+".pl")))
			doc := s.docs.set(string(uri), tc.src, nil)
			if doc == nil {
				t.Fatalf("expected document")
			}
			offset := findIndex(tc.src, "$app->")
			if offset < 0 {
				t.Fatalf("expected $app-> in source")
			}
			offset += len("$app->")
			pos := positionFromOffset(tc.src, offset)
			params := &protocol.CompletionParams{
				TextDocumentPositionParams: protocol.TextDocumentPositionParams{
					TextDocument: protocol.TextDocumentIdentifier{URI: uri},
					Position:     pos,
				},
			}
			got, err := s.completion(nil, params)
			if err != nil {
				t.Fatalf("completion error: %v", err)
			}
			list, ok := got.(protocol.CompletionList)
			if !ok {
				t.Fatalf("expected CompletionList, got %T", got)
			}
			if !hasCompletionLabel(list.Items, "bar") {
				t.Fatalf("expected bar completion, got %v", completionLabels(list.Items))
			}
		})
	}
}

func newServerWithModule(t *testing.T) (*Server, string) {
	t.Helper()
	tmp := t.TempDir()
	modPath := filepath.Join(tmp, "lib", "App", "cpm", "CLI.pm")
	if err := os.MkdirAll(filepath.Dir(modPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	modSrc := "package App::cpm::CLI;\nsub bar {}\n1;\n"
	if err := os.WriteFile(modPath, []byte(modSrc), 0o644); err != nil {
		t.Fatalf("write module: %v", err)
	}
	index, err := analysis.BuildWorkspaceIndex([]string{filepath.Join(tmp, "lib")})
	if err != nil {
		t.Fatalf("workspace index: %v", err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
	s := NewServer(logger)
	s.workspaceIndex = index
	return s, tmp
}

func hasCompletionLabel(items []protocol.CompletionItem, label string) bool {
	for _, item := range items {
		if item.Label == label {
			return true
		}
	}
	return false
}

func completionLabels(items []protocol.CompletionItem) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.Label)
	}
	return out
}
