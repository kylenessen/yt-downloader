package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"yt-downloader/internal/diagnostics"
	"yt-downloader/internal/ffmpeg"
	"yt-downloader/internal/toolchain"
	"yt-downloader/internal/video"
	"yt-downloader/internal/youtube"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx             context.Context
	downloader      *youtube.Downloader
	videoServer     *video.Server
	toolchain       *toolchain.Manager
	logger          *diagnostics.Logger
	previewServer   *http.Server
	previewListener net.Listener
	previewBaseURL  string
	previewErr      error
	tempDir         string
	loadMu          sync.Mutex
	loadCancel      context.CancelFunc
	loadGeneration  uint64
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{
		videoServer: video.NewServer(),
	}
}

// startup is called when the app starts
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	logger, err := diagnostics.NewDefault()
	if err != nil {
		runtime.LogError(ctx, fmt.Sprintf("Failed to initialize diagnostic log: %v", err))
	} else {
		a.logger = logger
		a.logger.Printf("Application starting")
	}

	manager, err := toolchain.NewManager(toolchain.Config{Logf: a.logf})
	if err != nil {
		a.logf("Toolchain manager initialization failed error=%v", err)
		runtime.LogError(ctx, fmt.Sprintf("Failed to initialize toolchain: %v", err))
	} else {
		a.toolchain = manager
		a.downloader = youtube.NewDownloader(manager, a.logf)
	}

	// Create temp directory for downloads
	tempDir, err := os.MkdirTemp("", "yt-downloader-*")
	if err != nil {
		runtime.LogError(ctx, fmt.Sprintf("Failed to create temp directory: %v", err))
	} else {
		a.tempDir = tempDir
		a.videoServer.SetAllowedDir(tempDir)
	}

	// Start a localhost HTTP server for video preview.
	// WebKit won't reliably play <video> from the custom "wails://" scheme.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		a.previewErr = err
		runtime.LogError(ctx, fmt.Sprintf("Failed to start preview server: %v", err))
	} else {
		a.previewListener = ln
		a.previewServer = &http.Server{Handler: a.videoServer}
		a.previewBaseURL = "http://" + ln.Addr().String()
		go func() {
			_ = a.previewServer.Serve(ln)
		}()
	}

	if a.toolchain != nil {
		go a.refreshToolchain(ctx)
	}
}

// shutdown is called when the app is closing
func (a *App) shutdown(ctx context.Context) {
	a.CancelLoad()
	if a.previewServer != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = a.previewServer.Shutdown(shutdownCtx)
		cancel()
	}
	if a.previewListener != nil {
		_ = a.previewListener.Close()
	}

	// Cleanup temp directory
	if a.tempDir != "" {
		os.RemoveAll(a.tempDir)
	}
	if a.logger != nil {
		a.logger.Printf("Application stopped")
		_ = a.logger.Close()
	}
}

// VideoInfo holds video metadata for the frontend
type VideoInfo struct {
	ID             string  `json:"id"`
	Title          string  `json:"title"`
	Author         string  `json:"author"`
	Duration       float64 `json:"duration"`
	Thumbnail      string  `json:"thumbnail"`
	VideoURL       string  `json:"videoUrl"`
	SourceWidth    int     `json:"sourceWidth"`
	SourceHeight   int     `json:"sourceHeight"`
	PreviewWidth   int     `json:"previewWidth"`
	PreviewHeight  int     `json:"previewHeight"`
	DownloadMethod string  `json:"downloadMethod"`
}

// LoadVideo downloads a YouTube video and returns its info
func (a *App) LoadVideo(url string) (*VideoInfo, error) {
	if a.downloader == nil || a.toolchain == nil {
		return nil, fmt.Errorf("self-contained toolchain is not available; reinstall the application")
	}
	if a.previewBaseURL == "" {
		if a.previewErr != nil {
			return nil, fmt.Errorf("preview server failed to start: %w", a.previewErr)
		}
		return nil, fmt.Errorf("preview server not available")
	}

	loadCtx, generation := a.beginLoad()
	defer a.finishLoad(generation)

	runtime.EventsEmit(a.ctx, "download:status", "Reading video information...")
	info, err := a.downloader.GetVideoInfo(loadCtx, url)
	if err != nil {
		a.logf("Video metadata failed error=%v", err)
		return nil, fmt.Errorf("failed to get video info: %w", err)
	}

	// Clear any previous video
	a.videoServer.ClearVideo()

	runtime.EventsEmit(a.ctx, "download:status", "Downloading the best available quality...")
	dlResult, err := a.downloader.DownloadForPreview(loadCtx, url, info, a.tempDir, func(progress float64) {
		runtime.EventsEmit(a.ctx, "download:progress", progress)
	})
	if err != nil {
		a.logf("Preview download failed id=%s error=%v", info.ID, err)
		return nil, fmt.Errorf("failed to download video: %w", err)
	}

	a.logf("Download result id=%s method=%s resolution=%dx%d", info.ID, dlResult.Method, dlResult.Width, dlResult.Height)

	// Warn the user if we fell back to a low-quality progressive stream
	if dlResult.Method == "progressive" {
		reason := "High-quality formats were rejected. The app updated yt-dlp and then used the best compatible progressive stream."
		runtime.EventsEmit(a.ctx, "download:quality-warning", map[string]interface{}{
			"method": dlResult.Method,
			"width":  dlResult.Width,
			"height": dlResult.Height,
			"reason": reason,
		})
	}

	// Set up video server
	a.videoServer.SetCurrentVideo(dlResult.FilePath, info.ID)

	runtime.EventsEmit(a.ctx, "download:complete", nil)

	return &VideoInfo{
		ID:             info.ID,
		Title:          info.Title,
		Author:         info.Author,
		Duration:       info.Duration,
		Thumbnail:      info.Thumbnail,
		VideoURL:       a.previewBaseURL + a.videoServer.GetCurrentVideoURL(),
		SourceWidth:    info.SourceWidth,
		SourceHeight:   info.SourceHeight,
		PreviewWidth:   dlResult.Width,
		PreviewHeight:  dlResult.Height,
		DownloadMethod: dlResult.Method,
	}, nil
}

// GetVideoInfo gets video metadata without downloading
func (a *App) GetVideoInfo(url string) (*youtube.VideoInfo, error) {
	if a.downloader == nil {
		return nil, fmt.Errorf("self-contained toolchain is not available")
	}
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
	StartTime     float64 `json:"startTime"`
	EndTime       float64 `json:"endTime"`
	RemoveAudio   bool    `json:"removeAudio"`
	Filename      string  `json:"filename"`
	OutputDir     string  `json:"outputDir"`
	QualityPreset string  `json:"qualityPreset"` // "high", "medium", "low"
	MaxResolution string  `json:"maxResolution"` // "original", "1080p", "720p", "480p", "360p"
}

func sanitizeFilename(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
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

// ExportClip trims and saves a video clip
func (a *App) ExportClip(opts ExportOptions) error {
	if a.toolchain == nil {
		return fmt.Errorf("self-contained toolchain is not available")
	}
	paths, _, err := a.toolchain.Ensure(a.ctx)
	if err != nil {
		return fmt.Errorf("verify media toolchain: %w", err)
	}

	// Get current video path from server
	inputPath := a.videoServer.GetCurrentVideoPath()
	if inputPath == "" {
		return fmt.Errorf("no video loaded")
	}

	// Build output path
	filename := sanitizeFilename(opts.Filename)
	if filename == "" {
		filename = "clip"
	}
	if !filepath.IsAbs(opts.OutputDir) {
		return fmt.Errorf("output directory must be an absolute path")
	}
	outputName := filename
	if !strings.EqualFold(filepath.Ext(outputName), ".mp4") {
		outputName += ".mp4"
	}
	outputPath := filepath.Join(opts.OutputDir, outputName)

	// Create processor
	processor := ffmpeg.NewProcessor(paths.FFmpeg)

	trimOpts := ffmpeg.TrimOptions{
		InputPath:   inputPath,
		OutputPath:  outputPath,
		StartTime:   opts.StartTime,
		EndTime:     opts.EndTime,
		RemoveAudio: opts.RemoveAudio,
	}

	// Quality preset controls encoding quality (CRF & preset)
	switch opts.QualityPreset {
	case "high":
		trimOpts.CRF = 18
		trimOpts.Preset = "slow"
		trimOpts.AudioBitrate = "192k"
	case "low":
		trimOpts.CRF = 26
		trimOpts.Preset = "veryfast"
		trimOpts.AudioBitrate = "96k"
	default: // "medium" or unspecified
		trimOpts.CRF = 21
		trimOpts.Preset = "medium"
		trimOpts.AudioBitrate = "128k"
	}

	// Max resolution controls output size
	switch opts.MaxResolution {
	case "1080p":
		trimOpts.MaxHeight = 1080
	case "720p":
		trimOpts.MaxHeight = 720
	case "480p":
		trimOpts.MaxHeight = 480
	case "360p":
		trimOpts.MaxHeight = 360
	default: // "original" or unspecified
		trimOpts.MaxHeight = 0
	}

	// Export with progress
	err = processor.TrimVideoWithProgress(a.ctx, trimOpts, func(progress float64) {
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
	if a.toolchain == nil {
		return false
	}
	_, _, err := a.toolchain.Ensure(a.ctx)
	return err == nil
}

// InstallFFmpeg downloads and installs FFmpeg
func (a *App) InstallFFmpeg() error {
	if a.toolchain == nil {
		return fmt.Errorf("self-contained toolchain is not available")
	}
	runtime.EventsEmit(a.ctx, "ffmpeg:progress", map[string]interface{}{"progress": 0.1, "status": "Verifying included tools..."})
	_, _, err := a.toolchain.Ensure(a.ctx)
	if err != nil {
		return err
	}
	runtime.EventsEmit(a.ctx, "ffmpeg:progress", map[string]interface{}{"progress": 1.0, "status": "Included tools are ready"})
	return nil
}

// CancelLoad cancels the active metadata or preview download.
func (a *App) CancelLoad() {
	a.loadMu.Lock()
	defer a.loadMu.Unlock()
	if a.loadCancel != nil {
		a.loadCancel()
		a.loadCancel = nil
	}
}

// GetDiagnostics returns recent log output for a bug report.
func (a *App) GetDiagnostics() (string, error) {
	if a.logger == nil {
		return "Diagnostic logging is unavailable.", nil
	}
	snapshot, err := a.logger.Snapshot()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("YT Downloader diagnostics\nLog file: %s\n\n%s", a.logger.Path(), snapshot), nil
}

// GetVideoServer returns the video server for use as HTTP handler
func (a *App) GetVideoServer() *video.Server {
	return a.videoServer
}

func (a *App) beginLoad() (context.Context, uint64) {
	a.loadMu.Lock()
	defer a.loadMu.Unlock()
	if a.loadCancel != nil {
		a.loadCancel()
	}
	a.loadGeneration++
	ctx, cancel := context.WithCancel(a.ctx)
	a.loadCancel = cancel
	return ctx, a.loadGeneration
}

func (a *App) finishLoad(generation uint64) {
	a.loadMu.Lock()
	defer a.loadMu.Unlock()
	if a.loadGeneration == generation {
		a.loadCancel = nil
	}
}

func (a *App) refreshToolchain(ctx context.Context) {
	paths, versions, err := a.toolchain.Ensure(ctx)
	if err != nil {
		a.logf("Toolchain verification failed error=%v", err)
		runtime.EventsEmit(a.ctx, "toolchain:error", err.Error())
		return
	}
	a.logf("Toolchain ready baseline=%s yt-dlp=%s root=%s", versions.Baseline, versions.YTDLP, filepath.Dir(paths.YTDLP))
	updated, updatedVersions, err := a.toolchain.CheckForYTDLPUpdate(ctx, false)
	if err != nil {
		a.logf("Background yt-dlp update check failed error=%v", err)
		return
	}
	if updated {
		a.logf("Background yt-dlp update activated version=%s", updatedVersions.YTDLP)
		runtime.EventsEmit(a.ctx, "toolchain:updated", updatedVersions.YTDLP)
	}
}

func (a *App) logf(format string, args ...any) {
	if a.logger != nil {
		a.logger.Printf(format, args...)
	}
}
