package infrastructure

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

// NewLogger creates a new structured logger configured from the application
// configuration.
//
// IMPORTANT: When the MCP transport is stdio, logs MUST be written to stderr.
// MCP uses stdin/stdout for JSON-RPC framing, so any log line written to stdout
// would corrupt the protocol stream.
func NewLogger(config *Config) *slog.Logger {
	level := parseLogLevel(config.Logging.Level)

	opts := &slog.HandlerOptions{
		Level: level,
	}

	// Console stream: stderr for stdio transport (stdout carries MCP JSON-RPC),
	// stdout otherwise.
	var logOutput io.Writer = os.Stderr
	if config.MCP.Transport != "stdio" {
		logOutput = os.Stdout
	}

	// Additionally tee to a file when configured. The handle intentionally lives
	// for the process lifetime and is reclaimed by the OS on exit.
	if config.Logging.File != "" {
		f, err := os.OpenFile(config.Logging.File, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "logger: failed to open log file %q, continuing without it: %v\n", config.Logging.File, err)
		} else {
			logOutput = io.MultiWriter(logOutput, f)
		}
	}

	var handler slog.Handler
	switch strings.ToLower(config.Logging.Format) {
	case "text":
		handler = slog.NewTextHandler(logOutput, opts)
	case "json":
		handler = slog.NewJSONHandler(logOutput, opts)
	default:
		handler = slog.NewJSONHandler(logOutput, opts)
	}

	return slog.New(handler)
}

// parseLogLevel parses the log level string into a slog.Level.
func parseLogLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
