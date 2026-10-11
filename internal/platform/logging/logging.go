// Package logging initializes the application-level structured logger.
// All log output goes to a file; never to stdout or stderr, which would
// break the TUI and the MCP stdio protocol.
package logging

import (
	"log/slog"
	"os"
)

// Logger is the package-level logger. It is nil until Init is called.
var Logger *slog.Logger

// Init opens (or creates) the log file at path and configures the package-level
// Logger with a JSON handler targeting that file.
// The caller is responsible for closing the returned file when the program exits,
// but the file handle is intentionally leaked for simplicity in a single-binary
// application (the OS reclaims it on exit).
func Init(path string) error {
	// Ensure parent directory exists.
	dir := dirOf(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}

	Logger = slog.New(slog.NewJSONHandler(f, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(Logger)
	return nil
}

// dirOf returns the directory component of a file path.
func dirOf(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[:i]
		}
	}
	return "."
}
