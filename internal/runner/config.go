package runner

import "time"

// Config holds configuration for the agent runner.
type Config struct {
	// MaxIterations is the maximum number of tool call iterations.
	// Default is 10.
	MaxIterations int `json:"max_iterations"`

	// MaxTokens is the maximum number of tokens for model output.
	// Default is 8000.
	MaxTokens int `json:"max_tokens"`

	// MaxMessages is the maximum number of messages to keep in history.
	// Default is 100.
	MaxMessages int `json:"max_messages"`

	// Timeout is the maximum duration for a single run.
	// Default is 5 minutes.
	Timeout time.Duration `json:"timeout"`

	// StreamOutput enables streaming output events.
	// Default is true.
	StreamOutput bool `json:"stream_output"`

	// Temperature controls the randomness of the model output.
	// Default is 0.7.
	Temperature float64 `json:"temperature"`

	// SystemPrompt is an optional custom system prompt.
	SystemPrompt string `json:"system_prompt,omitempty"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		MaxIterations: 10,
		MaxTokens:     8000,
		MaxMessages:   100,
		Timeout:       5 * time.Minute,
		StreamOutput:  true,
		Temperature:   0.7,
	}
}

// WithMaxIterations returns a copy of the config with the specified max iterations.
func (c Config) WithMaxIterations(n int) Config {
	c.MaxIterations = n
	return c
}

// WithMaxTokens returns a copy of the config with the specified max tokens.
func (c Config) WithMaxTokens(n int) Config {
	c.MaxTokens = n
	return c
}

// WithMaxMessages returns a copy of the config with the specified max messages.
func (c Config) WithMaxMessages(n int) Config {
	c.MaxMessages = n
	return c
}

// WithTimeout returns a copy of the config with the specified timeout.
func (c Config) WithTimeout(d time.Duration) Config {
	c.Timeout = d
	return c
}

// WithStreamOutput returns a copy of the config with the specified stream output setting.
func (c Config) WithStreamOutput(enabled bool) Config {
	c.StreamOutput = enabled
	return c
}

// WithTemperature returns a copy of the config with the specified temperature.
func (c Config) WithTemperature(t float64) Config {
	c.Temperature = t
	return c
}

// WithSystemPrompt returns a copy of the config with the specified system prompt.
func (c Config) WithSystemPrompt(prompt string) Config {
	c.SystemPrompt = prompt
	return c
}

// Validate returns an error if the configuration is invalid.
func (c Config) Validate() error {
	if c.MaxIterations <= 0 {
		c.MaxIterations = 10
	}
	if c.MaxTokens <= 0 {
		c.MaxTokens = 8000
	}
	if c.MaxMessages <= 0 {
		c.MaxMessages = 100
	}
	if c.Timeout <= 0 {
		c.Timeout = 5 * time.Minute
	}
	if c.Temperature < 0 {
		c.Temperature = 0
	}
	if c.Temperature > 2 {
		c.Temperature = 2
	}
	return nil
}
