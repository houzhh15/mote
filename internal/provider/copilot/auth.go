// Package copilot provides GitHub Copilot integration for Mote.
package copilot

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"mote/pkg/logger"
)

// OAuth Device Flow constants
const (
	// GitHubDeviceCodeURL is the endpoint for requesting a device code.
	GitHubDeviceCodeURL = "https://github.com/login/device/code"

	// GitHubAccessTokenURL is the endpoint for exchanging device code for access token.
	GitHubAccessTokenURL = "https://github.com/login/oauth/access_token"

	// CopilotClientID is the OAuth client ID for GitHub Copilot.
	// This is the public client ID used by VS Code.
	CopilotClientID = "Iv1.b507a08c87ecfe98"

	// CopilotScope is the OAuth scope required for Copilot access.
	// Must include "copilot" scope to access Copilot APIs.
	CopilotScope = "copilot"

	// DefaultPollInterval is the default polling interval for device flow.
	DefaultPollInterval = 5 * time.Second

	// DefaultDeviceCodeTimeout is the default timeout for device code authentication.
	DefaultDeviceCodeTimeout = 15 * time.Minute
)

// DeviceCodeResponse represents the response from the device code request.
type DeviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// AccessTokenResponse represents the response from the access token request.
type AccessTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
	Error       string `json:"error,omitempty"`
	ErrorDesc   string `json:"error_description,omitempty"`
}

// AuthError represents an authentication error.
type AuthError struct {
	Code    string
	Message string
}

func (e *AuthError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	return e.Code
}

// Common auth errors
var (
	ErrAuthorizationPending = &AuthError{Code: "authorization_pending", Message: "The user has not yet completed authorization"}
	ErrSlowDown             = &AuthError{Code: "slow_down", Message: "Polling too frequently, slow down"}
	ErrExpiredToken         = &AuthError{Code: "expired_token", Message: "The device code has expired"}
	ErrAccessDenied         = &AuthError{Code: "access_denied", Message: "The user denied the authorization request"}
)

// AuthManager handles OAuth Device Flow authentication for GitHub Copilot.
type AuthManager struct {
	httpClient   *http.Client
	clientID     string
	scope        string
	pollInterval time.Duration
	timeout      time.Duration
}

// AuthManagerOption is a functional option for AuthManager.
type AuthManagerOption func(*AuthManager)

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(client *http.Client) AuthManagerOption {
	return func(am *AuthManager) {
		am.httpClient = client
	}
}

// WithClientID sets a custom OAuth client ID.
func WithClientID(clientID string) AuthManagerOption {
	return func(am *AuthManager) {
		am.clientID = clientID
	}
}

// WithScope sets a custom OAuth scope.
func WithScope(scope string) AuthManagerOption {
	return func(am *AuthManager) {
		am.scope = scope
	}
}

// WithPollInterval sets the polling interval for device flow.
func WithPollInterval(interval time.Duration) AuthManagerOption {
	return func(am *AuthManager) {
		am.pollInterval = interval
	}
}

// WithTimeout sets the timeout for device code authentication.
func WithTimeout(timeout time.Duration) AuthManagerOption {
	return func(am *AuthManager) {
		am.timeout = timeout
	}
}

// NewAuthManager creates a new AuthManager with the given options.
func NewAuthManager(opts ...AuthManagerOption) *AuthManager {
	am := &AuthManager{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		clientID:     CopilotClientID,
		scope:        CopilotScope,
		pollInterval: DefaultPollInterval,
		timeout:      DefaultDeviceCodeTimeout,
	}

	for _, opt := range opts {
		opt(am)
	}

	return am
}

// RequestDeviceCode initiates the device authorization flow.
// Returns a DeviceCodeResponse containing the user_code and verification_uri
// that should be displayed to the user.
func (am *AuthManager) RequestDeviceCode(ctx context.Context) (*DeviceCodeResponse, error) {
	data := url.Values{}
	data.Set("client_id", am.clientID)
	data.Set("scope", am.scope)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, GitHubDeviceCodeURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := am.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request device code: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("device code request failed: status %d, body: %s", resp.StatusCode, string(body))
	}

	var dcResp DeviceCodeResponse
	if err := json.Unmarshal(body, &dcResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	logger.Debug().
		Str("user_code", dcResp.UserCode).
		Str("verification_uri", dcResp.VerificationURI).
		Int("expires_in", dcResp.ExpiresIn).
		Msg("Device code received")

	return &dcResp, nil
}

// PollForAccessToken polls for the access token after the user has authorized.
// This should be called after RequestDeviceCode and after displaying the user_code
// to the user. It will block until the user completes authorization, the code expires,
// or the context is cancelled.
func (am *AuthManager) PollForAccessToken(ctx context.Context, deviceCode string) (*AccessTokenResponse, error) {
	pollInterval := am.pollInterval

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(pollInterval):
			token, err := am.exchangeDeviceCode(ctx, deviceCode)
			if err == nil {
				return token, nil
			}

			// Handle specific errors
			var authErr *AuthError
			switch {
			case isAuthError(err, "authorization_pending"):
				// User hasn't completed authorization yet, continue polling
				continue
			case isAuthError(err, "slow_down"):
				// Increase poll interval
				pollInterval += 5 * time.Second
				logger.Debug().
					Dur("new_interval", pollInterval).
					Msg("Slowing down poll interval")
				continue
			case isAuthError(err, "expired_token"):
				return nil, ErrExpiredToken
			case isAuthError(err, "access_denied"):
				return nil, ErrAccessDenied
			default:
				// Check if it's an AuthError
				if authErr != nil {
					return nil, authErr
				}
				return nil, err
			}
		}
	}
}

// exchangeDeviceCode attempts to exchange the device code for an access token.
func (am *AuthManager) exchangeDeviceCode(ctx context.Context, deviceCode string) (*AccessTokenResponse, error) {
	data := url.Values{}
	data.Set("client_id", am.clientID)
	data.Set("device_code", deviceCode)
	data.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, GitHubAccessTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := am.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request access token: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	var tokenResp AccessTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	// Check for OAuth error response
	if tokenResp.Error != "" {
		return nil, &AuthError{
			Code:    tokenResp.Error,
			Message: tokenResp.ErrorDesc,
		}
	}

	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("no access token in response")
	}

	logger.Debug().
		Str("token_type", tokenResp.TokenType).
		Str("scope", tokenResp.Scope).
		Msg("Access token received")

	return &tokenResp, nil
}

// isAuthError checks if the error is an AuthError with the given code.
func isAuthError(err error, code string) bool {
	if authErr, ok := err.(*AuthError); ok {
		return authErr.Code == code
	}
	return false
}

// Authenticate performs the complete OAuth Device Flow authentication.
// It returns the GitHub access token that can be used to obtain a Copilot token.
//
// The onPrompt callback is called with the user_code and verification_uri when
// the user needs to complete authorization. The implementer should display these
// to the user and potentially open the verification URL in a browser.
func (am *AuthManager) Authenticate(ctx context.Context, onPrompt func(userCode, verificationURI string)) (string, error) {
	// Set up timeout context
	ctx, cancel := context.WithTimeout(ctx, am.timeout)
	defer cancel()

	// Request device code
	dcResp, err := am.RequestDeviceCode(ctx)
	if err != nil {
		return "", fmt.Errorf("request device code: %w", err)
	}

	// Notify caller to display user code
	if onPrompt != nil {
		onPrompt(dcResp.UserCode, dcResp.VerificationURI)
	}

	// Set poll interval from server response if available
	if dcResp.Interval > 0 {
		am.pollInterval = time.Duration(dcResp.Interval) * time.Second
	}

	// Poll for access token
	tokenResp, err := am.PollForAccessToken(ctx, dcResp.DeviceCode)
	if err != nil {
		return "", fmt.Errorf("poll for access token: %w", err)
	}

	return tokenResp.AccessToken, nil
}

// ValidateGitHubToken validates a GitHub token by making a request to the user endpoint.
func (am *AuthManager) ValidateGitHubToken(ctx context.Context, token string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user", nil)
	if err != nil {
		return false, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := am.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("validate token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return true, nil
	}

	return false, nil
}
