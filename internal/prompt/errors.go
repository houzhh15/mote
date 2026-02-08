// Package prompt provides system prompt building for agents.
package prompt

import "errors"

// Prompt errors.
var (
	// ErrTemplateRender indicates that template rendering failed.
	ErrTemplateRender = errors.New("prompt: template render failed")

	// ErrMemorySearch indicates that memory search failed.
	ErrMemorySearch = errors.New("prompt: memory search failed")
)
