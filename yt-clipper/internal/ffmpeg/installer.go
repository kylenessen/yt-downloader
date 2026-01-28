package ffmpeg

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	// Download URLs for ffmpeg binaries
	macFFmpegURL     = "https://evermeet.cx/ffmpeg/getrelease/zip"
	windowsFFmpegURL = "https://github.com/yt-dlp/FFmpeg-Builds/releases/download/latest/ffmpeg-master-latest-win64-gpl.zip"
	linuxFFmpegURL   = "https://github.com/yt-dlp/FFmpeg-Builds/releases/download/latest/ffmpeg-master-latest-linux64-gpl.tar.xz"
)

// ProgressCallback reports installation progress (0.0 to 1.0)
type ProgressCallback func(progress float64, status string)

// Installer handles FFmpeg installation
type Installer struct {
	cacheDir string
}

// NewInstaller creates a new FFmpeg installer
func NewInstaller() (*Installer, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	cacheDir := filepath.Join(homeDir, ".cache", "yt-clipper", "bin")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	return &Installer{cacheDir: cacheDir}, nil
}

// GetFFmpegPath returns the path to ffmpeg, checking system PATH and cache
func (i *Installer) GetFFmpegPath() string {
	// First check system PATH
	path, err := exec.LookPath("ffmpeg")
	if err == nil {
		return path
	}

	// Check cached installation
	var binaryName string
	if runtime.GOOS == "windows" {
		binaryName = "ffmpeg.exe"
	} else {
		binaryName = "ffmpeg"
	}

	cachedPath := filepath.Join(i.cacheDir, binaryName)
	if _, err := os.Stat(cachedPath); err == nil {
		return cachedPath
	}

	return ""
}

// IsInstalled checks if FFmpeg is available
func (i *Installer) IsInstalled() bool {
	return i.GetFFmpegPath() != ""
}

// Install downloads and installs FFmpeg
func (i *Installer) Install(ctx context.Context, progressCb ProgressCallback) error {
	if i.IsInstalled() {
		if progressCb != nil {
			progressCb(1.0, "FFmpeg already installed")
		}
		return nil
	}

	var downloadURL string
	switch runtime.GOOS {
	case "darwin":
		downloadURL = macFFmpegURL
	case "windows":
		downloadURL = windowsFFmpegURL
	case "linux":
		downloadURL = linuxFFmpegURL
	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}

	if progressCb != nil {
		progressCb(0.0, "Starting download...")
	}

	// Download the archive
	archivePath := filepath.Join(i.cacheDir, "ffmpeg-download")
	if err := i.downloadFile(ctx, downloadURL, archivePath, progressCb); err != nil {
		return fmt.Errorf("failed to download FFmpeg: %w", err)
	}
	defer os.Remove(archivePath)

	if progressCb != nil {
		progressCb(0.9, "Extracting...")
	}

	// Extract based on platform
	switch runtime.GOOS {
	case "darwin":
		if err := i.extractMacZip(archivePath); err != nil {
			return fmt.Errorf("failed to extract FFmpeg: %w", err)
		}
	case "windows":
		if err := i.extractWindowsZip(archivePath); err != nil {
			return fmt.Errorf("failed to extract FFmpeg: %w", err)
		}
	case "linux":
		if err := i.extractLinuxTarXz(archivePath); err != nil {
			return fmt.Errorf("failed to extract FFmpeg: %w", err)
		}
	}

	if progressCb != nil {
		progressCb(1.0, "Installation complete")
	}

	return nil
}

func (i *Installer) downloadFile(ctx context.Context, url, destPath string, progressCb ProgressCallback) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status: %s", resp.Status)
	}

	file, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer file.Close()

	if progressCb != nil && resp.ContentLength > 0 {
		reader := &progressReader{
			reader:     resp.Body,
			total:      resp.ContentLength,
			progressCb: progressCb,
		}
		_, err = io.Copy(file, reader)
	} else {
		_, err = io.Copy(file, resp.Body)
	}

	return err
}

type progressReader struct {
	reader     io.Reader
	total      int64
	read       int64
	progressCb ProgressCallback
}

func (pr *progressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	pr.read += int64(n)

	if pr.progressCb != nil && pr.total > 0 {
		// Scale to 0-0.9 range (0.9-1.0 is for extraction)
		progress := (float64(pr.read) / float64(pr.total)) * 0.9
		pr.progressCb(progress, fmt.Sprintf("Downloading: %.0f%%", progress*100/0.9))
	}

	return n, err
}

func (i *Installer) extractMacZip(archivePath string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		if strings.HasSuffix(f.Name, "ffmpeg") || f.Name == "ffmpeg" {
			rc, err := f.Open()
			if err != nil {
				return err
			}
			defer rc.Close()

			destPath := filepath.Join(i.cacheDir, "ffmpeg")
			outFile, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
			if err != nil {
				return err
			}
			defer outFile.Close()

			_, err = io.Copy(outFile, rc)
			return err
		}
	}

	return fmt.Errorf("ffmpeg binary not found in archive")
}

func (i *Installer) extractWindowsZip(archivePath string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		if strings.HasSuffix(f.Name, "ffmpeg.exe") {
			rc, err := f.Open()
			if err != nil {
				return err
			}
			defer rc.Close()

			destPath := filepath.Join(i.cacheDir, "ffmpeg.exe")
			outFile, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
			if err != nil {
				return err
			}
			defer outFile.Close()

			_, err = io.Copy(outFile, rc)
			return err
		}
	}

	return fmt.Errorf("ffmpeg.exe not found in archive")
}

func (i *Installer) extractLinuxTarXz(archivePath string) error {
	// Use system tar for .tar.xz extraction
	cmd := exec.Command("tar", "-xf", archivePath, "-C", i.cacheDir, "--strip-components=2", "--wildcards", "*/bin/ffmpeg")
	return cmd.Run()
}

// GetCacheDir returns the cache directory path
func (i *Installer) GetCacheDir() string {
	return i.cacheDir
}
