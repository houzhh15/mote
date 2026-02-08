// Package prompt provides slash command parsing for prompts.
package prompt

import (
	"fmt"
	"strings"
)

// SlashCommand represents a parsed slash command.
type SlashCommand struct {
	Name   string   // Command name (e.g., "prompt", "memory")
	Action string   // Action (e.g., "list", "use", "clear")
	Args   []string // Additional arguments
}

// String returns a string representation of the command.
func (c *SlashCommand) String() string {
	if c.Action == "" {
		return "/" + c.Name
	}
	if len(c.Args) == 0 {
		return fmt.Sprintf("/%s %s", c.Name, c.Action)
	}
	return fmt.Sprintf("/%s %s %s", c.Name, c.Action, strings.Join(c.Args, " "))
}

// SlashCommandParser parses slash commands from input text.
type SlashCommandParser struct {
	commands map[string][]string // command name -> valid actions
}

// NewSlashCommandParser creates a new slash command parser.
func NewSlashCommandParser() *SlashCommandParser {
	return &SlashCommandParser{
		commands: make(map[string][]string),
	}
}

// RegisterCommand registers a command with its valid actions.
func (p *SlashCommandParser) RegisterCommand(name string, actions []string) {
	p.commands[name] = actions
}

// Parse parses a slash command from input text.
// Returns nil if the input is not a valid slash command.
func (p *SlashCommandParser) Parse(input string) *SlashCommand {
	input = strings.TrimSpace(input)
	if !strings.HasPrefix(input, "/") {
		return nil
	}

	// Remove leading slash
	input = input[1:]
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return nil
	}

	name := parts[0]

	// Check if command is registered
	actions, exists := p.commands[name]
	if !exists {
		return nil
	}

	cmd := &SlashCommand{
		Name: name,
	}

	if len(parts) > 1 {
		action := parts[1]
		// Validate action if actions are specified
		if len(actions) > 0 {
			valid := false
			for _, a := range actions {
				if a == action {
					valid = true
					break
				}
			}
			if !valid {
				// Invalid action, treat as first arg
				cmd.Args = parts[1:]
				return cmd
			}
		}
		cmd.Action = action
		if len(parts) > 2 {
			cmd.Args = parts[2:]
		}
	}

	return cmd
}

// IsCommand checks if the input starts with a registered slash command.
func (p *SlashCommandParser) IsCommand(input string) bool {
	input = strings.TrimSpace(input)
	if !strings.HasPrefix(input, "/") {
		return false
	}

	parts := strings.Fields(input[1:])
	if len(parts) == 0 {
		return false
	}

	_, exists := p.commands[parts[0]]
	return exists
}

// BuiltinPromptCommands are the default prompt-related slash commands.
var BuiltinPromptCommands = []string{"list", "use", "clear", "show", "add", "delete"}

// RegisterBuiltinCommands registers the built-in slash commands.
func RegisterBuiltinCommands(parser *SlashCommandParser) {
	// Prompt management commands
	parser.RegisterCommand("prompt", BuiltinPromptCommands)

	// Memory commands
	parser.RegisterCommand("memory", []string{"search", "list", "clear"})

	// Help commands
	parser.RegisterCommand("help", []string{})

	// Clear session
	parser.RegisterCommand("clear", []string{})

	// Status commands
	parser.RegisterCommand("status", []string{"skills", "tools", "memory"})
}

// ExecutePromptCommand executes a prompt slash command.
// Returns the response message and whether the command was handled.
func ExecutePromptCommand(cmd *SlashCommand, store UserPromptStore) (string, bool) {
	if cmd == nil || cmd.Name != "prompt" {
		return "", false
	}

	switch cmd.Action {
	case "list":
		prompts, err := store.List()
		if err != nil {
			return fmt.Sprintf("Error listing prompts: %v", err), true
		}
		if len(prompts) == 0 {
			return "No user prompts defined.", true
		}
		var sb strings.Builder
		sb.WriteString("User Prompts:\n")
		for _, p := range prompts {
			desc := p.Description
			if desc == "" {
				desc = "(no description)"
			}
			sb.WriteString(fmt.Sprintf("- %s: %s\n", p.Name, desc))
		}
		return sb.String(), true

	case "show":
		if len(cmd.Args) == 0 {
			return "Usage: /prompt show <name>", true
		}
		prompt, err := store.Get(cmd.Args[0])
		if err != nil {
			return fmt.Sprintf("Error getting prompt: %v", err), true
		}
		if prompt == nil {
			return fmt.Sprintf("Prompt '%s' not found.", cmd.Args[0]), true
		}
		return fmt.Sprintf("**%s**\n%s\n\nContent:\n```\n%s\n```", prompt.Name, prompt.Description, prompt.Content), true

	case "delete":
		if len(cmd.Args) == 0 {
			return "Usage: /prompt delete <name>", true
		}
		if err := store.Delete(cmd.Args[0]); err != nil {
			return fmt.Sprintf("Error deleting prompt: %v", err), true
		}
		return fmt.Sprintf("Prompt '%s' deleted.", cmd.Args[0]), true

	case "use":
		if len(cmd.Args) == 0 {
			return "Usage: /prompt use <name>", true
		}
		prompt, err := store.Get(cmd.Args[0])
		if err != nil {
			return fmt.Sprintf("Error getting prompt: %v", err), true
		}
		if prompt == nil {
			return fmt.Sprintf("Prompt '%s' not found.", cmd.Args[0]), true
		}
		// The actual activation would be done by the caller
		return fmt.Sprintf("Activated prompt '%s'.", cmd.Args[0]), true

	case "clear":
		return "Session prompt cleared.", true

	default:
		return "Unknown prompt action. Available: list, show, use, delete, clear", true
	}
}
