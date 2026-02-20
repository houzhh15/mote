package provider

import (
	"encoding/json"
	"log/slog"
)

// SanitizeMessages cleans up tool call pairs in the message history.
// It removes tool calls with invalid JSON arguments and their corresponding
// tool result messages, preventing provider API errors from corrupted data
// (e.g., truncated arguments due to streaming issues).
//
// It also enforces tool call ordering: tool result messages must immediately
// follow their corresponding assistant message with tool_calls. This prevents
// provider API errors like MiniMax's "tool call result does not follow tool call"
// which occurs when context compression or merging breaks the pairing.
func SanitizeMessages(messages []Message) []Message {
	if len(messages) == 0 {
		return messages
	}

	// First pass: collect valid tool call IDs and fix assistant messages.
	validToolCallIDs := make(map[string]bool)
	cleaned := make([]Message, 0, len(messages))

	for _, msg := range messages {
		if msg.Role == RoleAssistant {
			var validCalls []ToolCall
			if len(msg.ToolCalls) > 0 {
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
			}

			// Replace tool calls with only valid ones
			newMsg := msg
			newMsg.ToolCalls = validCalls

			// Skip if message has no content AND no valid tool calls
			if newMsg.Content == "" && len(validCalls) == 0 {
				continue
			}
			cleaned = append(cleaned, newMsg)
		} else {
			cleaned = append(cleaned, msg)
		}
	}

	// Second pass: remove orphaned tool result messages.
	deorphaned := make([]Message, 0, len(cleaned))
	for _, msg := range cleaned {
		if msg.Role == RoleTool && msg.ToolCallID != "" {
			if !validToolCallIDs[msg.ToolCallID] {
				continue // Skip orphaned tool result
			}
		}
		deorphaned = append(deorphaned, msg)
	}

	// Third pass: enforce tool call ordering.
	// Tool result messages (role=tool) must immediately follow the assistant
	// message that contains the matching tool_call. If a tool result appears
	// elsewhere (e.g., after context compression merges messages), remove it
	// to prevent provider API errors.
	ordered := enforceToolCallOrdering(deorphaned)

	if removed := len(deorphaned) - len(ordered); removed > 0 {
		slog.Warn("sanitize: enforceToolCallOrdering removed messages",
			"before", len(deorphaned),
			"after", len(ordered),
			"removed", removed)
	}

	return ordered
}

// enforceToolCallOrdering ensures tool call/result pairs are correctly structured.
//
// Provider APIs (especially MiniMax) require:
//  1. Every assistant message with tool_calls must be immediately followed by
//     tool result messages for ALL of those tool_calls.
//  2. No non-tool messages may appear between the assistant and its tool results.
//  3. Every tool result must reference a tool_call from the immediately preceding assistant.
//
// When context compression or message merging breaks these invariants, this function
// repairs the message list by:
//   - Removing tool results that are separated from their owning assistant
//   - Stripping tool_calls from assistant messages whose results are missing/displaced
//   - Removing tool messages without a ToolCallID
func enforceToolCallOrdering(messages []Message) []Message {
	if len(messages) == 0 {
		return messages
	}

	// Phase 1: Identify which assistant+tool_call groups are complete.
	// A valid group: assistant with tool_calls at index i, immediately followed
	// by tool results at indices i+1..j-1 covering ALL tool_call IDs.
	invalidToolCallIDs := make(map[string]bool)
	validToolCallIDs := make(map[string]bool)

	for i := 0; i < len(messages); i++ {
		msg := messages[i]
		if msg.Role != RoleAssistant || len(msg.ToolCalls) == 0 {
			continue
		}

		// Collect expected tool call IDs from this assistant
		expectedIDs := make(map[string]bool)
		for _, tc := range msg.ToolCalls {
			if tc.ID != "" {
				expectedIDs[tc.ID] = true
			}
		}
		if len(expectedIDs) == 0 {
			continue
		}

		// Scan immediately following tool result messages
		j := i + 1
		foundIDs := make(map[string]bool)
		for j < len(messages) && messages[j].Role == RoleTool {
			if expectedIDs[messages[j].ToolCallID] {
				foundIDs[messages[j].ToolCallID] = true
			}
			j++
		}

		// Group is valid only if ALL expected tool results are present
		if len(foundIDs) == len(expectedIDs) {
			for id := range expectedIDs {
				validToolCallIDs[id] = true
			}
		} else {
			// Incomplete group — mark all its tool_call IDs as invalid
			for id := range expectedIDs {
				invalidToolCallIDs[id] = true
			}
		}
	}

	// Phase 2: Rebuild messages, cleaning up invalid groups.
	result := make([]Message, 0, len(messages))

	for _, msg := range messages {
		// Handle tool messages
		if msg.Role == RoleTool {
			// Drop tool messages without ToolCallID (malformed)
			if msg.ToolCallID == "" {
				continue
			}
			// Drop tool results from invalid/incomplete groups
			if invalidToolCallIDs[msg.ToolCallID] {
				continue
			}
			// Drop truly orphaned tool results (not in any group)
			if !validToolCallIDs[msg.ToolCallID] {
				continue
			}
			result = append(result, msg)
			continue
		}

		// Handle assistant messages with tool_calls
		if msg.Role == RoleAssistant && len(msg.ToolCalls) > 0 {
			hasInvalid := false
			for _, tc := range msg.ToolCalls {
				if invalidToolCallIDs[tc.ID] {
					hasInvalid = true
					break
				}
			}

			if hasInvalid {
				// Strip tool_calls from this assistant; keep only if it has text content
				newMsg := msg
				newMsg.ToolCalls = nil
				if newMsg.Content != "" {
					result = append(result, newMsg)
				}
				// else: no content AND no valid tool calls → drop entirely
				continue
			}
		}

		result = append(result, msg)
	}

	// Post-validation: log warning if any tool call ordering issues remain
	for i, msg := range result {
		if msg.Role == RoleAssistant && len(msg.ToolCalls) > 0 {
			expectedIDs := make(map[string]bool)
			for _, tc := range msg.ToolCalls {
				if tc.ID != "" {
					expectedIDs[tc.ID] = true
				}
			}
			j := i + 1
			for j < len(result) && result[j].Role == RoleTool {
				delete(expectedIDs, result[j].ToolCallID)
				j++
			}
			if len(expectedIDs) > 0 {
				slog.Error("sanitize: TOOL CALL ORDERING STILL BROKEN after enforcement",
					"assistant_index", i,
					"missing_tool_call_ids", expectedIDs,
					"total_messages", len(result))
			}
		}
		if msg.Role == RoleTool {
			// Check that the preceding message is either another tool or an assistant with tool_calls
			if i == 0 {
				slog.Error("sanitize: tool message at index 0", "tool_call_id", msg.ToolCallID)
			} else if result[i-1].Role != RoleTool && (result[i-1].Role != RoleAssistant || len(result[i-1].ToolCalls) == 0) {
				slog.Error("sanitize: tool message not preceded by assistant/tool",
					"index", i,
					"tool_call_id", msg.ToolCallID,
					"prev_role", result[i-1].Role)
			}
		}
	}

	return result
}
