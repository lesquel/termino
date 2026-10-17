package logging

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitIsIdempotentForTheSamePath(t *testing.T) {
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev); Logger = nil; current = "" })

	path := filepath.Join(t.TempDir(), "state", "termino.log")
	if err := Init(path); err != nil {
		t.Fatalf("init: %v", err)
	}
	first := Logger
	if err := Init(path); err != nil {
		t.Fatalf("second init: %v", err)
	}
	if Logger != first {
		t.Fatal("second Init with the same path replaced the logger (leaks a file handle)")
	}

	slog.Info("probe-line")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(data), "probe-line") {
		t.Fatalf("log file %q does not contain the default-logger record", data)
	}
}
