package memory

import (
	"context"
	"math"
	"testing"
)

func TestSimpleEmbedder_Embed(t *testing.T) {
	e := NewSimpleEmbedder(384)

	t.Run("correct dimensions", func(t *testing.T) {
		vec, err := e.Embed(context.Background(), "hello world")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(vec) != 384 {
			t.Errorf("expected 384 dimensions, got %d", len(vec))
		}
	})

	t.Run("deterministic output", func(t *testing.T) {
		text := "test input"
		vec1, _ := e.Embed(context.Background(), text)
		vec2, _ := e.Embed(context.Background(), text)

		for i := range vec1 {
			if vec1[i] != vec2[i] {
				t.Errorf("vectors differ at index %d: %f != %f", i, vec1[i], vec2[i])
				break
			}
		}
	})

	t.Run("different texts produce different vectors", func(t *testing.T) {
		vec1, _ := e.Embed(context.Background(), "hello")
		vec2, _ := e.Embed(context.Background(), "world")

		same := true
		for i := range vec1 {
			if vec1[i] != vec2[i] {
				same = false
				break
			}
		}
		if same {
			t.Error("expected different vectors for different texts")
		}
	})

	t.Run("empty text returns zero vector", func(t *testing.T) {
		vec, _ := e.Embed(context.Background(), "")
		for i, v := range vec {
			if v != 0 {
				t.Errorf("expected zero at index %d, got %f", i, v)
				break
			}
		}
	})

	t.Run("normalized vector", func(t *testing.T) {
		vec, _ := e.Embed(context.Background(), "test normalization")
		var sum float64
		for _, v := range vec {
			sum += float64(v) * float64(v)
		}
		norm := math.Sqrt(sum)
		if math.Abs(norm-1.0) > 0.0001 {
			t.Errorf("vector not normalized: norm = %f", norm)
		}
	})
}

func TestSimpleEmbedder_EmbedBatch(t *testing.T) {
	e := NewSimpleEmbedder(128)

	texts := []string{"hello", "world", "test"}
	results, err := e.EmbedBatch(context.Background(), texts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != len(texts) {
		t.Errorf("expected %d results, got %d", len(texts), len(results))
	}

	for i, vec := range results {
		if len(vec) != 128 {
			t.Errorf("result %d has wrong dimensions: %d", i, len(vec))
		}
	}
}

func TestSimpleEmbedder_Dimensions(t *testing.T) {
	tests := []struct {
		input    int
		expected int
	}{
		{384, 384},
		{128, 128},
		{0, 384},  // default
		{-1, 384}, // default
	}

	for _, tt := range tests {
		e := NewSimpleEmbedder(tt.input)
		if e.Dimensions() != tt.expected {
			t.Errorf("NewSimpleEmbedder(%d).Dimensions() = %d, want %d", tt.input, e.Dimensions(), tt.expected)
		}
	}
}
