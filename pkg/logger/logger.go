// Package logger provides structured logging functionality based on zerolog.
package logger

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/rs/zerolog"
)

// LogConfig holds logger configuration.
type LogConfig struct {
	Level  string `json:"level" mapstructure:"level"`   // debug, info, warn, error
	Format string `json:"format" mapstructure:"format"` // console, json
	File   string `json:"file" mapstructure:"file"`     // log file path, empty means no file
}

var (
	globalLogger zerolog.Logger
	logFile      *os.File
	mu           sync.RWMutex
	initialized  bool
)

// parseLevel converts string level to zerolog.Level.
func parseLevel(level string) zerolog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return zerolog.DebugLevel
	case "info":
		return zerolog.InfoLevel
	case "warn", "warning":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	case "fatal":
		return zerolog.FatalLevel
	default:
		return zerolog.InfoLevel
	}
}

// Init initializes the global logger with the given configuration.
func Init(config LogConfig) error {
	mu.Lock()
	defer mu.Unlock()

	level := parseLevel(config.Level)
	zerolog.SetGlobalLevel(level)

	var writers []io.Writer

	// Setup console/json output to stderr
	if strings.ToLower(config.Format) == "console" {
		writers = append(writers, zerolog.ConsoleWriter{
			Out:        os.Stderr,
			TimeFormat: "2006-01-02T15:04:05-07:00",
		})
	} else {
		writers = append(writers, os.Stderr)
	}

	// Setup file output if configured
	if config.File != "" {
		f, err := os.OpenFile(config.File, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return fmt.Errorf("open log file %s: %w", config.File, err)
		}
		logFile = f
		writers = append(writers, f)
	}

	var output io.Writer
	if len(writers) == 1 {
		output = writers[0]
	} else {
		output = io.MultiWriter(writers...)
	}

	globalLogger = zerolog.New(output).With().Timestamp().Caller().Logger()
	initialized = true
	return nil
}

// Get returns the global logger instance.
func Get() *zerolog.Logger {
	mu.RLock()
	defer mu.RUnlock()
	if !initialized {
		// Return a default logger if not initialized
		l := zerolog.New(os.Stderr).With().Timestamp().Logger()
		return &l
	}
	return &globalLogger
}

// With creates a new logger with additional fields.
func With(fields map[string]any) *zerolog.Logger {
	mu.RLock()
	defer mu.RUnlock()

	ctx := globalLogger.With()
	for k, v := range fields {
		ctx = ctx.Interface(k, v)
	}
	l := ctx.Logger()
	return &l
}

// Close closes the log file if opened.
func Close() error {
	mu.Lock()
	defer mu.Unlock()

	if logFile != nil {
		err := logFile.Close()
		logFile = nil
		return err
	}
	return nil
}

// Debug returns a debug level event.
func Debug() *zerolog.Event {
	return Get().Debug()
}

// Info returns an info level event.
func Info() *zerolog.Event {
	return Get().Info()
}

// Warn returns a warn level event.
func Warn() *zerolog.Event {
	return Get().Warn()
}

// Error returns an error level event.
func Error() *zerolog.Event {
	return Get().Error()
}

// Fatal returns a fatal level event.
func Fatal() *zerolog.Event {
	return Get().Fatal()
}

// Debugf logs a formatted debug message.
func Debugf(format string, args ...any) {
	Get().Debug().Msgf(format, args...)
}

// Infof logs a formatted info message.
func Infof(format string, args ...any) {
	Get().Info().Msgf(format, args...)
}

// Warnf logs a formatted warn message.
func Warnf(format string, args ...any) {
	Get().Warn().Msgf(format, args...)
}

// Errorf logs a formatted error message.
func Errorf(format string, args ...any) {
	Get().Error().Msgf(format, args...)
}
