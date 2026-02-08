package cli

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"mote/internal/server"
)

// NewServeCmd creates the serve command.
func NewServeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the Mote gateway server",
		Long: `Start the Mote gateway server.

This command starts the HTTP gateway server that provides:
- REST API endpoints
- WebSocket support for real-time communication
- Web UI serving
- Agent runtime

The server will listen on the configured host and port (default: localhost:18788).`,
		Example: `  # Start server with default configuration
  mote serve

  # Start server with custom port
  mote serve --port 8080

  # Start server with verbose logging
  mote serve --verbose`,
		RunE: runServe,
	}

	cmd.Flags().IntP("port", "p", 0, "port to listen on (overrides config)")
	cmd.Flags().String("host", "", "host to bind to (overrides config)")

	return cmd
}

func runServe(cmd *cobra.Command, args []string) error {
	cliCtx := GetCLIContext(cmd)
	if cliCtx == nil {
		return fmt.Errorf("CLI context not initialized")
	}

	cfg := cliCtx.Config
	log := cliCtx.Logger

	// Override config with flags if provided
	if port, _ := cmd.Flags().GetInt("port"); port > 0 {
		cfg.Gateway.Port = port
	}
	if host, _ := cmd.Flags().GetString("host"); host != "" {
		cfg.Gateway.Host = host
	}

	// Set default port if not configured
	if cfg.Gateway.Port == 0 {
		cfg.Gateway.Port = 18788
	}
	if cfg.Gateway.Host == "" {
		cfg.Gateway.Host = "localhost"
	}

	log.Info().Msg("Starting Mote server...")

	// Use the shared server implementation
	srv, err := server.NewServer(server.ServerConfig{
		ConfigPath:  cliCtx.ConfigPath,
		StoragePath: cliCtx.StoragePath,
		Logger:      *log,
	})
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}

	// Start server
	if err := srv.Start(); err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}

	log.Info().
		Str("address", fmt.Sprintf("http://%s:%d", cfg.Gateway.Host, cfg.Gateway.Port)).
		Msg("Server started successfully")

	// Wait for interrupt signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	select {
	case <-sigCh:
		log.Info().Msg("Shutting down server...")
	case err := <-srv.ErrorChan():
		if err != nil {
			log.Error().Err(err).Msg("Server error")
			return err
		}
	}

	// Graceful shutdown
	if err := srv.Stop(); err != nil {
		log.Error().Err(err).Msg("Error during shutdown")
		return err
	}

	log.Info().Msg("Server stopped")
	return nil
}
