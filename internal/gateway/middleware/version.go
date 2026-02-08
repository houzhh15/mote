package middleware

import (
	"net/http"
	"time"
)

// VersionConfig configures API versioning middleware.
type VersionConfig struct {
	// CurrentVersion is the current API version.
	CurrentVersion string
	// DeprecatedVersions lists deprecated versions with their sunset dates.
	DeprecatedVersions map[string]time.Time
	// DefaultVersion is used when no version is specified.
	DefaultVersion string
}

// DefaultVersionConfig returns the default version configuration.
func DefaultVersionConfig() VersionConfig {
	return VersionConfig{
		CurrentVersion:     "1",
		DeprecatedVersions: make(map[string]time.Time),
		DefaultVersion:     "1",
	}
}

// Version returns middleware that handles API versioning.
// It reads the Accept-Version header and sets API-Version, Deprecation, and Sunset headers.
func Version(config VersionConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get requested version from header
			version := r.Header.Get("Accept-Version")
			if version == "" {
				version = config.DefaultVersion
			}

			// Set API-Version header
			w.Header().Set("API-Version", version)

			// Check if version is deprecated
			if sunsetDate, deprecated := config.DeprecatedVersions[version]; deprecated {
				// Set Deprecation header (RFC 8594 format)
				w.Header().Set("Deprecation", "true")
				w.Header().Set("Sunset", sunsetDate.Format(http.TimeFormat))
			}

			next.ServeHTTP(w, r)
		})
	}
}

// APIVersionHandler wraps a handler with version-specific behavior.
type APIVersionHandler struct {
	handlers map[string]http.Handler
	fallback http.Handler
}

// NewAPIVersionHandler creates a new version-aware handler.
func NewAPIVersionHandler() *APIVersionHandler {
	return &APIVersionHandler{
		handlers: make(map[string]http.Handler),
	}
}

// Register registers a handler for a specific API version.
func (h *APIVersionHandler) Register(version string, handler http.Handler) {
	h.handlers[version] = handler
}

// SetFallback sets the fallback handler for unknown versions.
func (h *APIVersionHandler) SetFallback(handler http.Handler) {
	h.fallback = handler
}

// ServeHTTP dispatches to the appropriate version handler.
func (h *APIVersionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	version := r.Header.Get("Accept-Version")
	if version == "" {
		version = "1" // Default version
	}

	if handler, ok := h.handlers[version]; ok {
		handler.ServeHTTP(w, r)
		return
	}

	if h.fallback != nil {
		h.fallback.ServeHTTP(w, r)
		return
	}

	http.Error(w, "Unsupported API version", http.StatusNotAcceptable)
}
