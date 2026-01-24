package lsp

import (
	"io"
	"log/slog"
	"testing"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

func TestCompletionMethodWithoutSigNoMethodKind(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
	s := NewServer(logger)

	src := "package App::cpm::CLI;\nsub bar {}\nsub foo { my $app = shift; $app-> }\n"
	uri := protocol.DocumentUri("file:///test.pl")
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
	if hasCompletionLabelKind(list.Items, "bar", protocol.CompletionItemKindMethod) {
		t.Fatalf("unexpected method completion for bar")
	}
}

func hasCompletionLabelKind(items []protocol.CompletionItem, label string, kind protocol.CompletionItemKind) bool {
	for _, item := range items {
		if item.Label != label || item.Kind == nil {
			continue
		}
		if *item.Kind == kind {
			return true
		}
	}
	return false
}
