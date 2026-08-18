package video

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestServerRejectsSiblingDirectoryWithSharedPrefix(t *testing.T) {
	root := t.TempDir()
	allowed := filepath.Join(root, "video")
	sibling := filepath.Join(root, "video-private")
	if err := os.MkdirAll(allowed, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sibling, 0755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(sibling, "secret.mp4")
	if err := os.WriteFile(file, []byte("not a real video"), 0644); err != nil {
		t.Fatal(err)
	}

	server := NewServer()
	server.SetAllowedDir(allowed)
	server.SetCurrentVideo(file, "abc123xyz00")
	request := httptest.NewRequest(http.MethodGet, "/video/abc123xyz00", nil)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("expected forbidden, got %d", response.Code)
	}
}

func TestServerSupportsRangeRequests(t *testing.T) {
	allowed := t.TempDir()
	file := filepath.Join(allowed, "preview.mp4")
	if err := os.WriteFile(file, []byte("0123456789"), 0644); err != nil {
		t.Fatal(err)
	}
	server := NewServer()
	server.SetAllowedDir(allowed)
	server.SetCurrentVideo(file, "abc123xyz00")
	request := httptest.NewRequest(http.MethodGet, "/video/abc123xyz00", nil)
	request.Header.Set("Range", "bytes=2-5")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusPartialContent || response.Body.String() != "2345" {
		t.Fatalf("unexpected range response: status=%d body=%q", response.Code, response.Body.String())
	}
}
