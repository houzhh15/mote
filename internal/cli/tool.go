package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

// NewToolCmd creates the tool command.
func NewToolCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tool",
		Short: "Manage and execute tools",
		Long:  `List available tools, view tool details, and execute tools.`,
	}

	cmd.AddCommand(newToolListCmd())
	cmd.AddCommand(newToolInfoCmd())
	cmd.AddCommand(newToolRunCmd())

	return cmd
}

func newToolListCmd() *cobra.Command {
	var (
		enabled    bool
		category   string
		jsonOutput bool
		serverURL  string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all available tools",
		Long:  `List all tools registered in the Mote agent.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runToolList(serverURL, enabled, category, jsonOutput)
		},
	}

	cmd.Flags().BoolVar(&enabled, "enabled", false, "only show enabled tools")
	cmd.Flags().StringVar(&category, "category", "", "filter by category")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output in JSON format")
	cmd.Flags().StringVar(&serverURL, "url", "http://localhost:18788", "Mote server URL")

	return cmd
}

func newToolInfoCmd() *cobra.Command {
	var serverURL string

	cmd := &cobra.Command{
		Use:   "info <tool-name>",
		Short: "Show tool details",
		Long:  `Display detailed information about a specific tool.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runToolInfo(serverURL, args[0])
		},
	}

	cmd.Flags().StringVar(&serverURL, "url", "http://localhost:18788", "Mote server URL")

	return cmd
}

func newToolRunCmd() *cobra.Command {
	var (
		serverURL string
		argsJSON  string
	)

	cmd := &cobra.Command{
		Use:   "run <tool-name> [--args <json>]",
		Short: "Execute a tool",
		Long: `Execute a tool with the given arguments.

Arguments should be provided as a JSON object using the --args flag.`,
		Example: `  # Execute a simple tool
  mote tool run list_dir --args '{"path": "/home"}'

  # Read a file
  mote tool run read_file --args '{"path": "/etc/hosts"}'`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runToolExecute(serverURL, args[0], argsJSON)
		},
	}

	cmd.Flags().StringVar(&serverURL, "url", "http://localhost:18788", "Mote server URL")
	cmd.Flags().StringVar(&argsJSON, "args", "{}", "tool arguments as JSON")

	return cmd
}

type toolResponse struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Category    string         `json:"category,omitempty"`
	Enabled     bool           `json:"enabled"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type toolExecuteRequest struct {
	Args map[string]any `json:"args"`
}

type toolExecuteResponse struct {
	Content  string         `json:"content"`
	IsError  bool           `json:"is_error"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

func runToolList(serverURL string, enabledOnly bool, category string, jsonOutput bool) error {
	client := &http.Client{Timeout: 30 * time.Second}

	url := fmt.Sprintf("%s/api/v1/tools", serverURL)
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w\nIs the server running? Start it with: mote serve", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	var tools []toolResponse
	if err := json.NewDecoder(resp.Body).Decode(&tools); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	// Filter by enabled status
	if enabledOnly {
		filtered := make([]toolResponse, 0)
		for _, t := range tools {
			if t.Enabled {
				filtered = append(filtered, t)
			}
		}
		tools = filtered
	}

	// Filter by category
	if category != "" {
		filtered := make([]toolResponse, 0)
		for _, t := range tools {
			if strings.EqualFold(t.Category, category) {
				filtered = append(filtered, t)
			}
		}
		tools = filtered
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(tools)
	}

	if len(tools) == 0 {
		fmt.Println("No tools found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tDESCRIPTION\tCATEGORY\tENABLED")
	fmt.Fprintln(w, "----\t-----------\t--------\t-------")

	for _, t := range tools {
		desc := t.Description
		if len(desc) > 50 {
			desc = desc[:50] + "..."
		}

		enabledStr := "✓"
		if !t.Enabled {
			enabledStr = "✗"
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			t.Name,
			desc,
			t.Category,
			enabledStr,
		)
	}
	w.Flush()

	fmt.Printf("\nTotal: %d tools\n", len(tools))

	return nil
}

func runToolInfo(serverURL, toolName string) error {
	client := &http.Client{Timeout: 30 * time.Second}

	url := fmt.Sprintf("%s/api/v1/tools/%s", serverURL, toolName)
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("tool not found: %s", toolName)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	var tool toolResponse
	if err := json.NewDecoder(resp.Body).Decode(&tool); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	// Print tool details
	fmt.Printf("Tool: %s\n", tool.Name)
	fmt.Printf("Description: %s\n", tool.Description)
	if tool.Category != "" {
		fmt.Printf("Category: %s\n", tool.Category)
	}
	fmt.Printf("Enabled: %v\n", tool.Enabled)

	// Print parameters schema
	if len(tool.Parameters) > 0 {
		fmt.Println("\nParameters:")
		paramsJSON, _ := json.MarshalIndent(tool.Parameters, "", "  ")
		fmt.Println(string(paramsJSON))
	}

	return nil
}

func runToolExecute(serverURL, toolName, argsJSON string) error {
	client := &http.Client{Timeout: 60 * time.Second}

	// Parse args JSON
	var args map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return fmt.Errorf("invalid JSON arguments: %w", err)
	}

	// Create request
	reqBody := toolExecuteRequest{Args: args}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/tools/%s/execute", serverURL, toolName)
	resp, err := client.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("tool not found: %s", toolName)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	var result toolExecuteResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	// Print result
	if result.IsError {
		fmt.Printf("Error: %s\n", result.Content)
		return nil
	}

	fmt.Println(result.Content)

	return nil
}
