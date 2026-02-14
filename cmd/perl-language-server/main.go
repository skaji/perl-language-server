package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/skaji/perl-language-server/internal/lsp"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println(version)
		return
	}

	logger, closer, err := lsp.NewLoggerFromEnv()
	if err != nil {
		slog.Error("failed to initialize logger", "error", err)
		return
	}
	if closer != nil {
		defer closer.Close()
	}

	srv := lsp.NewServer(logger, version)
	if err := srv.RunStdio(); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
