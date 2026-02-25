package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"text/tabwriter"
	"time"

	"mote/internal/runner/delegate"

	"github.com/spf13/cobra"
)

// NewDelegateCmd creates the delegate management command.
func NewDelegateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delegate",
		Short: "Manage delegate agents",
		Long: `Manage Mote delegate agents.

Delegate agents allow the main agent to dispatch tasks to specialized
sub-agents, each with its own model, tools, and system prompt.`,
	}

	cmd.AddCommand(newDelegateListCmd())
	cmd.AddCommand(newDelegateShowCmd())
	cmd.AddCommand(newDelegateHistoryCmd())

	return cmd
}

func newDelegateListCmd() *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all configured delegate agents",
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx := GetCLIContext(cmd)
			if cliCtx == nil {
				return fmt.Errorf("CLI context not initialized")
			}

			agents := cliCtx.Config.Agents
			if len(agents) == 0 {
				fmt.Println("No delegate agents configured.")
				fmt.Println("Add agents to your config.yaml under the 'agents' key.")
				return nil
			}

			if jsonOutput {
				data, _ := json.MarshalIndent(agents, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			// Sort agent names for consistent output
			names := make([]string, 0, len(agents))
			for name := range agents {
				names = append(names, name)
			}
			sort.Strings(names)

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tMODEL\tMAX_DEPTH\tTIMEOUT\tTOOLS\tDESCRIPTION")
			for _, name := range names {
				cfg := agents[name]
				model := cfg.Model
				if model == "" {
					model = "(default)"
				}
				timeout := cfg.Timeout
				if timeout == "" {
					timeout = "5m"
				}
				toolCount := len(cfg.Tools)
				desc := cfg.Description
				if len(desc) > 50 {
					desc = desc[:47] + "..."
				}
				fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%d\t%s\n",
					name, model, cfg.GetMaxDepth(), timeout, toolCount, desc)
			}
			w.Flush()

			// Delegate config summary
			delegate := cliCtx.Config.Delegate
			fmt.Printf("\nDelegate: enabled=%v, global_max_depth=%d, default_timeout=%s\n",
				delegate.Enabled, delegate.GetMaxDepth(), delegate.GetDefaultTimeout())

			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output in JSON format")
	return cmd
}

func newDelegateShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <agent-name>",
		Short: "Show details of a delegate agent",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx := GetCLIContext(cmd)
			if cliCtx == nil {
				return fmt.Errorf("CLI context not initialized")
			}

			agentName := args[0]
			agents := cliCtx.Config.Agents
			cfg, ok := agents[agentName]
			if !ok {
				return fmt.Errorf("agent not found: %s", agentName)
			}

			fmt.Printf("Name:          %s\n", agentName)
			fmt.Printf("Description:   %s\n", cfg.Description)
			fmt.Printf("Provider:      %s\n", valueOrDefault(cfg.Provider, "(default)"))
			fmt.Printf("Model:         %s\n", valueOrDefault(cfg.Model, "(default)"))
			fmt.Printf("Max Depth:     %d\n", cfg.GetMaxDepth())
			fmt.Printf("Timeout:       %s\n", cfg.GetTimeout())
			fmt.Printf("Max Iterations:%d\n", cfg.MaxIterations)
			if cfg.Temperature > 0 {
				fmt.Printf("Temperature:   %.2f\n", cfg.Temperature)
			}

			if cfg.SystemPrompt != "" {
				fmt.Printf("\nSystem Prompt:\n  %s\n", cfg.SystemPrompt)
			}

			if len(cfg.Tools) > 0 {
				fmt.Println("\nTools:")
				for _, t := range cfg.Tools {
					fmt.Printf("  - %s\n", t)
				}
			} else {
				fmt.Println("\nTools: (inherits all from parent)")
			}

			return nil
		},
	}
}

func valueOrDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func newDelegateHistoryCmd() *cobra.Command {
	var (
		sessionID  string
		jsonOutput bool
		limit      int
	)

	cmd := &cobra.Command{
		Use:   "history",
		Short: "Show delegation execution history",
		Long:  `View delegation execution history for a specific session or recent sessions.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx := GetCLIContext(cmd)
			if cliCtx == nil {
				return fmt.Errorf("CLI context not initialized")
			}

			db, err := cliCtx.GetStorage()
			if err != nil {
				return fmt.Errorf("open storage: %w", err)
			}

			tracker := delegate.NewTracker(db.DB)

			if sessionID == "" {
				fmt.Println("Use --session to specify a session ID.")
				fmt.Println("Example: mote delegate history --session <session-id>")
				return nil
			}

			records, err := tracker.GetByParentSession(sessionID)
			if err != nil {
				return fmt.Errorf("query delegations: %w", err)
			}

			if len(records) == 0 {
				fmt.Println("No delegation records found for this session.")
				return nil
			}

			// Apply limit
			if limit > 0 && len(records) > limit {
				records = records[:limit]
			}

			if jsonOutput {
				data, _ := json.MarshalIndent(records, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "AGENT\tSTATUS\tDEPTH\tDURATION\tTOKENS\tSTARTED")
			for _, r := range records {
				duration := "running"
				if r.CompletedAt != nil {
					d := r.CompletedAt.Sub(r.StartedAt)
					duration = d.Round(time.Millisecond).String()
				}
				fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%d\t%s\n",
					r.AgentName, r.Status, r.Depth, duration,
					r.TokensUsed, r.StartedAt.Format("2006-01-02 15:04:05"))
			}
			w.Flush()

			fmt.Printf("\nTotal: %d delegation(s)\n", len(records))
			return nil
		},
	}

	cmd.Flags().StringVar(&sessionID, "session", "", "session ID to query delegations for")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output in JSON format")
	cmd.Flags().IntVar(&limit, "limit", 50, "maximum number of records to show")

	return cmd
}
