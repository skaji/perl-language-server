package main

import (
	"log/slog"
	"os"

	"github.com/skaji/perl-language-server/internal/lsp"
)

func main() {
	logger, closer, err := lsp.NewLoggerFromEnv()
	if err != nil {
		slog.Error("failed to initialize logger", "error", err)
		return
	}
	if closer != nil {
		defer closer.Close()
	}

	srv := lsp.NewServer(logger)
	if err := srv.RunStdio(); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
