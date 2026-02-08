package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"mote/internal/prompts"
)

// NewPromptCmd creates the prompt management command.
func NewPromptCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "prompt",
		Short: "Manage prompts",
		Long: `Manage Mote prompts.

Prompts extend the agent's system prompt with custom instructions and context.`,
	}

	cmd.AddCommand(newPromptListCmd())
	cmd.AddCommand(newPromptShowCmd())
	cmd.AddCommand(newPromptAddCmd())
	cmd.AddCommand(newPromptRemoveCmd())
	cmd.AddCommand(newPromptEnableCmd())
	cmd.AddCommand(newPromptDisableCmd())

	return cmd
}

func newPromptListCmd() *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all prompts",
		RunE: func(cmd *cobra.Command, args []string) error {
			manager := prompts.NewManager()

			promptList := manager.ListPrompts()

			if jsonOutput {
				data, _ := json.MarshalIndent(promptList, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			if len(promptList) == 0 {
				fmt.Println("No prompts found.")
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tNAME\tTYPE\tENABLED\tPRIORITY")
			for _, p := range promptList {
				enabled := "no"
				if p.Enabled {
					enabled = "yes"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\n", p.ID, p.Name, p.Type, enabled, p.Priority)
			}
			w.Flush()

			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output in JSON format")
	return cmd
}

func newPromptShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <prompt-id>",
		Short: "Show prompt details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			promptID := args[0]

			manager := prompts.NewManager()

			prompt, err := manager.GetPrompt(promptID)
			if err != nil {
				return fmt.Errorf("prompt not found: %s", promptID)
			}

			fmt.Printf("ID: %s\n", prompt.ID)
			fmt.Printf("Name: %s\n", prompt.Name)
			fmt.Printf("Type: %s\n", prompt.Type)
			fmt.Printf("Priority: %d\n", prompt.Priority)
			fmt.Printf("Enabled: %v\n", prompt.Enabled)
			fmt.Println("\nContent:")
			fmt.Println("---")
			fmt.Println(prompt.Content)
			fmt.Println("---")

			return nil
		},
	}
}

func newPromptAddCmd() *cobra.Command {
	var (
		name       string
		promptType string
		priority   int
		content    string
	)

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a new prompt",
		RunE: func(cmd *cobra.Command, args []string) error {
			if name == "" {
				return fmt.Errorf("name is required")
			}
			if content == "" {
				return fmt.Errorf("content is required")
			}

			manager := prompts.NewManager()

			prompt, err := manager.AddPrompt(prompts.PromptConfig{
				Name:     name,
				Type:     prompts.PromptType(promptType),
				Content:  content,
				Priority: priority,
				Enabled:  true,
			})
			if err != nil {
				return fmt.Errorf("failed to add prompt: %w", err)
			}

			fmt.Printf("Prompt '%s' added successfully (ID: %s).\n", prompt.Name, prompt.ID)
			return nil
		},
	}

	cmd.Flags().StringVarP(&name, "name", "n", "", "prompt name (required)")
	cmd.Flags().StringVarP(&promptType, "type", "t", "system", "prompt type (system, user, assistant)")
	cmd.Flags().IntVarP(&priority, "priority", "p", 0, "prompt priority (higher = later)")
	cmd.Flags().StringVarP(&content, "content", "c", "", "prompt content (required)")

	return cmd
}

func newPromptRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <prompt-id>",
		Short: "Remove a prompt",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			promptID := args[0]

			manager := prompts.NewManager()

			if err := manager.RemovePrompt(promptID); err != nil {
				return fmt.Errorf("failed to remove prompt: %w", err)
			}

			fmt.Printf("Prompt '%s' removed successfully.\n", promptID)
			return nil
		},
	}
}

func newPromptEnableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "enable <prompt-id>",
		Short: "Enable a prompt",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			promptID := args[0]

			manager := prompts.NewManager()

			if err := manager.EnablePrompt(promptID); err != nil {
				return fmt.Errorf("failed to enable prompt: %w", err)
			}

			fmt.Printf("Prompt '%s' enabled.\n", promptID)
			return nil
		},
	}
}

func newPromptDisableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "disable <prompt-id>",
		Short: "Disable a prompt",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			promptID := args[0]

			manager := prompts.NewManager()

			if err := manager.DisablePrompt(promptID); err != nil {
				return fmt.Errorf("failed to disable prompt: %w", err)
			}

			fmt.Printf("Prompt '%s' disabled.\n", promptID)
			return nil
		},
	}
}
