// Package backend provides Go bindings for the Wails frontend.
package backend

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// APIClient provides unified HTTP API calling capability.
type APIClient struct {
	baseURL    string
	httpClient *http.Client
	timeout    time.Duration
}

// NewAPIClient creates an API client instance.
func NewAPIClient(baseURL string, timeout time.Duration) *APIClient {
	return &APIClient{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: timeout,
		},
		timeout: timeout,
	}
}

// CallAPIWithBody executes an HTTP API call with interface{} body (for internal use).
// method: HTTP method (GET, POST, PUT, DELETE, PATCH)
// path: API path (without /api/v1 prefix, e.g., "/chat")
// body: request body (can be nil)
// Returns: JSON response as byte array
func (c *APIClient) CallAPIWithBody(method, path string, body interface{}) ([]byte, error) {
	// Build full URL
	url := c.buildURL(path)

	// Prepare request body
	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(jsonData)
	}

	// Create request
	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAPIRequestFailed, err)
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check for error status codes
	if resp.StatusCode >= 400 {
		apiErr := &APIError{
			StatusCode: resp.StatusCode,
			Code:       http.StatusText(resp.StatusCode),
			Message:    string(respBody),
		}
		// Try to parse error response
		var errResp struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Details any    `json:"details"`
		}
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Message != "" {
			apiErr.Code = errResp.Code
			apiErr.Message = errResp.Message
			apiErr.Details = errResp.Details
		}
		return nil, apiErr
	}

	return respBody, nil
}

// CallAPI executes an HTTP API call (exposed to Wails frontend).
// This is the unified entry point for all frontend API calls.
// method: HTTP method (GET, POST, PUT, DELETE, PATCH)
// path: API path (e.g., "/api/v1/sessions")
// bodyJSON: request body as JSON string (empty string for no body)
// Returns: JSON response as byte array
func (c *APIClient) CallAPI(method, path, bodyJSON string) ([]byte, error) {
	var body interface{}
	if bodyJSON != "" {
		if err := json.Unmarshal([]byte(bodyJSON), &body); err != nil {
			return nil, fmt.Errorf("failed to parse request body: %w", err)
		}
	}
	return c.CallAPIWithBody(method, path, body)
}

// CallAPITyped executes an HTTP API call and parses the response into the specified type.
func (c *APIClient) CallAPITyped(method, path string, body, result interface{}) error {
	respBody, err := c.CallAPIWithBody(method, path, body)
	if err != nil {
		return err
	}

	if result != nil {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("%w: %v", ErrAPIResponseInvalid, err)
		}
	}

	return nil
}

// Get executes a GET request.
func (c *APIClient) Get(path string) ([]byte, error) {
	return c.CallAPIWithBody(http.MethodGet, path, nil)
}

// Post executes a POST request.
func (c *APIClient) Post(path string, body interface{}) ([]byte, error) {
	return c.CallAPIWithBody(http.MethodPost, path, body)
}

// Put executes a PUT request.
func (c *APIClient) Put(path string, body interface{}) ([]byte, error) {
	return c.CallAPIWithBody(http.MethodPut, path, body)
}

// Delete executes a DELETE request.
func (c *APIClient) Delete(path string) ([]byte, error) {
	return c.CallAPIWithBody(http.MethodDelete, path, nil)
}

// Patch executes a PATCH request.
func (c *APIClient) Patch(path string, body interface{}) ([]byte, error) {
	return c.CallAPIWithBody(http.MethodPatch, path, body)
}

// buildURL builds the full URL for an API path.
func (c *APIClient) buildURL(path string) string {
	// Ensure path starts with /
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	// Add /api/v1 prefix if not present
	if !strings.HasPrefix(path, "/api/") {
		path = "/api/v1" + path
	}
	return c.baseURL + path
}

// SetTimeout sets the HTTP client timeout.
func (c *APIClient) SetTimeout(timeout time.Duration) {
	c.timeout = timeout
	c.httpClient.Timeout = timeout
}

// GetBaseURL returns the base URL.
func (c *APIClient) GetBaseURL() string {
	return c.baseURL
}
