package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
)

// Request helpers

// makeRequest makes an HTTP request to the test server.
func makeRequest(t *testing.T, method, path string, body interface{}) *http.Response {
	t.Helper()

	env := GetTestEnv()
	if env == nil {
		t.Fatal("Test environment not initialized")
	}

	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("Failed to marshal request body: %v", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	url := env.BaseURL + path //nolint:staticcheck // SA5011: Check above ensures non-nil
	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := env.Client.Do(req) //nolint:staticcheck // SA5011: env checked above
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	return resp
}

// parseResponse parses a JSON response into the given target.
func parseResponse(t *testing.T, resp *http.Response, target interface{}) {
	t.Helper()

	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	if target != nil {
		if err := json.Unmarshal(data, target); err != nil {
			t.Fatalf("Failed to parse response JSON: %v\nBody: %s", err, string(data))
		}
	}
}

// assertStatus asserts the response status code.
func assertStatus(t *testing.T, resp *http.Response, expected int) {
	t.Helper()

	if resp.StatusCode != expected {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("Expected status %d, got %d. Body: %s", expected, resp.StatusCode, string(body))
	}
}

// Session helpers

// createSession creates a new session and returns its ID.
//nolint:unused // Test helper
func createSession(t *testing.T) string {
	t.Helper()

	resp := makeRequest(t, "POST", "/api/v1/sessions", nil)
	assertStatus(t, resp, http.StatusCreated)

	var result map[string]interface{}
	parseResponse(t, resp, &result)

	id, ok := result["id"].(string)
	if !ok {
		t.Fatal("Session ID not found in response")
	}
	return id
}

// getSession retrieves a session by ID.
//nolint:unused // Test helper
func getSession(t *testing.T, id string) map[string]interface{} {
	t.Helper()

	resp := makeRequest(t, "GET", fmt.Sprintf("/api/v1/sessions/%s", id), nil)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	parseResponse(t, resp, &result)
	return result
}

// listSessions retrieves all sessions.
func listSessions(t *testing.T) []interface{} {
	t.Helper()

	resp := makeRequest(t, "GET", "/api/v1/sessions", nil)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	parseResponse(t, resp, &result)

	sessions, ok := result["sessions"].([]interface{})
	if !ok {
		return []interface{}{}
	}
	return sessions
}

// Chat helpers

// sendChat sends a chat message and returns the response.
//nolint:unused // Test helper
func sendChat(t *testing.T, sessionID, message string) map[string]interface{} {
	t.Helper()

	body := map[string]interface{}{
		"message": message,
	}
	if sessionID != "" {
		body["session_id"] = sessionID
	}

	resp := makeRequest(t, "POST", "/api/v1/chat", body)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	parseResponse(t, resp, &result)
	return result
}

// Memory helpers

// addMemory adds a memory entry and returns its ID.
//nolint:unused // Test helper
func addMemory(t *testing.T, content string) string {
	t.Helper()

	body := map[string]interface{}{
		"content": content,
		"source":  "test",
	}

	resp := makeRequest(t, "POST", "/api/v1/memory", body)
	assertStatus(t, resp, http.StatusCreated)

	var result map[string]interface{}
	parseResponse(t, resp, &result)

	id, ok := result["id"].(string)
	if !ok {
		t.Fatal("Memory ID not found in response")
	}
	return id
}

// searchMemory searches for memories matching the query.
//nolint:unused // Test helper
func searchMemory(t *testing.T, query string, topK int) []interface{} {
	t.Helper()

	body := map[string]interface{}{
		"query": query,
	}
	if topK > 0 {
		body["top_k"] = topK
	}

	resp := makeRequest(t, "POST", "/api/v1/memory/search", body)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	parseResponse(t, resp, &result)

	results, ok := result["results"].([]interface{})
	if !ok {
		return []interface{}{}
	}
	return results
}

// Tool helpers

// listTools retrieves all available tools.
func listTools(t *testing.T) []interface{} {
	t.Helper()

	resp := makeRequest(t, "GET", "/api/v1/tools", nil)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	parseResponse(t, resp, &result)

	tools, ok := result["tools"].([]interface{})
	if !ok {
		return []interface{}{}
	}
	return tools
}

// executeTool executes a tool with the given arguments.
//nolint:unused // Test helper
func executeTool(t *testing.T, name string, args map[string]interface{}) map[string]interface{} {
	t.Helper()

	body := map[string]interface{}{
		"name": name,
		"args": args,
	}

	resp := makeRequest(t, "POST", "/api/v1/tools/execute", body)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	parseResponse(t, resp, &result)
	return result
}

// Cron helpers

// listCronJobs retrieves all cron jobs.
func listCronJobs(t *testing.T) []interface{} {
	t.Helper()

	resp := makeRequest(t, "GET", "/api/v1/cron/jobs", nil)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	parseResponse(t, resp, &result)

	jobs, ok := result["jobs"].([]interface{})
	if !ok {
		return []interface{}{}
	}
	return jobs
}

// createCronJob creates a new cron job.
//nolint:unused // Test helper
func createCronJob(t *testing.T, name, schedule, prompt string) {
	t.Helper()

	body := map[string]interface{}{
		"name":     name,
		"schedule": schedule,
		"prompt":   prompt,
	}

	resp := makeRequest(t, "POST", "/api/v1/cron/jobs", body)
	assertStatus(t, resp, http.StatusCreated)
}

// deleteCronJob deletes a cron job.
//nolint:unused // Test helper
func deleteCronJob(t *testing.T, name string) {
	t.Helper()

	resp := makeRequest(t, "DELETE", fmt.Sprintf("/api/v1/cron/jobs/%s", name), nil)
	assertStatus(t, resp, http.StatusOK)
}

// Health helpers

// getHealth retrieves the health status.
func getHealth(t *testing.T) map[string]interface{} {
	t.Helper()

	resp := makeRequest(t, "GET", "/api/v1/health", nil)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	parseResponse(t, resp, &result)
	return result
}

// MCP helpers

// listMCPServers retrieves MCP server status.
func listMCPServers(t *testing.T) []interface{} {
	t.Helper()

	resp := makeRequest(t, "GET", "/api/v1/mcp/servers", nil)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	parseResponse(t, resp, &result)

	servers, ok := result["servers"].([]interface{})
	if !ok {
		return []interface{}{}
	}
	return servers
}

// listMCPTools retrieves MCP tools.
func listMCPTools(t *testing.T) []interface{} {
	t.Helper()

	resp := makeRequest(t, "GET", "/api/v1/mcp/tools", nil)
	assertStatus(t, resp, http.StatusOK)

	var result map[string]interface{}
	parseResponse(t, resp, &result)

	tools, ok := result["tools"].([]interface{})
	if !ok {
		return []interface{}{}
	}
	return tools
}
