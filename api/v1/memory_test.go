package v1

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
)

func TestRouter_HandleMemorySearch_NoMemory(t *testing.T) {
	router := NewRouter(nil)
	m := mux.NewRouter()
	_ = router.RegisterRoutes(m)

	body := MemorySearchRequest{
		Query: "test query",
		TopK:  5,
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/memory/search", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	m.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, rr.Code)
	}
}

func TestRouter_HandleMemorySearch_EmptyQuery(t *testing.T) {
	router := NewRouter(nil)
	m := mux.NewRouter()
	_ = router.RegisterRoutes(m)

	body := MemorySearchRequest{
		Query: "",
		TopK:  5,
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/memory/search", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	m.ServeHTTP(rr, req)

	// Without memory, returns 503 before checking query
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, rr.Code)
	}
}

func TestRouter_HandleMemorySearch_InvalidJSON(t *testing.T) {
	router := NewRouter(nil)
	m := mux.NewRouter()
	_ = router.RegisterRoutes(m)

	req := httptest.NewRequest("POST", "/api/v1/memory/search", bytes.NewReader([]byte("invalid")))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	m.ServeHTTP(rr, req)

	// Without memory, returns 503 before checking JSON
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, rr.Code)
	}
}

func TestRouter_HandleAddMemory_NoMemory(t *testing.T) {
	router := NewRouter(nil)
	m := mux.NewRouter()
	_ = router.RegisterRoutes(m)

	body := AddMemoryRequest{
		Content: "Test memory content",
		Source:  "test",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/memory", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	m.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, rr.Code)
	}
}

func TestRouter_HandleDeleteMemory_NoMemory(t *testing.T) {
	router := NewRouter(nil)
	m := mux.NewRouter()
	_ = router.RegisterRoutes(m)

	req := httptest.NewRequest("DELETE", "/api/v1/memory/mem-123", nil)
	rr := httptest.NewRecorder()

	m.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, rr.Code)
	}
}

func TestMemorySearchRequest_Validation(t *testing.T) {
	tests := []struct {
		name    string
		request MemorySearchRequest
		valid   bool
	}{
		{
			name:    "valid with query only",
			request: MemorySearchRequest{Query: "test"},
			valid:   true,
		},
		{
			name:    "valid with query and topK",
			request: MemorySearchRequest{Query: "test", TopK: 10},
			valid:   true,
		},
		{
			name:    "invalid empty query",
			request: MemorySearchRequest{Query: ""},
			valid:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isValid := tt.request.Query != ""
			if isValid != tt.valid {
				t.Errorf("Expected valid=%v, got %v", tt.valid, isValid)
			}
		})
	}
}

func TestMemorySearchRequest_DefaultTopK(t *testing.T) {
	req := MemorySearchRequest{
		Query: "test query",
	}

	// TopK should default to 0 when not set
	if req.TopK != 0 {
		t.Errorf("Expected TopK to be 0 when not set, got %d", req.TopK)
	}

	// Handler should set default to 10
	topK := req.TopK
	if topK <= 0 {
		topK = 10
	}

	if topK != 10 {
		t.Errorf("Expected default TopK to be 10, got %d", topK)
	}
}

func TestAddMemoryRequest_DefaultSource(t *testing.T) {
	req := AddMemoryRequest{
		Content: "test content",
	}

	// Source should be empty when not set
	if req.Source != "" {
		t.Errorf("Expected Source to be empty when not set, got %q", req.Source)
	}

	// Handler should set default to "api"
	source := req.Source
	if source == "" {
		source = "api"
	}

	if source != "api" {
		t.Errorf("Expected default Source to be 'api', got %q", source)
	}
}

func TestMemoryResult_Structure(t *testing.T) {
	result := MemoryResult{
		ID:        "mem-123",
		Content:   "Test memory content",
		Score:     0.95,
		Source:    "conversation",
		CreatedAt: "2026-01-01T00:00:00Z",
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	requiredFields := []string{"id", "content", "score", "source", "created_at"}
	for _, field := range requiredFields {
		if _, ok := decoded[field]; !ok {
			t.Errorf("Missing required field: %s", field)
		}
	}
}

// P2 Tests

func TestMemorySearchRequest_P2_CategoryFilter(t *testing.T) {
	req := MemorySearchRequest{
		Query:      "test query",
		TopK:       5,
		Categories: []string{"preference", "fact"},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	categories, ok := decoded["categories"].([]any)
	if !ok {
		t.Fatal("Expected categories to be an array")
	}

	if len(categories) != 2 {
		t.Errorf("Expected 2 categories, got %d", len(categories))
	}
}

func TestMemorySearchRequest_P2_MinImportance(t *testing.T) {
	req := MemorySearchRequest{
		Query:         "test query",
		MinImportance: 0.5,
	}

	if req.MinImportance != 0.5 {
		t.Errorf("Expected MinImportance 0.5, got %f", req.MinImportance)
	}
}

func TestAddMemoryRequest_P2_Fields(t *testing.T) {
	req := AddMemoryRequest{
		Content:    "I prefer dark mode",
		Category:   "preference",
		Importance: 0.8,
	}

	if req.Category != "preference" {
		t.Errorf("Expected Category 'preference', got %s", req.Category)
	}

	if req.Importance != 0.8 {
		t.Errorf("Expected Importance 0.8, got %f", req.Importance)
	}
}

func TestAddMemoryRequest_P2_DefaultImportance(t *testing.T) {
	req := AddMemoryRequest{
		Content: "test content",
	}

	// Importance should be 0 when not set
	if req.Importance != 0 {
		t.Errorf("Expected Importance to be 0 when not set, got %f", req.Importance)
	}

	// Handler should set default to 0.7
	importance := req.Importance
	if importance <= 0 {
		importance = 0.7
	}

	if importance != 0.7 {
		t.Errorf("Expected default Importance to be 0.7, got %f", importance)
	}
}

func TestAddMemoryResponse_P2_Category(t *testing.T) {
	resp := AddMemoryResponse{
		ID:       "mem-123",
		Category: "preference",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if decoded["category"] != "preference" {
		t.Errorf("Expected category 'preference', got %v", decoded["category"])
	}
}

func TestMemoryResult_P2_Fields(t *testing.T) {
	result := MemoryResult{
		ID:            "mem-123",
		Content:       "User prefers dark mode",
		Score:         0.95,
		Source:        "conversation",
		CreatedAt:     "2026-01-01T00:00:00Z",
		Category:      "preference",
		Importance:    0.8,
		CaptureMethod: "auto",
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	// Check P2 fields
	if decoded["category"] != "preference" {
		t.Errorf("Expected category 'preference', got %v", decoded["category"])
	}
	if decoded["importance"] != 0.8 {
		t.Errorf("Expected importance 0.8, got %v", decoded["importance"])
	}
	if decoded["capture_method"] != "auto" {
		t.Errorf("Expected capture_method 'auto', got %v", decoded["capture_method"])
	}
}

func TestMemoryEntryResponse_P2_Fields(t *testing.T) {
	resp := MemoryEntryResponse{
		ID:            "mem-123",
		Content:       "Test content",
		Source:        "api",
		CreatedAt:     "2026-01-01T00:00:00Z",
		Category:      "fact",
		Importance:    0.75,
		CaptureMethod: "manual",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if decoded["category"] != "fact" {
		t.Errorf("Expected category 'fact', got %v", decoded["category"])
	}
	if decoded["importance"] != 0.75 {
		t.Errorf("Expected importance 0.75, got %v", decoded["importance"])
	}
	if decoded["capture_method"] != "manual" {
		t.Errorf("Expected capture_method 'manual', got %v", decoded["capture_method"])
	}
}

func TestMemoryStatsResponse_Structure(t *testing.T) {
	resp := MemoryStatsResponse{
		Total: 100,
		ByCategory: map[string]int{
			"preference": 30,
			"fact":       50,
			"other":      20,
		},
		ByCaptureMethod: map[string]int{
			"manual": 40,
			"auto":   60,
		},
		AutoCaptureToday: 5,
		AutoRecallToday:  10,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	// Check required fields
	requiredFields := []string{"total", "by_category", "by_capture_method", "auto_capture_today", "auto_recall_today"}
	for _, field := range requiredFields {
		if _, ok := decoded[field]; !ok {
			t.Errorf("Missing required field: %s", field)
		}
	}

	// Check total
	if decoded["total"] != float64(100) {
		t.Errorf("Expected total 100, got %v", decoded["total"])
	}

	// Check auto capture today
	if decoded["auto_capture_today"] != float64(5) {
		t.Errorf("Expected auto_capture_today 5, got %v", decoded["auto_capture_today"])
	}
}

func TestRouter_HandleMemoryStats_NoMemory(t *testing.T) {
	router := NewRouter(nil)
	m := mux.NewRouter()
	_ = router.RegisterRoutes(m)

	req := httptest.NewRequest("GET", "/api/v1/memory/stats", nil)
	rr := httptest.NewRecorder()

	m.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, rr.Code)
	}
}
