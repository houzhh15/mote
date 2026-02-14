package provider

import "encoding/json"

// SanitizeMessages cleans up tool call pairs in the message history.
// It removes tool calls with invalid JSON arguments and their corresponding
// tool result messages, preventing provider API errors from corrupted data
// (e.g., truncated arguments due to streaming issues).
func SanitizeMessages(messages []Message) []Message {
	if len(messages) == 0 {
		return messages
	}

	// First pass: collect valid tool call IDs and fix assistant messages.
	validToolCallIDs := make(map[string]bool)
	cleaned := make([]Message, 0, len(messages))

	for _, msg := range messages {
		if msg.Role == RoleAssistant && len(msg.ToolCalls) > 0 {
			var validCalls []ToolCall
			for _, tc := range msg.ToolCalls {
				args := tc.Arguments
				if tc.Function != nil && tc.Function.Arguments != "" {
					args = tc.Function.Arguments
				}
				// Empty arguments are valid (some tool calls have no params)
				if args == "" || json.Valid([]byte(args)) {
					validCalls = append(validCalls, tc)
					if tc.ID != "" {
						validToolCallIDs[tc.ID] = true
					}
				}
			}
			// Replace tool calls with only valid ones
			newMsg := msg
			newMsg.ToolCalls = validCalls
			// Skip if all tool calls were invalid and no content
			if len(validCalls) == 0 && newMsg.Content == "" {
				continue
			}
			cleaned = append(cleaned, newMsg)
		} else {
			cleaned = append(cleaned, msg)
		}
	}

	// Second pass: remove orphaned tool result messages.
	result := make([]Message, 0, len(cleaned))
	for _, msg := range cleaned {
		if msg.Role == RoleTool && msg.ToolCallID != "" {
			if !validToolCallIDs[msg.ToolCallID] {
				continue // Skip orphaned tool result
			}
		}
		result = append(result, msg)
	}

	return result
}
