package compaction

// Default prompts for memory flush
const (
	defaultMemoryFlushPrompt = `Before context compaction, please save any important information to memory using the memory_save tool. Focus on:
- Key decisions made during this conversation
- Important facts or data mentioned
- User preferences expressed
- Any pending tasks or action items

Review the conversation and save what matters most.`

	defaultMemoryFlushSystemPrompt = `You are in a pre-compaction memory flush turn. Your task is to review the conversation history and identify important information that should be preserved in long-term memory before the context is compressed. Use the memory_save tool to store key facts, decisions, and preferences.`
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
	// Default: 100000
	MaxContextTokens int `json:"max_context_tokens"`

	// TriggerThreshold is the percentage of MaxContextTokens that triggers compaction.
	// Default: 0.8 (80%)
	TriggerThreshold float64 `json:"trigger_threshold"`

	// KeepRecentCount is the number of recent messages to keep uncompacted.
	// Default: 10
	KeepRecentCount int `json:"keep_recent_count"`

	// SummaryMaxTokens is the maximum tokens for each summary.
	// Default: 500
	SummaryMaxTokens int `json:"summary_max_tokens"`

	// ChunkMaxTokens is the maximum tokens per chunk for summarization.
	// Default: 4000
	ChunkMaxTokens int `json:"chunk_max_tokens"`

	// FlushOnCompact enables saving key info to memory before compaction.
	// P1: Default: true
	FlushOnCompact bool `json:"flush_on_compact"`

	// MemoryFlush holds configuration for pre-compaction memory flush.
	MemoryFlush MemoryFlushConfig `json:"memory_flush" yaml:"memoryFlush"`
}

// DefaultConfig returns a CompactionConfig with default values.
func DefaultConfig() CompactionConfig {
	return CompactionConfig{
		MaxContextTokens: 100000,
		TriggerThreshold: 0.8,
		KeepRecentCount:  10,
		SummaryMaxTokens: 500,
		ChunkMaxTokens:   4000,
		FlushOnCompact:   true, // P1: Enable by default
		MemoryFlush:      DefaultMemoryFlushConfig(),
	}
}
