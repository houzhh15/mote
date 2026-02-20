package runner

import (
	"strings"
	"testing"
)

func TestTruncateToolResult_Small(t *testing.T) {
	input := "small result"
	got := TruncateToolResult(input, DefaultMaxToolResultBytes)
	if got != input {
		t.Fatalf("expected no change for small input, got %q", got)
	}
}

func TestTruncateToolResult_ExactLimit(t *testing.T) {
	input := strings.Repeat("a", DefaultMaxToolResultBytes)
	got := TruncateToolResult(input, DefaultMaxToolResultBytes)
	if got != input {
		t.Fatal("expected no change when input == limit")
	}
}

func TestTruncateToolResult_Oversized(t *testing.T) {
	input := strings.Repeat("x", 200000)
	got := TruncateToolResult(input, 1000)
	if len(got) > 1200 { // allow some overhead for marker
		t.Fatalf("expected truncated output <= ~1200 bytes, got %d", len(got))
	}
	if !strings.Contains(got, "bytes truncated") {
		t.Fatal("expected truncation marker in output")
	}
}

func TestTruncateToolResult_Base64Stripping(t *testing.T) {
	// Build a large base64 block (>64 base64 chars after the comma)
	b64Payload := strings.Repeat("ABCD", 100) // 400 base64 chars
	b64 := "data:image/png;base64," + b64Payload
	input := "prefix " + b64 + " suffix"
	// Use maxBytes smaller than input to trigger stripping pipeline
	got := TruncateToolResult(input, 100)
	if strings.Contains(got, b64Payload) {
		t.Fatal("expected base64 block to be stripped")
	}
	if !strings.Contains(got, "base64 data removed") {
		t.Fatal("expected base64 replacement marker")
	}
	if !strings.Contains(got, "prefix") || !strings.Contains(got, "suffix") {
		t.Fatal("expected surrounding text to be preserved")
	}
}

func TestTruncateToolResult_HexBlobStripping(t *testing.T) {
	hex := strings.Repeat("0123456789abcdef", 20) // 320 hex chars
	input := "start " + hex + " end"
	// Use maxBytes smaller than input to trigger stripping pipeline
	got := TruncateToolResult(input, 100)
	if strings.Contains(got, hex) {
		t.Fatal("expected hex blob to be stripped")
	}
	if !strings.Contains(got, "hex data removed") {
		t.Fatal("expected hex replacement marker")
	}
}

func TestTruncateToolResult_NoFalsePositiveHex(t *testing.T) {
	// Short hex strings should not be stripped
	input := "hash: abcdef123456"
	got := TruncateToolResult(input, DefaultMaxToolResultBytes)
	if got != input {
		t.Fatalf("short hex should not be stripped, got %q", got)
	}
}

func TestTruncateToolResult_HeadTailSplit(t *testing.T) {
	// Create input just over the limit with no base64/hex
	maxBytes := 100
	input := strings.Repeat("H", 50) + strings.Repeat("M", 50) + strings.Repeat("T", 50)
	got := TruncateToolResult(input, maxBytes)
	// Should contain head portion and tail portion
	if !strings.HasPrefix(got, "HH") {
		t.Fatal("expected head to start with H characters")
	}
	if !strings.HasSuffix(got, "TT") {
		t.Fatal("expected tail to end with T characters")
	}
	if !strings.Contains(got, "bytes truncated") {
		t.Fatal("expected truncation marker")
	}
}
