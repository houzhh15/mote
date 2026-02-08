// Package main is the entry point for the Mote GUI application.
// It supports both GUI mode (no args) and CLI mode (with subcommands).
package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"os"

	"mote/internal/cli"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// Determine run mode based on arguments
	if shouldRunGUI() {
		runGUI()
	} else {
		runCLI()
	}
}

// shouldRunGUI determines if we should launch GUI mode
// GUI mode is launched when:
// - No arguments provided
// - First argument is "gui"
// CLI mode is launched when:
// - Arguments match CLI commands (serve, chat, version, etc.)
func shouldRunGUI() bool {
	if len(os.Args) <= 1 {
		// No arguments - launch GUI
		return true
	}

	firstArg := os.Args[1]

	// Explicit GUI mode
	if firstArg == "gui" {
		return true
	}

	// Help and version are handled by CLI
	if firstArg == "-h" || firstArg == "--help" || firstArg == "-v" || firstArg == "--version" {
		return false
	}

	// Known CLI commands
	cliCommands := []string{
		"serve", "chat", "session", "memory", "tool", "mcp",
		"config", "init", "auth", "version", "doctor", "mode",
		"cron", "usage", "help",
	}

	for _, cmd := range cliCommands {
		if firstArg == cmd {
			return false
		}
	}

	// Unknown argument - try CLI
	return false
}

// runGUI starts the Wails GUI application
func runGUI() {
	// Create application instance
	app := NewApp()

	// Sub filesystem to serve from dist directory
	assetsFS, err := fs.Sub(assets, "frontend/dist")
	if err != nil {
		log.Fatal("Failed to create sub filesystem:", err)
	}

	// Create application with options
	err = wails.Run(&options.App{
		Title:  "Mote",
		Width:  1024,
		Height: 768,
		AssetServer: &assetserver.Options{
			Assets: assetsFS,
		},
		BackgroundColour: &options.RGBA{R: 255, G: 255, B: 255, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		OnBeforeClose:    app.beforeClose,
		Bind: []interface{}{
			app,
			app.apiClient,
		},
	})

	if err != nil {
		log.Fatal("Error:", err.Error())
	}
}

// runCLI runs the CLI command parser
func runCLI() {
	rootCmd := cli.NewRootCmd()

	// Add "gui" subcommand for explicit GUI launch
	guiCmd := &cli.GuiCommand{
		RunGUI: runGUI,
	}
	rootCmd.AddCommand(guiCmd.Command())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
