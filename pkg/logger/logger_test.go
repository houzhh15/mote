package logger

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected zerolog.Level
	}{
		{"debug", zerolog.DebugLevel},
		{"DEBUG", zerolog.DebugLevel},
		{"info", zerolog.InfoLevel},
		{"INFO", zerolog.InfoLevel},
		{"warn", zerolog.WarnLevel},
		{"warning", zerolog.WarnLevel},
		{"error", zerolog.ErrorLevel},
		{"fatal", zerolog.FatalLevel},
		{"unknown", zerolog.InfoLevel},
		{"", zerolog.InfoLevel},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseLevel(tt.input)
			if got != tt.expected {
				t.Errorf("parseLevel(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestInitConsoleFormat(t *testing.T) {
	defer func() { _ = Close() }()

	err := Init(LogConfig{
		Level:  "debug",
		Format: "console",
	})
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	logger := Get()
	if logger == nil {
		t.Fatal("Get() returned nil")
	}
}

func TestInitJSONFormat(t *testing.T) {
	defer func() { _ = Close() }()

	err := Init(LogConfig{
		Level:  "info",
		Format: "json",
	})
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	logger := Get()
	if logger == nil {
		t.Fatal("Get() returned nil")
	}
}

func TestInitWithFile(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	defer func() { _ = Close() }()

	err := Init(LogConfig{
		Level:  "debug",
		Format: "json",
		File:   logPath,
	})
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	Info().Str("test", "value").Msg("test message")

	if err := Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Read log file failed: %v", err)
	}

	if !strings.Contains(string(content), "test message") {
		t.Errorf("Log file doesn't contain expected message, got: %s", string(content))
	}
}

func TestInitWithInvalidFile(t *testing.T) {
	defer func() { _ = Close() }()

	err := Init(LogConfig{
		Level:  "info",
		Format: "json",
		File:   "/nonexistent/directory/test.log",
	})
	if err == nil {
		t.Error("Expected error for invalid file path")
	}
}

func TestWith(t *testing.T) {
	defer func() { _ = Close() }()

	err := Init(LogConfig{
		Level:  "debug",
		Format: "json",
	})
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	logger := With(map[string]any{
		"service": "test",
		"version": "1.0",
	})
	if logger == nil {
		t.Fatal("With() returned nil")
	}
}

func TestLevelFiltering(t *testing.T) {
	var buf bytes.Buffer

	logger := zerolog.New(&buf).Level(zerolog.WarnLevel)

	logger.Debug().Msg("debug message")
	if buf.Len() > 0 {
		t.Error("Debug message should be filtered")
	}

	logger.Warn().Msg("warn message")
	if buf.Len() == 0 {
		t.Error("Warn message should be logged")
	}

	var logEntry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse log entry: %v", err)
	}
	if logEntry["level"] != "warn" {
		t.Errorf("Expected level 'warn', got %v", logEntry["level"])
	}
}

func TestConvenienceFunctions(t *testing.T) {
	defer func() { _ = Close() }()

	err := Init(LogConfig{
		Level:  "debug",
		Format: "json",
	})
	if err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	Debug().Msg("debug")
	Info().Msg("info")
	Warn().Msg("warn")
	Error().Msg("error")

	Debugf("debug %s", "formatted")
	Infof("info %s", "formatted")
	Warnf("warn %s", "formatted")
	Errorf("error %s", "formatted")
}

func TestGetWithoutInit(t *testing.T) {
	mu.Lock()
	initialized = false
	mu.Unlock()

	logger := Get()
	if logger == nil {
		t.Fatal("Get() should return a default logger when not initialized")
	}
}
