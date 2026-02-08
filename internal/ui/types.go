// Package ui provides UI system components for Mote.
package ui

import "time"

// Component represents a UI component's metadata.
type Component struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	File        string         `json:"file"`
	Version     string         `json:"version,omitempty"`
	Props       map[string]any `json:"props,omitempty"`
}

// Page represents a UI page definition.
type Page struct {
	Name        string `json:"name"`
	File        string `json:"file"`
	Route       string `json:"route"`
	Description string `json:"description,omitempty"`
}

// Manifest represents the UI component manifest file structure.
type Manifest struct {
	Version    string      `json:"version"`
	Components []Component `json:"components,omitempty"`
	Pages      []Page      `json:"pages,omitempty"`
}

// UIState represents the current UI state synchronized across clients.
type UIState struct {
	ActiveSession string         `json:"active_session,omitempty"`
	Theme         string         `json:"theme,omitempty"`
	SidebarOpen   bool           `json:"sidebar_open"`
	CurrentPage   string         `json:"current_page,omitempty"`
	Custom        map[string]any `json:"custom,omitempty"`
}

// SessionSummary represents a summary of a session for listing.
type SessionSummary struct {
	ID           string    `json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	MessageCount int       `json:"message_count"`
	Preview      string    `json:"preview,omitempty"`
}

// ConfigView represents the public configuration view.
type ConfigView struct {
	Gateway GatewayConfigView `json:"gateway"`
	Memory  MemoryConfigView  `json:"memory"`
	Cron    CronConfigView    `json:"cron"`
}

// GatewayConfigView represents gateway configuration for UI.
type GatewayConfigView struct {
	Port int    `json:"port"`
	Host string `json:"host"`
}

// MemoryConfigView represents memory configuration for UI.
type MemoryConfigView struct {
	Enabled bool `json:"enabled"`
}

// CronConfigView represents cron configuration for UI.
type CronConfigView struct {
	Enabled bool `json:"enabled"`
}

// ComponentsResponse is the API response for GET /api/ui/components.
type ComponentsResponse struct {
	Components []Component `json:"components"`
}

// SessionsResponse is the API response for GET /api/sessions.
type SessionsResponse struct {
	Sessions []SessionSummary `json:"sessions"`
}

// SuccessResponse is a generic success response.
type SuccessResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

// ErrorResponse is the API error response format.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail contains error details.
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
