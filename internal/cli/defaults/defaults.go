// Package defaults provides embedded default files for Mote initialization.
package defaults

import "embed"

//go:embed skills/*
var defaultsFS embed.FS

//go:embed agents.yaml
var defaultAgentsYAML []byte

//go:embed default-agents/*
var defaultAgentDirFS embed.FS

//go:embed prompts/*
var defaultPromptsFS embed.FS

// GetDefaultsFS returns the embedded filesystem containing default files.
func GetDefaultsFS() embed.FS {
	return defaultsFS
}

// GetDefaultAgentsYAML returns the embedded default agents.yaml content.
// This serves as the initial agent configuration for new installations.
func GetDefaultAgentsYAML() []byte {
	return defaultAgentsYAML
}

// GetDefaultAgentDirFiles returns a map of filename → content for all
// default agent files that should be installed into ~/.mote/agents/.
// Files are only installed when absent (never overwritten).
func GetDefaultAgentDirFiles() map[string][]byte {
	entries, err := defaultAgentDirFS.ReadDir("default-agents")
	if err != nil {
		return nil
	}
	result := make(map[string][]byte, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := defaultAgentDirFS.ReadFile("default-agents/" + e.Name())
		if err == nil {
			result[e.Name()] = data
		}
	}
	return result
}

// GetDefaultPromptsFS returns the embedded filesystem containing default prompt files.
func GetDefaultPromptsFS() embed.FS {
	return defaultPromptsFS
}
