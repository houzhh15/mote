// Package defaults provides embedded default files for Mote initialization.
package defaults

import "embed"

//go:embed skills/*
var defaultsFS embed.FS

// GetDefaultsFS returns the embedded filesystem containing default files.
func GetDefaultsFS() embed.FS {
	return defaultsFS
}
