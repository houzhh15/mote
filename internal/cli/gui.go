package cli

import (
	"github.com/spf13/cobra"
)

// GuiCommand represents the gui subcommand
type GuiCommand struct {
	RunGUI func()
}

// Command returns the cobra command for gui
func (g *GuiCommand) Command() *cobra.Command {
	return &cobra.Command{
		Use:   "gui",
		Short: "Launch the graphical user interface",
		Long:  `Launch Mote's graphical user interface (GUI) application.`,
		Run: func(cmd *cobra.Command, args []string) {
			if g.RunGUI != nil {
				g.RunGUI()
			}
		},
	}
}
