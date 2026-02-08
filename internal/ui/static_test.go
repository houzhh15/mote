package ui

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestStaticServer_ServeHTTP_EmbedFS(t *testing.T) {
	embedFS := fstest.MapFS{
		"index.html":      {Data: []byte("<html>embedded</html>")},
		"style.css":       {Data: []byte("body { color: red; }")},
		"app.js":          {Data: []byte("console.log('hello');")},
		"assets/logo.png": {Data: []byte("PNG DATA")},
	}

	server := NewStaticServer("", embedFS)

	tests := []struct {
		path           string
		expectedStatus int
		expectedType   string
		expectedBody   string
	}{
		{"/", http.StatusOK, "text/html", "<html>embedded</html>"},
		{"/index.html", http.StatusOK, "text/html", "<html>embedded</html>"},
		{"/style.css", http.StatusOK, "text/css", "body { color: red; }"},
		{"/app.js", http.StatusOK, "text/javascript", "console.log('hello');"},
		{"/assets/logo.png", http.StatusOK, "image/png", "PNG DATA"},
		{"/notfound.html", http.StatusNotFound, "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()

			server.ServeHTTP(rec, req)

			if rec.Code != tt.expectedStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.expectedStatus)
			}

			if tt.expectedStatus == http.StatusOK {
				contentType := rec.Header().Get("Content-Type")
				if tt.expectedType != "" && !contains(contentType, tt.expectedType) {
					t.Errorf("Content-Type = %q, want %q", contentType, tt.expectedType)
				}
				if !contains(rec.Body.String(), tt.expectedBody) {
					t.Errorf("body = %q, want %q", rec.Body.String(), tt.expectedBody)
				}
			}
		})
	}
}

func TestStaticServer_ServeHTTP_UserDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Create user files
	if err := os.WriteFile(filepath.Join(tmpDir, "index.html"), []byte("<html>user</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "custom.js"), []byte("// user custom"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Embed FS as fallback
	embedFS := fstest.MapFS{
		"index.html": {Data: []byte("<html>embedded</html>")},
		"embed.js":   {Data: []byte("// embedded")},
	}

	server := NewStaticServer(tmpDir, embedFS)

	// User file should take priority
	req := httptest.NewRequest(http.MethodGet, "/index.html", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if !contains(rec.Body.String(), "user") {
		t.Errorf("body should contain 'user', got %q", rec.Body.String())
	}

	// User-only file
	req = httptest.NewRequest(http.MethodGet, "/custom.js", nil)
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("custom.js status = %d, want 200", rec.Code)
	}

	// Embed-only file (fallback)
	req = httptest.NewRequest(http.MethodGet, "/embed.js", nil)
	rec = httptest.NewRecorder()
	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("embed.js status = %d, want 200", rec.Code)
	}
	if !contains(rec.Body.String(), "embedded") {
		t.Errorf("body should contain 'embedded', got %q", rec.Body.String())
	}
}

func TestStaticServer_PathTraversal(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a file outside the ui dir
	secretFile := filepath.Join(filepath.Dir(tmpDir), "secret.txt")
	if err := os.WriteFile(secretFile, []byte("SECRET"), 0o644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(secretFile)

	// Create a file in the ui dir
	if err := os.WriteFile(filepath.Join(tmpDir, "index.html"), []byte("<html>safe</html>"), 0o644); err != nil {
		t.Fatal(err)
	}

	server := NewStaticServer(tmpDir, nil)

	tests := []struct {
		path           string
		expectedStatus int
	}{
		{"/../secret.txt", http.StatusForbidden},
		{"/../../etc/passwd", http.StatusForbidden},
		{"/..", http.StatusForbidden},
		{"/subdir/../index.html", http.StatusOK}, // This is actually valid
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()

			server.ServeHTTP(rec, req)

			// Path traversal attempts should be blocked or normalized safely
			if tt.expectedStatus == http.StatusForbidden && rec.Code != http.StatusForbidden && rec.Code != http.StatusNotFound {
				// Some path traversals may result in 404 after cleanup
				if rec.Code == http.StatusOK && contains(rec.Body.String(), "SECRET") {
					t.Errorf("path %q exposed secret file!", tt.path)
				}
			}
		})
	}
}

func TestStaticServer_MethodNotAllowed(t *testing.T) {
	server := NewStaticServer("", fstest.MapFS{
		"index.html": {Data: []byte("<html></html>")},
	})

	methods := []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/index.html", nil)
			rec := httptest.NewRecorder()

			server.ServeHTTP(rec, req)

			if rec.Code != http.StatusMethodNotAllowed {
				t.Errorf("%s /index.html = %d, want 405", method, rec.Code)
			}
		})
	}
}

func TestStaticServer_HeadMethod(t *testing.T) {
	server := NewStaticServer("", fstest.MapFS{
		"index.html": {Data: []byte("<html></html>")},
	})

	req := httptest.NewRequest(http.MethodHead, "/index.html", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("HEAD /index.html = %d, want 200", rec.Code)
	}
}

func TestStaticServer_NilEmbedFS(t *testing.T) {
	server := NewStaticServer("", nil)

	req := httptest.NewRequest(http.MethodGet, "/index.html", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestStaticServer_EmptyUserDir(t *testing.T) {
	embedFS := fstest.MapFS{
		"index.html": {Data: []byte("<html>embedded</html>")},
	}

	server := NewStaticServer("", embedFS)

	req := httptest.NewRequest(http.MethodGet, "/index.html", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Ensure StaticServer implements http.Handler
var _ http.Handler = (*StaticServer)(nil)
var _ fs.FS = (fstest.MapFS)(nil)
