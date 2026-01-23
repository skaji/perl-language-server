package lsp

import (
	"io"
	"log/slog"

	ppi "github.com/skaji/go-ppi"
)

// ExportedStrictVars returns strict-vars allowlist based on use/imports and module exports.
func ExportedStrictVars(doc *ppi.Document, filePath string) map[string]struct{} {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := NewServer(logger)
	return srv.exportedStrictVars(doc, filePath)
}
