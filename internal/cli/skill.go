package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"mote/internal/skills"
)

// NewSkillCmd creates the skill management command.
func NewSkillCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "Manage skills",
		Long: `Manage Mote skills.

Skills extend Mote's capabilities with custom tools, prompts, and hooks.`,
	}

	cmd.AddCommand(newSkillListCmd())
	cmd.AddCommand(newSkillShowCmd())
	cmd.AddCommand(newSkillActivateCmd())
	cmd.AddCommand(newSkillDeactivateCmd())
	cmd.AddCommand(newSkillReloadCmd())

	return cmd
}

func newSkillListCmd() *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all available skills",
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx := GetCLIContext(cmd)
			if cliCtx == nil {
				return fmt.Errorf("CLI context not initialized")
			}

			homeDir, _ := os.UserHomeDir()
			skillsDir := filepath.Join(homeDir, ".mote", "skills")

			manager := skills.NewManager(skills.ManagerConfig{
				SkillsDir: skillsDir,
			})

			if err := manager.ScanDirectory(skillsDir); err != nil {
				return fmt.Errorf("failed to scan skills: %w", err)
			}

			skillList := manager.ListSkills()

			if jsonOutput {
				data, _ := json.MarshalIndent(skillList, "", "  ")
				fmt.Println(string(data))
				return nil
			}

			if len(skillList) == 0 {
				fmt.Println("No skills found.")
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tNAME\tVERSION\tSTATUS\tTOOLS")
			for _, s := range skillList {
				status := string(s.State)
				toolCount := 0
				if s.Skill != nil {
					toolCount = len(s.Skill.Tools)
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\n", s.Skill.ID, s.Skill.Name, s.Skill.Version, status, toolCount)
			}
			w.Flush()

			return nil
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output in JSON format")
	return cmd
}

func newSkillShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <skill-id>",
		Short: "Show skill details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			skillID := args[0]

			homeDir, _ := os.UserHomeDir()
			skillsDir := filepath.Join(homeDir, ".mote", "skills")

			manager := skills.NewManager(skills.ManagerConfig{
				SkillsDir: skillsDir,
			})

			if err := manager.ScanDirectory(skillsDir); err != nil {
				return fmt.Errorf("failed to scan skills: %w", err)
			}

			skill, found := manager.GetSkill(skillID)
			if !found {
				return fmt.Errorf("skill not found: %s", skillID)
			}

			fmt.Printf("ID: %s\n", skill.ID)
			fmt.Printf("Name: %s\n", skill.Name)
			fmt.Printf("Version: %s\n", skill.Version)
			fmt.Printf("Description: %s\n", skill.Description)
			fmt.Printf("Author: %s\n", skill.Author)
			fmt.Printf("Path: %s\n", skill.FilePath)

			if len(skill.Tools) > 0 {
				fmt.Println("\nTools:")
				for _, t := range skill.Tools {
					fmt.Printf("  - %s: %s\n", t.Name, t.Description)
				}
			}

			if len(skill.Prompts) > 0 {
				fmt.Println("\nPrompts:")
				for _, p := range skill.Prompts {
					fmt.Printf("  - %s\n", p.Name)
				}
			}

			if len(skill.Hooks) > 0 {
				fmt.Println("\nHooks:")
				for _, h := range skill.Hooks {
					fmt.Printf("  - %s (priority: %d)\n", h.Type, h.Priority)
				}
			}

			return nil
		},
	}
}

func newSkillActivateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "activate <skill-id>",
		Short: "Activate a skill",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			skillID := args[0]

			homeDir, _ := os.UserHomeDir()
			skillsDir := filepath.Join(homeDir, ".mote", "skills")

			manager := skills.NewManager(skills.ManagerConfig{
				SkillsDir: skillsDir,
			})

			if err := manager.ScanDirectory(skillsDir); err != nil {
				return fmt.Errorf("failed to scan skills: %w", err)
			}

			if err := manager.Activate(skillID, nil); err != nil {
				return fmt.Errorf("failed to activate skill: %w", err)
			}

			fmt.Printf("Skill '%s' activated successfully.\n", skillID)
			return nil
		},
	}
}

func newSkillDeactivateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "deactivate <skill-id>",
		Short: "Deactivate a skill",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			skillID := args[0]

			homeDir, _ := os.UserHomeDir()
			skillsDir := filepath.Join(homeDir, ".mote", "skills")

			manager := skills.NewManager(skills.ManagerConfig{
				SkillsDir: skillsDir,
			})

			if err := manager.ScanDirectory(skillsDir); err != nil {
				return fmt.Errorf("failed to scan skills: %w", err)
			}

			if err := manager.Deactivate(skillID); err != nil {
				return fmt.Errorf("failed to deactivate skill: %w", err)
			}

			fmt.Printf("Skill '%s' deactivated successfully.\n", skillID)
			return nil
		},
	}
}

func newSkillReloadCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reload",
		Short: "Reload all skills from disk",
		RunE: func(cmd *cobra.Command, args []string) error {
			homeDir, _ := os.UserHomeDir()
			skillsDir := filepath.Join(homeDir, ".mote", "skills")

			manager := skills.NewManager(skills.ManagerConfig{
				SkillsDir: skillsDir,
			})

			if err := manager.ScanDirectory(skillsDir); err != nil {
				return fmt.Errorf("failed to scan skills: %w", err)
			}

			skillList := manager.ListSkills()
			fmt.Printf("Reloaded %d skills.\n", len(skillList))
			return nil
		},
	}
}
