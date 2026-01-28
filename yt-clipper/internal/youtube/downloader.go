package youtube

import (
	"context"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/kkdai/youtube/v2"
)

// VideoInfo holds metadata about a YouTube video
type VideoInfo struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	Author      string  `json:"author"`
	Duration    float64 `json:"duration"` // in seconds
	Thumbnail   string  `json:"thumbnail"`
	Description string  `json:"description"`
}

// ProgressCallback is called with download progress (0.0 to 1.0)
type ProgressCallback func(progress float64)

// Downloader handles YouTube video operations
type Downloader struct {
	client *youtube.Client
}

// NewDownloader creates a new YouTube downloader
func NewDownloader() *Downloader {
	return &Downloader{
		client: &youtube.Client{},
	}
}

// ExtractVideoID extracts the video ID from a YouTube URL
func ExtractVideoID(url string) (string, error) {
	patterns := []string{
		`(?:v=|\/v\/|youtu\.be\/|\/embed\/|\/shorts\/)([a-zA-Z0-9_-]{11})`,
		`^([a-zA-Z0-9_-]{11})$`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(url)
		if len(matches) > 1 {
			return matches[1], nil
		}
	}

	return "", fmt.Errorf("could not extract video ID from URL: %s", url)
}

// GetVideoInfo fetches metadata for a YouTube video without downloading
func (d *Downloader) GetVideoInfo(ctx context.Context, url string) (*VideoInfo, error) {
	videoID, err := ExtractVideoID(url)
	if err != nil {
		return nil, err
	}

	video, err := d.client.GetVideoContext(ctx, videoID)
	if err != nil {
		return nil, fmt.Errorf("failed to get video info: %w", err)
	}

	// Get best thumbnail
	thumbnail := ""
	if len(video.Thumbnails) > 0 {
		thumbnail = video.Thumbnails[len(video.Thumbnails)-1].URL
	}

	return &VideoInfo{
		ID:          video.ID,
		Title:       sanitizeFilename(video.Title),
		Author:      video.Author,
		Duration:    video.Duration.Seconds(),
		Thumbnail:   thumbnail,
		Description: video.Description,
	}, nil
}

// DownloadForPreview downloads a video for preview (720p or best available)
func (d *Downloader) DownloadForPreview(ctx context.Context, url string, destDir string, ffmpegPath string, progressCb ProgressCallback) (string, error) {
	videoID, err := ExtractVideoID(url)
	if err != nil {
		return "", err
	}

	video, err := d.client.GetVideoContext(ctx, videoID)
	if err != nil {
		return "", fmt.Errorf("failed to get video: %w", err)
	}

	// Find a suitable format (prefer 720p with audio, fallback to best)
	format := selectFormat(video.Formats)
	if format == nil {
		return "", fmt.Errorf("no suitable video format found")
	}

	// Get the stream
	stream, contentLength, err := d.client.GetStreamContext(ctx, video, format)
	if err != nil {
		return "", fmt.Errorf("failed to get stream: %w", err)
	}
	defer stream.Close()

	// Create destination file
	ext := extensionFromMimeType(format.MimeType)
	if ext == "" {
		ext = ".mp4"
	}
	filename := sanitizeFilename(video.Title) + ext
	destPath := filepath.Join(destDir, filename)
	file, err := os.Create(destPath)
	if err != nil {
		return "", fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	// Download with progress tracking
	if progressCb != nil && contentLength > 0 {
		reader := &progressReader{
			reader:        stream,
			total:         contentLength,
			progressCb:    progressCb,
			lastReported:  0,
			reportedCount: 0,
		}
		_, err = io.Copy(file, reader)
	} else {
		_, err = io.Copy(file, stream)
	}

	if err != nil {
		os.Remove(destPath)
		return "", fmt.Errorf("failed to download video: %w", err)
	}

	// If we couldn't get a Safari/WebKit-friendly MP4 (H.264 + AAC), optionally transcode.
	if needsSafariTranscode(*format) {
		if ffmpegPath != "" {
			previewPath := filepath.Join(destDir, sanitizeFilename(video.Title)+"-preview.mp4")
			if err := transcodeToMP4(ctx, ffmpegPath, destPath, previewPath); err == nil {
				_ = os.Remove(destPath)
				return previewPath, nil
			}
		}
	}

	return destPath, nil
}

// selectFormat picks the best format for preview (720p with audio preferred)
func selectFormat(formats youtube.FormatList) *youtube.Format {
	// Prefer MP4 container with H.264 + AAC (most compatible with WebKit/Safari).
	var mp4H264 []youtube.Format
	var mp4Any []youtube.Format
	var withAudio []youtube.Format
	for _, f := range formats {
		if f.AudioChannels > 0 && strings.Contains(f.MimeType, "video") {
			withAudio = append(withAudio, f)
			if strings.Contains(f.MimeType, "video/mp4") {
				mp4Any = append(mp4Any, f)
				if strings.Contains(f.MimeType, "avc1") && strings.Contains(f.MimeType, "mp4a") {
					mp4H264 = append(mp4H264, f)
				}
			}
		}
	}

	if best := pickBestWithAudio(mp4H264); best != nil {
		return best
	}

	if best := pickBestWithAudio(mp4Any); best != nil {
		return best
	}

	if best := pickBestWithAudio(withAudio); best != nil {
		return best
	}

	// Last resort: any video format
	for _, f := range formats {
		if strings.Contains(f.MimeType, "video") {
			return &f
		}
	}

	return nil
}

func pickBestWithAudio(formats []youtube.Format) *youtube.Format {
	if len(formats) == 0 {
		return nil
	}

	// Prefer 720p if available, otherwise highest resolution; break ties by bitrate.
	bestIdx := 0
	for i := 1; i < len(formats); i++ {
		f := formats[i]
		best := formats[bestIdx]
		if f.Height == 720 && best.Height != 720 {
			bestIdx = i
			continue
		}
		if best.Height == 720 && f.Height != 720 {
			continue
		}
		if f.Height > best.Height {
			bestIdx = i
			continue
		}
		if f.Height == best.Height && f.Bitrate > best.Bitrate {
			bestIdx = i
		}
	}

	return &formats[bestIdx]
}

func extensionFromMimeType(mimeType string) string {
	switch {
	case strings.Contains(mimeType, "video/mp4"):
		return ".mp4"
	case strings.Contains(mimeType, "video/webm"):
		return ".webm"
	default:
		return ""
	}
}

func needsSafariTranscode(f youtube.Format) bool {
	// If it's a progressive MP4 with H.264 + AAC, we can usually play it directly.
	if strings.Contains(f.MimeType, "video/mp4") && strings.Contains(f.MimeType, "avc1") && strings.Contains(f.MimeType, "mp4a") {
		return false
	}
	return true
}

func transcodeToMP4(ctx context.Context, ffmpegPath string, inputPath string, outputPath string) error {
	// H.264 + AAC, yuv420p for broad compatibility; faststart improves seeking.
	cmd := exec.CommandContext(ctx, ffmpegPath,
		"-y",
		"-hide_banner",
		"-loglevel", "error",
		"-i", inputPath,
		"-c:v", "libx264",
		"-pix_fmt", "yuv420p",
		"-c:a", "aac",
		"-movflags", "+faststart",
		outputPath,
	)
	var stderr bytes.Buffer
	cmd.Stdout = nil
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return err
		}
		return fmt.Errorf("%w: %s", err, msg)
	}
	return nil
}

// progressReader wraps a reader to report progress
type progressReader struct {
	reader        io.Reader
	total         int64
	read          int64
	progressCb    ProgressCallback
	lastReported  float64
	reportedCount int
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	pr.read += int64(n)

	if pr.progressCb != nil && pr.total > 0 {
		progress := float64(pr.read) / float64(pr.total)
		// Only report every 1% change to avoid flooding
		if progress-pr.lastReported >= 0.01 || progress >= 1.0 {
			pr.progressCb(progress)
			pr.lastReported = progress
		}
	}

	return n, err
}

// sanitizeFilename removes or replaces invalid characters for filenames
func sanitizeFilename(name string) string {
	// Replace invalid characters
	invalid := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|"}
	result := name
	for _, char := range invalid {
		result = strings.ReplaceAll(result, char, "_")
	}
	// Trim spaces and dots from ends
	result = strings.TrimSpace(result)
	result = strings.Trim(result, ".")
	// Limit length
	if len(result) > 200 {
		result = result[:200]
	}
	return result
}
