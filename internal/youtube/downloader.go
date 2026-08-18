package youtube

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"yt-downloader/internal/toolchain"
)

const (
	highQualitySelector = "bestvideo[vcodec^=avc1]+bestaudio[acodec^=mp4a]/bestvideo[vcodec^=avc1]+bestaudio/bestvideo+bestaudio"
	progressiveSelector = "best[ext=mp4][vcodec^=avc1]/best[ext=mp4]/best"
)

var progressPattern = regexp.MustCompile(`([0-9]+(?:\.[0-9]+)?)%`)

// VideoInfo holds metadata reported by yt-dlp.
type VideoInfo struct {
	ID           string  `json:"id"`
	Title        string  `json:"title"`
	Author       string  `json:"author"`
	Duration     float64 `json:"duration"`
	Thumbnail    string  `json:"thumbnail"`
	Description  string  `json:"description"`
	SourceWidth  int     `json:"sourceWidth"`
	SourceHeight int     `json:"sourceHeight"`
}

// DownloadAttempt records one internal attempt for diagnostics and UI status.
type DownloadAttempt struct {
	Method       string `json:"method"`
	YTDLPVersion string `json:"ytDlpVersion"`
	Error        string `json:"error,omitempty"`
}

// DownloadResult holds the preview download outcome.
type DownloadResult struct {
	FilePath string            `json:"filePath"`
	Method   string            `json:"method"`
	Width    int               `json:"width"`
	Height   int               `json:"height"`
	Attempts []DownloadAttempt `json:"attempts"`
}

// ProgressCallback is called with download progress from 0 to 1.
type ProgressCallback func(progress float64)

// Toolchain is the subset of the app-owned manager used by Downloader.
type Toolchain interface {
	Ensure(ctx context.Context) (toolchain.Paths, toolchain.Versions, error)
	CheckForYTDLPUpdate(ctx context.Context, force bool) (bool, toolchain.Versions, error)
	RollbackYTDLP(ctx context.Context) (bool, error)
}

// Downloader delegates all YouTube-specific behavior to yt-dlp.
type Downloader struct {
	tools Toolchain
	logf  func(format string, args ...any)
}

// NewDownloader creates a yt-dlp adapter.
func NewDownloader(tools Toolchain, logf func(format string, args ...any)) *Downloader {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	return &Downloader{tools: tools, logf: logf}
}

// GetVideoInfo fetches metadata and refreshes yt-dlp once if extraction fails.
func (d *Downloader) GetVideoInfo(ctx context.Context, url string) (*VideoInfo, error) {
	paths, versions, err := d.tools.Ensure(ctx)
	if err != nil {
		return nil, err
	}
	info, stderr, err := d.getVideoInfo(ctx, paths, url)
	if err == nil {
		d.logf("Metadata loaded id=%s yt-dlp=%s source=%dx%d", info.ID, versions.YTDLP, info.SourceWidth, info.SourceHeight)
		return info, nil
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	d.logf("Metadata extraction failed yt-dlp=%s error=%v output=%s", versions.YTDLP, err, stderr)

	updated, updateVersions, updateErr := d.tools.CheckForYTDLPUpdate(ctx, true)
	if updateErr != nil {
		d.logf("yt-dlp recovery update failed error=%v", updateErr)
		return nil, fmt.Errorf("yt-dlp metadata error: %w; update failed: %v", err, updateErr)
	}
	if !updated {
		return nil, fmt.Errorf("yt-dlp metadata error: %w: %s", err, stderr)
	}
	paths, _, err = d.tools.Ensure(ctx)
	if err != nil {
		return nil, err
	}
	info, stderr, err = d.getVideoInfo(ctx, paths, url)
	if err != nil {
		return nil, fmt.Errorf("yt-dlp metadata error after update to %s: %w: %s", updateVersions.YTDLP, err, stderr)
	}
	d.logf("Metadata recovery succeeded id=%s yt-dlp=%s", info.ID, updateVersions.YTDLP)
	return info, nil
}

// DownloadForPreview downloads the best compatible preview. It refreshes
// yt-dlp before using a progressive fallback.
func (d *Downloader) DownloadForPreview(ctx context.Context, url string, info *VideoInfo, destDir string, progressCb ProgressCallback) (*DownloadResult, error) {
	if info == nil || info.ID == "" {
		return nil, errors.New("video metadata is required")
	}
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return nil, fmt.Errorf("create preview directory: %w", err)
	}

	paths, versions, err := d.tools.Ensure(ctx)
	if err != nil {
		return nil, err
	}
	outPath := filepath.Join(destDir, info.ID+"-preview.mp4")
	attempts := make([]DownloadAttempt, 0, 4)

	result, stderr, err := d.download(ctx, paths, url, outPath, highQualitySelector, "yt-dlp", progressCb)
	attempts = append(attempts, attempt("yt-dlp", versions.YTDLP, err, stderr))
	if err == nil {
		result.Attempts = attempts
		d.logResult(*result)
		return result, nil
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	d.logf("High-quality download failed yt-dlp=%s error=%v output=%s", versions.YTDLP, err, stderr)
	cleanupDownloadArtifacts(destDir, filepath.Base(strings.TrimSuffix(outPath, filepath.Ext(outPath))))

	updated := false
	if isRetryableYTDLPError(err, stderr) {
		var updateErr error
		updated, versions, updateErr = d.tools.CheckForYTDLPUpdate(ctx, true)
		if updateErr != nil {
			attempts = append(attempts, DownloadAttempt{Method: "update", YTDLPVersion: versions.YTDLP, Error: updateErr.Error()})
			d.logf("yt-dlp recovery update failed error=%v", updateErr)
		} else if updated {
			paths, versions, err = d.tools.Ensure(ctx)
			if err != nil {
				return nil, err
			}
			result, stderr, err = d.download(ctx, paths, url, outPath, highQualitySelector, "yt-dlp", progressCb)
			attempts = append(attempts, attempt("yt-dlp-after-update", versions.YTDLP, err, stderr))
			if err == nil {
				result.Attempts = attempts
				d.logResult(*result)
				return result, nil
			}
			d.logf("High-quality retry failed yt-dlp=%s error=%v output=%s", versions.YTDLP, err, stderr)
			cleanupDownloadArtifacts(destDir, filepath.Base(strings.TrimSuffix(outPath, filepath.Ext(outPath))))
		}
	}

	result, fallbackOutput, fallbackErr := d.download(ctx, paths, url, outPath, progressiveSelector, "progressive", progressCb)
	attempts = append(attempts, attempt("progressive", versions.YTDLP, fallbackErr, fallbackOutput))
	if fallbackErr == nil {
		result.Attempts = attempts
		d.logResult(*result)
		return result, nil
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	if updated {
		if rolledBack, rollbackErr := d.tools.RollbackYTDLP(ctx); rollbackErr != nil {
			d.logf("yt-dlp rollback failed error=%v", rollbackErr)
		} else if rolledBack {
			paths, versions, err = d.tools.Ensure(ctx)
			if err == nil {
				cleanupDownloadArtifacts(destDir, filepath.Base(strings.TrimSuffix(outPath, filepath.Ext(outPath))))
				result, fallbackOutput, fallbackErr = d.download(ctx, paths, url, outPath, progressiveSelector, "progressive", progressCb)
				attempts = append(attempts, attempt("progressive-after-rollback", versions.YTDLP, fallbackErr, fallbackOutput))
				if fallbackErr == nil {
					result.Attempts = attempts
					d.logResult(*result)
					return result, nil
				}
			}
		}
	}

	return nil, fmt.Errorf("high-quality and progressive downloads failed: %w: %s", fallbackErr, fallbackOutput)
}

func (d *Downloader) getVideoInfo(ctx context.Context, paths toolchain.Paths, url string) (*VideoInfo, string, error) {
	args := []string{
		"--ignore-config",
		"--no-playlist",
		"--no-color",
		"--dump-single-json",
		"--skip-download",
		"--ffmpeg-location", filepath.Dir(paths.FFmpeg),
		"--js-runtimes", "deno:" + paths.Deno,
		"--", url,
	}
	cmd := exec.CommandContext(ctx, paths.YTDLP, args...)
	var stdout bytes.Buffer
	stderr := &limitedBuffer{limit: 128 * 1024}
	cmd.Stdout = &stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return nil, strings.TrimSpace(stderr.String()), fmt.Errorf("run yt-dlp metadata: %w", err)
	}

	var raw struct {
		ID          string  `json:"id"`
		Title       string  `json:"title"`
		Channel     string  `json:"channel"`
		Uploader    string  `json:"uploader"`
		Duration    float64 `json:"duration"`
		Thumbnail   string  `json:"thumbnail"`
		Description string  `json:"description"`
		Width       int     `json:"width"`
		Height      int     `json:"height"`
		Thumbnails  []struct {
			URL string `json:"url"`
		} `json:"thumbnails"`
		Formats []struct {
			Width  int `json:"width"`
			Height int `json:"height"`
		} `json:"formats"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &raw); err != nil {
		return nil, strings.TrimSpace(stderr.String()), fmt.Errorf("parse yt-dlp metadata: %w", err)
	}
	if raw.ID == "" || raw.Title == "" {
		return nil, strings.TrimSpace(stderr.String()), errors.New("yt-dlp returned incomplete metadata")
	}
	width, height := raw.Width, raw.Height
	for _, format := range raw.Formats {
		if format.Height > height || format.Height == height && format.Width > width {
			width, height = format.Width, format.Height
		}
	}
	thumbnail := raw.Thumbnail
	if thumbnail == "" && len(raw.Thumbnails) > 0 {
		thumbnail = raw.Thumbnails[len(raw.Thumbnails)-1].URL
	}
	author := raw.Channel
	if author == "" {
		author = raw.Uploader
	}
	return &VideoInfo{
		ID:           raw.ID,
		Title:        sanitizeFilename(raw.Title),
		Author:       author,
		Duration:     raw.Duration,
		Thumbnail:    thumbnail,
		Description:  raw.Description,
		SourceWidth:  width,
		SourceHeight: height,
	}, strings.TrimSpace(stderr.String()), nil
}

func (d *Downloader) download(ctx context.Context, paths toolchain.Paths, url, outPath, selector, method string, progressCb ProgressCallback) (*DownloadResult, string, error) {
	_ = os.Remove(outPath)
	args := []string{
		"--ignore-config",
		"--no-playlist",
		"--newline",
		"--no-color",
		"--progress",
		"--progress-template", "download:YT_PROGRESS:%(progress._percent_str)s",
		"--ffmpeg-location", filepath.Dir(paths.FFmpeg),
		"--js-runtimes", "deno:" + paths.Deno,
		"-f", selector,
		"--merge-output-format", "mp4",
		"--recode-video", "mp4",
		"-o", outPath,
		"--", url,
	}
	cmd := exec.CommandContext(ctx, paths.YTDLP, args...)
	output, err := runWithProgress(cmd, progressCb)
	if err != nil {
		return nil, output, fmt.Errorf("run yt-dlp download: %w", err)
	}
	info, err := os.Stat(outPath)
	if err != nil || info.Size() == 0 {
		return nil, output, errors.New("yt-dlp did not create a preview file")
	}
	width, height, err := probeResolution(ctx, paths.FFprobe, outPath)
	if err != nil {
		d.logf("Preview resolution probe failed path=%s error=%v", filepath.Base(outPath), err)
	}
	return &DownloadResult{FilePath: outPath, Method: method, Width: width, Height: height}, output, nil
}

func runWithProgress(cmd *exec.Cmd, callback ProgressCallback) (string, error) {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", err
	}
	collector := &lineCollector{limit: 256 * 1024}
	reporter := &progressReporter{callback: callback}
	if err := cmd.Start(); err != nil {
		return "", err
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go consumeOutput(stdout, collector, reporter, &wg)
	go consumeOutput(stderr, collector, reporter, &wg)
	wg.Wait()
	waitErr := cmd.Wait()
	if waitErr == nil {
		reporter.complete()
	}
	return collector.String(), waitErr
}

func consumeOutput(reader io.Reader, collector *lineCollector, reporter *progressReporter, wg *sync.WaitGroup) {
	defer wg.Done()
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		collector.Add(line)
		if strings.Contains(line, "YT_PROGRESS:") {
			reporter.reportLine(line)
		}
	}
	if err := scanner.Err(); err != nil {
		collector.Add("output read error: " + err.Error())
	}
}

type progressReporter struct {
	mu       sync.Mutex
	callback ProgressCallback
	last     float64
}

func (p *progressReporter) reportLine(line string) {
	if p.callback == nil {
		return
	}
	match := progressPattern.FindStringSubmatch(line)
	if len(match) != 2 {
		return
	}
	percent, err := strconv.ParseFloat(match[1], 64)
	if err != nil {
		return
	}
	progress := percent / 100 * 0.95
	p.mu.Lock()
	defer p.mu.Unlock()
	if progress <= p.last {
		return
	}
	p.last = progress
	p.callback(progress)
}

func (p *progressReporter) complete() {
	if p.callback == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.last = 1
	p.callback(1)
}

type lineCollector struct {
	mu    sync.Mutex
	buf   bytes.Buffer
	limit int
}

func (c *lineCollector) Add(line string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.buf.Len() >= c.limit {
		return
	}
	remaining := c.limit - c.buf.Len()
	if len(line)+1 > remaining {
		line = line[:max(0, remaining-1)]
	}
	_, _ = c.buf.WriteString(line)
	_ = c.buf.WriteByte('\n')
}

func (c *lineCollector) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return strings.TrimSpace(c.buf.String())
}

type limitedBuffer struct {
	buf   bytes.Buffer
	limit int
}

func (l *limitedBuffer) Write(p []byte) (int, error) {
	remaining := l.limit - l.buf.Len()
	if remaining > 0 {
		if len(p) > remaining {
			_, _ = l.buf.Write(p[:remaining])
		} else {
			_, _ = l.buf.Write(p)
		}
	}
	return len(p), nil
}

func (l *limitedBuffer) String() string {
	return l.buf.String()
}

func probeResolution(ctx context.Context, ffprobePath, filePath string) (int, int, error) {
	output, err := exec.CommandContext(ctx, ffprobePath,
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height",
		"-of", "csv=p=0:s=x",
		filePath,
	).CombinedOutput()
	if err != nil {
		return 0, 0, fmt.Errorf("run ffprobe: %w: %s", err, strings.TrimSpace(string(output)))
	}
	width, height := parseWxH(strings.TrimSpace(string(output)))
	if width == 0 || height == 0 {
		return 0, 0, fmt.Errorf("parse ffprobe resolution %q", strings.TrimSpace(string(output)))
	}
	return width, height, nil
}

func parseWxH(value string) (int, int) {
	parts := strings.SplitN(value, "x", 2)
	if len(parts) != 2 {
		return 0, 0
	}
	width, widthErr := strconv.Atoi(strings.TrimSpace(parts[0]))
	height, heightErr := strconv.Atoi(strings.TrimSpace(parts[1]))
	if widthErr != nil || heightErr != nil {
		return 0, 0
	}
	return width, height
}

func cleanupDownloadArtifacts(dir, base string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, base) {
			continue
		}
		if strings.HasSuffix(name, ".part") || strings.HasSuffix(name, ".ytdl") || strings.Contains(name, ".f") {
			_ = os.Remove(filepath.Join(dir, name))
		}
	}
}

func attempt(method, version string, err error, output string) DownloadAttempt {
	attempt := DownloadAttempt{Method: method, YTDLPVersion: version}
	if err != nil {
		message := err.Error()
		if output != "" {
			message += ": " + lastLines(output, 8)
		}
		attempt.Error = message
	}
	return attempt
}

func isRetryableYTDLPError(err error, output string) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error() + " " + output)
	for _, marker := range []string{
		"http error 403",
		"forbidden",
		"extractor",
		"signature",
		"challenge",
		"requested format is not available",
		"unable to download",
		"unsupported url",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func lastLines(value string, count int) string {
	lines := strings.Split(strings.TrimSpace(value), "\n")
	if len(lines) > count {
		lines = lines[len(lines)-count:]
	}
	return strings.Join(lines, " | ")
}

func sanitizeFilename(name string) string {
	name = strings.TrimSpace(name)
	name = strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', 0:
			return '_'
		default:
			return r
		}
	}, name)
	name = strings.Trim(name, ". ")
	if len(name) > 120 {
		name = name[:120]
	}
	return name
}

func (d *Downloader) logResult(result DownloadResult) {
	d.logf("Preview ready method=%s resolution=%dx%d file=%s attempts=%d", result.Method, result.Width, result.Height, filepath.Base(result.FilePath), len(result.Attempts))
}
