package memory

import (
	"context"
	"testing"
)

func TestBatchEmbedder_EmptyEntries(t *testing.T) {
	embedder := NewSimpleEmbedder(384)
	config := DefaultBatchConfig()
	be := NewBatchEmbedder(embedder, config, testLogger())

	result, err := be.EmbedBatch(context.Background(), nil)
	if err != nil {
		t.Fatalf("embed: %v", err)
	}

	if result.Succeeded != 0 {
		t.Errorf("expected 0 succeeded, got %d", result.Succeeded)
	}
}

func TestBatchEmbedder_SingleEntry(t *testing.T) {
	embedder := NewSimpleEmbedder(384)
	config := DefaultBatchConfig()
	be := NewBatchEmbedder(embedder, config, testLogger())

	entries := []MemoryEntry{
		{ID: "test-1", Content: "Hello world"},
	}

	result, err := be.EmbedBatch(context.Background(), entries)
	if err != nil {
		t.Fatalf("embed: %v", err)
	}

	if result.Succeeded != 1 {
		t.Errorf("expected 1 succeeded, got %d", result.Succeeded)
	}
	if len(result.Failed) != 0 {
		t.Errorf("expected 0 failed, got %d", len(result.Failed))
	}
}

func TestBatchEmbedder_MultipleBatches(t *testing.T) {
	embedder := NewSimpleEmbedder(384)
	config := BatchConfig{BatchSize: 2, Workers: 2}
	be := NewBatchEmbedder(embedder, config, testLogger())

	entries := make([]MemoryEntry, 5)
	for i := range entries {
		entries[i] = MemoryEntry{
			ID:      "batch-" + string(rune('A'+i)),
			Content: "Test content " + string(rune('A'+i)),
		}
	}

	result, err := be.EmbedBatch(context.Background(), entries)
	if err != nil {
		t.Fatalf("embed: %v", err)
	}

	if result.Succeeded != 5 {
		t.Errorf("expected 5 succeeded, got %d", result.Succeeded)
	}
}

func TestSplitIntoBatches(t *testing.T) {
	entries := make([]MemoryEntry, 7)
	for i := range entries {
		entries[i].ID = "e" + string(rune('0'+i))
	}

	batches := splitIntoBatches(entries, 3)
	if len(batches) != 3 {
		t.Errorf("expected 3 batches, got %d", len(batches))
	}
	if len(batches[0]) != 3 {
		t.Errorf("batch[0]: expected 3, got %d", len(batches[0]))
	}
	if len(batches[2]) != 1 {
		t.Errorf("batch[2]: expected 1, got %d", len(batches[2]))
	}
}
