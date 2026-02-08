// Package config provides configuration path utilities.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DefaultConfigDir returns the default configuration directory (~/.mote).
func DefaultConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	return filepath.Join(home, ".mote"), nil
}

// DefaultConfigPath returns the default configuration file path (~/.mote/config.yaml).
func DefaultConfigPath() (string, error) {
	dir, err := DefaultConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// DefaultDataPath returns the default database file path (~/.mote/data.db).
func DefaultDataPath() (string, error) {
	dir, err := DefaultConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "data.db"), nil
}

// ExpandPath expands ~ prefix in path to user home directory.
func ExpandPath(path string) (string, error) {
	if path == "" {
		return "", nil
	}

	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("get home dir: %w", err)
		}
		return filepath.Join(home, path[2:]), nil
	}

	if path == "~" {
		return os.UserHomeDir()
	}

	return path, nil
}
