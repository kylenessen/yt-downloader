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

	cacheDir := filepath.Join(homeDir, ".cache", "yt-downloader", "bin")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	return &Installer{cacheDir: cacheDir}, nil
}

// GetFFmpegPath returns the path to ffmpeg, checking bundled, cache, and system locations
func (i *Installer) GetFFmpegPath() string {
	// First, check for bundled FFmpeg inside the app bundle (macOS)
	if runtime.GOOS == "darwin" {
		if bundledPath := i.getBundledFFmpegPath(); bundledPath != "" {
			// Ensure the bundled binary is executable (quarantine can block it)
			_ = os.Chmod(bundledPath, 0755)
			_ = exec.Command("xattr", "-d", "com.apple.quarantine", bundledPath).Run()
			return bundledPath
		}
	}

	// Check system PATH
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

	// macOS GUI apps don't inherit shell PATH, so check common installation paths
	if runtime.GOOS == "darwin" {
		commonPaths := []string{
			"/opt/homebrew/bin/ffmpeg",   // Apple Silicon Homebrew
			"/usr/local/bin/ffmpeg",      // Intel Homebrew / manual install
			"/usr/bin/ffmpeg",            // System install
		}
		for _, p := range commonPaths {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}

	return ""
}

// getBundledFFmpegPath returns the path to FFmpeg bundled inside the .app bundle
func (i *Installer) getBundledFFmpegPath() string {
	return i.getBundledBinaryPath("ffmpeg")
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

// GetYtdlpPath returns the path to yt-dlp, checking bundled, cache, and system locations
func (i *Installer) GetYtdlpPath() string {
	// Check for bundled yt-dlp inside the app bundle (macOS)
	if runtime.GOOS == "darwin" {
		if bundledPath := i.getBundledBinaryPath("yt-dlp"); bundledPath != "" {
			if preparedPath := i.prepareBundledBinary(bundledPath); preparedPath != "" {
				return preparedPath
			}
		}
	}

	// On Windows, check next to the executable
	if runtime.GOOS == "windows" {
		if exePath, err := os.Executable(); err == nil {
			candidate := filepath.Join(filepath.Dir(exePath), "yt-dlp.exe")
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
		}
	}

	// Check system PATH
	if path, err := exec.LookPath("yt-dlp"); err == nil {
		return path
	}

	// macOS GUI apps don't inherit shell PATH, so check common installation paths
	if runtime.GOOS == "darwin" {
		commonPaths := []string{
			"/opt/homebrew/bin/yt-dlp",
			"/usr/local/bin/yt-dlp",
			"/usr/bin/yt-dlp",
		}
		for _, p := range commonPaths {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}

	return ""
}

// prepareBundledBinary ensures a bundled binary is executable by removing macOS
// quarantine attributes and setting proper permissions. Returns the path if the
// binary can be executed, or empty string if it cannot.
func (i *Installer) prepareBundledBinary(binaryPath string) string {
	// Ensure the binary is executable
	_ = os.Chmod(binaryPath, 0755)

	// Remove macOS quarantine attribute that blocks execution of downloaded binaries
	if runtime.GOOS == "darwin" {
		_ = exec.Command("xattr", "-d", "com.apple.quarantine", binaryPath).Run()
	}

	// Verify the binary actually runs
	cmd := exec.Command(binaryPath, "--version")
	if err := cmd.Run(); err != nil {
		fmt.Printf("[DEBUG] Bundled binary at %s failed verification: %v\n", binaryPath, err)
		return ""
	}

	return binaryPath
}

// getBundledBinaryPath returns the path to a binary bundled inside the .app bundle
func (i *Installer) getBundledBinaryPath(name string) string {
	execPath, err := os.Executable()
	if err != nil {
		return ""
	}

	macosDir := filepath.Dir(execPath)
	contentsDir := filepath.Dir(macosDir)
	bundledPath := filepath.Join(contentsDir, "Resources", name)

	if _, err := os.Stat(bundledPath); err == nil {
		return bundledPath
	}
	return ""
}

// GetCacheDir returns the cache directory path
func (i *Installer) GetCacheDir() string {
	return i.cacheDir
}
