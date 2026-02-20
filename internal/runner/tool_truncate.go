package runner

import (
	"fmt"
	"regexp"
)

const (
	// DefaultMaxToolResultBytes is the default maximum size for tool results
	// before truncation.  64 KB is enough for most useful tool output while
	// preventing huge HTTP responses from bloating the context.
	DefaultMaxToolResultBytes = 65536
)

var (
	// base64Pattern matches inline data URIs: data:...;base64,...
	// Captures the full data URI (base64 chars only, no whitespace before terminator).
	base64Pattern = regexp.MustCompile(`data:[a-zA-Z0-9+/=\-]+;base64,[A-Za-z0-9+/=]{64,}`)

	// hexBlobPattern matches contiguous hex strings longer than 256 characters.
	hexBlobPattern = regexp.MustCompile(`[0-9a-fA-F]{256,}`)
)

// TruncateToolResult truncates an oversized tool result to fit within
// maxBytes.  Processing order:
//  1. Strip inline base64 data URIs
//  2. Strip large hex blobs
//  3. If still oversized, keep head + tail and insert a truncation marker
//
// Returns the original content unchanged if it fits within maxBytes.
func TruncateToolResult(content string, maxBytes int) string {
	if len(content) <= maxBytes {
		return content
	}

	// Step 1: strip base64 blocks
	content = stripBase64Blocks(content)
	if len(content) <= maxBytes {
		return content
	}

	// Step 2: strip hex blobs
	content = stripHexBlobs(content)
	if len(content) <= maxBytes {
		return content
	}

	// Step 3: keep head + tail, truncate middle
	headLen := maxBytes * 2 / 5
	tailLen := maxBytes * 2 / 5
	if headLen+tailLen >= len(content) {
		return content
	}

	removed := len(content) - headLen - tailLen
	return content[:headLen] +
		fmt.Sprintf("\n\n[... %d bytes truncated ...]\n\n", removed) +
		content[len(content)-tailLen:]
}

// stripBase64Blocks replaces inline base64 data URIs with a short placeholder
// indicating the original byte count.
func stripBase64Blocks(s string) string {
	return base64Pattern.ReplaceAllStringFunc(s, func(match string) string {
		return fmt.Sprintf("[base64 data removed, %d bytes]", len(match))
	})
}

// stripHexBlobs replaces contiguous hex strings (>= 256 chars) with a short
// placeholder.
func stripHexBlobs(s string) string {
	return hexBlobPattern.ReplaceAllStringFunc(s, func(match string) string {
		return fmt.Sprintf("[hex data removed, %d bytes]", len(match))
	})
}
