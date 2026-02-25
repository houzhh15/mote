package compaction

// Default prompts for memory flush
const (
	defaultMemoryFlushPrompt = `Before context compaction, please save any important information to memory using the mote_memory_add tool. Focus on:
- Key decisions made during this conversation
- Important facts or data mentioned
- User preferences expressed
- Any pending tasks or action items

Review the conversation and save what matters most.`

	defaultMemoryFlushSystemPrompt = `You are in a pre-compaction memory flush turn. Your task is to review the conversation history and identify important information that should be preserved in long-term memory before the context is compressed. Use the mote_memory_add tool to store key facts, decisions, and preferences.`
)

// MemoryFlushConfig holds configuration for pre-compaction memory flush.
type MemoryFlushConfig struct {
	// Enabled controls whether memory flush runs before compaction.
	// Default: true
	Enabled bool `json:"enabled" yaml:"enabled"`

	// SoftThresholdTokens is the buffer before compaction threshold to trigger flush.
	// Default: 4000
	SoftThresholdTokens int64 `json:"soft_threshold_tokens" yaml:"softThresholdTokens"`

	// ReserveTokens is the minimum tokens to reserve for response.
	// Default: 8000
	ReserveTokens int64 `json:"reserve_tokens" yaml:"reserveTokens"`

	// Prompt is the user prompt for the memory flush turn.
	Prompt string `json:"prompt" yaml:"prompt"`

	// SystemPrompt is the system prompt for the memory flush turn.
	SystemPrompt string `json:"system_prompt" yaml:"systemPrompt"`
}

// DefaultMemoryFlushConfig returns a MemoryFlushConfig with default values.
func DefaultMemoryFlushConfig() MemoryFlushConfig {
	return MemoryFlushConfig{
		Enabled:             true,
		SoftThresholdTokens: 4000,
		ReserveTokens:       8000,
		Prompt:              defaultMemoryFlushPrompt,
		SystemPrompt:        defaultMemoryFlushSystemPrompt,
	}
}

// CompactionConfig holds configuration for history compaction.
type CompactionConfig struct {
	// MaxContextTokens is the maximum number of tokens allowed in context.
	// For best results, set this to the actual model's context window size.
	// Default: 48000
	MaxContextTokens int `json:"max_context_tokens" yaml:"maxContextTokens"`

	// TriggerThreshold is the percentage of MaxContextTokens that triggers compaction.
	// Used as fallback when ReserveTokens is 0.
	// Default: 0.8 (80%)
	TriggerThreshold float64 `json:"trigger_threshold" yaml:"triggerThreshold"`

	// ReserveTokens is the number of tokens to keep as recent context after
	// compaction. When > 0, it replaces TriggerThreshold for triggering
	// (trigger = tokens > MaxContextTokens - ReserveTokens) and uses a
	// token-based cut point instead of the fixed KeepRecentCount.
	// For a 200K model: set MaxContextTokens=200000, ReserveTokens=20000
	// → compaction triggers at ~180K, keeping ~20K tokens of recent messages.
	// Default: 10000
	ReserveTokens int `json:"reserve_tokens" yaml:"reserveTokens"`

	// KeepRecentCount is the minimum number of recent messages to keep
	// uncompacted. When ReserveTokens > 0, this serves as a floor (always
	// keep at least this many messages even if they exceed ReserveTokens).
	// When ReserveTokens == 0, this is used as the primary cut point.
	// Default: 10
	KeepRecentCount int `json:"keep_recent_count" yaml:"keepRecentCount"`

	// SummaryMaxTokens is the maximum tokens for each summary.
	// Default: 1000
	SummaryMaxTokens int `json:"summary_max_tokens" yaml:"summaryMaxTokens"`

	// ChunkMaxTokens is the maximum tokens per chunk for summarization.
	// When AdaptiveChunkMinRatio > 0, adaptive chunking overrides this value.
	// Default: 64000
	ChunkMaxTokens int `json:"chunk_max_tokens" yaml:"chunkMaxTokens"`

	// FlushOnCompact enables saving key info to memory before compaction.
	// P1: Default: true
	FlushOnCompact bool `json:"flush_on_compact" yaml:"flushOnCompact"`

	// MemoryFlush holds configuration for pre-compaction memory flush.
	MemoryFlush MemoryFlushConfig `json:"memory_flush" yaml:"memoryFlush"`

	// ToolResultMaxBytes is the maximum byte size of a single tool result.
	// Results exceeding this limit are pre-truncated before storing in history.
	// Default: 65536 (64 KB)
	ToolResultMaxBytes int `json:"tool_result_max_bytes" yaml:"toolResultMaxBytes"`

	// MaxMessageCount is the maximum number of messages before compaction is triggered,
	// regardless of token count. This prevents excessive tool-call iterations from
	// accumulating too many messages before the token threshold is reached.
	// Default: 40 (about 20 tool-call iterations)
	MaxMessageCount int `json:"max_message_count" yaml:"maxMessageCount"`

	// CompactedToolResultMaxBytes limits tool result size in kept (recent) messages
	// after compaction. Full tool results are only needed during the active session;
	// after compaction they serve as context reminders and can be shorter.
	// Default: 4096 (4 KB)
	CompactedToolResultMaxBytes int `json:"compacted_tool_result_max_bytes" yaml:"compactedToolResultMaxBytes"`

	// MaxRequestBytes is the hard upper limit on the estimated request body size
	// (in bytes) sent to a provider. Before each provider call, older tool results
	// are progressively truncated to keep the total under this budget.
	// This prevents large requests from triggering empty-body responses on
	// providers like MiniMax whose ALB rejects oversized payloads.
	// Default: 65536 (64 KB)
	MaxRequestBytes int `json:"max_request_bytes" yaml:"maxRequestBytes"`

	// MaxSingleMsgRatio is the maximum ratio of MaxContextTokens that a single
	// message can consume. Messages exceeding this threshold during compaction
	// are replaced with a stub notice before summarization to prevent one
	// oversized tool result from dominating the summary.
	// Default: 0.5 (50%)
	MaxSingleMsgRatio float64 `json:"max_single_msg_ratio" yaml:"maxSingleMsgRatio"`

	// AdaptiveChunkMinRatio is the minimum chunk size as a ratio of MaxContextTokens.
	// When > 0, chunk sizes are computed adaptively instead of using fixed ChunkMaxTokens.
	// Default: 0.15 (15%)
	AdaptiveChunkMinRatio float64 `json:"adaptive_chunk_min_ratio" yaml:"adaptiveChunkMinRatio"`

	// AdaptiveChunkMaxRatio is the maximum chunk size as a ratio of MaxContextTokens.
	// Default: 0.40 (40%)
	AdaptiveChunkMaxRatio float64 `json:"adaptive_chunk_max_ratio" yaml:"adaptiveChunkMaxRatio"`
}

// DefaultConfig returns a CompactionConfig with default values.
func DefaultConfig() CompactionConfig {
	return CompactionConfig{
		MaxContextTokens:            48000,
		TriggerThreshold:            0.8,
		ReserveTokens:               10000,
		KeepRecentCount:             10,
		SummaryMaxTokens:            1000,
		ChunkMaxTokens:              64000,
		FlushOnCompact:              true, // P1: Enable by default
		MemoryFlush:                 DefaultMemoryFlushConfig(),
		ToolResultMaxBytes:          65536, // 64 KB
		MaxMessageCount:             40,    // ~20 tool-call iterations
		CompactedToolResultMaxBytes: 4096,  // 4 KB per tool result in compacted context
		MaxRequestBytes:             65536, // 64 KB hard budget (messages portion; tools ~20 KB counted separately)
		MaxSingleMsgRatio:           0.5,   // 50%  — single message > this ratio → replaced with notice
		AdaptiveChunkMinRatio:       0.15,  // 15% of MaxContextTokens
		AdaptiveChunkMaxRatio:       0.40,  // 40% of MaxContextTokens
	}
}

// AdaptForModel adjusts MaxContextTokens, ReserveTokens, and MaxRequestBytes
// based on the actual model context window.  This should be called once when
// the session's model is known.  The scaling is proportional:
//
//	ReserveTokens    = contextWindow × (defaultReserve / defaultMaxContext)
//	MaxContextTokens = contextWindow
//	MaxRequestBytes  = MaxRequestBytes × (contextWindow / defaultMaxContext)
//
// If contextWindow is 0 or smaller than the current MaxContextTokens,
// no changes are made.
func (c *CompactionConfig) AdaptForModel(contextWindow int) {
	if contextWindow <= 0 || contextWindow <= c.MaxContextTokens {
		return
	}

	// Compute scaling factor once for all proportional fields.
	scale := float64(contextWindow) / float64(c.MaxContextTokens)

	// Scale ReserveTokens proportionally.
	// Default ratio: 10000 / 48000 ≈ 20.8%
	ratio := float64(c.ReserveTokens) / float64(c.MaxContextTokens)
	c.MaxContextTokens = contextWindow
	c.ReserveTokens = int(float64(contextWindow) * ratio)

	// Also scale ChunkMaxTokens to 1/3 context window (was 64K for 48K context).
	newChunk := contextWindow / 3
	if newChunk > c.ChunkMaxTokens {
		c.ChunkMaxTokens = newChunk
	}

	// Scale MaxRequestBytes proportionally with the context window.
	//
	// The default 65536 bytes for a 48000-token model corresponds to ~1.37
	// bytes/token.  For larger models (e.g. MiniMax M2.5 at 204800 tokens)
	// this prevents the budget from being artificially small and causing
	// catastrophic context loss where BudgetMessages drops ALL historical
	// context (including compaction summaries) because a single tool result
	// + tools JSON + system prompt exceeds the tiny budget.
	//
	// Example: MiniMax M2.5 (204800 tokens):
	//   scale = 204800 / 48000 ≈ 4.27
	//   MaxRequestBytes = 65536 × 4.27 ≈ 279 KB
	//   Actual capacity: 204800 tokens × ~4 bytes ≈ 800 KB
	//   → 279 KB uses ~35% of capacity, leaving room for response tokens.
	newMaxReqBytes := int(float64(c.MaxRequestBytes) * scale)
	if newMaxReqBytes > c.MaxRequestBytes {
		c.MaxRequestBytes = newMaxReqBytes
	}

	// Scale MaxMessageCount super-linearly with the context window.
	//
	// The default 40 messages for a 48000-token model is ~20 tool-call
	// iterations.  For larger models, tool-heavy agentic workflows generate
	// many SHORT messages (tool calls ~100 tokens, tool results ~200-500
	// tokens), so message count grows much faster than token count.
	//
	// Linear scaling (×4.27 = 170 for a 200K model) triggers premature
	// compaction at 170 messages while tokens are only at ~66% of the
	// threshold.  Using ×2.5 super-linear scaling gives enough headroom:
	//
	// Example: MiniMax M2.5 (204800 tokens):
	//   scale ≈ 4.27 → MaxMessageCount = 40 × 4.27 × 2.5 ≈ 427
	//   tokenThreshold = 162134 → at avg 340 tokens/msg ≈ 477 msgs at limit
	//   427 aligns well with the token-based threshold, acting as a safety
	//   net only for extreme message accumulation.
	newMaxMsgCount := int(float64(c.MaxMessageCount) * scale * 2.5)
	if newMaxMsgCount > c.MaxMessageCount {
		c.MaxMessageCount = newMaxMsgCount
	}
}
