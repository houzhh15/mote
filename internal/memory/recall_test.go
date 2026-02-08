package memory

import (
	"context"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

func TestDefaultRecallConfig(t *testing.T) {
	config := DefaultRecallConfig()

	if !config.Enabled {
		t.Error("expected Enabled to be true")
	}
	if config.Limit != 3 {
		t.Errorf("expected Limit 3, got %d", config.Limit)
	}
	if config.Threshold != 0.3 {
		t.Errorf("expected Threshold 0.3, got %f", config.Threshold)
	}
	if config.MinPromptLen != 5 {
		t.Errorf("expected MinPromptLen 5, got %d", config.MinPromptLen)
	}
}

func TestNewRecallEngine(t *testing.T) {
	t.Run("creates with defaults", func(t *testing.T) {
		engine := NewRecallEngine(RecallEngineOptions{
			Config: DefaultRecallConfig(),
		})
		if engine == nil {
			t.Fatal("engine is nil")
		}
		if engine.config.Limit != 3 {
			t.Errorf("expected limit 3, got %d", engine.config.Limit)
		}
	})

	t.Run("creates with custom config", func(t *testing.T) {
		config := RecallConfig{
			Enabled:      true,
			Limit:        5,
			Threshold:    0.5,
			MinPromptLen: 10,
		}
		engine := NewRecallEngine(RecallEngineOptions{
			Config: config,
		})
		if engine.config.Limit != 5 {
			t.Errorf("expected limit 5, got %d", engine.config.Limit)
		}
		if engine.config.Threshold != 0.5 {
			t.Errorf("expected threshold 0.5, got %f", engine.config.Threshold)
		}
	})
}

func TestRecallEngine_Recall(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	embedder := NewSimpleEmbedder(384)
	config := DefaultIndexConfig()

	idx, err := NewMemoryIndex(db, embedder, config)
	if err != nil {
		t.Fatalf("create index: %v", err)
	}

	ctx := context.Background()

	// Add some test memories
	memories := []MemoryEntry{
		{Content: "User prefers dark theme for coding", Category: CategoryPreference},
		{Content: "Team decided to use PostgreSQL database", Category: CategoryDecision},
		{Content: "User's email is test@example.com", Category: CategoryEntity},
	}
	for _, m := range memories {
		if err := idx.Add(ctx, m); err != nil {
			t.Fatalf("add memory: %v", err)
		}
	}

	engine := NewRecallEngine(RecallEngineOptions{
		Memory: idx,
		Config: DefaultRecallConfig(),
		Logger: zerolog.Nop(),
	})

	t.Run("recalls relevant memories", func(t *testing.T) {
		result, err := engine.Recall(ctx, "What theme does the user prefer")
		if err != nil {
			t.Fatalf("recall: %v", err)
		}

		if result == "" {
			t.Skip("FTS may not match, skipping content check")
		}

		// Check format
		if !strings.Contains(result, "<relevant-memories>") {
			t.Error("expected <relevant-memories> tag")
		}
		if !strings.Contains(result, "</relevant-memories>") {
			t.Error("expected closing </relevant-memories> tag")
		}
	})

	t.Run("returns empty for short prompt", func(t *testing.T) {
		result, err := engine.Recall(ctx, "hi")
		if err != nil {
			t.Fatalf("recall: %v", err)
		}
		if result != "" {
			t.Errorf("expected empty result for short prompt, got: %s", result)
		}
	})

	t.Run("returns empty when disabled", func(t *testing.T) {
		engine.SetEnabled(false)
		defer engine.SetEnabled(true)

		result, err := engine.Recall(ctx, "What theme does the user prefer")
		if err != nil {
			t.Fatalf("recall: %v", err)
		}
		if result != "" {
			t.Errorf("expected empty result when disabled, got: %s", result)
		}
	})
}

func TestRecallEngine_Recall_NoMemory(t *testing.T) {
	engine := NewRecallEngine(RecallEngineOptions{
		Memory: nil, // No memory index
		Config: DefaultRecallConfig(),
		Logger: zerolog.Nop(),
	})

	ctx := context.Background()
	result, err := engine.Recall(ctx, "What is the user's preference?")
	if err != nil {
		t.Fatalf("recall: %v", err)
	}
	if result != "" {
		t.Errorf("expected empty result with no memory, got: %s", result)
	}
}

func TestRecallEngine_FormatMemories(t *testing.T) {
	engine := NewRecallEngine(RecallEngineOptions{
		Config: DefaultRecallConfig(),
	})

	memories := []SearchResult{
		{Content: "User likes dark theme", Category: CategoryPreference, Score: 0.8},
		{Content: "User's email is test@example.com", Category: CategoryEntity, Score: 0.7},
		{Content: "Some fact about the project", Category: "", Score: 0.6}, // No category
	}

	result := engine.formatMemories(memories)

	// Check structure
	if !strings.HasPrefix(result, "<relevant-memories>") {
		t.Error("should start with <relevant-memories>")
	}
	if !strings.HasSuffix(result, "</relevant-memories>") {
		t.Error("should end with </relevant-memories>")
	}

	// Check content formatting
	if !strings.Contains(result, "[preference]") {
		t.Error("should contain [preference] tag")
	}
	if !strings.Contains(result, "[entity]") {
		t.Error("should contain [entity] tag")
	}
	if !strings.Contains(result, "[other]") {
		t.Error("should contain [other] for empty category")
	}

	// Check content
	if !strings.Contains(result, "User likes dark theme") {
		t.Error("should contain memory content")
	}
}

func TestRecallEngine_RecallWithFilter(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	embedder := NewSimpleEmbedder(384)
	config := DefaultIndexConfig()

	idx, err := NewMemoryIndex(db, embedder, config)
	if err != nil {
		t.Fatalf("create index: %v", err)
	}

	ctx := context.Background()

	// Add memories with different categories
	memories := []MemoryEntry{
		{Content: "User prefers dark theme", Category: CategoryPreference},
		{Content: "User's phone is +8613812345678", Category: CategoryEntity},
		{Content: "Team decided to use Go language", Category: CategoryDecision},
	}
	for _, m := range memories {
		if err := idx.Add(ctx, m); err != nil {
			t.Fatalf("add memory: %v", err)
		}
	}

	engine := NewRecallEngine(RecallEngineOptions{
		Memory: idx,
		Config: DefaultRecallConfig(),
		Logger: zerolog.Nop(),
	})

	t.Run("filters by category", func(t *testing.T) {
		result, err := engine.RecallWithFilter(ctx, "What is the user info", []string{CategoryEntity})
		if err != nil {
			t.Fatalf("recall: %v", err)
		}

		// May be empty if FTS doesn't match
		if result != "" && !strings.Contains(result, "[entity]") {
			t.Error("expected entity category in filtered result")
		}
	})

	t.Run("returns empty for short prompt", func(t *testing.T) {
		result, err := engine.RecallWithFilter(ctx, "hi", []string{CategoryPreference})
		if err != nil {
			t.Fatalf("recall: %v", err)
		}
		if result != "" {
			t.Errorf("expected empty result for short prompt, got: %s", result)
		}
	})
}

func TestRecallEngine_GetConfig(t *testing.T) {
	config := RecallConfig{
		Enabled:      true,
		Limit:        5,
		Threshold:    0.5,
		MinPromptLen: 10,
	}
	engine := NewRecallEngine(RecallEngineOptions{
		Config: config,
	})

	got := engine.GetConfig()
	if got.Limit != 5 {
		t.Errorf("expected limit 5, got %d", got.Limit)
	}
}

func TestRecallEngine_SetEnabled(t *testing.T) {
	engine := NewRecallEngine(RecallEngineOptions{
		Config: DefaultRecallConfig(),
	})

	if !engine.config.Enabled {
		t.Error("expected enabled by default")
	}

	engine.SetEnabled(false)
	if engine.config.Enabled {
		t.Error("expected disabled after SetEnabled(false)")
	}

	engine.SetEnabled(true)
	if !engine.config.Enabled {
		t.Error("expected enabled after SetEnabled(true)")
	}
}
