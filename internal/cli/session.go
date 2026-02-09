package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

// NewSessionCmd creates the session command.
func NewSessionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "session",
		Short: "Manage conversation sessions",
		Long:  `List, view, and delete conversation sessions.`,
	}

	cmd.AddCommand(newSessionListCmd())
	cmd.AddCommand(newSessionShowCmd())
	cmd.AddCommand(newSessionDeleteCmd())
	cmd.AddCommand(newSessionClearCmd())
	cmd.AddCommand(newSessionModelCmd())

	return cmd
}

func newSessionListCmd() *cobra.Command {
	var (
		limit      int
		jsonOutput bool
		serverURL  string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all sessions",
		Long:  `List all conversation sessions.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSessionList(serverURL, limit, jsonOutput)
		},
	}

	cmd.Flags().IntVarP(&limit, "limit", "l", 20, "maximum number of sessions to show")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output in JSON format")
	cmd.Flags().StringVar(&serverURL, "url", "http://localhost:18788", "Mote server URL")

	return cmd
}

func newSessionShowCmd() *cobra.Command {
	var serverURL string

	cmd := &cobra.Command{
		Use:   "show <session-id>",
		Short: "Show session details",
		Long:  `Display detailed information about a specific session.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSessionShow(serverURL, args[0])
		},
	}

	cmd.Flags().StringVar(&serverURL, "url", "http://localhost:18788", "Mote server URL")

	return cmd
}

func newSessionDeleteCmd() *cobra.Command {
	var serverURL string

	cmd := &cobra.Command{
		Use:   "delete <session-id>",
		Short: "Delete a session",
		Long:  `Delete a specific conversation session.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSessionDelete(serverURL, args[0])
		},
	}

	cmd.Flags().StringVar(&serverURL, "url", "http://localhost:18788", "Mote server URL")

	return cmd
}

func newSessionClearCmd() *cobra.Command {
	var (
		force     bool
		serverURL string
	)

	cmd := &cobra.Command{
		Use:   "clear",
		Short: "Clear all sessions",
		Long:  `Delete all conversation sessions.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSessionClear(serverURL, force)
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "skip confirmation prompt")
	cmd.Flags().StringVar(&serverURL, "url", "http://localhost:18788", "Mote server URL")

	return cmd
}

type sessionResponse struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Metadata  any       `json:"metadata,omitempty"`
}

type messageResponse struct {
	ID         string    `json:"id"`
	SessionID  string    `json:"session_id"`
	Role       string    `json:"role"`
	Content    string    `json:"content"`
	ToolCalls  any       `json:"tool_calls,omitempty"`
	ToolCallID string    `json:"tool_call_id,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

func runSessionList(serverURL string, limit int, jsonOutput bool) error {
	client := &http.Client{Timeout: 30 * time.Second}

	url := fmt.Sprintf("%s/api/v1/sessions?limit=%d", serverURL, limit)
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w\nIs the server running? Start it with: mote serve", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	var sessions []sessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&sessions); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(sessions)
	}

	if len(sessions) == 0 {
		fmt.Println("No sessions found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tCREATED\tUPDATED")
	fmt.Fprintln(w, "--\t-------\t-------")

	for _, s := range sessions {
		fmt.Fprintf(w, "%s\t%s\t%s\n",
			s.ID,
			s.CreatedAt.Format("2006-01-02 15:04"),
			s.UpdatedAt.Format("2006-01-02 15:04"),
		)
	}
	w.Flush()

	fmt.Printf("\nTotal: %d sessions\n", len(sessions))

	return nil
}

func runSessionShow(serverURL, sessionID string) error {
	client := &http.Client{Timeout: 30 * time.Second}

	// Get session details
	sessionURL := fmt.Sprintf("%s/api/v1/sessions/%s", serverURL, sessionID)
	resp, err := client.Get(sessionURL)
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	var session sessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return fmt.Errorf("failed to decode session: %w", err)
	}

	// Get messages
	messagesURL := fmt.Sprintf("%s/api/v1/sessions/%s/messages", serverURL, sessionID)
	msgResp, err := client.Get(messagesURL)
	if err != nil {
		return fmt.Errorf("failed to get messages: %w", err)
	}
	defer msgResp.Body.Close()

	var messages []messageResponse
	if msgResp.StatusCode == http.StatusOK {
		_ = json.NewDecoder(msgResp.Body).Decode(&messages)
	}

	// Print session details
	fmt.Printf("Session: %s\n", session.ID)
	fmt.Printf("Created: %s\n", session.CreatedAt.Format(time.RFC3339))
	fmt.Printf("Updated: %s\n", session.UpdatedAt.Format(time.RFC3339))
	fmt.Printf("Messages: %d\n", len(messages))
	fmt.Println()

	// Print messages
	if len(messages) > 0 {
		fmt.Println("Messages:")
		fmt.Println("---------")
		for _, msg := range messages {
			rolePrefix := "👤"
			if msg.Role == "assistant" {
				rolePrefix = "🤖"
			} else if msg.Role == "system" {
				rolePrefix = "⚙️"
			} else if msg.Role == "tool" {
				rolePrefix = "🔧"
			}

			content := msg.Content
			if len(content) > 200 {
				content = content[:200] + "..."
			}

			fmt.Printf("\n%s [%s] %s\n", rolePrefix, msg.Role, msg.CreatedAt.Format("15:04:05"))
			fmt.Println(content)
		}
	}

	return nil
}

func runSessionDelete(serverURL, sessionID string) error {
	client := &http.Client{Timeout: 30 * time.Second}

	req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/api/v1/sessions/%s", serverURL, sessionID), nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("session not found: %s", sessionID)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	fmt.Printf("✓ Session deleted: %s\n", sessionID)
	return nil
}

func runSessionClear(serverURL string, force bool) error {
	if !force {
		fmt.Print("Are you sure you want to delete all sessions? (y/N): ")
		var response string
		_, _ = fmt.Scanln(&response)
		if response != "y" && response != "Y" && response != "yes" {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	// First get all sessions
	client := &http.Client{Timeout: 30 * time.Second}

	resp, err := client.Get(fmt.Sprintf("%s/api/v1/sessions?limit=1000", serverURL))
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w", err)
	}
	defer resp.Body.Close()

	var sessions []sessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&sessions); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if len(sessions) == 0 {
		fmt.Println("No sessions to delete.")
		return nil
	}

	// Delete each session
	deleted := 0
	for _, s := range sessions {
		req, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/api/v1/sessions/%s", serverURL, s.ID), nil)
		delResp, err := client.Do(req)
		if err == nil && (delResp.StatusCode == http.StatusOK || delResp.StatusCode == http.StatusNoContent) {
			deleted++
		}
		if delResp != nil {
			delResp.Body.Close()
		}
	}

	fmt.Printf("✓ Deleted %d sessions\n", deleted)
	return nil
}

// Helper for making HTTP requests (unused but kept for potential future use)
//nolint:unused // Future use
func doHTTPRequest(client *http.Client, method, url string, body interface{}) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(jsonData)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, err
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return client.Do(req)
}

func newSessionModelCmd() *cobra.Command {
	var serverURL string

	cmd := &cobra.Command{
		Use:   "model <session-id> [new-model]",
		Short: "View or set session model",
		Long: `View or set the LLM model for a specific session.

Without a new-model argument, displays the current model.
With a new-model argument, updates the session to use that model.

Example:
  mote session model abc123              # View current model
  mote session model abc123 gpt-4o       # Set model to gpt-4o`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			sessionID := args[0]
			if len(args) == 1 {
				return runSessionModelGet(serverURL, sessionID)
			}
			return runSessionModelSet(serverURL, sessionID, args[1])
		},
	}

	cmd.Flags().StringVar(&serverURL, "url", "http://localhost:18788", "Mote server URL")

	return cmd
}

func runSessionModelGet(serverURL, sessionID string) error {
	client := &http.Client{Timeout: 10 * time.Second}

	resp, err := client.Get(fmt.Sprintf("%s/api/v1/sessions/%s", serverURL, sessionID))
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server error: %s", resp.Status)
	}

	var session struct {
		ID       string `json:"id"`
		Model    string `json:"model"`
		Scenario string `json:"scenario"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	model := session.Model
	if model == "" {
		model = "(default)"
	}
	scenario := session.Scenario
	if scenario == "" {
		scenario = "chat"
	}

	fmt.Printf("Session: %s\n", sessionID)
	fmt.Printf("Model:   %s\n", model)
	fmt.Printf("Scenario: %s\n", scenario)
	return nil
}

func runSessionModelSet(serverURL, sessionID, model string) error {
	client := &http.Client{Timeout: 10 * time.Second}

	body := map[string]string{"model": model}
	jsonData, _ := json.Marshal(body)

	req, err := http.NewRequest(http.MethodPut, fmt.Sprintf("%s/api/v1/sessions/%s/model", serverURL, sessionID), bytes.NewReader(jsonData))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errResp)
		if errResp.Error != "" {
			return fmt.Errorf("failed: %s", errResp.Error)
		}
		return fmt.Errorf("server error: %s", resp.Status)
	}

	fmt.Printf("✓ Session %s model set to: %s\n", sessionID, model)
	return nil
}
