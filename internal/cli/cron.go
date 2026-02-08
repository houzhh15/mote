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

// NewCronCmd creates the cron command.
func NewCronCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cron",
		Short: "Manage scheduled tasks",
		Long:  `List, create, and manage scheduled cron jobs.`,
	}

	cmd.AddCommand(newCronListCmd())
	cmd.AddCommand(newCronAddCmd())
	cmd.AddCommand(newCronRemoveCmd())
	cmd.AddCommand(newCronRunCmd())

	return cmd
}

func newCronListCmd() *cobra.Command {
	var (
		jsonOutput bool
		serverURL  string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all cron jobs",
		Long:  `List all scheduled cron jobs.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCronList(serverURL, jsonOutput)
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output in JSON format")
	cmd.Flags().StringVar(&serverURL, "url", "http://localhost:18788", "Mote server URL")

	return cmd
}

func newCronAddCmd() *cobra.Command {
	var (
		jobType    string
		message    string
		toolName   string
		webhookURL string
		disabled   bool
		serverURL  string
	)

	cmd := &cobra.Command{
		Use:   "add <name> <schedule>",
		Short: "Add a cron job",
		Long: `Add a new scheduled cron job.

Schedule format follows standard cron syntax:
  ┌───────────── minute (0 - 59)
  │ ┌───────────── hour (0 - 23)
  │ │ ┌───────────── day of month (1 - 31)
  │ │ │ ┌───────────── month (1 - 12)
  │ │ │ │ ┌───────────── day of week (0 - 6)
  │ │ │ │ │
  * * * * *

Job types:
  - prompt: Send a message to the agent
  - tool: Execute a registered tool
  - script: Run a JavaScript script`,
		Example: `  # Run a prompt every day at 9 AM
  mote cron add daily_summary "0 9 * * *" --type prompt --message "Summarize yesterday's work"

  # Run a tool every 5 minutes
  mote cron add health_check "*/5 * * * *" --type tool --tool health_check

  # Run hourly (disabled by default)
  mote cron add hourly_task "0 * * * *" --type prompt --message "Check status" --disabled`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCronAdd(serverURL, args[0], args[1], jobType, message, toolName, webhookURL, disabled)
		},
	}

	cmd.Flags().StringVar(&jobType, "type", "prompt", "job type (prompt, tool, script)")
	cmd.Flags().StringVar(&message, "message", "", "message for prompt type")
	cmd.Flags().StringVar(&toolName, "tool", "", "tool name for tool type")
	cmd.Flags().StringVar(&webhookURL, "webhook", "", "webhook URL (deprecated)")
	cmd.Flags().BoolVar(&disabled, "disabled", false, "create job in disabled state")
	cmd.Flags().StringVar(&serverURL, "url", "http://localhost:18788", "Mote server URL")

	return cmd
}

func newCronRemoveCmd() *cobra.Command {
	var serverURL string

	cmd := &cobra.Command{
		Use:     "remove <name>",
		Aliases: []string{"delete", "rm"},
		Short:   "Remove a cron job",
		Long:    `Remove a scheduled cron job.`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCronRemove(serverURL, args[0])
		},
	}

	cmd.Flags().StringVar(&serverURL, "url", "http://localhost:18788", "Mote server URL")

	return cmd
}

func newCronRunCmd() *cobra.Command {
	var serverURL string

	cmd := &cobra.Command{
		Use:   "run <name>",
		Short: "Run a cron job immediately",
		Long:  `Trigger a cron job to run immediately.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCronRun(serverURL, args[0])
		},
	}

	cmd.Flags().StringVar(&serverURL, "url", "http://localhost:18788", "Mote server URL")

	return cmd
}

type cronJobResponse struct {
	Name      string     `json:"name"`
	Schedule  string     `json:"schedule"`
	Type      string     `json:"type"`
	Payload   string     `json:"payload"`
	Enabled   bool       `json:"enabled"`
	LastRun   *time.Time `json:"last_run,omitempty"`
	NextRun   *time.Time `json:"next_run,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

type cronJobCreateRequest struct {
	Name     string `json:"name"`
	Schedule string `json:"schedule"`
	Type     string `json:"type"`
	Payload  string `json:"payload"`
	Enabled  bool   `json:"enabled"`
}

// cronJobsListResponse wraps the list response from the API.
type cronJobsListResponse struct {
	Jobs []cronJobResponse `json:"jobs"`
}

func runCronList(serverURL string, jsonOutput bool) error {
	client := &http.Client{Timeout: 30 * time.Second}

	url := fmt.Sprintf("%s/api/v1/cron/jobs", serverURL)
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w\nIs the server running? Start it with: mote serve", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	var response cronJobsListResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}
	jobs := response.Jobs

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(jobs)
	}

	if len(jobs) == 0 {
		fmt.Println("No cron jobs found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tSCHEDULE\tTYPE\tENABLED\tLAST RUN\tNEXT RUN")
	fmt.Fprintln(w, "----\t--------\t----\t-------\t--------\t--------")

	for _, j := range jobs {
		enabledStr := "✓"
		if !j.Enabled {
			enabledStr = "✗"
		}

		lastRun := "-"
		if j.LastRun != nil {
			lastRun = j.LastRun.Format("01-02 15:04")
		}

		nextRun := "-"
		if j.NextRun != nil {
			nextRun = j.NextRun.Format("01-02 15:04")
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			j.Name,
			j.Schedule,
			j.Type,
			enabledStr,
			lastRun,
			nextRun,
		)
	}
	w.Flush()

	fmt.Printf("\nTotal: %d jobs\n", len(jobs))

	return nil
}

func runCronAdd(serverURL, name, schedule, jobType, message, toolName, webhookURL string, disabled bool) error {
	client := &http.Client{Timeout: 30 * time.Second}

	// Build payload based on job type
	var payload string
	switch jobType {
	case "prompt":
		if message == "" {
			return fmt.Errorf("--message is required for prompt type")
		}
		payloadData, _ := json.Marshal(map[string]string{"message": message})
		payload = string(payloadData)
	case "tool":
		if toolName == "" {
			return fmt.Errorf("--tool is required for tool type")
		}
		payloadData, _ := json.Marshal(map[string]string{"tool": toolName})
		payload = string(payloadData)
	case "script":
		// For script type, payload would be the script content
		// For now, we'll just use an empty payload
		payload = "{}"
	default:
		return fmt.Errorf("invalid job type: %s (must be prompt, tool, or script)", jobType)
	}

	reqBody := cronJobCreateRequest{
		Name:     name,
		Schedule: schedule,
		Type:     jobType,
		Payload:  payload,
		Enabled:  !disabled,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/cron/jobs", serverURL)
	resp, err := client.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	var job cronJobResponse
	if err := json.NewDecoder(resp.Body).Decode(&job); err != nil {
		// Job was created even if we can't decode the response
		fmt.Printf("✓ Cron job '%s' created\n", name)
		return nil
	}

	fmt.Printf("✓ Cron job '%s' created\n", job.Name)
	fmt.Printf("  Schedule: %s\n", job.Schedule)
	fmt.Printf("  Type: %s\n", job.Type)
	fmt.Printf("  Enabled: %v\n", job.Enabled)

	return nil
}

func runCronRemove(serverURL, name string) error {
	client := &http.Client{Timeout: 30 * time.Second}

	req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/api/v1/cron/jobs/%s", serverURL, name), nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("cron job not found: %s", name)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	fmt.Printf("✓ Cron job '%s' removed\n", name)
	return nil
}

func runCronRun(serverURL, name string) error {
	client := &http.Client{Timeout: 60 * time.Second}

	url := fmt.Sprintf("%s/api/v1/cron/jobs/%s/run", serverURL, name)
	resp, err := client.Post(url, "application/json", nil)
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("cron job not found: %s", name)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	fmt.Printf("✓ Cron job '%s' triggered\n", name)

	// Try to decode and show the result
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err == nil {
		if status, ok := result["status"].(string); ok {
			fmt.Printf("  Status: %s\n", status)
		}
	}

	return nil
}
