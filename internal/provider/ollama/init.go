package ollama

import (
	"mote/internal/provider"
	"mote/pkg/logger"

	"github.com/spf13/viper"
)

// Register registers the Ollama provider with the global registry.
// This should be called during application initialization.
func Register() {
	cfg := Config{
		Endpoint:  viper.GetString("ollama.endpoint"),
		Model:     viper.GetString("ollama.model"),
		Timeout:   viper.GetDuration("ollama.timeout"),
		KeepAlive: viper.GetString("ollama.keep_alive"),
	}

	// Apply defaults
	if cfg.Endpoint == "" {
		cfg.Endpoint = DefaultEndpoint
	}
	if cfg.Model == "" {
		cfg.Model = DefaultModel
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = DefaultTimeout
	}
	if cfg.KeepAlive == "" {
		cfg.KeepAlive = DefaultKeepAlive
	}

	p := NewOllamaProvider(cfg)
	provider.Register(p)

	logger.Debug().
		Str("endpoint", cfg.Endpoint).
		Str("model", cfg.Model).
		Msg("Ollama provider registered")
}

// MustRegister registers the Ollama provider and panics on error.
// Use this in init() functions where error handling is not possible.
func MustRegister() {
	Register()
}
