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

// NewMemoryCmd creates the memory command.
func NewMemoryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "memory",
		Short: "Manage agent memory",
		Long:  `Search, add, and manage agent memories.`,
	}

	cmd.AddCommand(newMemorySearchCmd())
	cmd.AddCommand(newMemoryAddCmd())
	cmd.AddCommand(newMemoryGetCmd())
	cmd.AddCommand(newMemoryListCmd())
	cmd.AddCommand(newMemoryClearCmd())
	// P1 commands
	cmd.AddCommand(newMemorySyncCmd())
	cmd.AddCommand(newMemoryDailyCmd())
	cmd.AddCommand(newMemoryExportCmd())
	cmd.AddCommand(newMemoryLogCmd())

	return cmd
}

func newMemorySearchCmd() *cobra.Command {
	var (
		limit         int
		threshold     float64
		categories    []string
		minImportance float64
		serverURL     string
	)

	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search memories",
		Long:  `Search for memories using semantic similarity.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMemorySearch(serverURL, args[0], limit, threshold, categories, minImportance)
		},
	}

	cmd.Flags().IntVarP(&limit, "limit", "l", 10, "maximum number of results")
	cmd.Flags().Float64VarP(&threshold, "threshold", "t", 0.0, "minimum similarity threshold (0-1)")
	cmd.Flags().StringSliceVar(&categories, "category", nil, "filter by category (preference, fact, decision, entity, other)")
	cmd.Flags().Float64Var(&minImportance, "min-importance", 0.0, "minimum importance threshold (0-1)")
	cmd.Flags().StringVar(&serverURL, "url", "http://localhost:18788", "Mote server URL")

	return cmd
}

func newMemoryAddCmd() *cobra.Command {
	var (
		source     string
		category   string
		importance float64
		serverURL  string
	)

	cmd := &cobra.Command{
		Use:   "add <content>",
		Short: "Add a memory",
		Long:  `Add a new memory entry to the agent's memory store.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMemoryAdd(serverURL, args[0], source, category, importance)
		},
	}

	cmd.Flags().StringVar(&source, "source", "manual", "memory source (manual, conversation, document, tool)")
	cmd.Flags().StringVar(&category, "category", "", "memory category (preference, fact, decision, entity, other; auto-detected if not set)")
	cmd.Flags().Float64Var(&importance, "importance", 0.0, "importance score (0-1; default 0.7)")
	cmd.Flags().StringVar(&serverURL, "url", "http://localhost:18788", "Mote server URL")

	return cmd
}

func newMemoryClearCmd() *cobra.Command {
	var (
		force     bool
		serverURL string
	)

	cmd := &cobra.Command{
		Use:   "clear",
		Short: "Clear all memories",
		Long:  `Delete all memories from the agent's memory store.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMemoryClear(serverURL, force)
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "skip confirmation prompt")
	cmd.Flags().StringVar(&serverURL, "url", "http://localhost:18788", "Mote server URL")

	return cmd
}

type memorySearchRequest struct {
	Query         string   `json:"query"`
	TopK          int      `json:"top_k,omitempty"`
	Threshold     float64  `json:"threshold,omitempty"`
	Categories    []string `json:"categories,omitempty"`
	MinImportance float64  `json:"min_importance,omitempty"`
}

type memorySearchResult struct {
	ID         string    `json:"id"`
	Content    string    `json:"content"`
	Score      float64   `json:"score"`
	Source     string    `json:"source"`
	CreatedAt  time.Time `json:"created_at"`
	Category   string    `json:"category,omitempty"`
	Importance float64   `json:"importance,omitempty"`
}

type memorySearchResponse struct {
	Results []memorySearchResult `json:"results"`
}

type memoryAddRequest struct {
	Content    string  `json:"content"`
	Source     string  `json:"source,omitempty"`
	Category   string  `json:"category,omitempty"`
	Importance float64 `json:"importance,omitempty"`
}

func runMemorySearch(serverURL, query string, limit int, threshold float64, categories []string, minImportance float64) error {
	client := &http.Client{Timeout: 30 * time.Second}

	reqBody := memorySearchRequest{
		Query:         query,
		TopK:          limit,
		Threshold:     threshold,
		Categories:    categories,
		MinImportance: minImportance,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/memory/search", serverURL)
	resp, err := client.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w\nIs the server running? Start it with: mote serve", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	var response memorySearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	results := response.Results
	if len(results) == 0 {
		fmt.Println("No memories found matching your query.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SCORE\tCATEGORY\tIMPORT\tSOURCE\tCONTENT")
	fmt.Fprintln(w, "-----\t--------\t------\t------\t-------")

	for _, r := range results {
		content := r.Content
		if len(content) > 50 {
			content = content[:50] + "..."
		}
		// Replace newlines for table display
		content = strings.ReplaceAll(content, "\n", " ")

		category := r.Category
		if category == "" {
			category = "-"
		}

		fmt.Fprintf(w, "%.3f\t%s\t%.2f\t%s\t%s\n",
			r.Score,
			category,
			r.Importance,
			r.Source,
			content,
		)
	}
	w.Flush()

	fmt.Printf("\nFound %d results\n", len(results))

	return nil
}

func runMemoryAdd(serverURL, content, source, category string, importance float64) error {
	client := &http.Client{Timeout: 30 * time.Second}

	reqBody := memoryAddRequest{
		Content:    content,
		Source:     source,
		Category:   category,
		Importance: importance,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/memory", serverURL)
	resp, err := client.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w\nIs the server running? Start it with: mote serve", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	fmt.Println("✓ Memory added successfully")

	return nil
}

func runMemoryClear(serverURL string, force bool) error {
	if !force {
		fmt.Print("Are you sure you want to delete all memories? (y/N): ")
		var response string
		_, _ = fmt.Scanln(&response)
		if response != "y" && response != "Y" && response != "yes" {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	// Note: The API doesn't have a bulk delete endpoint yet.
	// This is a placeholder - in a real implementation, we might need
	// to add a DELETE /api/v1/memory endpoint or iterate through memories.
	fmt.Println("⚠️  Memory clear functionality requires a bulk delete API endpoint.")
	fmt.Println("   This feature will be available in a future version.")

	return nil
}

func newMemoryGetCmd() *cobra.Command {
	var serverURL string

	cmd := &cobra.Command{
		Use:   "get <id>",
		Short: "Get a memory entry by ID",
		Long:  `Retrieve the full content of a specific memory entry by its ID.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMemoryGet(serverURL, args[0])
		},
	}

	cmd.Flags().StringVar(&serverURL, "url", "http://localhost:18788", "Mote server URL")

	return cmd
}

func newMemoryListCmd() *cobra.Command {
	var (
		limit     int
		serverURL string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List recent memories",
		Long:  `List the most recent memory entries.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMemoryList(serverURL, limit)
		},
	}

	cmd.Flags().IntVarP(&limit, "limit", "l", 10, "maximum number of results")
	cmd.Flags().StringVar(&serverURL, "url", "http://localhost:18788", "Mote server URL")

	return cmd
}

type memoryGetResponse struct {
	ID        string         `json:"id"`
	Content   string         `json:"content"`
	Source    string         `json:"source"`
	CreatedAt string         `json:"created_at"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

func runMemoryGet(serverURL, id string) error {
	client := &http.Client{Timeout: 30 * time.Second}

	url := fmt.Sprintf("%s/api/v1/memory/%s", serverURL, id)
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w\nIs the server running? Start it with: mote serve", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("memory not found: %s", id)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	var memory memoryGetResponse
	if err := json.NewDecoder(resp.Body).Decode(&memory); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	fmt.Printf("ID:         %s\n", memory.ID)
	fmt.Printf("Source:     %s\n", memory.Source)
	fmt.Printf("Created At: %s\n", memory.CreatedAt)
	if len(memory.Metadata) > 0 {
		fmt.Printf("Metadata:   %v\n", memory.Metadata)
	}
	fmt.Printf("\nContent:\n%s\n", memory.Content)

	return nil
}

type memoryListItem struct {
	ID        string  `json:"id"`
	Content   string  `json:"content"`
	Score     float64 `json:"score"`
	Source    string  `json:"source"`
	CreatedAt string  `json:"created_at"`
}

type memoryListResponse struct {
	Memories []memoryListItem `json:"memories"`
}

func runMemoryList(serverURL string, limit int) error {
	client := &http.Client{Timeout: 30 * time.Second}

	url := fmt.Sprintf("%s/api/v1/memory?limit=%d", serverURL, limit)
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w\nIs the server running? Start it with: mote serve", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	var response memoryListResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	if len(response.Memories) == 0 {
		fmt.Println("No memories found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tSOURCE\tCREATED AT\tCONTENT")
	fmt.Fprintln(w, "--\t------\t----------\t-------")

	for _, m := range response.Memories {
		content := m.Content
		if len(content) > 50 {
			content = content[:50] + "..."
		}
		content = strings.ReplaceAll(content, "\n", " ")

		createdAt := m.CreatedAt
		if len(createdAt) > 19 {
			createdAt = createdAt[:19] // Trim timezone for display
		}

		// Truncate ID for display
		idDisplay := m.ID
		if len(idDisplay) > 8 {
			idDisplay = idDisplay[:8] + "..."
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			idDisplay,
			m.Source,
			createdAt,
			content,
		)
	}
	w.Flush()

	fmt.Printf("\nShowing %d memories\n", len(response.Memories))

	return nil
}

// =============================================================================
// P1 Commands: Sync, Daily, Export, Log
// =============================================================================

func newMemorySyncCmd() *cobra.Command {
	var (
		force     bool
		serverURL string
	)

	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Sync memory index from markdown files",
		Long:  `Scan MEMORY.md and daily logs to rebuild the memory index.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMemorySync(serverURL, force)
		},
	}

	cmd.Flags().BoolVarP(&force, "force", "f", false, "force full resync")
	cmd.Flags().StringVar(&serverURL, "url", "http://localhost:18788", "Mote server URL")

	return cmd
}

func runMemorySync(serverURL string, force bool) error {
	client := &http.Client{Timeout: 60 * time.Second}

	reqBody := map[string]bool{"force": force}
	jsonData, _ := json.Marshal(reqBody)

	url := fmt.Sprintf("%s/api/v1/memory/sync", serverURL)
	resp, err := client.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w\nIs the server running? Start it with: mote serve", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	synced, _ := result["synced"].(float64)
	fmt.Printf("✓ Synced %d entries from markdown files\n", int(synced))

	return nil
}

func newMemoryDailyCmd() *cobra.Command {
	var (
		date      string
		serverURL string
	)

	cmd := &cobra.Command{
		Use:   "daily",
		Short: "View daily memory log",
		Long:  `View the memory log for today or a specific date.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMemoryDaily(serverURL, date)
		},
	}

	cmd.Flags().StringVar(&date, "date", "", "date in YYYY-MM-DD format (default: today)")
	cmd.Flags().StringVar(&serverURL, "url", "http://localhost:18788", "Mote server URL")

	return cmd
}

func runMemoryDaily(serverURL, date string) error {
	client := &http.Client{Timeout: 30 * time.Second}

	url := fmt.Sprintf("%s/api/v1/memory/daily", serverURL)
	if date != "" {
		url += "?date=" + date
	}

	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w\nIs the server running? Start it with: mote serve", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}

	dateStr, _ := result["date"].(string)
	content, _ := result["content"].(string)

	if content == "" {
		fmt.Printf("No daily log for %s\n", dateStr)
		return nil
	}

	fmt.Printf("# Daily Log: %s\n\n", dateStr)
	fmt.Println(content)

	return nil
}

func newMemoryExportCmd() *cobra.Command {
	var (
		format    string
		output    string
		serverURL string
	)

	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export all memories",
		Long:  `Export all memories to JSON or Markdown format.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMemoryExport(serverURL, format, output)
		},
	}

	cmd.Flags().StringVar(&format, "format", "json", "output format (json or markdown)")
	cmd.Flags().StringVarP(&output, "output", "o", "", "output file (default: stdout)")
	cmd.Flags().StringVar(&serverURL, "url", "http://localhost:18788", "Mote server URL")

	return cmd
}

func runMemoryExport(serverURL, format, output string) error {
	client := &http.Client{Timeout: 60 * time.Second}

	url := fmt.Sprintf("%s/api/v1/memory/export?format=%s", serverURL, format)
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w\nIs the server running? Start it with: mote serve", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if output != "" {
		if err := os.WriteFile(output, body, 0644); err != nil {
			return fmt.Errorf("failed to write file: %w", err)
		}
		fmt.Printf("✓ Exported to %s\n", output)
		return nil
	}

	fmt.Println(string(body))
	return nil
}

func newMemoryLogCmd() *cobra.Command {
	var (
		section   string
		serverURL string
	)

	cmd := &cobra.Command{
		Use:   "log <content>",
		Short: "Append to today's daily log",
		Long:  `Append a note or memory to today's daily log file.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMemoryLog(serverURL, args[0], section)
		},
	}

	cmd.Flags().StringVar(&section, "section", "", "section header (e.g., 'Notes', 'Tasks')")
	cmd.Flags().StringVar(&serverURL, "url", "http://localhost:18788", "Mote server URL")

	return cmd
}

func runMemoryLog(serverURL, content, section string) error {
	client := &http.Client{Timeout: 30 * time.Second}

	reqBody := map[string]string{
		"content": content,
		"section": section,
	}
	jsonData, _ := json.Marshal(reqBody)

	url := fmt.Sprintf("%s/api/v1/memory/daily", serverURL)
	resp, err := client.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to connect to server: %w\nIs the server running? Start it with: mote serve", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	fmt.Println("✓ Added to daily log")

	return nil
}
