package v1

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"mote/internal/gateway/handlers"
	"mote/internal/provider"
	"mote/internal/runner"
	"mote/pkg/logger"
)

// HandleChat handles synchronous chat requests.
func (r *Router) HandleChat(w http.ResponseWriter, req *http.Request) {
	var chatReq ChatRequest
	if err := json.NewDecoder(req.Body).Decode(&chatReq); err != nil {
		handlers.SendError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid JSON body")
		return
	}

	if chatReq.Message == "" && len(chatReq.Images) == 0 {
		handlers.SendError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Message or images required")
		return
	}

	if r.runner == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable, "Agent runner not available")
		return
	}

	ctx := req.Context()

	// Get or create session
	sessionID := chatReq.SessionID
	if sessionID == "" {
		sessionID = generateSessionID()
	}

	// Convert images to provider attachments
	var attachments []provider.Attachment
	if len(chatReq.Images) > 0 {
		attachments = buildAttachmentsFromImages(chatReq.Images)
	}

	// Provide a default message if only images are sent
	message := chatReq.Message
	if message == "" && len(attachments) > 0 {
		message = "请描述这张图片"
	}

	// Run agent and collect response
	var events <-chan runner.Event
	var err error
	if chatReq.Model != "" {
		events, err = r.runner.RunWithModel(ctx, sessionID, message, chatReq.Model, "chat", attachments...)
	} else {
		events, err = r.runner.Run(ctx, sessionID, message, attachments...)
	}
	if err != nil {
		handlers.SendError(w, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}

	// Collect events into response
	var responseMsg string
	var toolCalls []ToolCallResult

	for event := range events {
		switch event.Type {
		case runner.EventTypeContent:
			responseMsg += event.Content
		case runner.EventTypeToolResult:
			if event.ToolResult != nil {
				toolCalls = append(toolCalls, ToolCallResult{
					Name:   event.ToolResult.ToolName,
					Result: event.ToolResult.Output,
				})
			}
		case runner.EventTypeError:
			handlers.SendError(w, http.StatusInternalServerError, ErrCodeInternalError, event.ErrorMsg)
			return
		}
	}

	handlers.SendJSON(w, http.StatusOK, ChatResponse{
		SessionID: sessionID,
		Message:   responseMsg,
		ToolCalls: toolCalls,
	})
}

// parseFileReferences extracts @filepath references from message.
// Returns the original message and a list of file paths.
func parseFileReferences(message string) (cleanMessage string, fileRefs []string) {
	// Regex: match @(non-whitespace) at word boundaries (start, space, newline)
	// Negative lookbehind for alphanumeric to avoid matching email addresses
	re := regexp.MustCompile(`(?:^|\s)@([^\s@]+)`)
	matches := re.FindAllStringSubmatch(message, -1)

	for _, match := range matches {
		if len(match) > 1 {
			filepath := match[1]
			// Validate path safety
			if isValidFilePath(filepath) {
				fileRefs = append(fileRefs, filepath)
			} else {
				logger.Warn().Str("path", filepath).Msg("Invalid file path in reference")
			}
		}
	}

	// Keep original message (including @ references for UI display)
	cleanMessage = message
	return
}

// isValidFilePath validates file path for security.
func isValidFilePath(path string) bool {
	// 1. Prevent path traversal
	if strings.Contains(path, "..") {
		return false
	}

	// 2. Block sensitive files (check filename only, not full path)
	filename := filepath.Base(path)
	filenameLower := strings.ToLower(filename)

	sensitivePaths := []string{".env", "id_rsa", ".ssh", "password", "secret", "private_key"}
	for _, s := range sensitivePaths {
		if strings.Contains(filenameLower, s) {
			return false
		}
	}

	return true
}

// buildAttachmentFromFile reads a file and constructs an Attachment.
func buildAttachmentFromFile(filePath string) (provider.Attachment, error) {
	// 1. Read file
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return provider.Attachment{}, fmt.Errorf("file not found: %s", filePath)
		}
		if os.IsPermission(err) {
			return provider.Attachment{}, fmt.Errorf("permission denied: %s", filePath)
		}
		return provider.Attachment{}, fmt.Errorf("read file: %w", err)
	}

	// 2. Check file size (10MB limit)
	const maxFileSize = 10 * 1024 * 1024
	if len(data) > maxFileSize {
		return provider.Attachment{}, fmt.Errorf("file too large: %s (%.1f MB, limit: 10 MB)",
			filePath, float64(len(data))/1024/1024)
	}

	// 3. Detect MIME type
	mimeType := detectMimeType(filePath, data)

	// 4. Construct attachment
	attachment := provider.Attachment{
		Filepath: filePath,
		Filename: filepath.Base(filePath),
		MimeType: mimeType,
		Size:     len(data),
	}

	// 5. Process based on type
	if strings.HasPrefix(mimeType, "image/") {
		// Image: Base64 encode
		attachment.Type = "image_url"
		attachment.ImageURL = &provider.ImageURL{
			URL: fmt.Sprintf("data:%s;base64,%s",
				mimeType,
				base64.StdEncoding.EncodeToString(data)),
		}
	} else {
		// Code/text: direct text
		attachment.Type = "text"
		attachment.Text = string(data)

		// Add metadata
		attachment.Metadata = map[string]any{
			"filepath": filePath,
			"language": detectLanguage(filePath),
		}
	}

	return attachment, nil
}

// buildAttachmentsFromImages converts ImageData from the API request to provider Attachments.
func buildAttachmentsFromImages(images []ImageData) []provider.Attachment {
	var attachments []provider.Attachment
	for _, img := range images {
		// Validate MIME type
		if !strings.HasPrefix(img.MimeType, "image/") {
			continue
		}
		// Validate base64 data is not empty
		if img.Data == "" {
			continue
		}
		// Build data URI
		dataURI := fmt.Sprintf("data:%s;base64,%s", img.MimeType, img.Data)
		attachments = append(attachments, provider.Attachment{
			Type:     "image_url",
			ImageURL: &provider.ImageURL{URL: dataURI},
			MimeType: img.MimeType,
			Filename: img.Name,
		})
	}
	return attachments
}
func detectMimeType(filePath string, data []byte) string {
	// Extension-based detection
	ext := strings.ToLower(filepath.Ext(filePath))
	extToMime := map[string]string{
		".png":  "image/png",
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".gif":  "image/gif",
		".webp": "image/webp",
		".svg":  "image/svg+xml",
		".bmp":  "image/bmp",
		".js":   "text/javascript",
		".ts":   "text/typescript",
		".tsx":  "text/typescript",
		".jsx":  "text/javascript",
		".go":   "text/x-go",
		".py":   "text/x-python",
		".java": "text/x-java",
		".cpp":  "text/x-c++",
		".c":    "text/x-c",
		".rs":   "text/x-rust",
		".md":   "text/markdown",
		".json": "application/json",
		".yaml": "text/yaml",
		".yml":  "text/yaml",
		".xml":  "text/xml",
		".txt":  "text/plain",
	}

	if mime, ok := extToMime[ext]; ok {
		return mime
	}

	// Fallback: content-based detection
	return http.DetectContentType(data)
}

// detectLanguage detects programming language from file extension.
func detectLanguage(filePath string) string {
	ext := strings.ToLower(filepath.Ext(filePath))
	langMap := map[string]string{
		".go":    "go",
		".js":    "javascript",
		".ts":    "typescript",
		".tsx":   "typescript",
		".jsx":   "javascript",
		".py":    "python",
		".java":  "java",
		".cpp":   "cpp",
		".c":     "c",
		".h":     "c",
		".hpp":   "cpp",
		".rs":    "rust",
		".rb":    "ruby",
		".php":   "php",
		".swift": "swift",
		".kt":    "kotlin",
		".cs":    "csharp",
		".sh":    "bash",
		".bash":  "bash",
		".md":    "markdown",
		".json":  "json",
		".yaml":  "yaml",
		".yml":   "yaml",
		".xml":   "xml",
	}

	if lang, ok := langMap[ext]; ok {
		return lang
	}

	return "text"
}

// HandleChatStream handles streaming chat requests using SSE.
func (r *Router) HandleChatStream(w http.ResponseWriter, req *http.Request) {
	var chatReq ChatRequest
	if err := json.NewDecoder(req.Body).Decode(&chatReq); err != nil {
		handlers.SendError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid JSON body")
		return
	}

	if chatReq.Message == "" && len(chatReq.Images) == 0 {
		handlers.SendError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Message or images required")
		return
	}

	if r.runner == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable, "Agent runner not available")
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		handlers.SendError(w, http.StatusInternalServerError, ErrCodeInternalError, "Streaming not supported")
		return
	}

	ctx := req.Context()

	// Get or create session
	sessionID := chatReq.SessionID
	if sessionID == "" {
		sessionID = generateSessionID()
	}

	// Create a context with timeout (30 minutes for complex tasks)
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	// Convert images to provider attachments
	var attachments []provider.Attachment
	if len(chatReq.Images) > 0 {
		attachments = buildAttachmentsFromImages(chatReq.Images)
	}

	// Provide a default message if only images are sent
	message := chatReq.Message
	if message == "" && len(attachments) > 0 {
		message = "请描述这张图片"
	}

	// Run agent with streaming
	var events <-chan runner.Event
	var err error
	if chatReq.Model != "" {
		events, err = r.runner.RunWithModel(ctx, sessionID, message, chatReq.Model, "chat", attachments...)
	} else {
		events, err = r.runner.Run(ctx, sessionID, message, attachments...)
	}
	if err != nil {
		sendSSEError(w, flusher, err)
		return
	}

	for event := range events {
		var sseEvent ChatStreamEvent

		switch event.Type {
		case runner.EventTypeContent:
			sseEvent = ChatStreamEvent{
				Type:  "content",
				Delta: event.Content,
			}
		case runner.EventTypeThinking:
			// Send thinking event for temporary display
			if event.Thinking != "" {
				sseEvent = ChatStreamEvent{
					Type:     "thinking",
					Thinking: event.Thinking,
				}
			} else {
				continue
			}
		case runner.EventTypeToolCall:
			if event.ToolCall != nil {
				sseEvent = ChatStreamEvent{
					Type: "tool_call",
					ToolCall: &ToolCallResult{
						Name:      event.ToolCall.GetName(),
						Arguments: event.ToolCall.GetArguments(),
					},
				}
			} else {
				continue
			}
		case runner.EventTypeToolCallUpdate:
			// Send tool call update event for progress display
			if event.ToolCallUpdate != nil {
				sseEvent = ChatStreamEvent{
					Type: "tool_call_update",
					ToolCallUpdate: &ToolCallUpdateEvent{
						ToolCallID: event.ToolCallUpdate.ToolCallID,
						ToolName:   event.ToolCallUpdate.ToolName,
						Status:     event.ToolCallUpdate.Status,
						Arguments:  event.ToolCallUpdate.Arguments,
					},
				}
			} else {
				continue
			}
		case runner.EventTypeToolResult:
			if event.ToolResult != nil {
				sseEvent = ChatStreamEvent{
					Type: "tool_result",
					ToolResult: &ToolResultEvent{
						ToolCallID: event.ToolResult.ToolCallID,
						ToolName:   event.ToolResult.ToolName,
						Output:     event.ToolResult.Output,
						IsError:    event.ToolResult.IsError,
						DurationMs: event.ToolResult.DurationMs,
					},
				}
			} else {
				continue
			}
		case runner.EventTypeError:
			sseEvent = ChatStreamEvent{
				Type:  "error",
				Error: event.ErrorMsg,
			}
		case runner.EventTypeTruncated:
			// Response was truncated due to max_tokens limit
			sseEvent = ChatStreamEvent{
				Type:             "truncated",
				TruncatedReason:  event.TruncatedReason,
				PendingToolCalls: event.PendingToolCalls,
				SessionID:        sessionID,
			}
			logger.Warn().
				Str("sessionID", sessionID).
				Str("reason", event.TruncatedReason).
				Int("pendingToolCalls", event.PendingToolCalls).
				Msg("Response truncated, user can choose to continue")
		case runner.EventTypeDone:
			// Skip internal done event, we'll send our own
			continue
		case runner.EventTypeHeartbeat:
			// Send heartbeat to keep connection alive
			sseEvent = ChatStreamEvent{
				Type: "heartbeat",
			}
			// Log heartbeat being sent to client
			logger.Info().Msg("Sending heartbeat to client")
		default:
			continue
		}

		data, _ := json.Marshal(sseEvent)
		if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
			logger.Error().Err(err).Msg("Failed to write SSE event to client")
			return
		}
		flusher.Flush()
	}

	// Send done event
	doneEvent := ChatStreamEvent{
		Type:      "done",
		SessionID: sessionID,
	}
	data, _ := json.Marshal(doneEvent)
	fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()
}

// sendSSEError sends an error event via SSE.
func sendSSEError(w http.ResponseWriter, flusher http.Flusher, err error) {
	errEvent := ChatStreamEvent{
		Type:  "error",
		Error: err.Error(), // Legacy field for backward compatibility
	}

	// Try to extract detailed error info from ProviderError
	if pe, ok := err.(*provider.ProviderError); ok {
		errEvent.ErrorDetail = &ErrorDetail{
			Code:       string(pe.Code),
			Message:    pe.Message,
			Provider:   pe.Provider,
			Retryable:  pe.Retryable,
			RetryAfter: pe.RetryAfter,
		}
	} else {
		// Generic error - classify it
		errEvent.ErrorDetail = &ErrorDetail{
			Code:      string(provider.ErrCodeUnknown),
			Message:   err.Error(),
			Retryable: false,
		}
	}

	data, _ := json.Marshal(errEvent)
	fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()
}
