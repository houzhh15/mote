package cli

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"mote/internal/config"
	"mote/internal/provider/copilot"
)

// NewAuthCmd creates the auth command.
func NewAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Authentication commands",
		Long:  `Manage authentication tokens for Mote.`,
	}

	cmd.AddCommand(newAuthLoginCmd())
	cmd.AddCommand(newAuthDeviceLoginCmd())
	cmd.AddCommand(newAuthLogoutCmd())
	cmd.AddCommand(newAuthStatusCmd())

	return cmd
}

func newAuthLoginCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Configure authentication token",
		Long: `Configure GitHub Copilot authentication token.

You can obtain a GitHub Copilot token by:
1. Having an active GitHub Copilot subscription
2. Using the GitHub CLI: gh auth token
3. Or by visiting: https://github.com/settings/tokens

The token will be securely stored in your Mote configuration file.`,
		Example: `  # Interactive login (recommended)
  mote auth login

  # Provide token directly
  mote auth login --token ghp_xxxxx

  # Use GitHub CLI token
  mote auth login --token $(gh auth token)`,
		RunE: runAuthLogin,
	}

	cmd.Flags().StringP("token", "t", "", "GitHub token (if not provided, will prompt)")

	return cmd
}

func newAuthLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Remove authentication token",
		Long:  `Remove the stored authentication token from Mote configuration.`,
		RunE:  runAuthLogout,
	}
}

func newAuthStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Check authentication status",
		Long:  `Display the current authentication status.`,
		RunE:  runAuthStatus,
	}
}

func runAuthLogin(cmd *cobra.Command, args []string) error {
	cliCtx := GetCLIContext(cmd)
	if cliCtx == nil {
		return fmt.Errorf("CLI context not initialized")
	}

	cfg := cliCtx.Config
	log := cliCtx.Logger

	// Check if token is provided via flag
	token, _ := cmd.Flags().GetString("token")

	// If not provided, prompt for it
	if token == "" {
		fmt.Println("GitHub Copilot Authentication")
		fmt.Println("------------------------------")
		fmt.Println("")
		fmt.Println("To use Mote, you need a GitHub token with Copilot access.")
		fmt.Println("You can get one by running: gh auth token")
		fmt.Println("")
		fmt.Print("Enter your GitHub token: ")

		// Read token securely (hidden input)
		tokenBytes, err := term.ReadPassword(int(syscall.Stdin))
		if err != nil {
			return fmt.Errorf("failed to read token: %w", err)
		}
		token = strings.TrimSpace(string(tokenBytes))
		fmt.Println() // New line after hidden input
	}

	if token == "" {
		return fmt.Errorf("token cannot be empty")
	}

	// Validate token format (basic check)
	if !strings.HasPrefix(token, "ghp_") && !strings.HasPrefix(token, "github_pat_") {
		fmt.Println("")
		fmt.Println("⚠️  Warning: Token doesn't look like a GitHub personal access token")
		fmt.Print("Continue anyway? (y/N): ")

		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))

		if response != "y" && response != "yes" {
			return fmt.Errorf("authentication cancelled")
		}
	}

	// Update configuration
	cfg.Copilot.Token = token

	// Save configuration
	configPath := cliCtx.ConfigPath
	if configPath == "" {
		var err error
		configPath, err = config.DefaultConfigPath()
		if err != nil {
			return fmt.Errorf("failed to determine config path: %w", err)
		}
	}

	if err := config.SaveTo(cfg, configPath); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	fmt.Println("")
	fmt.Println("✓ Authentication token saved successfully!")
	fmt.Println("")
	fmt.Printf("Configuration saved to: %s\n", configPath)
	fmt.Println("")
	fmt.Println("You can now start the server with: mote serve")

	log.Info().Msg("Authentication token configured")

	return nil
}

func runAuthLogout(cmd *cobra.Command, args []string) error {
	cliCtx := GetCLIContext(cmd)
	if cliCtx == nil {
		return fmt.Errorf("CLI context not initialized")
	}

	cfg := cliCtx.Config
	log := cliCtx.Logger

	if cfg.Copilot.Token == "" {
		fmt.Println("No authentication token configured.")
		return nil
	}

	// Clear token
	cfg.Copilot.Token = ""

	// Save configuration
	configPath := cliCtx.ConfigPath
	if configPath == "" {
		var err error
		configPath, err = config.DefaultConfigPath()
		if err != nil {
			return fmt.Errorf("failed to determine config path: %w", err)
		}
	}

	if err := config.SaveTo(cfg, configPath); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	fmt.Println("✓ Authentication token removed successfully!")
	log.Info().Msg("Authentication token cleared")

	return nil
}

func runAuthStatus(cmd *cobra.Command, args []string) error {
	cliCtx := GetCLIContext(cmd)
	if cliCtx == nil {
		return fmt.Errorf("CLI context not initialized")
	}

	cfg := cliCtx.Config

	fmt.Println("Authentication Status")
	fmt.Println("--------------------")
	fmt.Println("")

	if cfg.Copilot.Token == "" {
		fmt.Println("Status: ❌ Not authenticated")
		fmt.Println("")
		fmt.Println("Run 'mote auth login' to configure authentication.")
		return nil
	}

	// Show masked token
	token := cfg.Copilot.Token
	maskedToken := maskToken(token)

	fmt.Println("Status: ✓ Token configured")
	fmt.Printf("Token:  %s\n", maskedToken)
	fmt.Println("")

	// Verify Copilot API access
	fmt.Println("Verifying Copilot API access...")
	tokenMgr := copilot.NewTokenManager(token)
	_, err := tokenMgr.GetToken()
	if err != nil {
		fmt.Println("")
		fmt.Println("⚠️  Copilot API Error:")
		fmt.Printf("   %v\n", err)
		fmt.Println("")
		fmt.Println("Possible causes:")
		fmt.Println("  1. You don't have an active GitHub Copilot subscription")
		fmt.Println("  2. The token doesn't have the required scopes")
		fmt.Println("  3. Your Copilot subscription has expired")
		fmt.Println("")
		fmt.Println("To fix:")
		fmt.Println("  - Ensure you have GitHub Copilot enabled: https://github.com/settings/copilot")
		fmt.Println("  - Try 'mote auth device-login' for OAuth authentication")
		return nil
	}

	fmt.Println("✓ Copilot API access verified")
	fmt.Println("")
	fmt.Println("You can start the server with: mote serve")

	return nil
}

func maskToken(token string) string {
	if len(token) <= 8 {
		return "***"
	}
	return token[:4] + "..." + token[len(token)-4:]
}

func newAuthDeviceLoginCmd() *cobra.Command {
	var noBrowser bool

	cmd := &cobra.Command{
		Use:   "device-login",
		Short: "Authenticate using OAuth Device Flow",
		Long: `Authenticate using GitHub OAuth Device Flow.

This is the recommended way to authenticate with GitHub Copilot.
It will open your browser and prompt you to enter a code to authorize Mote.`,
		Example: `  # Interactive device login (opens browser)
  mote auth device-login

  # Without auto-opening browser
  mote auth device-login --no-browser`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAuthDeviceLogin(cmd, noBrowser)
		},
	}

	cmd.Flags().BoolVar(&noBrowser, "no-browser", false, "Don't automatically open browser")

	return cmd
}

func runAuthDeviceLogin(cmd *cobra.Command, noBrowser bool) error {
	cliCtx := GetCLIContext(cmd)
	if cliCtx == nil {
		return fmt.Errorf("CLI context not initialized")
	}

	cfg := cliCtx.Config
	log := cliCtx.Logger

	fmt.Println("GitHub Copilot Device Authentication")
	fmt.Println("-------------------------------------")
	fmt.Println("")

	// Use the real OAuth Device Flow
	authMgr := copilot.NewAuthManager()

	// Request device code
	fmt.Println("Requesting device code...")
	ctx := cmd.Context()
	deviceResp, err := authMgr.RequestDeviceCode(ctx)
	if err != nil {
		return fmt.Errorf("failed to get device code: %w", err)
	}

	fmt.Println("")
	fmt.Println("Please visit the following URL to authenticate:")
	fmt.Printf("  %s\n", deviceResp.VerificationURI)
	fmt.Println("")
	fmt.Printf("Enter this code: %s\n", deviceResp.UserCode)
	fmt.Println("")

	// Open browser if not disabled
	if !noBrowser {
		fmt.Println("Opening browser...")
		openBrowser(deviceResp.VerificationURI)
	}

	fmt.Println("Waiting for authorization (this may take a moment)...")
	fmt.Println("")

	// Poll for access token
	tokenResp, err := authMgr.PollForAccessToken(ctx, deviceResp.DeviceCode)
	if err != nil {
		return fmt.Errorf("authorization failed: %w", err)
	}

	// Update configuration with the access token
	cfg.Copilot.Token = tokenResp.AccessToken

	// Save configuration
	configPath := cliCtx.ConfigPath
	if configPath == "" {
		var err error
		configPath, err = config.DefaultConfigPath()
		if err != nil {
			return fmt.Errorf("failed to determine config path: %w", err)
		}
	}

	if err := config.SaveTo(cfg, configPath); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
	}

	fmt.Println("✓ Authentication successful!")
	fmt.Println("")

	// Verify Copilot API access
	fmt.Println("Verifying Copilot API access...")
	tokenMgr := copilot.NewTokenManager(tokenResp.AccessToken)
	_, err = tokenMgr.GetToken()
	if err != nil {
		fmt.Printf("⚠️  Warning: Could not verify Copilot access: %v\n", err)
		fmt.Println("   You may need a GitHub Copilot subscription.")
	} else {
		fmt.Println("✓ Copilot API access verified")
	}

	log.Info().Msg("Device authentication completed")

	return nil
}

// openBrowser opens a URL in the default browser
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default: // linux and others
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start() // Ignore errors, browser may not be available
}
