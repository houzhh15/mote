package compaction

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"

	"mote/internal/provider"
)

// Structured summary prompt — produces Goal / Progress / Key Decisions /
// Open Issues sections, which are far more useful for continuity than a
// free-form paragraph.
const structuredSummaryPrompt = `You are summarizing a conversation between a user and an AI assistant.
Produce a STRUCTURED summary using the exact sections below.
Be concise but preserve all actionable information.

## Goal
What is the user's primary objective or task?

## Progress
What has been accomplished so far? List key actions taken and their outcomes.

## Key Decisions
Important decisions, conclusions, preferences, or technical choices made.

## Important Context
Critical context that must be preserved across compactions (e.g. environment details, constraints, user preferences, file paths).

## Open Issues / Next Steps
Pending tasks, unresolved questions, or planned next steps.
%s
---
Conversation to summarize:
%s

Produce the structured summary now:`

// Incremental update prompt — when a previous summary exists, the model
// merges it with new conversation instead of starting from scratch.
const incrementalSummaryPrompt = `You are updating a running conversation summary.
Below is the PREVIOUS summary followed by NEW conversation that occurred after it.
Merge them into a single, up-to-date structured summary using the exact sections below.
Drop information that is no longer relevant; keep everything still actionable.

## Goal
What is the user's primary objective or task?

## Progress
What has been accomplished so far? List key actions taken and their outcomes.

## Key Decisions
Important decisions, conclusions, preferences, or technical choices made.

## Important Context
Critical context that must be preserved across compactions (e.g. environment details, constraints, user preferences, file paths).

## Open Issues / Next Steps
Pending tasks, unresolved questions, or planned next steps.
%s
---
Previous summary:
%s

---
New conversation:
%s

Produce the updated structured summary now:`

// Compactor handles compression of conversation history.
// It is safe for concurrent use from multiple goroutines (e.g. cron jobs
// and interactive sessions sharing the same Compactor instance).
type Compactor struct {
	config          CompactionConfig
	provider        provider.Provider
	counter         *TokenCounter
	compactionCount map[string]int // session → compaction count
	countMu         sync.RWMutex   // protects compactionCount
}

// NewCompactor creates a new Compactor.
func NewCompactor(config CompactionConfig, prov provider.Provider) *Compactor {
	return &Compactor{
		config:          config,
		provider:        prov,
		counter:         NewTokenCounter(),
		compactionCount: make(map[string]int),
	}
}

// WithContextWindow returns a new Compactor whose config has been adapted
// for the given model context window.  The clone shares the stateless
// TokenCounter but has its own compaction-count map (callers should keep
// using the original Compactor for count management).
//
// This is designed for per-session usage in the orchestrator: the session
// creates an adapted clone for NeedsCompaction / BudgetMessages / Compact,
// while the shared original Compactor tracks compaction counts.
func (c *Compactor) WithContextWindow(contextWindow int) *Compactor {
	adapted := c.config // copy by value
	adapted.AdaptForModel(contextWindow)
	slog.Info("compactor: adapted config for model context window",
		"contextWindow", contextWindow,
		"maxContextTokens", adapted.MaxContextTokens,
		"reserveTokens", adapted.ReserveTokens,
		"chunkMaxTokens", adapted.ChunkMaxTokens,
		"maxRequestBytes", adapted.MaxRequestBytes,
		"maxMessageCount", adapted.MaxMessageCount)
	return &Compactor{
		config:          adapted,
		provider:        c.provider,
		counter:         c.counter,
		compactionCount: make(map[string]int),
	}
}

// WithMaxRequestBytes returns a new Compactor whose MaxRequestBytes has been
// overridden to the given value.  All other config fields are inherited.
// This is used by the orchestrator to force aggressive byte-level truncation
// when a provider silently rejects oversized requests (empty stop response).
func (c *Compactor) WithMaxRequestBytes(maxBytes int) *Compactor {
	adapted := c.config // copy by value
	adapted.MaxRequestBytes = maxBytes
	return &Compactor{
		config:          adapted,
		provider:        c.provider,
		counter:         c.counter,
		compactionCount: make(map[string]int),
	}
}

// EstimateRequestBytes returns the estimated JSON-serialized request size
// in bytes for the given messages and tools overhead.  This is an exported
// wrapper around estimateRequestBytes for use by the orchestrator.
func (c *Compactor) EstimateRequestBytes(messages []provider.Message, toolsOverhead int) int {
	return c.estimateRequestBytes(messages, toolsOverhead)
}

// GetCompactionCount returns the compaction count for a session.
func (c *Compactor) GetCompactionCount(sessionID string) int {
	c.countMu.RLock()
	defer c.countMu.RUnlock()
	return c.compactionCount[sessionID]
}

// IncrementCompactionCount increments the compaction count for a session.
func (c *Compactor) IncrementCompactionCount(sessionID string) {
	c.countMu.Lock()
	defer c.countMu.Unlock()
	if c.compactionCount == nil {
		c.compactionCount = make(map[string]int)
	}
	c.compactionCount[sessionID]++
}

// ResetCompactionCount resets the compaction count for a session.
// Should be called at the start of each task execution (user prompt)
// so that the first compaction within a task always uses LLM summarization.
func (c *Compactor) ResetCompactionCount(sessionID string) {
	c.countMu.Lock()
	defer c.countMu.Unlock()
	delete(c.compactionCount, sessionID)
}

// NeedsCompaction checks if the message history needs compression.
// Triggers on any of the following conditions:
//  1. When ReserveTokens > 0: estimated tokens exceed MaxContextTokens - ReserveTokens
//  2. When ReserveTokens == 0: estimated tokens exceed MaxContextTokens * TriggerThreshold (legacy)
//  3. Message count exceeds MaxMessageCount (if configured, >0)
func (c *Compactor) NeedsCompaction(messages []provider.Message) bool {
	tokens := c.counter.EstimateMessages(messages)

	var threshold int
	if c.config.ReserveTokens > 0 && c.config.ReserveTokens < c.config.MaxContextTokens {
		threshold = c.config.MaxContextTokens - c.config.ReserveTokens
	} else {
		threshold = int(float64(c.config.MaxContextTokens) * c.config.TriggerThreshold)
	}

	if tokens > threshold {
		slog.Info("NeedsCompaction: triggered by token threshold",
			"estimatedTokens", tokens, "threshold", threshold,
			"messageCount", len(messages))
		return true
	}
	if c.config.MaxMessageCount > 0 && len(messages) > c.config.MaxMessageCount {
		slog.Info("NeedsCompaction: triggered by message count",
			"messageCount", len(messages), "maxMessageCount", c.config.MaxMessageCount,
			"estimatedTokens", tokens, "tokenThreshold", threshold)
		return true
	}
	return false
}

// NeedsMemoryFlush checks if the message history is approaching the
// compaction threshold and a memory flush should be triggered first.
// Returns true when tokens > compaction_threshold - MemoryFlush.SoftThresholdTokens.
// The orchestrator should call this before each iteration; when true,
// inject a memory-flush turn before compaction fires.
func (c *Compactor) NeedsMemoryFlush(messages []provider.Message) bool {
	if !c.config.MemoryFlush.Enabled {
		return false
	}
	tokens := c.counter.EstimateMessages(messages)

	var compactionThreshold int
	if c.config.ReserveTokens > 0 && c.config.ReserveTokens < c.config.MaxContextTokens {
		compactionThreshold = c.config.MaxContextTokens - c.config.ReserveTokens
	} else {
		compactionThreshold = int(float64(c.config.MaxContextTokens) * c.config.TriggerThreshold)
	}

	flushThreshold := compactionThreshold - int(c.config.MemoryFlush.SoftThresholdTokens)
	return tokens > flushThreshold
}

// BudgetMessages returns a copy of messages where the total estimated request
// size is brought within MaxRequestBytes through progressively aggressive
// strategies.  This is called BEFORE every provider call — it is fast (no LLM
// calls) and ensures the provider never receives a request that is too large.
//
// toolsOverhead optionally specifies the actual tools JSON size in bytes.
// When provided (>0), it replaces the default 20 KB baseline estimate.
// Callers should compute this from json.Marshal(tools) for accuracy.
//
// protectedTail optionally specifies the number of messages at the tail that
// must NEVER be truncated.  These are the current iteration's messages
// (assistant tool_calls + tool results) that the model needs in full to
// formulate its next response.  Only historical messages are subject to
// truncation.  When 0 or omitted, a default recentWindow of 4 is used.
//
// Strategy (three phases, each checked before proceeding):
//  1. Truncate HISTORICAL tool results progressively (4 KB → 1 KB → 256 B).
//  2. Truncate HISTORICAL non-system message content (2 KB → 512 B) — handles
//     large assistant/user messages that survived Phase 1.
//  3. Drop oldest non-system messages entirely (keeping tool-call pairs
//     intact) — last resort when sheer message count × overhead exceeds
//     the budget.  Protected tail messages are never dropped.
func (c *Compactor) BudgetMessages(messages []provider.Message, toolsOverhead int, protectedTail ...int) []provider.Message {
	budget := c.config.MaxRequestBytes
	if budget <= 0 {
		return messages
	}

	totalBytes := c.estimateRequestBytes(messages, toolsOverhead)
	if totalBytes <= budget {
		return messages
	}

	// Determine protected tail: the current iteration's messages that must
	// be preserved in full so the model can process fresh tool results.
	// When the caller provides a count (even 0), use it as-is.
	// Default (no argument): 4 (≈2 tool-call rounds) for backward compat.
	protected := 4
	if len(protectedTail) > 0 {
		protected = protectedTail[0]
	}

	slog.Info("BudgetMessages: estimate exceeds budget, truncating",
		"estimatedBytes", totalBytes,
		"budgetBytes", budget,
		"messageCount", len(messages),
		"toolsOverhead", toolsOverhead,
		"protectedTail", protected)

	// Make a shallow copy so we don't mutate the caller's slice
	result := make([]provider.Message, len(messages))
	copy(result, messages)

	// Truncation tiers: progressively more aggressive
	tiers := []int{4096, 1024, 256}

	// Phase 1: truncate HISTORICAL tool results (skip protected tail).
	for _, maxBytes := range tiers {
		if c.estimateRequestBytes(result, toolsOverhead) <= budget {
			return result
		}
		cutoff := len(result) - protected
		if cutoff < 0 {
			cutoff = 0
		}
		for i := 0; i < cutoff; i++ {
			if result[i].Role != provider.RoleTool {
				continue
			}
			if len(result[i].Content) <= maxBytes {
				continue
			}
			result[i] = provider.Message{
				Role:       result[i].Role,
				Content:    result[i].Content[:maxBytes] + "\n\n[... truncated to fit context budget ...]",
				ToolCallID: result[i].ToolCallID,
			}
		}
	}

	// Phase 2: Truncate HISTORICAL non-system message content (assistant, user).
	// Skip protected tail — only compress older context.
	contentTiers := []int{2048, 512}
	for _, maxBytes := range contentTiers {
		if c.estimateRequestBytes(result, toolsOverhead) <= budget {
			return result
		}
		cutoff := len(result) - protected
		if cutoff < 0 {
			cutoff = 0
		}
		for i := 0; i < cutoff; i++ {
			if result[i].Role == provider.RoleSystem || result[i].Role == provider.RoleTool {
				continue // system is untouchable; tool results handled in Phase 1
			}
			if len(result[i].Content) <= maxBytes {
				continue
			}
			// Preserve both head and tail of the message.  LLM responses
			// often start with analysis/work and END with a question or
			// instruction to the user.  Keeping only the head would lose
			// the critical tail content (e.g. "what is your API key?").
			headLen := maxBytes * 2 / 3
			tailLen := maxBytes - headLen
			truncatedContent := result[i].Content[:headLen] +
				"\n\n[... middle truncated to fit context budget ...]\n\n" +
				result[i].Content[len(result[i].Content)-tailLen:]
			result[i] = provider.Message{
				Role:       result[i].Role,
				Content:    truncatedContent,
				ToolCalls:  result[i].ToolCalls,
				ToolCallID: result[i].ToolCallID,
			}
		}
	}

	// Phase 3: Drop oldest non-system messages until under budget.
	// Protected tail messages are never dropped.  The model needs them
	// (current iteration's tool results) to formulate its next response.
	if c.estimateRequestBytes(result, toolsOverhead) > budget {
		result = c.dropOldestToBudget(result, budget, toolsOverhead, protected)
	}

	// Phase 4 (safety net): If still over budget after dropping all
	// droppable messages, truncate protected-tail tool results.
	//
	// This handles the pathological case where a single tool result is so
	// large that it alone exceeds the budget (e.g. a 100 KB file read when
	// the budget is 280 KB but tools+system consume 40 KB and the tool
	// result + a few conversation messages push it over).
	//
	// We use progressive tiers and head+tail truncation so the model still
	// sees the beginning and end of the tool output.
	if c.estimateRequestBytes(result, toolsOverhead) > budget {
		protectedStart := len(result) - protected
		if protectedStart < 0 {
			protectedStart = 0
		}
		// Tiers: 25% of budget, 12.5% of budget, 4 KB, 1 KB
		phase4Tiers := []int{budget / 4, budget / 8, 4096, 1024}
		for _, maxBytes := range phase4Tiers {
			if c.estimateRequestBytes(result, toolsOverhead) <= budget {
				break
			}
			for i := protectedStart; i < len(result); i++ {
				if result[i].Role != provider.RoleTool {
					continue
				}
				if len(result[i].Content) <= maxBytes {
					continue
				}
				headLen := maxBytes * 2 / 3
				tailLen := maxBytes - headLen
				if headLen+tailLen >= len(result[i].Content) {
					continue
				}
				slog.Warn("BudgetMessages Phase 4: truncating protected-tail tool result",
					"index", i, "originalBytes", len(result[i].Content), "maxBytes", maxBytes)
				result[i] = provider.Message{
					Role:       result[i].Role,
					Content:    result[i].Content[:headLen] + "\n\n[... middle truncated to fit context budget ...]\n\n" + result[i].Content[len(result[i].Content)-tailLen:],
					ToolCallID: result[i].ToolCallID,
				}
			}
		}
	}

	return result
}

// estimateRequestBytes estimates the JSON-serialized request size in bytes.
// This is a rough estimate: content bytes + overhead per message + tool metadata.
//
// toolsOverhead optionally provides the actual tools JSON size (from
// json.Marshal).  When >0, it replaces the default 20 KB fallback and
// adds a 2 KB envelope overhead.  This eliminates the ~2x underestimation
// that occurs when the tool registry is large (30-50 KB).
func (c *Compactor) estimateRequestBytes(messages []provider.Message, toolsOverhead ...int) int {
	// Determine baseline: use caller-provided tools size if available,
	// otherwise fall back to a conservative default.
	baseline := 20000 // ~20 KB default for tools + request envelope
	if len(toolsOverhead) > 0 && toolsOverhead[0] > 0 {
		baseline = toolsOverhead[0] + 2000 // actual tools JSON + 2 KB envelope
	}
	total := baseline
	for _, msg := range messages {
		total += len(msg.Content)
		total += 80 // JSON overhead per message (role, key names, braces, etc.)
		for _, tc := range msg.ToolCalls {
			total += len(tc.Arguments) + 100 // tool call overhead
			if tc.Function != nil {
				total += len(tc.Function.Name) + len(tc.Function.Arguments) + 50
			}
		}
	}
	return total
}

// estimateSingleMessageBytes returns the estimated byte contribution of a
// single message, excluding the per-request baseline (tools, envelope).
func (c *Compactor) estimateSingleMessageBytes(msg provider.Message) int {
	total := len(msg.Content) + 80 // content + JSON overhead
	for _, tc := range msg.ToolCalls {
		total += len(tc.Arguments) + 100
		if tc.Function != nil {
			total += len(tc.Function.Name) + len(tc.Function.Arguments) + 50
		}
	}
	return total
}

// dropOldestToBudget removes the oldest non-system messages until the
// estimated request size fits within the budget. Tool-call pairs are kept
// intact (never split an assistant tool_call from its tool result).
// protectedTail specifies how many tail messages must never be dropped.
func (c *Compactor) dropOldestToBudget(messages []provider.Message, budget int, toolsOverhead int, protectedTail int) []provider.Message {
	var systemMsgs, convMsgs []provider.Message
	for _, msg := range messages {
		if msg.Role == provider.RoleSystem {
			systemMsgs = append(systemMsgs, msg)
		} else {
			convMsgs = append(convMsgs, msg)
		}
	}
	if len(convMsgs) == 0 {
		return messages
	}

	// Determine which tail messages are protected (current iteration).
	// We protect them from being dropped — they're what the model needs NOW.
	minKeptFromEnd := protectedTail
	if minKeptFromEnd > len(convMsgs) {
		minKeptFromEnd = len(convMsgs)
	}

	// Available bytes = budget minus tools/system baseline
	systemBaseline := c.estimateRequestBytes(systemMsgs, toolsOverhead)
	available := budget - systemBaseline
	// Floor: ensure at least 8 KB for conversation messages even when
	// tools JSON + system prompt consume most of the budget.  Without this,
	// a large tool registry (~47 KB) + system prompt (~16 KB) can squeeze
	// available to near zero, causing all conversation messages to be dropped
	// and the LLM to lose critical user input.
	if available < 8192 {
		available = 8192
	}

	// Walk backward from newest, keeping messages that fit.
	keptBytes := 0
	splitIdx := len(convMsgs)
	for i := len(convMsgs) - 1; i >= 0; i-- {
		msgBytes := c.estimateSingleMessageBytes(convMsgs[i])
		// Always keep the protected tail regardless of budget
		if len(convMsgs)-i <= minKeptFromEnd {
			keptBytes += msgBytes
			splitIdx = i
			continue
		}
		if keptBytes+msgBytes > available {
			break
		}
		keptBytes += msgBytes
		splitIdx = i
	}

	// Adjust boundary to avoid splitting tool call pairs
	splitIdx = adjustKeepBoundary(convMsgs, splitIdx)

	keptConv := convMsgs[splitIdx:]

	// Safety net: if nothing fits, force-keep at least the most recent
	// conversation round (user + assistant + tool results) with aggressive
	// truncation.  An oversized request is better than no conversation at all,
	// which causes "messages 参数非法" from GLM.
	if len(keptConv) == 0 && len(convMsgs) > 0 {
		slog.Warn("dropOldestToBudget: keptConv empty, force-keeping last round")

		// Find the start of the last conversation round.
		// Walk backward to the last user message.
		roundStart := len(convMsgs) - 1
		for roundStart > 0 && convMsgs[roundStart].Role != provider.RoleUser {
			roundStart--
		}
		// If the user message is preceded by an assistant with tool_calls whose
		// tool results follow, include the whole tool-call group.
		if roundStart > 0 {
			idx := adjustKeepBoundary(convMsgs, roundStart)
			if idx < roundStart {
				roundStart = idx
			}
		}

		round := make([]provider.Message, len(convMsgs[roundStart:]))
		copy(round, convMsgs[roundStart:])

		// Aggressively truncate every message in the round to fit.
		const forceMax = 256
		for i := range round {
			if round[i].Role == provider.RoleTool && len(round[i].Content) > forceMax {
				round[i] = provider.Message{
					Role:       round[i].Role,
					Content:    round[i].Content[:forceMax] + "\n\n[... truncated to fit context budget ...]",
					ToolCallID: round[i].ToolCallID,
				}
			} else if round[i].Role != provider.RoleSystem && len(round[i].Content) > forceMax {
				round[i] = provider.Message{
					Role:       round[i].Role,
					Content:    round[i].Content[:forceMax] + "\n\n[... truncated to fit context budget ...]",
					ToolCalls:  round[i].ToolCalls,
					ToolCallID: round[i].ToolCallID,
				}
			}
		}
		keptConv = round
		splitIdx = roundStart
	}

	// Build result with a notice about the drop
	result := make([]provider.Message, 0, len(systemMsgs)+1+len(keptConv))
	result = append(result, systemMsgs...)
	if splitIdx > 0 {
		// Choose role to avoid consecutive same-role with the first kept message.
		role := provider.RoleAssistant
		if len(keptConv) > 0 {
			role = noticeRoleFor(keptConv[0].Role)
		}
		result = append(result, provider.Message{
			Role:    role,
			Content: "[Earlier context dropped to fit request size budget.]",
		})
	}
	result = append(result, keptConv...)

	slog.Info("BudgetMessages phase 3: dropped oldest messages to fit budget",
		"droppedCount", splitIdx,
		"keptConvCount", len(keptConv),
		"remainingMessages", len(result),
		"estimatedBytes", c.estimateRequestBytes(result, toolsOverhead),
		"budgetBytes", budget)

	return result
}

// resolveProvider returns the override provider if non-nil, otherwise falls back to c.provider.
func (c *Compactor) resolveProvider(override provider.Provider) provider.Provider {
	if override != nil {
		return override
	}
	return c.provider
}

// noticeRoleFor returns the role a notice/summary message should use
// so that it does not create consecutive same-role messages with the
// next message.  OpenAI-compatible APIs (including GLM) require role
// alternation; inserting a notice with the same role as the following
// message causes "messages 参数非法" rejections.
func noticeRoleFor(nextRole string) string {
	if nextRole == provider.RoleAssistant {
		return provider.RoleUser
	}
	return provider.RoleAssistant
}

// adjustKeepBoundary adjusts a split index so that tool call pairs
// (assistant with tool_calls → one or more tool result messages) are never split.
// Given convMsgs and a proposed splitIdx (messages[:splitIdx] are compacted,
// messages[splitIdx:] are kept), it moves splitIdx earlier if the first kept
// message is a tool result whose corresponding assistant tool_call would be
// compacted away.
func adjustKeepBoundary(convMsgs []provider.Message, splitIdx int) int {
	if splitIdx <= 0 || splitIdx >= len(convMsgs) {
		return splitIdx
	}

	// Walk backward from splitIdx while the message at splitIdx is role=tool,
	// meaning we'd be keeping orphan tool results.
	for splitIdx > 0 && convMsgs[splitIdx].Role == provider.RoleTool {
		splitIdx--
	}

	// Now splitIdx should point to either:
	// - an assistant message (possibly with tool_calls) — include it in kept
	// - a user message — safe boundary
	// - 0 — can't go further back
	return splitIdx
}

// Compact compresses the message history using LLM summarization.
// An optional provider override can be passed; if nil, the default provider is used.
//
// Compared to simple truncation this method:
//   - Uses a token-based cut point (ReserveTokens) instead of a fixed message count,
//     which adapts to message size variation.
//   - Generates a structured summary (Goal / Progress / Key Decisions / Next Steps).
//   - Detects a previous summary and uses an incremental-update prompt so context
//     from earlier compactions is preserved.
//   - Replaces oversized messages (>50% context) with stubs before summarization.
//   - Appends tool failure records and file tracking metadata to the summary.
//   - Uses adaptive chunking (15–40% of context window per chunk).
func (c *Compactor) Compact(ctx context.Context, messages []provider.Message, provOverride ...provider.Provider) ([]provider.Message, error) {
	var override provider.Provider
	if len(provOverride) > 0 {
		override = provOverride[0]
	}
	prov := c.resolveProvider(override)
	if prov == nil {
		return nil, ErrNoProvider
	}

	// Separate system and conversation messages
	systemMsgs, convMsgs := c.separateMessages(messages)

	// If not enough messages to compact, return as-is
	if len(convMsgs) <= c.config.KeepRecentCount {
		return messages, ErrMessagesTooShort
	}

	// --- Cut-point selection ---
	splitIdx := c.findCutPoint(convMsgs)

	keptMsgs := convMsgs[splitIdx:]
	toCompact := convMsgs[:splitIdx]

	if len(toCompact) == 0 {
		return messages, ErrMessagesTooShort
	}

	// Detect a previous summary from an earlier compaction.
	previousSummary := c.detectPreviousSummary(toCompact)

	// Replace oversized messages with stubs before summarization.
	toCompact = c.replaceOversizedMessages(toCompact)

	// Extract metadata for the summary.
	toolFailures := c.extractToolFailures(toCompact, 8)
	readFiles, modifiedFiles := c.extractFileInfo(toCompact)

	// Chunk messages for summarization (adaptive sizing).
	chunks := c.chunkMessages(toCompact)

	// Generate summary for each chunk.
	var summaries []string
	for i, chunk := range chunks {
		var summary string
		var err error
		// Use incremental prompt for the first chunk when a previous summary exists.
		if i == 0 && previousSummary != "" {
			summary, err = c.summarizeChunkIncremental(ctx, chunk, prov, previousSummary, toolFailures, readFiles, modifiedFiles)
		} else {
			summary, err = c.summarizeChunk(ctx, chunk, prov, toolFailures, readFiles, modifiedFiles)
		}
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrSummaryFailed, err)
		}
		summaries = append(summaries, summary)
		// Only attach metadata to the first chunk's prompt.
		toolFailures = nil
		readFiles = nil
		modifiedFiles = nil
	}

	// Merge summaries if multiple.
	finalSummary := strings.Join(summaries, "\n\n---\n\n")

	// Truncate tool results in kept messages to reduce post-compaction size.
	// Full tool results were already consumed by the model; kept copies only
	// need enough context for continuity.
	if c.config.CompactedToolResultMaxBytes > 0 {
		for i, msg := range keptMsgs {
			if msg.Role == provider.RoleTool && len(msg.Content) > c.config.CompactedToolResultMaxBytes {
				keptMsgs[i] = provider.Message{
					Role:       msg.Role,
					Content:    msg.Content[:c.config.CompactedToolResultMaxBytes] + "\n\n[... tool result truncated for compacted context ...]",
					ToolCallID: msg.ToolCallID,
				}
			}
		}
	}

	// Build result: system + summary + kept messages
	result := make([]provider.Message, 0, len(systemMsgs)+1+len(keptMsgs))
	result = append(result, systemMsgs...)
	if finalSummary != "" {
		// Choose role to avoid consecutive same-role messages.
		// After adjustKeepBoundary the first kept message may be assistant
		// (tool_calls), so the summary must use a different role.
		role := provider.RoleAssistant
		if len(keptMsgs) > 0 {
			role = noticeRoleFor(keptMsgs[0].Role)
		}
		result = append(result, provider.Message{
			Role:    role,
			Content: fmt.Sprintf("[Previous conversation summary]\n%s", finalSummary),
		})
	}
	result = append(result, keptMsgs...)

	return result, nil
}

// findCutPoint determines where to split conversation messages.
// When ReserveTokens > 0 it walks backward from the newest message,
// accumulating tokens until ReserveTokens is reached, then cuts at the
// nearest user/assistant boundary.  KeepRecentCount serves as a minimum
// floor.
// When ReserveTokens == 0 it falls back to the fixed KeepRecentCount.
func (c *Compactor) findCutPoint(convMsgs []provider.Message) int {
	if c.config.ReserveTokens <= 0 || c.config.ReserveTokens >= c.config.MaxContextTokens {
		// Legacy / fallback: fixed message count.
		keepCount := c.config.KeepRecentCount
		if keepCount > len(convMsgs) {
			keepCount = len(convMsgs)
		}
		rawSplitIdx := len(convMsgs) - keepCount
		return adjustKeepBoundary(convMsgs, rawSplitIdx)
	}

	// Token-based: walk backward until we accumulate ReserveTokens.
	reserveTokens := c.config.ReserveTokens
	accum := 0
	rawSplitIdx := len(convMsgs)
	for i := len(convMsgs) - 1; i >= 0; i-- {
		msgTokens := c.counter.EstimateMessages([]provider.Message{convMsgs[i]})
		if accum+msgTokens > reserveTokens {
			break
		}
		accum += msgTokens
		rawSplitIdx = i
	}

	// Apply minimum floor: keep at least KeepRecentCount messages.
	minKeptIdx := len(convMsgs) - c.config.KeepRecentCount
	if minKeptIdx < 0 {
		minKeptIdx = 0
	}
	if rawSplitIdx > minKeptIdx {
		rawSplitIdx = minKeptIdx
	}

	// Apply maximum ceiling: when all messages fit within ReserveTokens
	// (rawSplitIdx == 0), still enforce KeepRecentCount as an upper bound
	// so Compact always makes progress.  This handles the case where
	// compaction was triggered by MaxMessageCount rather than token count.
	maxKeptIdx := len(convMsgs) - c.config.KeepRecentCount
	if maxKeptIdx < 0 {
		maxKeptIdx = 0
	}
	if rawSplitIdx < maxKeptIdx {
		rawSplitIdx = maxKeptIdx
	}

	return adjustKeepBoundary(convMsgs, rawSplitIdx)
}

// detectPreviousSummary scans the beginning of toCompact for a message
// that looks like a previous compaction summary.  Returns the summary text
// (without the prefix) or "" if none found.
func (c *Compactor) detectPreviousSummary(toCompact []provider.Message) string {
	const prefix = "[Previous conversation summary]\n"
	for _, msg := range toCompact {
		if (msg.Role == provider.RoleAssistant || msg.Role == provider.RoleUser) &&
			strings.HasPrefix(msg.Content, prefix) {
			return strings.TrimPrefix(msg.Content, prefix)
		}
	}
	return ""
}

// replaceOversizedMessages replaces messages whose token count exceeds
// MaxSingleMsgRatio * MaxContextTokens with a brief notice.
// This prevents one huge tool result from dominating the summary.
func (c *Compactor) replaceOversizedMessages(messages []provider.Message) []provider.Message {
	if c.config.MaxSingleMsgRatio <= 0 {
		return messages
	}
	threshold := int(c.config.MaxSingleMsgRatio * float64(c.config.MaxContextTokens))
	if threshold <= 0 {
		return messages
	}

	result := make([]provider.Message, len(messages))
	copy(result, messages)
	for i, msg := range result {
		if msg.Role == provider.RoleSystem {
			continue
		}
		msgTokens := c.counter.EstimateMessages([]provider.Message{msg})
		if msgTokens > threshold {
			notice := fmt.Sprintf("[Oversized %s message removed — %d tokens, exceeded %.0f%% of context budget]",
				msg.Role, msgTokens, c.config.MaxSingleMsgRatio*100)
			result[i] = provider.Message{
				Role:       msg.Role,
				Content:    notice,
				ToolCalls:  msg.ToolCalls,
				ToolCallID: msg.ToolCallID,
			}
		}
	}
	return result
}

// extractToolFailures extracts the most recent tool failure messages.
// A tool result is considered a failure when it starts with common error
// prefixes.  Returns at most maxCount entries.
func (c *Compactor) extractToolFailures(messages []provider.Message, maxCount int) []string {
	var failures []string
	for i := len(messages) - 1; i >= 0 && len(failures) < maxCount; i-- {
		msg := messages[i]
		if msg.Role != provider.RoleTool {
			continue
		}
		lower := strings.ToLower(msg.Content)
		if strings.HasPrefix(lower, "error") ||
			strings.HasPrefix(lower, "failed") ||
			strings.Contains(lower, "error:") ||
			strings.Contains(lower, "permission denied") ||
			strings.Contains(lower, "not found") ||
			strings.Contains(lower, "timed out") {
			// Truncate to keep summary metadata concise.
			text := msg.Content
			if len(text) > 200 {
				text = text[:200] + "…"
			}
			failures = append(failures, text)
		}
	}
	// Reverse so oldest first.
	for i, j := 0, len(failures)-1; i < j; i, j = i+1, j-1 {
		failures[i], failures[j] = failures[j], failures[i]
	}
	return failures
}

// extractFileInfo scans assistant tool calls for file-related operations
// and returns deduplicated lists of read and modified files.
func (c *Compactor) extractFileInfo(messages []provider.Message) (readFiles, modifiedFiles []string) {
	readSet := make(map[string]bool)
	modifiedSet := make(map[string]bool)

	for _, msg := range messages {
		if msg.Role != provider.RoleAssistant || len(msg.ToolCalls) == 0 {
			continue
		}
		for _, tc := range msg.ToolCalls {
			toolName := ""
			args := ""
			if tc.Function != nil {
				toolName = tc.Function.Name
				args = tc.Function.Arguments
			}
			if toolName == "" {
				toolName = tc.Name
				if args == "" {
					args = tc.Arguments
				}
			}
			if toolName == "" || args == "" {
				continue
			}

			// Parse file path from arguments.
			var argMap map[string]any
			if err := json.Unmarshal([]byte(args), &argMap); err != nil {
				continue
			}
			filePath := ""
			for _, key := range []string{"path", "file", "filePath", "file_path", "filename", "target_file"} {
				if v, ok := argMap[key]; ok {
					if s, ok := v.(string); ok && s != "" {
						filePath = s
						break
					}
				}
			}
			if filePath == "" {
				continue
			}

			switch toolName {
			case "read_file", "search_file", "file_search", "grep_search", "semantic_search":
				readSet[filePath] = true
			case "write_file", "edit_file", "create_file", "replace_string_in_file",
				"multi_replace_string_in_file", "delete_file", "rename_file":
				modifiedSet[filePath] = true
			}
		}
	}

	for f := range readSet {
		readFiles = append(readFiles, f)
	}
	for f := range modifiedSet {
		modifiedFiles = append(modifiedFiles, f)
	}
	sort.Strings(readFiles)
	sort.Strings(modifiedFiles)
	return
}

// buildMetadataBlock builds the optional metadata section that is appended
// to the summary prompt (tool failures, file tracking).
func (c *Compactor) buildMetadataBlock(toolFailures, readFiles, modifiedFiles []string) string {
	var sb strings.Builder
	if len(toolFailures) > 0 {
		sb.WriteString("\n\nRecent tool failures (include in summary if relevant):\n")
		for _, f := range toolFailures {
			sb.WriteString(fmt.Sprintf("- %s\n", f))
		}
	}
	if len(readFiles) > 0 {
		sb.WriteString("\n<read-files>\n")
		for _, f := range readFiles {
			sb.WriteString(fmt.Sprintf("- %s\n", f))
		}
		sb.WriteString("</read-files>")
	}
	if len(modifiedFiles) > 0 {
		sb.WriteString("\n<modified-files>\n")
		for _, f := range modifiedFiles {
			sb.WriteString(fmt.Sprintf("- %s\n", f))
		}
		sb.WriteString("</modified-files>")
	}
	return sb.String()
}

// TruncateOnly performs compaction without any LLM calls.
// It keeps the most recent messages (respecting tool-call boundaries),
// drops old messages, and truncates tool results in kept messages to
// CompactedToolResultMaxBytes. A notice is prepended so the model knows
// earlier context was dropped.
//
// Use this for second+ compactions in a task to avoid calling prov.Chat()
// mid-task, which causes MiniMax ALB session degradation.
func (c *Compactor) TruncateOnly(messages []provider.Message) []provider.Message {
	truncated := c.truncate(messages)

	// Apply CompactedToolResultMaxBytes to tool results in kept messages,
	// same as Compact() does.
	if c.config.CompactedToolResultMaxBytes > 0 {
		for i, msg := range truncated {
			if msg.Role == provider.RoleTool && len(msg.Content) > c.config.CompactedToolResultMaxBytes {
				truncated[i] = provider.Message{
					Role:       msg.Role,
					Content:    msg.Content[:c.config.CompactedToolResultMaxBytes] + "\n\n[... tool result truncated for compacted context ...]",
					ToolCallID: msg.ToolCallID,
				}
			}
		}
	}

	// Prepend a notice after system messages so the model knows context was dropped.
	insertIdx := 0
	for i, msg := range truncated {
		if msg.Role != provider.RoleSystem {
			insertIdx = i
			break
		}
		insertIdx = i + 1
	}
	// Choose role to avoid consecutive same-role messages with the
	// first non-system message (which may be assistant after adjustKeepBoundary).
	noticeRole := provider.RoleAssistant
	if insertIdx < len(truncated) {
		noticeRole = noticeRoleFor(truncated[insertIdx].Role)
	}
	notice := provider.Message{
		Role:    noticeRole,
		Content: "[Previous conversation context was truncated to keep the session manageable. Earlier messages have been dropped. Continue with the current task based on the remaining context.]",
	}
	result := make([]provider.Message, 0, len(truncated)+1)
	result = append(result, truncated[:insertIdx]...)
	result = append(result, notice)
	result = append(result, truncated[insertIdx:]...)

	return result
}

// CompactWithFallback attempts Compact and falls back to truncation on error.
// An optional provider override can be passed to use the session's provider.
func (c *Compactor) CompactWithFallback(ctx context.Context, messages []provider.Message, provOverride ...provider.Provider) []provider.Message {
	result, err := c.Compact(ctx, messages, provOverride...)
	if err == nil && c.hasConversationContent(result) {
		return result
	}

	// Fallback to simple truncation
	truncated := c.truncate(messages)
	if c.hasConversationContent(truncated) {
		return truncated
	}

	// Safety net: if both compact and truncate produced invalid results,
	// return the original messages unchanged.  Sending too many tokens is
	// better than sending an invalid (empty) request.
	return messages
}

// hasConversationContent checks that a message list contains at least one
// user or assistant message, which is the minimum for a valid LLM request.
func (c *Compactor) hasConversationContent(messages []provider.Message) bool {
	for _, m := range messages {
		if m.Role == provider.RoleUser || m.Role == provider.RoleAssistant {
			return true
		}
	}
	return false
}

// separateMessages splits messages into system and conversation messages.
func (c *Compactor) separateMessages(messages []provider.Message) ([]provider.Message, []provider.Message) {
	var systemMsgs, convMsgs []provider.Message
	for _, msg := range messages {
		if msg.Role == provider.RoleSystem {
			systemMsgs = append(systemMsgs, msg)
		} else {
			convMsgs = append(convMsgs, msg)
		}
	}
	return systemMsgs, convMsgs
}

// chunkMessages splits messages into chunks based on an adaptive token limit.
// When AdaptiveChunkMinRatio > 0, the chunk size is computed as a fraction of
// MaxContextTokens (clamped between min and max ratio).  Otherwise, the fixed
// ChunkMaxTokens value is used.
func (c *Compactor) chunkMessages(messages []provider.Message) [][]provider.Message {
	if len(messages) == 0 {
		return nil
	}

	// Determine effective chunk token limit.
	chunkLimit := c.config.ChunkMaxTokens
	if c.config.AdaptiveChunkMinRatio > 0 && c.config.MaxContextTokens > 0 {
		totalTokens := c.counter.EstimateMessages(messages)
		// Target: each chunk should be roughly 1/N of total where N produces
		// chunks in the [min, max] ratio range of the context window.
		minChunk := int(c.config.AdaptiveChunkMinRatio * float64(c.config.MaxContextTokens))
		maxChunk := int(c.config.AdaptiveChunkMaxRatio * float64(c.config.MaxContextTokens))
		if maxChunk <= 0 {
			maxChunk = chunkLimit
		}
		if minChunk <= 0 {
			minChunk = maxChunk / 3
		}
		// Use the larger of minChunk and a size that puts us at ≤4 chunks.
		adaptive := totalTokens / 4
		if adaptive < minChunk {
			adaptive = minChunk
		}
		if adaptive > maxChunk {
			adaptive = maxChunk
		}
		chunkLimit = adaptive
	}
	if chunkLimit <= 0 {
		chunkLimit = 64000 // safety fallback
	}

	var chunks [][]provider.Message
	var currentChunk []provider.Message
	currentTokens := 0

	for _, msg := range messages {
		msgTokens := c.counter.EstimateMessages([]provider.Message{msg})
		if currentTokens+msgTokens > chunkLimit && len(currentChunk) > 0 {
			chunks = append(chunks, currentChunk)
			currentChunk = nil
			currentTokens = 0
		}
		currentChunk = append(currentChunk, msg)
		currentTokens += msgTokens
	}

	if len(currentChunk) > 0 {
		chunks = append(chunks, currentChunk)
	}

	return chunks
}

// summarizeChunk generates a structured summary for a chunk of messages.
func (c *Compactor) summarizeChunk(ctx context.Context, chunk []provider.Message, prov provider.Provider,
	toolFailures, readFiles, modifiedFiles []string) (string, error) {

	conversationText := c.formatChunkForSummary(chunk)
	metadata := c.buildMetadataBlock(toolFailures, readFiles, modifiedFiles)
	prompt := fmt.Sprintf(structuredSummaryPrompt, metadata, conversationText)

	req := provider.ChatRequest{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: prompt},
		},
		MaxTokens: c.config.SummaryMaxTokens,
	}

	resp, err := prov.Chat(ctx, req)
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// summarizeChunkIncremental generates a summary by merging a previous
// summary with new conversation content (incremental update).
func (c *Compactor) summarizeChunkIncremental(ctx context.Context, chunk []provider.Message, prov provider.Provider,
	previousSummary string, toolFailures, readFiles, modifiedFiles []string) (string, error) {

	conversationText := c.formatChunkForSummary(chunk)
	metadata := c.buildMetadataBlock(toolFailures, readFiles, modifiedFiles)
	prompt := fmt.Sprintf(incrementalSummaryPrompt, metadata, previousSummary, conversationText)

	req := provider.ChatRequest{
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: prompt},
		},
		MaxTokens: c.config.SummaryMaxTokens,
	}

	resp, err := prov.Chat(ctx, req)
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

// formatChunkForSummary formats a chunk of messages into text suitable for
// the summary prompt.  Tool results are truncated to keep the request small.
func (c *Compactor) formatChunkForSummary(chunk []provider.Message) string {
	const maxToolContentForSummary = 2048 // 2 KB per tool result in summary prompt
	var sb strings.Builder
	for _, msg := range chunk {
		content := msg.Content
		if msg.Role == provider.RoleTool && len(content) > maxToolContentForSummary {
			content = content[:maxToolContentForSummary] + "\n[... tool result truncated for summarization ...]"
		}
		sb.WriteString(fmt.Sprintf("[%s]: %s\n", msg.Role, content))
	}
	return sb.String()
}

// truncateMessageContent truncates a single message's content to fit within
// maxTokens, appending a truncation notice.  Returns a shallow copy.
func (c *Compactor) truncateMessageContent(msg provider.Message, maxTokens int) provider.Message {
	msgTokens := c.counter.EstimateMessages([]provider.Message{msg})
	if msgTokens <= maxTokens || len(msg.Content) == 0 {
		return msg
	}

	// Rough ratio: cut content proportionally.
	ratio := float64(maxTokens) / float64(msgTokens)
	if ratio > 0.95 {
		return msg
	}
	cutLen := int(float64(len(msg.Content)) * ratio)
	if cutLen < 200 {
		cutLen = 200
	}
	if cutLen >= len(msg.Content) {
		return msg
	}

	// Preserve both head and tail: LLM messages often end with a question
	// or instruction to the user, so keeping only the head would lose
	// critical context at the end.
	headLen := cutLen * 2 / 3
	tailLen := cutLen - headLen
	if tailLen < 100 {
		tailLen = 100
		headLen = cutLen - tailLen
	}
	if headLen < 0 {
		headLen = 0
	}

	truncated := msg
	if headLen > 0 && tailLen > 0 && headLen+tailLen < len(msg.Content) {
		truncated.Content = msg.Content[:headLen] +
			"\n\n[... content truncated due to length ...]\n\n" +
			msg.Content[len(msg.Content)-tailLen:]
	} else {
		truncated.Content = msg.Content[:cutLen] + "\n\n[... content truncated due to length ...]"
	}
	return truncated
}

// truncate performs simple truncation keeping recent messages.
// It guarantees the result always contains at least the most recent user
// message round (user + optional assistant/tool follow-ups) so the request
// sent to the provider is never empty.
//
// Strategy:
//  1. Walk backward through conversation messages, keeping as many as fit.
//  2. If a single message is too large to fit, truncate its content instead
//     of discarding it entirely.  This handles the common case of HTTP tool
//     results containing huge web pages.
//  3. As a last resort, force-keep the most recent round with truncation.
func (c *Compactor) truncate(messages []provider.Message) []provider.Message {
	systemMsgs, convMsgs := c.separateMessages(messages)

	if len(convMsgs) == 0 {
		return messages
	}

	// Calculate available space
	systemTokens := c.counter.EstimateMessages(systemMsgs)
	availableTokens := c.config.MaxContextTokens - systemTokens
	if availableTokens < 0 {
		availableTokens = 1000
	}

	// Maximum tokens a single message is allowed to consume.
	// This prevents one huge tool result from eating all available space.
	maxSingleMsgTokens := availableTokens / 2

	// Keep most recent messages that fit (walk backward).
	// Oversized messages are truncated to maxSingleMsgTokens before the
	// fit check so they don't block subsequent (older) messages.
	keptTokens := 0
	splitIdx := len(convMsgs)
	truncatedMap := make(map[int]provider.Message) // index → truncated copy

	for i := len(convMsgs) - 1; i >= 0; i-- {
		msg := convMsgs[i]
		msgTokens := c.counter.EstimateMessages([]provider.Message{msg})

		// If this single message exceeds the cap, truncate its content.
		if msgTokens > maxSingleMsgTokens {
			msg = c.truncateMessageContent(msg, maxSingleMsgTokens)
			msgTokens = c.counter.EstimateMessages([]provider.Message{msg})
			truncatedMap[i] = msg
		}

		if keptTokens+msgTokens > availableTokens {
			break
		}
		keptTokens += msgTokens
		splitIdx = i
	}

	// Adjust boundary to avoid splitting tool call pairs
	splitIdx = adjustKeepBoundary(convMsgs, splitIdx)

	// Build kept messages, using truncated copies where available.
	var keptMsgs []provider.Message
	for i := splitIdx; i < len(convMsgs); i++ {
		if tm, ok := truncatedMap[i]; ok {
			keptMsgs = append(keptMsgs, tm)
		} else {
			keptMsgs = append(keptMsgs, convMsgs[i])
		}
	}

	// Safety: if nothing fits even after truncation, force-keep the most
	// recent complete conversation round with aggressive truncation.
	if len(keptMsgs) == 0 {
		roundStart := len(convMsgs) - 1
		for roundStart > 0 && convMsgs[roundStart].Role != provider.RoleUser {
			roundStart--
		}
		round := convMsgs[roundStart:]

		maxPerMsg := availableTokens / len(round)
		if maxPerMsg < 50 {
			maxPerMsg = 50
		}
		for _, m := range round {
			keptMsgs = append(keptMsgs, c.truncateMessageContent(m, maxPerMsg))
		}
	}

	result := make([]provider.Message, 0, len(systemMsgs)+len(keptMsgs))
	result = append(result, systemMsgs...)
	result = append(result, keptMsgs...)
	return result
}
