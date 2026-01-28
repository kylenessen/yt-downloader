package video

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Server serves video files for HTML5 preview
type Server struct {
	mu             sync.RWMutex
	allowedDir     string
	currentVideo   string
	currentVideoID string
}

// NewServer creates a new video server
func NewServer() *Server {
	return &Server{}
}

// SetAllowedDir sets the directory from which videos can be served
func (s *Server) SetAllowedDir(dir string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.allowedDir = dir
}

// SetCurrentVideo sets the current video file that can be served
func (s *Server) SetCurrentVideo(path string, videoID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.currentVideo = path
	s.currentVideoID = videoID
}

// GetCurrentVideoURL returns the URL path for the current video
func (s *Server) GetCurrentVideoURL() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.currentVideoID == "" {
		return ""
	}
	return fmt.Sprintf("/video/%s", s.currentVideoID)
}

// ServeHTTP implements http.Handler for serving video files
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Only handle paths starting with /video/
	if !strings.HasPrefix(r.URL.Path, "/video/") {
		http.NotFound(w, r)
		return
	}

	// Extract video ID from path
	requestedID := strings.TrimPrefix(r.URL.Path, "/video/")
	requestedID = strings.Split(requestedID, "/")[0] // Remove any trailing path

	s.mu.RLock()
	videoPath := s.currentVideo
	videoID := s.currentVideoID
	allowedDir := s.allowedDir
	s.mu.RUnlock()

	// Security check: verify the requested video matches current
	if requestedID != videoID || videoPath == "" {
		http.NotFound(w, r)
		return
	}

	// Security check: ensure video is in allowed directory
	if allowedDir != "" {
		absPath, err := filepath.Abs(videoPath)
		if err != nil {
			http.Error(w, "Invalid path", http.StatusBadRequest)
			return
		}
		absAllowed, err := filepath.Abs(allowedDir)
		if err != nil {
			http.Error(w, "Invalid directory", http.StatusInternalServerError)
			return
		}
		if !strings.HasPrefix(absPath, absAllowed) {
			http.Error(w, "Access denied", http.StatusForbidden)
			return
		}
	}

	// Check if file exists
	info, err := os.Stat(videoPath)
	if os.IsNotExist(err) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "Error accessing file", http.StatusInternalServerError)
		return
	}
	if info.IsDir() {
		http.Error(w, "Not a file", http.StatusBadRequest)
		return
	}

	// Open and serve the file
	file, err := os.Open(videoPath)
	if err != nil {
		http.Error(w, "Could not open file", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	// Set content type for video
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Accept-Ranges", "bytes")

	// Use ServeContent for range request support (seeking in video)
	http.ServeContent(w, r, filepath.Base(videoPath), info.ModTime(), file)
}

// ClearVideo clears the current video
func (s *Server) ClearVideo() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.currentVideo = ""
	s.currentVideoID = ""
}
