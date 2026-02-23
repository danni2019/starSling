package logging

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

func New(level string) *slog.Logger {
	return NewWithWriter(level, os.Stdout)
}

func NewWithWriter(level string, writer io.Writer) *slog.Logger {
	resolved := parseLevel(level)
	if writer == nil {
		writer = io.Discard
	}
	handler := slog.NewTextHandler(writer, &slog.HandlerOptions{Level: resolved})
	return slog.New(handler)
}

func NewDiscard(level string) *slog.Logger {
	resolved := parseLevel(level)
	handler := slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: resolved})
	return slog.New(handler)
}

func parseLevel(level string) slog.Level {
	switch strings.ToUpper(level) {
	case "DEBUG":
		return slog.LevelDebug
	case "WARN", "WARNING":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
