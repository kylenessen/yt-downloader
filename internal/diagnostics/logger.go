package diagnostics

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

const (
	maxLogSize  = 2 * 1024 * 1024
	maxSnapshot = 128 * 1024
)

// Logger writes a small persistent diagnostic log for GUI launches.
type Logger struct {
	mu     sync.Mutex
	path   string
	file   *os.File
	logger *log.Logger
}

// NewDefault creates the application logger in the platform log directory.
func NewDefault() (*Logger, error) {
	path, err := defaultLogPath()
	if err != nil {
		return nil, err
	}
	return New(path)
}

// New creates a logger at path and rotates one previous log when necessary.
func New(path string) (*Logger, error) {
	if !filepath.IsAbs(path) {
		return nil, errors.New("diagnostic log path must be absolute")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("create diagnostic log directory: %w", err)
	}
	if info, err := os.Stat(path); err == nil && info.Size() >= maxLogSize {
		_ = os.Remove(path + ".1")
		if err := os.Rename(path, path+".1"); err != nil {
			return nil, fmt.Errorf("rotate diagnostic log: %w", err)
		}
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("open diagnostic log: %w", err)
	}
	return &Logger{
		path:   path,
		file:   file,
		logger: log.New(file, "", log.Ldate|log.Ltime|log.Lmicroseconds),
	}, nil
}

// Printf writes one diagnostic line.
func (l *Logger) Printf(format string, args ...any) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.logger.Printf(format, args...)
}

// Path returns the diagnostic log path.
func (l *Logger) Path() string {
	if l == nil {
		return ""
	}
	return l.path
}

// Snapshot returns the end of the active log for copying into a bug report.
func (l *Logger) Snapshot() (string, error) {
	if l == nil {
		return "", nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.file.Sync(); err != nil {
		return "", err
	}
	file, err := os.Open(l.path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	start := info.Size() - maxSnapshot
	if start < 0 {
		start = 0
	}
	if _, err := file.Seek(start, io.SeekStart); err != nil {
		return "", err
	}
	data, err := io.ReadAll(io.LimitReader(file, maxSnapshot))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// Close flushes and closes the diagnostic log.
func (l *Logger) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.file.Close()
}

func defaultLogPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home directory: %w", err)
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Logs", "YT Downloader", "app.log"), nil
	}
	config, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate configuration directory: %w", err)
	}
	return filepath.Join(config, "YT Downloader", "logs", "app.log"), nil
}
