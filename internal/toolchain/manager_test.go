package toolchain

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestEnsureInstallsVerifiedBootstrap(t *testing.T) {
	archive := makeTestArchive(t, "baseline-1", map[string]string{
		"yt-dlp":  "#!/bin/sh\necho baseline-ytdlp\n",
		"ffmpeg":  "#!/bin/sh\necho baseline-ffmpeg\n",
		"ffprobe": "#!/bin/sh\necho baseline-ffprobe\n",
		"deno":    "#!/bin/sh\necho baseline-deno\n",
	})
	m, err := NewManager(Config{
		RootDir:          filepath.Join(t.TempDir(), "tools"),
		BootstrapArchive: archive,
		GOOS:             runtime.GOOS,
		GOARCH:           runtime.GOARCH,
	})
	if err != nil {
		t.Fatal(err)
	}

	paths, versions, err := m.Ensure(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if versions.Baseline != "baseline-1" || versions.YTDLP != "baseline-ytdlp" {
		t.Fatalf("unexpected versions: %+v", versions)
	}
	for _, path := range []string{paths.YTDLP, paths.FFmpeg, paths.FFprobe, paths.Deno} {
		if !pathInside(m.RootDir(), path) {
			t.Fatalf("path escaped root: %s", path)
		}
	}
}

func TestEnsureRejectsCorruptBootstrap(t *testing.T) {
	archive := makeTestArchive(t, "baseline-1", map[string]string{
		"yt-dlp":  "#!/bin/sh\necho baseline-ytdlp\n",
		"ffmpeg":  "#!/bin/sh\necho baseline-ffmpeg\n",
		"ffprobe": "#!/bin/sh\necho baseline-ffprobe\n",
		"deno":    "#!/bin/sh\necho baseline-deno\n",
	})
	corruptZipEntry(t, archive, "yt-dlp")
	m, err := NewManager(Config{
		RootDir:          filepath.Join(t.TempDir(), "tools"),
		BootstrapArchive: archive,
		GOOS:             runtime.GOOS,
		GOARCH:           runtime.GOARCH,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = m.Ensure(context.Background())
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected checksum error, got %v", err)
	}
}

func TestNightlyUpdateAndRollback(t *testing.T) {
	archive := makeTestArchive(t, "baseline-1", map[string]string{
		"yt-dlp":  "#!/bin/sh\necho baseline-ytdlp\n",
		"ffmpeg":  "#!/bin/sh\necho baseline-ffmpeg\n",
		"ffprobe": "#!/bin/sh\necho baseline-ffprobe\n",
		"deno":    "#!/bin/sh\necho baseline-deno\n",
	})
	updateBody := []byte("#!/bin/sh\necho 2099.01.02\n")
	updateSum := sha256.Sum256(updateBody)
	assetName := nightlyAssetName(runtime.GOOS, runtime.GOARCH)

	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/release":
			_ = json.NewEncoder(w).Encode(githubRelease{
				TagName: "2099.01.02",
				Assets: []struct {
					Name               string `json:"name"`
					BrowserDownloadURL string `json:"browser_download_url"`
				}{
					{Name: assetName, BrowserDownloadURL: serverURL + "/asset"},
					{Name: "SHA2-256SUMS", BrowserDownloadURL: serverURL + "/sums"},
				},
			})
		case "/sums":
			_, _ = fmt.Fprintf(w, "%s  %s\n", hex.EncodeToString(updateSum[:]), assetName)
		case "/asset":
			_, _ = w.Write(updateBody)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	serverURL = server.URL

	m, err := NewManager(Config{
		RootDir:          filepath.Join(t.TempDir(), "tools"),
		BootstrapArchive: archive,
		GOOS:             runtime.GOOS,
		GOARCH:           runtime.GOARCH,
		HTTPClient:       server.Client(),
		NightlyAPIURL:    server.URL + "/release",
		Now:              func() time.Time { return time.Date(2099, 1, 2, 3, 4, 5, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.Ensure(context.Background()); err != nil {
		t.Fatal(err)
	}
	updated, versions, err := m.CheckForYTDLPUpdate(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if !updated || versions.YTDLP != "2099.01.02" {
		t.Fatalf("unexpected update result: updated=%v versions=%+v", updated, versions)
	}
	paths, activeVersions, err := m.Ensure(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if activeVersions.YTDLP != "2099.01.02" || !strings.Contains(paths.YTDLP, "updates") {
		t.Fatalf("update not active: paths=%+v versions=%+v", paths, activeVersions)
	}
	rolledBack, err := m.RollbackYTDLP(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !rolledBack {
		t.Fatal("expected rollback")
	}
	_, rolledBackVersions, err := m.Ensure(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rolledBackVersions.YTDLP != "baseline-ytdlp" {
		t.Fatalf("unexpected rollback version: %+v", rolledBackVersions)
	}
}

func makeTestArchive(t *testing.T, version string, tools map[string]string) string {
	t.Helper()
	archivePath := filepath.Join(t.TempDir(), "toolchain.zip")
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	manifest := archiveManifest{
		SchemaVersion: archiveSchemaVersion,
		Version:       version,
		GOOS:          runtime.GOOS,
		GOARCH:        runtime.GOARCH,
		Tools: map[string]string{
			"yt-dlp":  "yt-dlp",
			"ffmpeg":  "ffmpeg",
			"ffprobe": "ffprobe",
			"deno":    "deno",
		},
		Files: make(map[string]archiveFile),
	}
	for name, body := range tools {
		sum := sha256.Sum256([]byte(body))
		manifest.Files[name] = archiveFile{SHA256: hex.EncodeToString(sum[:])}
		entry, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(entry, body); err != nil {
			t.Fatal(err)
		}
	}
	manifestEntry, err := zw.Create("manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(manifestEntry).Encode(manifest); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return archivePath
}

func corruptZipEntry(t *testing.T, archivePath, target string) {
	t.Helper()
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	replacement := archivePath + ".replacement"
	out, err := os.Create(replacement)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(out)
	for _, entry := range reader.File {
		writer, err := zw.Create(entry.Name)
		if err != nil {
			t.Fatal(err)
		}
		if entry.Name == target {
			_, _ = io.WriteString(writer, "corrupt")
			continue
		}
		r, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		_, copyErr := io.Copy(writer, r)
		_ = r.Close()
		if copyErr != nil {
			t.Fatal(copyErr)
		}
	}
	_ = reader.Close()
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, archivePath); err != nil {
		t.Fatal(err)
	}
}
