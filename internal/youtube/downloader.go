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
	ID           string  `json:"id"`
	Title        string  `json:"title"`
	Author       string  `json:"author"`
	Duration     float64 `json:"duration"` // in seconds
	Thumbnail    string  `json:"thumbnail"`
	Description  string  `json:"description"`
	SourceWidth  int     `json:"sourceWidth"`
	SourceHeight int     `json:"sourceHeight"`
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

	// Get source resolution from best available format
	sourceWidth, sourceHeight := 0, 0
	for _, f := range video.Formats {
		if f.Width > sourceWidth {
			sourceWidth = f.Width
			sourceHeight = f.Height
		}
	}

	return &VideoInfo{
		ID:           video.ID,
		Title:        sanitizeFilename(video.Title),
		Author:       video.Author,
		Duration:     video.Duration.Seconds(),
		Thumbnail:    thumbnail,
		Description:  video.Description,
		SourceWidth:  sourceWidth,
		SourceHeight: sourceHeight,
	}, nil
}

// DownloadForPreview downloads a video for preview (best quality available)
func (d *Downloader) DownloadForPreview(ctx context.Context, url string, destDir string, ffmpegPath string, progressCb ProgressCallback) (string, error) {
	videoID, err := ExtractVideoID(url)
	if err != nil {
		return "", err
	}

	video, err := d.client.GetVideoContext(ctx, videoID)
	if err != nil {
		return "", fmt.Errorf("failed to get video: %w", err)
	}

	baseName := sanitizeFilename(video.Title)
	outPath := filepath.Join(destDir, baseName+"-preview.mp4")

	// Try yt-dlp first - most reliable for high-quality downloads
	ytdlpPath, _ := exec.LookPath("yt-dlp")
	if ytdlpPath != "" && ffmpegPath != "" {
		fmt.Printf("[DEBUG] Trying yt-dlp for high-quality download\n")
		err := d.downloadWithYtdlp(ctx, url, outPath, ffmpegPath, progressCb)
		if err == nil {
			return outPath, nil
		}
		fmt.Printf("[DEBUG] yt-dlp failed: %v, falling back to Go library\n", err)
	}

	// If ffmpeg is available, prefer muxing high-quality separate streams (video-only + audio-only).
	// This makes export quality options meaningful because progressive (audio+video) streams are often capped at 720p or lower.
	if ffmpegPath != "" {
		v, a := selectMuxFormats(video.Formats)
		if v != nil && a != nil {
			fmt.Printf("[DEBUG] Selected video format: %dx%d, mime=%s, bitrate=%d\n", v.Width, v.Height, v.MimeType, v.Bitrate)
			fmt.Printf("[DEBUG] Selected audio format: mime=%s, bitrate=%d\n", a.MimeType, a.Bitrate)
			out, muxErr := d.downloadAndMux(ctx, video, v, a, destDir, ffmpegPath, progressCb)
			if muxErr == nil {
				return out, nil
			}
			fmt.Printf("[DEBUG] Mux failed: %v, falling back to progressive stream\n", muxErr)
			// Fall back to single-stream download if mux fails for any reason.
		} else {
			fmt.Printf("[DEBUG] No suitable mux formats found (video=%v, audio=%v)\n", v != nil, a != nil)
		}
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

// downloadWithYtdlp uses yt-dlp for reliable high-quality downloads
func (d *Downloader) downloadWithYtdlp(ctx context.Context, url string, outPath string, ffmpegPath string, progressCb ProgressCallback) error {
	ytdlpPath, err := exec.LookPath("yt-dlp")
	if err != nil {
		return fmt.Errorf("yt-dlp not found: %w", err)
	}

	// Download best H.264 video + AAC audio for Safari/WebKit compatibility
	// Prefer H.264 (avc1) which Safari can play natively without re-encoding
	args := []string{
		"-f", "bestvideo[vcodec^=avc1]+bestaudio[acodec^=mp4a]/bestvideo[vcodec^=avc1]+bestaudio/best[vcodec^=avc1]/bestvideo+bestaudio/best",
		"--merge-output-format", "mp4",
		"--ffmpeg-location", filepath.Dir(ffmpegPath),
		"-o", outPath,
		"--no-playlist",
		"--no-warnings",
		url,
	}

	cmd := exec.CommandContext(ctx, ytdlpPath, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	// Report some progress (yt-dlp progress is hard to parse, so we fake it)
	if progressCb != nil {
		progressCb(0.1)
	}

	err = cmd.Run()

	if progressCb != nil {
		progressCb(1.0)
	}

	if err != nil {
		return fmt.Errorf("yt-dlp error: %w: %s", err, stderr.String())
	}

	// Verify output exists
	if _, err := os.Stat(outPath); os.IsNotExist(err) {
		return fmt.Errorf("output file not created")
	}

	return nil
}

func weightedProgress(parent ProgressCallback, base float64, weight float64) ProgressCallback {
	return func(p float64) {
		if parent == nil {
			return
		}
		if p < 0 {
			p = 0
		}
		if p > 1 {
			p = 1
		}
		parent(base + p*weight)
	}
}

func (d *Downloader) downloadToFile(ctx context.Context, video *youtube.Video, format *youtube.Format, destPath string, progressCb ProgressCallback) error {
	stream, contentLength, err := d.client.GetStreamContext(ctx, video, format)
	if err != nil {
		return fmt.Errorf("failed to get stream: %w", err)
	}
	defer stream.Close()

	file, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

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
	return err
}

func (d *Downloader) downloadAndMux(ctx context.Context, video *youtube.Video, videoFmt *youtube.Format, audioFmt *youtube.Format, destDir string, ffmpegPath string, progressCb ProgressCallback) (string, error) {
	videoExt := extensionFromMimeType(videoFmt.MimeType)
	if videoExt == "" {
		videoExt = ".mp4"
	}
	audioExt := extensionFromMimeType(audioFmt.MimeType)
	if audioExt == "" {
		// audio/mp4 is typically .m4a
		if strings.Contains(audioFmt.MimeType, "audio/mp4") {
			audioExt = ".m4a"
		} else {
			audioExt = ".webm"
		}
	}

	baseName := sanitizeFilename(video.Title)
	videoPath := filepath.Join(destDir, baseName+"-video"+videoExt)
	audioPath := filepath.Join(destDir, baseName+"-audio"+audioExt)
	outPath := filepath.Join(destDir, baseName+"-preview.mp4")

	// 0-0.75 video, 0.75-0.95 audio, 0.95-1.0 mux
	if err := d.downloadToFile(ctx, video, videoFmt, videoPath, weightedProgress(progressCb, 0.0, 0.75)); err != nil {
		_ = os.Remove(videoPath)
		return "", fmt.Errorf("failed to download video stream: %w", err)
	}
	if err := d.downloadToFile(ctx, video, audioFmt, audioPath, weightedProgress(progressCb, 0.75, 0.20)); err != nil {
		_ = os.Remove(videoPath)
		_ = os.Remove(audioPath)
		return "", fmt.Errorf("failed to download audio stream: %w", err)
	}

	// Determine if we need to transcode video (VP9/AV1 needs conversion to H.264 for Safari/WebKit)
	needsVideoTranscode := strings.Contains(videoFmt.MimeType, "vp9") || strings.Contains(videoFmt.MimeType, "vp09") ||
		strings.Contains(videoFmt.MimeType, "av01") || strings.Contains(videoFmt.MimeType, "webm")
	needsAudioTranscode := strings.Contains(audioFmt.MimeType, "opus") || strings.Contains(audioFmt.MimeType, "webm")

	var args []string
	args = append(args, "-y", "-hide_banner", "-loglevel", "error")
	args = append(args, "-i", videoPath, "-i", audioPath)
	args = append(args, "-map", "0:v:0", "-map", "1:a:0")

	if needsVideoTranscode {
		// Transcode to H.264 with high quality settings
		args = append(args, "-c:v", "libx264", "-preset", "medium", "-crf", "18", "-pix_fmt", "yuv420p")
	} else {
		args = append(args, "-c:v", "copy")
	}

	if needsAudioTranscode {
		args = append(args, "-c:a", "aac", "-b:a", "192k")
	} else {
		args = append(args, "-c:a", "copy")
	}

	args = append(args, "-movflags", "+faststart", "-shortest", outPath)

	cmd := exec.CommandContext(ctx, ffmpegPath, args...)
	var stderr bytes.Buffer
	cmd.Stdout = nil
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		_ = os.Remove(videoPath)
		_ = os.Remove(audioPath)
		_ = os.Remove(outPath)
		if msg == "" {
			return "", err
		}
		return "", fmt.Errorf("%w: %s", err, msg)
	}

	_ = os.Remove(videoPath)
	_ = os.Remove(audioPath)
	if progressCb != nil {
		progressCb(1.0)
	}
	return outPath, nil
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

func selectMuxFormats(formats youtube.FormatList) (*youtube.Format, *youtube.Format) {
	// Video-only: prefer highest resolution, accepting MP4/H.264, WebM/VP9, or MP4/AV1.
	// Modern YouTube often serves highest quality in VP9 or AV1 rather than H.264.
	var videoOnly []youtube.Format
	for _, f := range formats {
		if f.AudioChannels != 0 {
			continue
		}
		if !strings.Contains(f.MimeType, "video") {
			continue
		}
		// Accept mp4 (avc1/av01) or webm (vp9)
		isMP4 := strings.Contains(f.MimeType, "video/mp4")
		isWebM := strings.Contains(f.MimeType, "video/webm")
		if !isMP4 && !isWebM {
			continue
		}
		videoOnly = append(videoOnly, f)
	}

	// Audio-only: prefer MP4/AAC (mp4a), fall back to WebM/Opus if needed.
	var audioOnly []youtube.Format
	for _, f := range formats {
		if f.AudioChannels <= 0 {
			continue
		}
		if !strings.Contains(f.MimeType, "audio") {
			continue
		}
		// Accept mp4 audio or webm audio
		isMP4 := strings.Contains(f.MimeType, "audio/mp4")
		isWebM := strings.Contains(f.MimeType, "audio/webm")
		if !isMP4 && !isWebM {
			continue
		}
		audioOnly = append(audioOnly, f)
	}

	bestVideo := pickBestVideoOnly(videoOnly)
	bestAudio := pickBestAudioOnly(audioOnly)
	return bestVideo, bestAudio
}

func pickBestVideoOnly(formats []youtube.Format) *youtube.Format {
	if len(formats) == 0 {
		return nil
	}
	bestIdx := 0
	for i := 1; i < len(formats); i++ {
		f := formats[i]
		best := formats[bestIdx]
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

func pickBestAudioOnly(formats []youtube.Format) *youtube.Format {
	if len(formats) == 0 {
		return nil
	}
	bestIdx := 0
	for i := 1; i < len(formats); i++ {
		f := formats[i]
		best := formats[bestIdx]
		if f.Bitrate > best.Bitrate {
			bestIdx = i
		}
	}
	return &formats[bestIdx]
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
