package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"mote/internal/provider/copilot"
)

// NewModeCmd creates the mode command.
func NewModeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mode",
		Short: "Manage agent mode",
		Long: `Manage the Copilot agent mode.

Mote supports four agent modes:
  ask    - Simple Q&A mode for answering questions
  edit   - Code editing mode for focused modifications
  agent  - Autonomous mode for complex multi-step tasks (default)
  plan   - Planning mode for generating execution plans`,
	}

	cmd.AddCommand(newModeGetCmd())
	cmd.AddCommand(newModeSetCmd())
	cmd.AddCommand(newModeListCmd())

	return cmd
}

func newModeGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get",
		Short: "Get current agent mode",
		Long:  `Display the current agent mode.`,
		RunE:  runModeGet,
	}
}

func newModeSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <mode>",
		Short: "Set agent mode",
		Long: `Set the agent mode.

Available modes:
  ask    - Simple Q&A mode for answering questions
  edit   - Code editing mode for focused modifications
  agent  - Autonomous mode for complex multi-step tasks
  plan   - Planning mode for generating execution plans`,
		Example: `  # Set to agent mode (default)
  mote mode set agent

  # Set to ask mode for simple questions
  mote mode set ask

  # Set to edit mode for code editing
  mote mode set edit`,
		Args: cobra.ExactArgs(1),
		RunE: runModeSet,
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) > 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			modes := []string{"ask", "edit", "agent", "plan"}
			return modes, cobra.ShellCompDirectiveNoFileComp
		},
	}
}

func newModeListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List available modes",
		Long:  `Display all available agent modes with descriptions.`,
		RunE:  runModeList,
	}
}

func runModeGet(cmd *cobra.Command, args []string) error {
	cliCtx := GetCLIContext(cmd)
	if cliCtx == nil {
		return fmt.Errorf("CLI context not initialized")
	}

	mm := copilot.NewModeManager()
	info := mm.GetModeInfo()

	fmt.Println("Current Mode")
	fmt.Println("------------")
	fmt.Printf("Mode:        %s\n", info.Mode)
	fmt.Printf("Display:     %s\n", info.DisplayName)
	fmt.Printf("Description: %s\n", info.Description)

	if info.IsDefault {
		fmt.Println("             (default)")
	}

	return nil
}

func runModeSet(cmd *cobra.Command, args []string) error {
	cliCtx := GetCLIContext(cmd)
	if cliCtx == nil {
		return fmt.Errorf("CLI context not initialized")
	}

	modeStr := strings.ToLower(strings.TrimSpace(args[0]))

	mode, err := copilot.ParseMode(modeStr)
	if err != nil {
		return fmt.Errorf("invalid mode '%s': must be one of ask, edit, agent, plan", modeStr)
	}

	mm := copilot.NewModeManager()
	if err := mm.SetMode(mode); err != nil {
		return fmt.Errorf("failed to set mode: %w", err)
	}

	fmt.Printf("✓ Mode set to: %s\n", mode)

	// Show mode info
	info := mm.GetModeInfo()
	fmt.Printf("  %s\n", info.Description)

	return nil
}

func runModeList(cmd *cobra.Command, args []string) error {
	cliCtx := GetCLIContext(cmd)
	if cliCtx == nil {
		return fmt.Errorf("CLI context not initialized")
	}

	mm := copilot.NewModeManager()
	currentMode := mm.GetMode()
	modes := mm.ListModes()

	fmt.Println("Available Modes")
	fmt.Println("---------------")
	fmt.Println("")

	for _, info := range modes {
		marker := "  "
		if info.Mode == currentMode {
			marker = "→ "
		}

		defaultMarker := ""
		if info.IsDefault {
			defaultMarker = " (default)"
		}

		fmt.Printf("%s%-8s %s%s\n", marker, info.Mode, info.DisplayName, defaultMarker)
		fmt.Printf("           %s\n", info.Description)
		fmt.Println("")
	}

	return nil
}
