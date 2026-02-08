package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"mote/internal/workspace"
)

// NewWorkspaceCmd creates the workspace management command.
func NewWorkspaceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "workspace",
		Short: "Manage workspaces",
		Long: `Manage Mote workspaces.

Workspaces bind file system paths with context for tool execution.`,
	}

	cmd.AddCommand(newWorkspaceListCmd())
	cmd.AddCommand(newWorkspaceBindCmd())
	cmd.AddCommand(newWorkspaceUnbindCmd())
	cmd.AddCommand(newWorkspaceShowCmd())

	return cmd
}

func newWorkspaceListCmd() *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all bound workspaces",
		RunE: func(cmd *cobra.Command, args []string) error {
			manager := workspace.NewWorkspaceManager()

			workspaces, err := manager.ListWorkspaces()
			if err != nil {
				return fmt.Errorf("failed to list workspaces: %w", err)
			}

			if jsonOutput {
				data, _ := json.MarshalIndent(workspaces, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			if len(workspaces) == 0 {
				fmt.Println("No workspaces bound.")
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tNAME\tPATH\tACTIVE")
			for _, ws := range workspaces {
				active := "no"
				if ws.Active {
					active = "yes"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", ws.ID, ws.Name, ws.Path, active)
			}
			w.Flush()

			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output in JSON format")
	return cmd
}

func newWorkspaceBindCmd() *cobra.Command {
	var name string

	cmd := &cobra.Command{
		Use:   "bind <path>",
		Short: "Bind a workspace path",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			wsPath := args[0]
			if name == "" {
				name = wsPath // Use path as name if not provided
			}

			manager := workspace.NewWorkspaceManager()

			ws, err := manager.BindWorkspace(name, wsPath)
			if err != nil {
				return fmt.Errorf("failed to bind workspace: %w", err)
			}

			fmt.Printf("Workspace '%s' bound successfully (ID: %s).\n", ws.Name, ws.ID)
			return nil
		},
	}

	cmd.Flags().StringVarP(&name, "name", "n", "", "workspace name (defaults to path)")
	return cmd
}

func newWorkspaceUnbindCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unbind <workspace-id>",
		Short: "Unbind a workspace",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			wsID := args[0]

			manager := workspace.NewWorkspaceManager()

			if err := manager.UnbindWorkspace(wsID); err != nil {
				return fmt.Errorf("failed to unbind workspace: %w", err)
			}

			fmt.Printf("Workspace '%s' unbound successfully.\n", wsID)
			return nil
		},
	}
}

func newWorkspaceShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <workspace-id>",
		Short: "Show workspace details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			wsID := args[0]

			manager := workspace.NewWorkspaceManager()

			ws, err := manager.GetWorkspace(wsID)
			if err != nil {
				return fmt.Errorf("workspace not found: %s", wsID)
			}

			fmt.Printf("ID: %s\n", ws.ID)
			fmt.Printf("Name: %s\n", ws.Name)
			fmt.Printf("Path: %s\n", ws.Path)
			fmt.Printf("Active: %v\n", ws.Active)
			fmt.Printf("Created: %s\n", ws.CreatedAt.Format("2006-01-02 15:04:05"))
			fmt.Printf("Updated: %s\n", ws.UpdatedAt.Format("2006-01-02 15:04:05"))

			if len(ws.Metadata) > 0 {
				fmt.Println("\nMetadata:")
				for k, v := range ws.Metadata {
					fmt.Printf("  %s: %v\n", k, v)
				}
			}

			return nil
		},
	}
}
