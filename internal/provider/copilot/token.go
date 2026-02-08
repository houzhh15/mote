// Package copilot provides the GitHub Copilot AI provider implementation.
package copilot

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"mote/pkg/logger"
)

const (
	// GitHubCopilotTokenURL is the endpoint to exchange GitHub token for Copilot token.
	GitHubCopilotTokenURL = "https://api.github.com/copilot_internal/v2/token"

	// DefaultBaseURL is the default Copilot API base URL.
	DefaultBaseURL = "https://api.githubcopilot.com"

	// TokenRefreshMargin is the time before expiration to refresh the token.
	TokenRefreshMargin = 5 * time.Minute

	// CopilotEditorVersion is the editor version header value.
	CopilotEditorVersion = "vscode/1.95.0"

	// CopilotEditorPluginVersion is the plugin version header value.
	CopilotEditorPluginVersion = "copilot/1.250.0"

	// CopilotUserAgent is the User-Agent for Copilot requests.
	CopilotUserAgent = "GithubCopilot/1.250.0"
)

// Error definitions for token operations.
var (
	ErrUnauthorized       = errors.New("unauthorized: invalid or expired GitHub token")
	ErrRateLimited        = errors.New("rate limited: too many requests")
	ErrServiceUnavailable = errors.New("service unavailable")
	ErrTokenExpired       = errors.New("copilot token expired")
)

// CachedToken represents a cached Copilot API token.
type CachedToken struct {
	Token     string
	ExpiresAt time.Time
	UpdatedAt time.Time
	BaseURL   string
}

// IsValid checks if the token is still valid with a safety margin.
func (t *CachedToken) IsValid() bool {
	if t == nil {
		return false
	}
	return time.Now().Before(t.ExpiresAt.Add(-TokenRefreshMargin))
}

// proxyEpRegex matches the proxy-ep field in a Copilot token.
// Token format: "...;proxy-ep=proxy.example.com;..."
var proxyEpRegex = regexp.MustCompile(`(?:^|;)\s*proxy-ep=([^;\s]+)`)

// DeriveBaseURLFromToken extracts the proxy-ep field from a Copilot token
// and converts it to an API base URL.
// Reference: OpenClaw src/providers/github-copilot-token.ts#deriveCopilotApiBaseUrlFromToken
func DeriveBaseURLFromToken(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}

	// Match proxy-ep field
	match := proxyEpRegex.FindStringSubmatch(token)
	if len(match) < 2 {
		return ""
	}

	proxyEp := strings.TrimSpace(match[1])
	if proxyEp == "" {
		return ""
	}

	// Remove protocol prefix if present
	host := strings.TrimPrefix(proxyEp, "https://")
	host = strings.TrimPrefix(host, "http://")

	// Convert proxy.* -> api.*
	if strings.HasPrefix(host, "proxy.") {
		host = "api." + strings.TrimPrefix(host, "proxy.")
	}

	if host == "" {
		return ""
	}

	return "https://" + host
}

// tokenResponse represents the response from GitHub's Copilot token endpoint.
type tokenResponse struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"`
	Endpoints struct {
		API string `json:"api"`
	} `json:"endpoints"`
}

// TokenManager manages GitHub Copilot API tokens.
type TokenManager struct {
	githubToken string
	cache       *CachedToken
	mu          sync.Mutex
	httpClient  *http.Client
}

// NewTokenManager creates a new TokenManager with the given GitHub token.
func NewTokenManager(githubToken string) *TokenManager {
	return &TokenManager{
		githubToken: githubToken,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// GetToken returns a valid Copilot API token, refreshing if necessary.
func (tm *TokenManager) GetToken() (string, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Return cached token if still valid
	if tm.cache.IsValid() {
		return tm.cache.Token, nil
	}

	// Fetch new token
	token, err := tm.fetchToken()
	if err != nil {
		return "", err
	}

	return token.Token, nil
}

// fetchToken fetches a new Copilot token from GitHub.
func (tm *TokenManager) fetchToken() (*CachedToken, error) {
	req, err := http.NewRequest(http.MethodGet, GitHubCopilotTokenURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Use Bearer for OAuth tokens (ghu_, gho_), token for PAT (ghp_)
	authPrefix := "token"
	if strings.HasPrefix(tm.githubToken, "ghu_") || strings.HasPrefix(tm.githubToken, "gho_") {
		authPrefix = "Bearer"
	}
	req.Header.Set("Authorization", authPrefix+" "+tm.githubToken)
	req.Header.Set("Accept", "application/json")

	// Set headers to identify as VS Code Copilot client
	req.Header.Set("User-Agent", CopilotUserAgent)
	req.Header.Set("Editor-Version", CopilotEditorVersion)
	req.Header.Set("Editor-Plugin-Version", CopilotEditorPluginVersion)
	req.Header.Set("Copilot-Integration-Id", "vscode-chat")

	resp, err := tm.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch token: %w", err)
	}
	defer resp.Body.Close()

	if err := tm.handleHTTPError(resp); err != nil {
		return nil, err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var tokenResp tokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	baseURL := tokenResp.Endpoints.API
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}

	tm.cache = &CachedToken{
		Token:     tokenResp.Token,
		ExpiresAt: time.Unix(tokenResp.ExpiresAt, 0),
		UpdatedAt: time.Now(),
		BaseURL:   baseURL,
	}

	logger.Debug().
		Time("expires_at", tm.cache.ExpiresAt).
		Str("base_url", baseURL).
		Msg("Copilot token refreshed")

	return tm.cache, nil
}

// handleHTTPError converts HTTP error responses to specific errors.
func (tm *TokenManager) handleHTTPError(resp *http.Response) error {
	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusUnauthorized:
		body, _ := io.ReadAll(resp.Body)
		logger.Debug().
			Int("status", resp.StatusCode).
			Str("body", string(body)).
			Msg("Copilot token request unauthorized")
		return ErrUnauthorized
	case http.StatusForbidden:
		body, _ := io.ReadAll(resp.Body)
		logger.Debug().
			Int("status", resp.StatusCode).
			Str("body", string(body)).
			Msg("Copilot token request forbidden")
		return ErrUnauthorized
	case http.StatusNotFound:
		body, _ := io.ReadAll(resp.Body)
		logger.Debug().
			Int("status", resp.StatusCode).
			Str("body", string(body)).
			Msg("Copilot token endpoint not found")
		return fmt.Errorf("copilot API not accessible (404): ensure you have an active GitHub Copilot subscription")
	case http.StatusTooManyRequests:
		return ErrRateLimited
	case http.StatusServiceUnavailable, http.StatusBadGateway, http.StatusGatewayTimeout:
		return ErrServiceUnavailable
	default:
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}
}

// Invalidate clears the cached token.
func (tm *TokenManager) Invalidate() {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.cache = nil
}

// GetBaseURL returns the API base URL from the cached token.
// Priority: 1. proxy-ep from token, 2. endpoints.api, 3. default
func (tm *TokenManager) GetBaseURL() string {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if tm.cache != nil {
		// Priority 1: Derive from token's proxy-ep field
		if derivedURL := DeriveBaseURLFromToken(tm.cache.Token); derivedURL != "" {
			return derivedURL
		}

		// Priority 2: Use endpoints.api from response
		if tm.cache.BaseURL != "" {
			return tm.cache.BaseURL
		}
	}

	// Priority 3: Default
	return DefaultBaseURL
}

// SetHTTPClient sets a custom HTTP client for testing.
func (tm *TokenManager) SetHTTPClient(client *http.Client) {
	tm.httpClient = client
}

// TokenStatus represents the current status of the token
type TokenStatus struct {
	Valid     bool       `json:"valid"`
	Message   string     `json:"message,omitempty"`
	Warning   string     `json:"warning,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// GetStatus returns the current status of the token without triggering a refresh.
func (tm *TokenManager) GetStatus() TokenStatus {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if tm.cache == nil {
		return TokenStatus{
			Valid:   false,
			Message: "未认证：请配置 GitHub Token",
		}
	}

	now := time.Now()
	expiresAt := tm.cache.ExpiresAt

	// Check if token is expired
	if now.After(expiresAt) {
		return TokenStatus{
			Valid:     false,
			Message:   "Token 已过期",
			ExpiresAt: &expiresAt,
		}
	}

	// Check if token is about to expire
	remaining := time.Until(expiresAt)
	if remaining < TokenRefreshMargin {
		return TokenStatus{
			Valid:     true,
			Warning:   fmt.Sprintf("Token 即将过期（剩余 %s）", remaining.Round(time.Second)),
			ExpiresAt: &expiresAt,
		}
	}

	return TokenStatus{
		Valid:     true,
		ExpiresAt: &expiresAt,
	}
}
