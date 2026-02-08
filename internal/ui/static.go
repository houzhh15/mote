package ui

import (
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"mote/pkg/logger"
)

// StaticServer serves static files from user directory or embedded FS.
type StaticServer struct {
	userDir string
	embedFS fs.FS
}

// NewStaticServer creates a new static file server.
// userDir is the user's UI directory (takes priority), embedFS is the fallback.
func NewStaticServer(userDir string, embedFS fs.FS) *StaticServer {
	return &StaticServer{
		userDir: userDir,
		embedFS: embedFS,
	}
}

// ServeHTTP implements http.Handler.
func (s *StaticServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Only allow GET and HEAD
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Clean and validate path
	path := r.URL.Path
	if path == "/" {
		path = "/index.html"
	}

	// Remove leading slash for file operations
	cleanPath := strings.TrimPrefix(path, "/")

	// Prevent path traversal
	cleanPath = filepath.Clean(cleanPath)
	if strings.HasPrefix(cleanPath, "..") || strings.Contains(cleanPath, ".."+string(os.PathSeparator)) {
		logger.Warn().
			Str("path", r.URL.Path).
			Str("clean_path", cleanPath).
			Msg("Path traversal attempt blocked")
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// Try user directory first
	if s.userDir != "" {
		fullPath := filepath.Join(s.userDir, cleanPath)

		// Verify the resolved path is still under userDir
		absUserDir, err := filepath.Abs(s.userDir)
		if err == nil {
			absFullPath, err := filepath.Abs(fullPath)
			if err == nil && strings.HasPrefix(absFullPath, absUserDir+string(os.PathSeparator)) {
				if data, err := os.ReadFile(fullPath); err == nil {
					s.serveContent(w, cleanPath, data)
					return
				}
			}
		}
	}

	// Fall back to embedded FS
	if s.embedFS != nil {
		if data, err := fs.ReadFile(s.embedFS, cleanPath); err == nil {
			s.serveContent(w, cleanPath, data)
			return
		}
	}

	// Not found
	http.NotFound(w, r)
}

// serveContent writes the content with appropriate Content-Type.
func (s *StaticServer) serveContent(w http.ResponseWriter, path string, data []byte) {
	// Determine content type from extension
	ext := filepath.Ext(path)
	contentType := mime.TypeByExtension(ext)
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, strings.NewReader(string(data)))
}
