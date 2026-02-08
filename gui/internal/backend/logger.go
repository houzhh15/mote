// Package backend provides Go bindings for the Wails frontend.
package backend

import (
	"os"
	"path/filepath"

	"github.com/rs/zerolog"
)

// Logger is the global logger for the GUI application.
var Logger zerolog.Logger

// InitLogger initializes the GUI logger.
func InitLogger() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	logDir := filepath.Join(homeDir, ".mote", "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return err
	}

	logPath := filepath.Join(logDir, "gui.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		Logger = zerolog.New(os.Stderr).With().Timestamp().Str("component", "gui").Logger()
		return nil
	}

	Logger = zerolog.New(logFile).With().Timestamp().Str("component", "gui").Logger()
	return nil
}

// LogInfo logs an info message.
func LogInfo(msg string) {
	Logger.Info().Msg(msg)
}

// LogError logs an error message.
func LogError(err error, msg string) {
	Logger.Error().Err(err).Msg(msg)
}

// LogDebug logs a debug message.
func LogDebug(msg string) {
	Logger.Debug().Msg(msg)
}

// LogServiceStatus logs a service status change.
func LogServiceStatus(running bool, pid int) {
	Logger.Info().Bool("running", running).Int("pid", pid).Msg("Service status changed")
}

// LogAPIRequest logs an API request.
func LogAPIRequest(method, path string) {
	Logger.Debug().Str("method", method).Str("path", path).Msg("API request")
}

// LogIPCCommand logs an IPC command.
func LogIPCCommand(action string) {
	Logger.Debug().Str("action", action).Msg("IPC command received")
}
