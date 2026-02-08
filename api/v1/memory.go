package v1

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"

	"mote/internal/gateway/handlers"
	"mote/internal/memory"
)

// HandleMemorySearch searches memories by semantic similarity.
func (r *Router) HandleMemorySearch(w http.ResponseWriter, req *http.Request) {
	if r.memory == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable, "Memory index not available")
		return
	}

	var searchReq MemorySearchRequest
	if err := json.NewDecoder(req.Body).Decode(&searchReq); err != nil {
		handlers.SendError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid JSON body")
		return
	}

	if searchReq.Query == "" {
		handlers.SendError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Query is required")
		return
	}

	topK := searchReq.TopK
	if topK <= 0 {
		topK = 10
	}

	ctx := req.Context()
	results, err := r.memory.Search(ctx, searchReq.Query, topK)
	if err != nil {
		handlers.SendError(w, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}

	// P2: Apply category and importance filters
	var memoryResults []MemoryResult
	for _, m := range results {
		// Filter by categories if specified
		if len(searchReq.Categories) > 0 {
			matched := false
			for _, cat := range searchReq.Categories {
				if m.Category == cat {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}

		// Filter by minimum importance if specified
		if searchReq.MinImportance > 0 && m.Importance < searchReq.MinImportance {
			continue
		}

		memoryResults = append(memoryResults, MemoryResult{
			ID:            m.ID,
			Content:       m.Content,
			Score:         m.Score,
			Source:        m.Source,
			CreatedAt:     m.CreatedAt.Format(time.RFC3339),
			Category:      m.Category,
			Importance:    m.Importance,
			CaptureMethod: m.CaptureMethod,
			ChunkIndex:    m.ChunkIndex,
			ChunkTotal:    m.ChunkTotal,
			SourceFile:    m.SourceFile,
		})
	}

	if memoryResults == nil {
		memoryResults = []MemoryResult{}
	}

	handlers.SendJSON(w, http.StatusOK, MemorySearchResponse{
		Results: memoryResults,
	})
}

// HandleAddMemory adds a new memory entry.
func (r *Router) HandleAddMemory(w http.ResponseWriter, req *http.Request) {
	if r.memory == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable, "Memory index not available")
		return
	}

	var addReq AddMemoryRequest
	if err := json.NewDecoder(req.Body).Decode(&addReq); err != nil {
		handlers.SendError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid JSON body")
		return
	}

	if addReq.Content == "" {
		handlers.SendError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Content is required")
		return
	}

	source := addReq.Source
	if source == "" {
		source = "api"
	}

	// P2: Auto-detect category if not provided
	category := addReq.Category
	if category == "" {
		detector, _ := memory.NewCategoryDetector()
		if detector != nil {
			category = detector.Detect(addReq.Content)
		} else {
			category = memory.CategoryOther
		}
	}

	// P2: Use default importance if not provided
	importance := addReq.Importance
	if importance <= 0 {
		importance = memory.DefaultImportance
	}

	ctx := req.Context()
	id := uuid.New().String() // Pre-generate ID so we can return it
	entry := memory.MemoryEntry{
		ID:            id,
		Content:       addReq.Content,
		Source:        source,
		Category:      category,
		Importance:    importance,
		CaptureMethod: memory.CaptureMethodManual,
	}

	if err := r.memory.Add(ctx, entry); err != nil {
		handlers.SendError(w, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}

	handlers.SendJSON(w, http.StatusCreated, AddMemoryResponse{
		ID:       id,
		Category: category,
	})
}

// HandleDeleteMemory deletes a memory entry.
func (r *Router) HandleDeleteMemory(w http.ResponseWriter, req *http.Request) {
	if r.memory == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable, "Memory index not available")
		return
	}

	vars := mux.Vars(req)
	id := vars["id"]

	ctx := req.Context()
	if err := r.memory.Delete(ctx, id); err != nil {
		handlers.SendError(w, http.StatusNotFound, ErrCodeNotFound, "Memory not found")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// UpdateMemoryRequest represents a request to update a memory entry.
type UpdateMemoryRequest struct {
	Content    string  `json:"content"`
	Category   string  `json:"category,omitempty"`
	Importance float64 `json:"importance,omitempty"`
}

// HandleUpdateMemory updates an existing memory entry.
func (r *Router) HandleUpdateMemory(w http.ResponseWriter, req *http.Request) {
	if r.memory == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable, "Memory index not available")
		return
	}

	vars := mux.Vars(req)
	id := vars["id"]
	if id == "" {
		handlers.SendError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Memory ID is required")
		return
	}

	var updateReq UpdateMemoryRequest
	if err := json.NewDecoder(req.Body).Decode(&updateReq); err != nil {
		handlers.SendError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid JSON body")
		return
	}

	if updateReq.Content == "" {
		handlers.SendError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Content is required")
		return
	}

	ctx := req.Context()

	// Get existing entry to preserve fields
	existing, err := r.memory.GetByID(ctx, id)
	if err != nil {
		if err == memory.ErrMemoryNotFound {
			handlers.SendError(w, http.StatusNotFound, ErrCodeNotFound, "Memory not found")
			return
		}
		handlers.SendError(w, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}

	// Update fields
	existing.Content = updateReq.Content
	if updateReq.Category != "" {
		existing.Category = updateReq.Category
	}
	if updateReq.Importance > 0 {
		existing.Importance = updateReq.Importance
	}

	// Delete old and add updated (since we don't have a direct update method)
	if err := r.memory.Delete(ctx, id); err != nil {
		handlers.SendError(w, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}

	if err := r.memory.Add(ctx, *existing); err != nil {
		handlers.SendError(w, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}

	handlers.SendJSON(w, http.StatusOK, MemoryEntryResponse{
		ID:            existing.ID,
		Content:       existing.Content,
		Source:        existing.Source,
		CreatedAt:     existing.CreatedAt.Format(time.RFC3339),
		Category:      existing.Category,
		Importance:    existing.Importance,
		CaptureMethod: existing.CaptureMethod,
	})
}

// HandleGetMemory retrieves a single memory entry by ID.
func (r *Router) HandleGetMemory(w http.ResponseWriter, req *http.Request) {
	if r.memory == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable, "Memory index not available")
		return
	}

	vars := mux.Vars(req)
	id := vars["id"]
	if id == "" {
		handlers.SendError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Memory ID is required")
		return
	}

	ctx := req.Context()
	entry, err := r.memory.GetByID(ctx, id)
	if err != nil {
		if err == memory.ErrMemoryNotFound {
			handlers.SendError(w, http.StatusNotFound, ErrCodeNotFound, "Memory not found")
			return
		}
		handlers.SendError(w, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}

	handlers.SendJSON(w, http.StatusOK, MemoryEntryResponse{
		ID:            entry.ID,
		Content:       entry.Content,
		Source:        entry.Source,
		CreatedAt:     entry.CreatedAt.Format(time.RFC3339),
		Metadata:      entry.Metadata,
		Category:      entry.Category,
		Importance:    entry.Importance,
		CaptureMethod: entry.CaptureMethod,
	})
}

// HandleListMemory returns all memories with pagination.
func (r *Router) HandleListMemory(w http.ResponseWriter, req *http.Request) {
	if r.memory == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable, "Memory index not available")
		return
	}

	limit := 100
	offset := 0
	// TODO: parse limit/offset from query params if needed

	ctx := req.Context()
	results, err := r.memory.List(ctx, limit, offset)
	if err != nil {
		handlers.SendError(w, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}

	var memoryResults []MemoryResult
	for _, m := range results {
		memoryResults = append(memoryResults, MemoryResult{
			ID:            m.ID,
			Content:       m.Content,
			Score:         m.Score,
			Source:        m.Source,
			CreatedAt:     m.CreatedAt.Format(time.RFC3339),
			Category:      m.Category,
			Importance:    m.Importance,
			CaptureMethod: m.CaptureMethod,
			ChunkIndex:    m.ChunkIndex,
			ChunkTotal:    m.ChunkTotal,
			SourceFile:    m.SourceFile,
		})
	}

	if memoryResults == nil {
		memoryResults = []MemoryResult{}
	}

	handlers.SendJSON(w, http.StatusOK, MemoryListResponse{
		Memories: memoryResults,
	})
}

// Searcher is an interface for memory search operations.
type Searcher interface {
	Search(ctx context.Context, query string, topK int) ([]memory.SearchResult, error)
}

// Adder is an interface for memory add operations.
type Adder interface {
	Add(ctx context.Context, entry memory.MemoryEntry) error
}

// Deleter is an interface for memory delete operations.
type Deleter interface {
	Delete(ctx context.Context, id string) error
}

// HandleMemorySync handles POST /memory/sync - rebuilds index from markdown files.
func (r *Router) HandleMemorySync(w http.ResponseWriter, req *http.Request) {
	if r.memory == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable, "Memory index not available")
		return
	}

	var syncReq struct {
		Force bool `json:"force"`
	}
	// Decode body if present (optional)
	json.NewDecoder(req.Body).Decode(&syncReq)

	ctx := req.Context()
	count, err := r.memory.SyncFromMarkdown(ctx, syncReq.Force)
	if err != nil {
		handlers.SendError(w, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}

	handlers.SendJSON(w, http.StatusOK, map[string]any{
		"synced": count,
		"status": "ok",
	})
}

// HandleGetDaily handles GET /memory/daily - gets today's or a specific date's log.
func (r *Router) HandleGetDaily(w http.ResponseWriter, req *http.Request) {
	if r.memory == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable, "Memory index not available")
		return
	}

	dateStr := req.URL.Query().Get("date")
	var date time.Time
	var err error

	if dateStr != "" {
		date, err = time.Parse("2006-01-02", dateStr)
		if err != nil {
			handlers.SendError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid date format, use YYYY-MM-DD")
			return
		}
	} else {
		date = time.Now()
	}

	content, err := r.memory.GetDailyLog(req.Context(), date)
	if err != nil {
		handlers.SendError(w, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}

	handlers.SendJSON(w, http.StatusOK, map[string]any{
		"date":    date.Format("2006-01-02"),
		"content": content,
	})
}

// HandleAppendDaily handles POST /memory/daily - appends to today's log.
func (r *Router) HandleAppendDaily(w http.ResponseWriter, req *http.Request) {
	if r.memory == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable, "Memory index not available")
		return
	}

	var appendReq struct {
		Content string `json:"content"`
		Section string `json:"section"`
	}
	if err := json.NewDecoder(req.Body).Decode(&appendReq); err != nil {
		handlers.SendError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid JSON body")
		return
	}

	if appendReq.Content == "" {
		handlers.SendError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Content is required")
		return
	}

	if err := r.memory.AppendDailyLog(req.Context(), appendReq.Content, appendReq.Section); err != nil {
		handlers.SendError(w, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}

	handlers.SendJSON(w, http.StatusCreated, map[string]any{
		"status": "ok",
		"date":   time.Now().Format("2006-01-02"),
	})
}

// HandleMemoryExport handles GET /memory/export - exports memories.
func (r *Router) HandleMemoryExport(w http.ResponseWriter, req *http.Request) {
	if r.memory == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable, "Memory index not available")
		return
	}

	format := req.URL.Query().Get("format")
	if format == "" {
		format = "json"
	}

	ctx := req.Context()
	entries, err := r.memory.List(ctx, 10000, 0) // Export up to 10k entries
	if err != nil {
		handlers.SendError(w, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}

	if format == "markdown" {
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Header().Set("Content-Disposition", "attachment; filename=memory-export.md")
		w.WriteHeader(http.StatusOK)

		w.Write([]byte("# Memory Export\n\n"))
		w.Write([]byte("Exported at: " + time.Now().Format(time.RFC3339) + "\n\n"))

		for _, entry := range entries {
			w.Write([]byte("---\n\n"))
			w.Write([]byte("## " + entry.ID + "\n\n"))
			w.Write([]byte("**Source:** " + entry.Source + "\n"))
			w.Write([]byte("**Created:** " + entry.CreatedAt.Format(time.RFC3339) + "\n\n"))
			w.Write([]byte(entry.Content + "\n\n"))
		}
		return
	}

	// Default: JSON export
	var exportEntries []map[string]any
	for _, entry := range entries {
		exportEntries = append(exportEntries, map[string]any{
			"id":         entry.ID,
			"content":    entry.Content,
			"source":     entry.Source,
			"created_at": entry.CreatedAt.Format(time.RFC3339),
		})
	}

	handlers.SendJSON(w, http.StatusOK, map[string]any{
		"count":    len(exportEntries),
		"exported": time.Now().Format(time.RFC3339),
		"memories": exportEntries,
	})
}

// HandleMemoryImport handles POST /memory/import - imports memories from file.
func (r *Router) HandleMemoryImport(w http.ResponseWriter, req *http.Request) {
	if r.memory == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable, "Memory index not available")
		return
	}

	var importReq struct {
		Memories []struct {
			Content string `json:"content"`
			Source  string `json:"source"`
		} `json:"memories"`
		Mode string `json:"mode"` // "merge" or "replace"
	}

	if err := json.NewDecoder(req.Body).Decode(&importReq); err != nil {
		handlers.SendError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid JSON body")
		return
	}

	ctx := req.Context()
	imported := 0

	for _, m := range importReq.Memories {
		if m.Content == "" {
			continue
		}
		source := m.Source
		if source == "" {
			source = "import"
		}
		entry := memory.MemoryEntry{
			Content: m.Content,
			Source:  source,
		}
		if err := r.memory.Add(ctx, entry); err != nil {
			// Log but continue importing
			continue
		}
		imported++
	}

	handlers.SendJSON(w, http.StatusOK, map[string]any{
		"imported": imported,
		"total":    len(importReq.Memories),
		"status":   "ok",
	})
}

// HandleBatchDelete handles DELETE /memory/batch - deletes multiple memories.
func (r *Router) HandleBatchDelete(w http.ResponseWriter, req *http.Request) {
	if r.memory == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable, "Memory index not available")
		return
	}

	var deleteReq struct {
		IDs []string `json:"ids"`
	}

	if err := json.NewDecoder(req.Body).Decode(&deleteReq); err != nil {
		handlers.SendError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "Invalid JSON body")
		return
	}

	if len(deleteReq.IDs) == 0 {
		handlers.SendError(w, http.StatusBadRequest, ErrCodeInvalidRequest, "IDs array is required")
		return
	}

	ctx := req.Context()
	deleted := 0
	for _, id := range deleteReq.IDs {
		if err := r.memory.Delete(ctx, id); err == nil {
			deleted++
		}
	}

	handlers.SendJSON(w, http.StatusOK, map[string]any{
		"deleted": deleted,
		"total":   len(deleteReq.IDs),
		"status":  "ok",
	})
}

// HandleMemoryStats handles GET /memory/stats - returns memory statistics.
func (r *Router) HandleMemoryStats(w http.ResponseWriter, req *http.Request) {
	if r.memory == nil {
		handlers.SendError(w, http.StatusServiceUnavailable, ErrCodeServiceUnavailable, "Memory index not available")
		return
	}

	ctx := req.Context()

	// Get all memories to calculate stats
	entries, err := r.memory.List(ctx, 100000, 0)
	if err != nil {
		handlers.SendError(w, http.StatusInternalServerError, ErrCodeInternalError, err.Error())
		return
	}

	// Calculate statistics
	total := len(entries)
	byCategory := make(map[string]int)
	byCaptureMethod := make(map[string]int)
	autoCaptureToday := 0
	today := time.Now().Truncate(24 * time.Hour)

	for _, entry := range entries {
		// Count by category
		cat := entry.Category
		if cat == "" {
			cat = memory.CategoryOther
		}
		byCategory[cat]++

		// Count by capture method
		method := entry.CaptureMethod
		if method == "" {
			method = memory.CaptureMethodManual
		}
		byCaptureMethod[method]++

		// Count auto captures today
		if method == memory.CaptureMethodAuto && !entry.CreatedAt.Before(today) {
			autoCaptureToday++
		}
	}

	// Note: auto_recall_today would require tracking in a separate stats store
	// For now, return 0 as placeholder
	autoRecallToday := 0

	handlers.SendJSON(w, http.StatusOK, MemoryStatsResponse{
		Total:            total,
		ByCategory:       byCategory,
		ByCaptureMethod:  byCaptureMethod,
		AutoCaptureToday: autoCaptureToday,
		AutoRecallToday:  autoRecallToday,
	})
}
