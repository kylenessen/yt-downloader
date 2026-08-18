package diagnostics

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoggerSnapshot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "app.log")
	logger, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer logger.Close()
	logger.Printf("download method=%s resolution=%s", "yt-dlp", "1920x1080")
	snapshot, err := logger.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(snapshot, "method=yt-dlp resolution=1920x1080") {
		t.Fatalf("unexpected snapshot: %q", snapshot)
	}
}

func TestLoggerRotatesLargeFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	if err := os.WriteFile(path, make([]byte, maxLogSize), 0644); err != nil {
		t.Fatal(err)
	}
	logger, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer logger.Close()
	if _, err := os.Stat(path + ".1"); err != nil {
		t.Fatalf("rotated log missing: %v", err)
	}
}
