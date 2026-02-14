package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"mote/internal/config"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// NewProviderCmd creates the provider command group.
func NewProviderCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "provider",
		Short: "Manage LLM providers",
		Long:  "Configure and manage LLM providers (copilot, ollama, minimax)",
	}

	cmd.AddCommand(newProviderListCmd())
	cmd.AddCommand(newProviderUseCmd())
	cmd.AddCommand(newProviderStatusCmd())
	cmd.AddCommand(newProviderEnableCmd())
	cmd.AddCommand(newProviderDisableCmd())

	return cmd
}

// newProviderListCmd lists available providers.
func newProviderListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List available providers",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.GetConfig()
			if cfg == nil {
				return fmt.Errorf("failed to get config: config not loaded")
			}

			defaultProvider := cfg.Provider.Default
			if defaultProvider == "" {
				defaultProvider = "copilot"
			}

			enabledProviders := cfg.Provider.GetEnabledProviders()
			enabledMap := make(map[string]bool)
			for _, p := range enabledProviders {
				enabledMap[p] = true
			}

			providers := []struct {
				name string
				desc string
			}{
				{"copilot-acp", "GitHub Copilot ACP (CLI mode, recommended)"},
				{"copilot", "GitHub Copilot API (REST, temporarily disabled)"},
				{"ollama", "Local Ollama server"},
				{"minimax", "MiniMax AI (cloud)"},
			}

			fmt.Println("Available providers:")
			fmt.Println()
			for _, p := range providers {
				defaultMarker := "  "
				if p.name == defaultProvider {
					defaultMarker = "* "
				}
				enabledStatus := "[disabled]"
				if enabledMap[p.name] {
					enabledStatus = "[enabled]"
				}
				fmt.Printf("%s%-10s  %-10s  %s\n", defaultMarker, p.name, enabledStatus, p.desc)
			}
			fmt.Println()
			fmt.Printf("Default: %s\n", defaultProvider)
			fmt.Printf("Enabled: %s\n", strings.Join(enabledProviders, ", "))
			fmt.Println("\nUse 'mote provider enable <name>' to enable a provider")
			fmt.Println("Use 'mote provider disable <name>' to disable a provider")
			fmt.Println("Use 'mote provider use <name>' to set the default provider")

			return nil
		},
	}
}

// newProviderUseCmd switches the active provider.
func newProviderUseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "use <provider>",
		Short: "Switch to a provider",
		Long:  "Switch to a different LLM provider (copilot or ollama)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			providerName := strings.ToLower(args[0])

			// Validate provider name
			validProviders := map[string]bool{
				"copilot":     true,
				"copilot-acp": true,
				"ollama":      true,
			}

			if !validProviders[providerName] {
				return fmt.Errorf("unknown provider: %s (valid: copilot, copilot-acp, ollama)", providerName)
			}

			// Load current config
			cfg := config.GetConfig()
			if cfg == nil {
				return fmt.Errorf("failed to get config: config not loaded")
			}

			// Update provider
			cfg.Provider.Default = providerName

			// Save config
			configDir, err := config.DefaultConfigDir()
			if err != nil {
				return fmt.Errorf("failed to get config dir: %w", err)
			}

			configPath := filepath.Join(configDir, "config.yaml")

			// Read existing config file to preserve other settings
			data, err := os.ReadFile(configPath)
			if err != nil {
				return fmt.Errorf("failed to read config: %w", err)
			}

			var configMap map[string]any
			if err := yaml.Unmarshal(data, &configMap); err != nil {
				return fmt.Errorf("failed to parse config: %w", err)
			}

			// Update provider section
			if configMap["provider"] == nil {
				configMap["provider"] = map[string]any{}
			}
			configMap["provider"].(map[string]any)["default"] = providerName

			// Write back
			newData, err := yaml.Marshal(configMap)
			if err != nil {
				return fmt.Errorf("failed to marshal config: %w", err)
			}

			if err := os.WriteFile(configPath, newData, 0644); err != nil {
				return fmt.Errorf("failed to write config: %w", err)
			}

			fmt.Printf("Switched to provider: %s\n", providerName)

			// Show relevant hints
			switch providerName {
			case "copilot":
				fmt.Println("\nWarning: Copilot REST API mode is temporarily disabled.")
				fmt.Println("Consider using 'copilot-acp' instead: mote provider use copilot-acp")
			case "copilot-acp":
				fmt.Println("\nNote: Copilot ACP uses the Copilot CLI for authentication.")
				fmt.Println("Make sure 'copilot' CLI is installed and authenticated.")
			case "ollama":
				fmt.Printf("\nNote: Make sure Ollama is running at %s\n", cfg.Ollama.Endpoint)
				fmt.Println("You can configure Ollama settings with 'mote config set ollama.*'")
			case "minimax":
				if cfg.Minimax.APIKey == "" {
					fmt.Println("\nWarning: MiniMax API key not configured.")
					fmt.Println("Set it with: mote config set minimax.api_key <your-key>")
				} else {
					fmt.Println("\nMiniMax provider configured.")
				}
			}

			return nil
		},
	}
}

// newProviderStatusCmd shows current provider status.
func newProviderStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show current provider status",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := config.GetConfig()
			if cfg == nil {
				return fmt.Errorf("failed to get config: config not loaded")
			}

			current := cfg.Provider.Default
			if current == "" {
				current = "copilot"
			}

			enabled := cfg.Provider.GetEnabledProviders()

			fmt.Printf("Default Provider: %s\n", current)
			fmt.Printf("Enabled Providers: %s\n\n", strings.Join(enabled, ", "))

			switch current {
			case "copilot":
				fmt.Println("Copilot Configuration (REST API - TEMPORARILY DISABLED):")
				fmt.Println("  Status: Temporarily disabled. Use 'copilot-acp' instead.")
				if cfg.Copilot.Token != "" {
					fmt.Println("  Token: ****configured****")
				} else {
					fmt.Println("  Token: not configured")
				}
				model := cfg.Copilot.Model
				if model == "" {
					model = "(default)"
				}
				fmt.Printf("  Model: %s\n", model)
				fmt.Printf("  Max Tokens: %d\n", cfg.Copilot.MaxTokens)

			case "copilot-acp":
				fmt.Println("Copilot ACP Configuration (CLI mode):")
				model := cfg.Copilot.Model
				if model == "" {
					model = "(default)"
				}
				fmt.Printf("  Model: %s\n", model)
				fmt.Printf("  Max Tokens: %d\n", cfg.Copilot.MaxTokens)

			case "ollama":
				fmt.Println("Ollama Configuration:")
				endpoint := cfg.Ollama.Endpoint
				if endpoint == "" {
					endpoint = "http://localhost:11434"
				}
				model := cfg.Ollama.Model
				if model == "" {
					model = "llama3.2"
				}
				fmt.Printf("  Endpoint: %s\n", endpoint)
				fmt.Printf("  Model: %s\n", model)
				fmt.Printf("  Timeout: %s\n", cfg.Ollama.Timeout)
				fmt.Printf("  Keep Alive: %s\n", cfg.Ollama.KeepAlive)

			case "minimax":
				fmt.Println("MiniMax Configuration:")
				endpoint := cfg.Minimax.Endpoint
				if endpoint == "" {
					endpoint = "https://api.minimaxi.com/v1"
				}
				model := cfg.Minimax.Model
				if model == "" {
					model = "MiniMax-M2.5"
				}
				if cfg.Minimax.APIKey != "" {
					fmt.Println("  API Key: ****configured****")
				} else {
					fmt.Println("  API Key: not configured")
				}
				fmt.Printf("  Endpoint: %s\n", endpoint)
				fmt.Printf("  Model: %s\n", model)
				fmt.Printf("  Max Tokens: %d\n", cfg.Minimax.MaxTokens)
			}

			return nil
		},
	}
}

// newProviderEnableCmd enables a provider.
func newProviderEnableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "enable <provider>",
		Short: "Enable a provider",
		Long:  "Enable a provider to be included in the model list (copilot, copilot-acp, ollama, or minimax)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			providerName := strings.ToLower(args[0])

			// Validate provider name
			validProviders := map[string]bool{
				"copilot":     true,
				"copilot-acp": true,
				"ollama":      true,
				"minimax":     true,
			}

			if !validProviders[providerName] {
				return fmt.Errorf("unknown provider: %s (valid: copilot, copilot-acp, ollama, minimax)", providerName)
			}

			// Load current config
			cfg := config.GetConfig()
			if cfg == nil {
				return fmt.Errorf("failed to get config: config not loaded")
			}

			// Get current enabled list
			enabled := cfg.Provider.GetEnabledProviders()

			// Check if already enabled
			for _, p := range enabled {
				if p == providerName {
					fmt.Printf("Provider '%s' is already enabled\n", providerName)
					return nil
				}
			}

			// Add to enabled list
			enabled = append(enabled, providerName)

			// Save config
			if err := updateProviderEnabled(enabled); err != nil {
				return err
			}

			fmt.Printf("Enabled provider: %s\n", providerName)
			fmt.Printf("Current enabled providers: %s\n", strings.Join(enabled, ", "))
			fmt.Println("\nRestart 'mote serve' to apply changes.")

			return nil
		},
	}
}

// newProviderDisableCmd disables a provider.
func newProviderDisableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "disable <provider>",
		Short: "Disable a provider",
		Long:  "Disable a provider from the model list (copilot, copilot-acp, ollama, or minimax)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			providerName := strings.ToLower(args[0])

			// Validate provider name
			validProviders := map[string]bool{
				"copilot":     true,
				"copilot-acp": true,
				"ollama":      true,
				"minimax":     true,
			}

			if !validProviders[providerName] {
				return fmt.Errorf("unknown provider: %s (valid: copilot, copilot-acp, ollama, minimax)", providerName)
			}

			// Load current config
			cfg := config.GetConfig()
			if cfg == nil {
				return fmt.Errorf("failed to get config: config not loaded")
			}

			// Get current enabled list
			enabled := cfg.Provider.GetEnabledProviders()

			// Check if provider is in the list
			found := false
			var newEnabled []string
			for _, p := range enabled {
				if p == providerName {
					found = true
				} else {
					newEnabled = append(newEnabled, p)
				}
			}

			if !found {
				fmt.Printf("Provider '%s' is not enabled\n", providerName)
				return nil
			}

			// Ensure at least one provider is enabled
			if len(newEnabled) == 0 {
				return fmt.Errorf("cannot disable '%s': at least one provider must be enabled", providerName)
			}

			// Save config
			if err := updateProviderEnabled(newEnabled); err != nil {
				return err
			}

			fmt.Printf("Disabled provider: %s\n", providerName)
			fmt.Printf("Current enabled providers: %s\n", strings.Join(newEnabled, ", "))
			fmt.Println("\nRestart 'mote serve' to apply changes.")

			return nil
		},
	}
}

// updateProviderEnabled updates the provider.enabled config.
func updateProviderEnabled(enabled []string) error {
	configDir, err := config.DefaultConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config dir: %w", err)
	}

	configPath := filepath.Join(configDir, "config.yaml")

	// Read existing config file to preserve other settings
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}

	var configMap map[string]any
	if err := yaml.Unmarshal(data, &configMap); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	// Update provider section
	if configMap["provider"] == nil {
		configMap["provider"] = map[string]any{}
	}
	configMap["provider"].(map[string]any)["enabled"] = enabled

	// Write back
	newData, err := yaml.Marshal(configMap)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, newData, 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}
