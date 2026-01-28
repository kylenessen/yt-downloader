package youtube

import (
	"context"
	"fmt"
	"io"
	"os"
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
func (d *Downloader) DownloadForPreview(ctx context.Context, url string, destDir string, progressCb ProgressCallback) (string, error) {
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
	filename := sanitizeFilename(video.Title) + ".mp4"
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

	return destPath, nil
}

// selectFormat picks the best format for preview (720p with audio preferred)
func selectFormat(formats youtube.FormatList) *youtube.Format {
	// Filter formats with both video and audio
	var withAudio []youtube.Format
	for _, f := range formats {
		if f.AudioChannels > 0 && strings.Contains(f.MimeType, "video") {
			withAudio = append(withAudio, f)
		}
	}

	// Prefer 720p
	for _, f := range withAudio {
		if f.Height == 720 {
			return &f
		}
	}

	// Fallback to any format with audio, prefer higher quality
	if len(withAudio) > 0 {
		best := withAudio[0]
		for _, f := range withAudio {
			if f.Height > best.Height {
				best = f
			}
		}
		return &best
	}

	// Last resort: any video format
	for _, f := range formats {
		if strings.Contains(f.MimeType, "video") {
			return &f
		}
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
