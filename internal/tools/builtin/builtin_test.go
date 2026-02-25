package builtin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"mote/internal/tools"
)

func TestShellTool(t *testing.T) {
	tool := NewShellTool()

	t.Run("Name and Description", func(t *testing.T) {
		if tool.Name() != "shell" {
			t.Errorf("expected name 'shell', got %q", tool.Name())
		}
		if tool.Description() == "" {
			t.Error("expected non-empty description")
		}
	})

	t.Run("Execute echo", func(t *testing.T) {
		args := map[string]any{"command": "echo hello"}
		result, err := tool.Execute(context.Background(), args)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result.Content, "hello") {
			t.Errorf("expected output to contain 'hello', got %q", result.Content)
		}
	})

	t.Run("Execute with working directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		args := map[string]any{
			"command":  "pwd",
			"work_dir": tmpDir,
		}
		if runtime.GOOS == "windows" {
			args["command"] = "cd"
		}

		result, err := tool.Execute(context.Background(), args)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result.Content, filepath.Base(tmpDir)) {
			t.Errorf("expected output to contain temp dir, got %q", result.Content)
		}
	})

	t.Run("Missing command", func(t *testing.T) {
		args := map[string]any{}
		_, err := tool.Execute(context.Background(), args)
		if err == nil {
			t.Error("expected error for missing command")
		}
	})

	t.Run("Command failure", func(t *testing.T) {
		args := map[string]any{"command": "exit 1"}
		if runtime.GOOS == "windows" {
			args["command"] = "cmd /c exit 1"
		}

		result, err := tool.Execute(context.Background(), args)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Error("expected IsError to be true for failed command")
		}
	})
}

func TestReadFileTool(t *testing.T) {
	tool := NewReadFileTool()

	t.Run("Name", func(t *testing.T) {
		if tool.Name() != "read_file" {
			t.Errorf("expected name 'read_file', got %q", tool.Name())
		}
	})

	t.Run("Read entire file", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "test.txt")
		content := "line1\nline2\nline3"
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		result, err := tool.Execute(context.Background(), map[string]any{"path": path})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Content != content {
			t.Errorf("expected %q, got %q", content, result.Content)
		}
	})

	t.Run("Read line range", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "lines.txt")
		content := "line1\nline2\nline3\nline4\nline5"
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		result, err := tool.Execute(context.Background(), map[string]any{
			"path":       path,
			"start_line": float64(2),
			"end_line":   float64(4),
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result.Content, "line2") || !strings.Contains(result.Content, "line4") {
			t.Errorf("expected lines 2-4, got %q", result.Content)
		}
	})

	t.Run("File not found", func(t *testing.T) {
		result, err := tool.Execute(context.Background(), map[string]any{"path": "/nonexistent/file.txt"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Error("expected error result for nonexistent file")
		}
	})

	t.Run("Missing path", func(t *testing.T) {
		_, err := tool.Execute(context.Background(), map[string]any{})
		if err == nil {
			t.Error("expected error for missing path")
		}
	})
}

func TestWriteFileTool(t *testing.T) {
	tool := NewWriteFileTool()

	t.Run("Name", func(t *testing.T) {
		if tool.Name() != "write_file" {
			t.Errorf("expected name 'write_file', got %q", tool.Name())
		}
	})

	t.Run("Write new file", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "new.txt")

		result, err := tool.Execute(context.Background(), map[string]any{
			"path":    path,
			"content": "hello world",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.IsError {
			t.Errorf("unexpected error result: %s", result.Content)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "hello world" {
			t.Errorf("expected 'hello world', got %q", string(data))
		}
	})

	t.Run("Create parent directories", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "subdir", "nested", "file.txt")

		result, err := tool.Execute(context.Background(), map[string]any{
			"path":    path,
			"content": "nested content",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.IsError {
			t.Errorf("unexpected error result: %s", result.Content)
		}

		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Error("expected file to exist")
		}
	})

	t.Run("Append mode", func(t *testing.T) {
		tmpDir := t.TempDir()
		path := filepath.Join(tmpDir, "append.txt")
		if err := os.WriteFile(path, []byte("first"), 0644); err != nil {
			t.Fatal(err)
		}

		result, err := tool.Execute(context.Background(), map[string]any{
			"path":    path,
			"content": "second",
			"append":  true,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.IsError {
			t.Errorf("unexpected error result: %s", result.Content)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "firstsecond" {
			t.Errorf("expected 'firstsecond', got %q", string(data))
		}
	})
}

func TestListDirTool(t *testing.T) {
	tool := NewListDirTool()

	t.Run("Name", func(t *testing.T) {
		if tool.Name() != "list_dir" {
			t.Errorf("expected name 'list_dir', got %q", tool.Name())
		}
	})

	t.Run("List directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		for _, name := range []string{"a.txt", "b.txt", "c.go"} {
			if err := os.WriteFile(filepath.Join(tmpDir, name), []byte("test"), 0644); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.Mkdir(filepath.Join(tmpDir, "subdir"), 0755); err != nil {
			t.Fatal(err)
		}

		result, err := tool.Execute(context.Background(), map[string]any{"path": tmpDir})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(result.Content, "a.txt") {
			t.Error("expected a.txt in output")
		}
		if !strings.Contains(result.Content, "subdir/") {
			t.Error("expected subdir/ in output")
		}
	})

	t.Run("List with pattern", func(t *testing.T) {
		tmpDir := t.TempDir()
		for _, name := range []string{"a.txt", "b.txt", "c.go"} {
			if err := os.WriteFile(filepath.Join(tmpDir, name), []byte("test"), 0644); err != nil {
				t.Fatal(err)
			}
		}

		result, err := tool.Execute(context.Background(), map[string]any{
			"path":    tmpDir,
			"pattern": "*.txt",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(result.Content, "a.txt") {
			t.Error("expected a.txt in output")
		}
		if strings.Contains(result.Content, "c.go") {
			t.Error("c.go should be filtered out")
		}
	})

	t.Run("List recursive", func(t *testing.T) {
		tmpDir := t.TempDir()
		subDir := filepath.Join(tmpDir, "sub")
		if err := os.Mkdir(subDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(subDir, "nested.txt"), []byte("test"), 0644); err != nil {
			t.Fatal(err)
		}

		result, err := tool.Execute(context.Background(), map[string]any{
			"path":      tmpDir,
			"recursive": true,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(result.Content, "nested.txt") {
			t.Error("expected nested.txt in output")
		}
	})

	t.Run("Directory not found", func(t *testing.T) {
		result, err := tool.Execute(context.Background(), map[string]any{"path": "/nonexistent/dir"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Error("expected error result")
		}
	})
}

func TestHTTPTool(t *testing.T) {
	tool := NewHTTPTool()
	tool.BlockPrivate = false // Disable SSRF for local httptest servers

	t.Run("Name", func(t *testing.T) {
		if tool.Name() != "http" {
			t.Errorf("expected name 'http', got %q", tool.Name())
		}
	})

	t.Run("GET request", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "GET" {
				t.Errorf("expected GET, got %s", r.Method)
			}
			w.Header().Set("X-Custom", "test")
			w.Write([]byte("hello"))
		}))
		defer server.Close()

		result, err := tool.Execute(context.Background(), map[string]any{"url": server.URL})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !strings.Contains(result.Content, "Status: 200") {
			t.Error("expected status 200")
		}
		if !strings.Contains(result.Content, "hello") {
			t.Error("expected body 'hello'")
		}
	})

	t.Run("POST request with body", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				t.Errorf("expected POST, got %s", r.Method)
			}
			w.Write([]byte("received"))
		}))
		defer server.Close()

		result, err := tool.Execute(context.Background(), map[string]any{
			"url":    server.URL,
			"method": "POST",
			"body":   "test body",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.IsError {
			t.Error("expected success result")
		}
	})

	t.Run("Error response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("not found"))
		}))
		defer server.Close()

		result, err := tool.Execute(context.Background(), map[string]any{"url": server.URL})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !result.IsError {
			t.Error("expected error result for 404")
		}
		if !strings.Contains(result.Content, "404") {
			t.Error("expected 404 in content")
		}
	})

	t.Run("Missing URL", func(t *testing.T) {
		_, err := tool.Execute(context.Background(), map[string]any{})
		if err == nil {
			t.Error("expected error for missing URL")
		}
	})
}

func TestRegisterBuiltins(t *testing.T) {
	r := tools.NewRegistry()

	err := RegisterBuiltins(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedTools := ToolNames()
	for _, name := range expectedTools {
		if _, ok := r.Get(name); !ok {
			t.Errorf("expected tool %q to be registered", name)
		}
	}

	if r.Len() != len(expectedTools) {
		t.Errorf("expected %d tools, got %d", len(expectedTools), r.Len())
	}
}

func TestNewRegistryWithBuiltins(t *testing.T) {
	r := NewRegistryWithBuiltins()

	// Currently we have 6 builtin tools: shell, read_file, write_file, edit_file, list_dir, http
	expected := len(ToolNames())
	if r.Len() != expected {
		t.Errorf("expected %d tools, got %d", expected, r.Len())
	}
}
