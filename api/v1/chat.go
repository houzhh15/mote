package v1

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
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

	"github.com/gorilla/mux"
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

	// Auto-detect image file paths in message text
	if imagePaths := extractImagePaths(chatReq.Message); len(imagePaths) > 0 {
		for _, imgPath := range imagePaths {
			att, err := buildAttachmentFromFile(imgPath)
			if err != nil {
				logger.Warn().Err(err).Str("path", imgPath).Msg("Failed to build attachment from image path")
				continue
			}
			attachments = append(attachments, att)
		}
		if len(attachments) > 0 {
			logger.Info().Int("count", len(attachments)).Msg("Auto-detected image attachments from message")
		}
	}

	// Provide a default message if only images are sent
	message := chatReq.Message
	if message == "" && len(attachments) > 0 {
		message = "请描述这张图片"
	}

	// Run agent and collect response
	var events <-chan runner.Event
	var err error
	if chatReq.TargetAgent != "" {
		// Direct delegate: bypass main agent LLM, route directly to sub-agent
		events, err = r.runner.RunDirectDelegate(ctx, sessionID, chatReq.TargetAgent, message)
	} else if chatReq.Model != "" {
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

// imageExtensions lists file extensions recognized as images.
var imageExtensions = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true,
	".webp": true, ".bmp": true, ".svg": true, ".tiff": true, ".tif": true,
}

// imagePathRegex matches absolute file paths ending in image extensions.
// Supports POSIX paths (/...) and home-relative paths (~/).
// Uses a simple greedy match — path existence is validated by extractImagePaths.
var imagePathRegex = regexp.MustCompile(`((?:/|~/)[^\s,，。！？\)]+\.(?:jpg|jpeg|png|gif|webp|bmp|svg|tiff|tif))`)

// extractImagePaths extracts image file paths from message text.
// Only returns paths that actually exist on disk and are image files.
// This allows users to simply paste file paths in their message to include images.
func extractImagePaths(message string) []string {
	matches := imagePathRegex.FindAllStringSubmatch(message, -1)
	var paths []string
	seen := make(map[string]bool)

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		path := match[1]

		// Expand ~ to home directory
		if strings.HasPrefix(path, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				path = home + path[1:]
			}
		}

		// Deduplicate
		if seen[path] {
			continue
		}
		seen[path] = true

		// Validate path safety
		if !isValidFilePath(path) {
			logger.Warn().Str("path", path).Msg("Skipping unsafe image path")
			continue
		}

		// Check file actually exists
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			logger.Debug().Str("path", path).Msg("Image path not found or is directory, skipping")
			continue
		}

		// Verify extension is an image
		ext := strings.ToLower(filepath.Ext(path))
		if !imageExtensions[ext] {
			continue
		}

		paths = append(paths, path)
		logger.Info().Str("path", path).Msg("Detected image file path in message")
	}

	return paths
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

// HandleCancelSession cancels a running chat session.
// This stops the runner execution for the given session.
func (r *Router) HandleCancelSession(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	sessionID := vars["id"]
	if sessionID == "" {
		handlers.SendError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "session ID required")
		return
	}

	if r.runner == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable, "Agent runner not available")
		return
	}

	r.runner.CancelSession(sessionID)
	logger.Info().Str("sessionID", sessionID).Msg("Session cancelled via API")
	handlers.SendJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
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

	// Auto-detect image file paths in message text
	if imagePaths := extractImagePaths(chatReq.Message); len(imagePaths) > 0 {
		for _, imgPath := range imagePaths {
			att, err := buildAttachmentFromFile(imgPath)
			if err != nil {
				logger.Warn().Err(err).Str("path", imgPath).Msg("Failed to build attachment from image path")
				continue
			}
			attachments = append(attachments, att)
		}
		if len(attachments) > 0 {
			logger.Info().Int("count", len(attachments)).Msg("Auto-detected image attachments from message")
		}
	}

	// Provide a default message if only images are sent
	message := chatReq.Message
	if message == "" && len(attachments) > 0 {
		message = "请描述这张图片"
	}

	// Run agent with streaming
	var events <-chan runner.Event
	var err error
	if chatReq.TargetAgent != "" {
		// Direct delegate: bypass main agent LLM, route directly to sub-agent
		events, err = r.runner.RunDirectDelegate(ctx, sessionID, chatReq.TargetAgent, message)
	} else if chatReq.Model != "" {
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
				logger.Debug().
					Str("sessionID", sessionID).
					Int("thinkingLen", len(event.Thinking)).
					Msg("SSE: sending thinking event")
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
			// Extract structured error detail from the error chain
			if event.Error != nil {
				var pe *provider.ProviderError
				if errors.As(event.Error, &pe) {
					sseEvent.ErrorDetail = &ErrorDetail{
						Code:       string(pe.Code),
						Message:    pe.Message,
						Provider:   pe.Provider,
						Retryable:  pe.Retryable,
						RetryAfter: pe.RetryAfter,
					}
				} else {
					sseEvent.ErrorDetail = &ErrorDetail{
						Code:      string(provider.ErrCodeUnknown),
						Message:   event.ErrorMsg,
						Retryable: false,
					}
				}
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
		case runner.EventTypeApprovalRequest:
			if event.ApprovalRequest != nil {
				sseEvent = ChatStreamEvent{
					Type: "approval_request",
					ApprovalRequest: &ApprovalRequestSSEEvent{
						ID:        event.ApprovalRequest.ID,
						ToolName:  event.ApprovalRequest.ToolName,
						Arguments: event.ApprovalRequest.Arguments,
						Reason:    event.ApprovalRequest.Reason,
						SessionID: event.ApprovalRequest.SessionID,
						ExpiresAt: event.ApprovalRequest.ExpiresAt,
					},
				}
			} else {
				continue
			}
		case runner.EventTypeApprovalResolved:
			if event.ApprovalResolved != nil {
				sseEvent = ChatStreamEvent{
					Type: "approval_resolved",
					ApprovalResolved: &ApprovalResolvedSSEEvent{
						ID:        event.ApprovalResolved.ID,
						Approved:  event.ApprovalResolved.Approved,
						DecidedAt: event.ApprovalResolved.DecidedAt,
					},
				}
			} else {
				continue
			}
		case runner.EventTypePDAProgress:
			if event.PDAProgress != nil {
				// Map parent steps
				var parentSteps []PDAParentStepSSE
				for _, ps := range event.PDAProgress.ParentSteps {
					parentSteps = append(parentSteps, PDAParentStepSSE{
						AgentName:  ps.AgentName,
						StepIndex:  ps.StepIndex,
						TotalSteps: ps.TotalSteps,
						StepLabel:  ps.StepLabel,
					})
				}
				sseEvent = ChatStreamEvent{
					Type: "pda_progress",
					PDAProgress: &PDAProgressSSEEvent{
						AgentName:     event.PDAProgress.AgentName,
						StepIndex:     event.PDAProgress.StepIndex,
						TotalSteps:    event.PDAProgress.TotalSteps,
						StepLabel:     event.PDAProgress.StepLabel,
						StepType:      event.PDAProgress.StepType,
						Phase:         event.PDAProgress.Phase,
						StackDepth:    event.PDAProgress.StackDepth,
						ExecutedSteps: event.PDAProgress.ExecutedSteps,
						TotalTokens:   event.PDAProgress.TotalTokens,
						Model:         event.PDAProgress.Model,
						ParentSteps:   parentSteps,
					},
				}
			} else {
				continue
			}
		default:
			continue
		}

		// Propagate sub-agent identity if present
		if event.AgentName != "" {
			sseEvent.AgentName = event.AgentName
			sseEvent.AgentDepth = event.AgentDepth
		}

		data, _ := json.Marshal(sseEvent)
		if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
			logger.Error().Err(err).Msg("Failed to write SSE event to client")
			// Drain remaining events to prevent goroutine leak.
			// Without this, the runner goroutine blocks on channel send
			// and can never finish, blocking the session queue.
			go func() {
				for range events {
				}
			}()
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
