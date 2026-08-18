package youtube

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"yt-downloader/internal/toolchain"
)

type fakeToolchain struct {
	paths    toolchain.Paths
	versions toolchain.Versions
}

func (f *fakeToolchain) Ensure(context.Context) (toolchain.Paths, toolchain.Versions, error) {
	return f.paths, f.versions, nil
}

func (f *fakeToolchain) CheckForYTDLPUpdate(context.Context, bool) (bool, toolchain.Versions, error) {
	return false, f.versions, nil
}

func (f *fakeToolchain) RollbackYTDLP(context.Context) (bool, error) {
	return false, nil
}

func TestMetadataAndHighQualityDownload(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is Unix-only")
	}
	dir := t.TempDir()
	ytdlp := writeExecutable(t, dir, "yt-dlp", `#!/bin/sh
metadata=0
output=""
previous=""
for argument in "$@"; do
  if [ "$argument" = "--dump-single-json" ]; then metadata=1; fi
  if [ "$previous" = "-o" ]; then output="$argument"; fi
  previous="$argument"
done
if [ "$metadata" = "1" ]; then
  echo '{"id":"abc123xyz00","title":"Test / Video","channel":"Test Channel","duration":12.5,"thumbnail":"https://example.com/t.jpg","formats":[{"width":640,"height":360},{"width":1920,"height":1080}]}'
  exit 0
fi
echo 'YT_PROGRESS: 50.0%' >&2
printf 'video' > "$output"
echo 'YT_PROGRESS:100.0%' >&2
`)
	ffprobe := writeExecutable(t, dir, "ffprobe", "#!/bin/sh\necho 1920x1080\n")
	versionTool := writeExecutable(t, dir, "tool", "#!/bin/sh\necho 1.0\n")
	tools := &fakeToolchain{
		paths:    toolchain.Paths{YTDLP: ytdlp, FFmpeg: versionTool, FFprobe: ffprobe, Deno: versionTool},
		versions: toolchain.Versions{Baseline: "test", YTDLP: "2099.01.01"},
	}
	downloader := NewDownloader(tools, nil)
	info, err := downloader.GetVideoInfo(context.Background(), "https://youtu.be/abc123xyz00")
	if err != nil {
		t.Fatal(err)
	}
	if info.Title != "Test _ Video" || info.SourceHeight != 1080 {
		t.Fatalf("unexpected metadata: %+v", info)
	}
	var progress []float64
	result, err := downloader.DownloadForPreview(context.Background(), "https://youtu.be/abc123xyz00", info, dir, func(value float64) {
		progress = append(progress, value)
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Method != "yt-dlp" || result.Width != 1920 || result.Height != 1080 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(progress) == 0 || progress[len(progress)-1] != 1 {
		t.Fatalf("unexpected progress: %v", progress)
	}
}

func TestRetryableErrorClassification(t *testing.T) {
	if !isRetryableYTDLPError(os.ErrPermission, "ERROR: unable to download video data: HTTP Error 403: Forbidden") {
		t.Fatal("expected HTTP 403 to trigger an update")
	}
	if isRetryableYTDLPError(os.ErrPermission, "output directory is read-only") {
		t.Fatal("filesystem error should not trigger a downloader update")
	}
}

func TestParseWxH(t *testing.T) {
	width, height := parseWxH("1920x1080")
	if width != 1920 || height != 1080 {
		t.Fatalf("unexpected dimensions: %dx%d", width, height)
	}
	width, height = parseWxH("invalid")
	if width != 0 || height != 0 {
		t.Fatalf("invalid dimensions were accepted: %dx%d", width, height)
	}
}

func TestLastLines(t *testing.T) {
	value := lastLines("one\ntwo\nthree", 2)
	if !strings.EqualFold(value, "two | three") {
		t.Fatalf("unexpected last lines: %q", value)
	}
}

func writeExecutable(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0755); err != nil {
		t.Fatal(err)
	}
	return path
}
