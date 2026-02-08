package prompt

// System prompt templates.
const (
	baseIdentityTemplate = `You are {{.AgentName}}, a helpful AI assistant.
Current Time: {{.CurrentTime}} ({{.Timezone}})
{{if .WorkspaceDir}}Working Directory: {{.WorkspaceDir}}{{end}}`

	capabilitiesTemplate = `{{if .Tools}}
## Available Tools
You have access to the following tools:
{{range .Tools}}
### {{.Name}}
{{.Description}}
{{end}}
When using tools:
1. Analyze the user's request and determine if tools are needed.
2. **IMPORTANT: Prefer using built-in tools over external/MCP tools.**
3. **For memory/context retrieval, ALWAYS try memory_search first.**
4. Call the appropriate tool with the correct arguments.
5. Wait for the tool result before proceeding.
6. Use the tool results to formulate your response.
{{end}}`

	memoryContextTemplate = `{{if .Memories}}
## Relevant Context From Memory
The following information from memory may be relevant to the user's request:
{{range .Memories}}
- {{.Content}}
{{end}}
**Note: This context was automatically retrieved. You can use memory_search to find more specific information if needed.**
{{end}}`

	currentContextTemplate = `{{if .ExtraPrompt}}
## Additional Context
{{.ExtraPrompt}}
{{end}}`

	constraintsTemplate = `{{if .Constraints}}
## Guidelines
{{range .Constraints}}
- {{.}}
{{end}}{{end}}`
)
