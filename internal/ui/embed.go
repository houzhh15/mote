package ui

import (
	"embed"
	"io/fs"
)

//go:embed all:ui
var embeddedUI embed.FS

// GetEmbedFS returns the embedded UI filesystem.
// The returned fs.FS is rooted at the "ui" directory.
func GetEmbedFS() fs.FS {
	sub, err := fs.Sub(embeddedUI, "ui")
	if err != nil {
		// This should never happen as "ui" is embedded at compile time
		panic("failed to get embedded ui subdirectory: " + err.Error())
	}
	return sub
}
