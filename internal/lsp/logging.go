package lsp

import (
	"io"
	"log/slog"
	"os"
)

func NewLoggerFromEnv() (*slog.Logger, io.Closer, error) {
	var (
		writer io.Writer = os.Stderr
		closer io.Closer
	)

	if path := os.Getenv("LOG_FILE"); path != "" {
		file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, nil, err
		}
		writer = file
		closer = file
	}

	level := slog.LevelInfo
	if os.Getenv("DEBUG") != "" {
		level = slog.LevelDebug
	}

	handler := slog.NewJSONHandler(writer, &slog.HandlerOptions{
		Level: level,
	})
	logger := slog.New(handler)
	slog.SetDefault(logger)

	return logger, closer, nil
}
