package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"yt-clipper/internal/ffmpeg"
	"yt-clipper/internal/video"
	"yt-clipper/internal/youtube"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx             context.Context
	downloader      *youtube.Downloader
	videoServer     *video.Server
	ffmpegInstaller *ffmpeg.Installer
	tempDir         string
	currentVideoID  string
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{
		downloader:  youtube.NewDownloader(),
		videoServer: video.NewServer(),
	}
}

// startup is called when the app starts
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Create temp directory for downloads
	tempDir, err := os.MkdirTemp("", "yt-clipper-*")
	if err != nil {
		runtime.LogError(ctx, fmt.Sprintf("Failed to create temp directory: %v", err))
	} else {
		a.tempDir = tempDir
		a.videoServer.SetAllowedDir(tempDir)
	}

	// Initialize FFmpeg installer
	installer, err := ffmpeg.NewInstaller()
	if err != nil {
		runtime.LogError(ctx, fmt.Sprintf("Failed to initialize FFmpeg installer: %v", err))
	} else {
		a.ffmpegInstaller = installer
	}
}

// shutdown is called when the app is closing
func (a *App) shutdown(ctx context.Context) {
	// Cleanup temp directory
	if a.tempDir != "" {
		os.RemoveAll(a.tempDir)
	}
}

// VideoInfo holds video metadata for the frontend
type VideoInfo struct {
	ID        string  `json:"id"`
	Title     string  `json:"title"`
	Author    string  `json:"author"`
	Duration  float64 `json:"duration"`
	Thumbnail string  `json:"thumbnail"`
	VideoURL  string  `json:"videoUrl"`
}

// LoadVideo downloads a YouTube video and returns its info
func (a *App) LoadVideo(url string) (*VideoInfo, error) {
	// Get video info first
	info, err := a.downloader.GetVideoInfo(a.ctx, url)
	if err != nil {
		return nil, fmt.Errorf("failed to get video info: %w", err)
	}

	// Clear any previous video
	a.videoServer.ClearVideo()

	// Download video with progress updates
	videoPath, err := a.downloader.DownloadForPreview(a.ctx, url, a.tempDir, func(progress float64) {
		runtime.EventsEmit(a.ctx, "download:progress", progress)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to download video: %w", err)
	}

	// Set up video server
	a.currentVideoID = info.ID
	a.videoServer.SetCurrentVideo(videoPath, info.ID)

	runtime.EventsEmit(a.ctx, "download:complete", nil)

	return &VideoInfo{
		ID:        info.ID,
		Title:     info.Title,
		Author:    info.Author,
		Duration:  info.Duration,
		Thumbnail: info.Thumbnail,
		VideoURL:  a.videoServer.GetCurrentVideoURL(),
	}, nil
}

// GetVideoInfo gets video metadata without downloading
func (a *App) GetVideoInfo(url string) (*youtube.VideoInfo, error) {
	return a.downloader.GetVideoInfo(a.ctx, url)
}

// SelectOutputDirectory opens a native directory picker
func (a *App) SelectOutputDirectory() (string, error) {
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select Output Directory",
	})
	if err != nil {
		return "", err
	}
	return dir, nil
}

// ExportOptions specifies export settings
type ExportOptions struct {
	StartTime   float64 `json:"startTime"`
	EndTime     float64 `json:"endTime"`
	RemoveAudio bool    `json:"removeAudio"`
	Filename    string  `json:"filename"`
	OutputDir   string  `json:"outputDir"`
}

// ExportClip trims and saves a video clip
func (a *App) ExportClip(opts ExportOptions) error {
	if a.ffmpegInstaller == nil || !a.ffmpegInstaller.IsInstalled() {
		return fmt.Errorf("FFmpeg is not installed")
	}

	// Get current video path from server
	videoURL := a.videoServer.GetCurrentVideoURL()
	if videoURL == "" {
		return fmt.Errorf("no video loaded")
	}

	// Find the input video path
	inputPath := filepath.Join(a.tempDir, fmt.Sprintf("*.mp4"))
	matches, err := filepath.Glob(inputPath)
	if err != nil || len(matches) == 0 {
		return fmt.Errorf("video file not found")
	}
	inputPath = matches[0]

	// Build output path
	filename := opts.Filename
	if filename == "" {
		filename = "clip"
	}
	if !filepath.IsAbs(opts.OutputDir) {
		return fmt.Errorf("output directory must be an absolute path")
	}
	outputPath := filepath.Join(opts.OutputDir, filename+".mp4")

	// Create processor
	processor := ffmpeg.NewProcessor(a.ffmpegInstaller.GetFFmpegPath())

	// Export with progress
	err = processor.TrimVideoWithProgress(a.ctx, ffmpeg.TrimOptions{
		InputPath:   inputPath,
		OutputPath:  outputPath,
		StartTime:   opts.StartTime,
		EndTime:     opts.EndTime,
		RemoveAudio: opts.RemoveAudio,
	}, func(progress float64) {
		runtime.EventsEmit(a.ctx, "export:progress", progress)
	})

	if err != nil {
		return fmt.Errorf("failed to export clip: %w", err)
	}

	runtime.EventsEmit(a.ctx, "export:complete", outputPath)
	return nil
}

// CheckFFmpeg checks if FFmpeg is installed
func (a *App) CheckFFmpeg() bool {
	if a.ffmpegInstaller == nil {
		return false
	}
	return a.ffmpegInstaller.IsInstalled()
}

// InstallFFmpeg downloads and installs FFmpeg
func (a *App) InstallFFmpeg() error {
	if a.ffmpegInstaller == nil {
		return fmt.Errorf("FFmpeg installer not initialized")
	}

	return a.ffmpegInstaller.Install(a.ctx, func(progress float64, status string) {
		runtime.EventsEmit(a.ctx, "ffmpeg:progress", map[string]interface{}{
			"progress": progress,
			"status":   status,
		})
	})
}

// GetVideoServer returns the video server for use as HTTP handler
func (a *App) GetVideoServer() *video.Server {
	return a.videoServer
}
