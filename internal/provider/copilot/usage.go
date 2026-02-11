// Package copilot provides GitHub Copilot integration for Mote.
package copilot

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"mote/pkg/logger"
)

// UsageRecord represents a single usage record.
type UsageRecord struct {
	Timestamp    time.Time `json:"timestamp"`
	ModelID      string    `json:"model_id"`
	Multiplier   int       `json:"multiplier"`
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
	Mode         Mode      `json:"mode"`
	IsPremium    bool      `json:"is_premium"`
}

// MonthlyUsage represents aggregated usage for a month.
type MonthlyUsage struct {
	Year          int                   `json:"year"`
	Month         int                   `json:"month"`
	TotalRequests int                   `json:"total_requests"`
	FreeRequests  int                   `json:"free_requests"`
	PremiumUnits  int                   `json:"premium_units"` // Sum of multipliers for premium requests
	InputTokens   int                   `json:"input_tokens"`
	OutputTokens  int                   `json:"output_tokens"`
	ByModel       map[string]ModelUsage `json:"by_model"`
	ByMode        map[Mode]int          `json:"by_mode"`
	LastUpdated   time.Time             `json:"last_updated"`
}

// ModelUsage represents usage for a specific model.
type ModelUsage struct {
	Requests     int `json:"requests"`
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	PremiumUnits int `json:"premium_units"`
}

// UsageData represents all stored usage data.
type UsageData struct {
	Records []UsageRecord           `json:"records"`
	Monthly map[string]MonthlyUsage `json:"monthly"` // Key: "2024-01" format
}

// UsageTracker tracks Copilot usage and quota consumption.
type UsageTracker struct {
	mu       sync.RWMutex
	dataPath string
	data     UsageData

	// Current month quota limits (if known)
	monthlyFreeLimit    int
	monthlyPremiumLimit int
}

// UsageTrackerOption is a functional option for UsageTracker.
type UsageTrackerOption func(*UsageTracker)

// WithDataPath sets the data file path.
func WithDataPath(path string) UsageTrackerOption {
	return func(ut *UsageTracker) {
		ut.dataPath = path
	}
}

// WithMonthlyLimits sets the monthly quota limits.
func WithMonthlyLimits(freeLimit, premiumLimit int) UsageTrackerOption {
	return func(ut *UsageTracker) {
		ut.monthlyFreeLimit = freeLimit
		ut.monthlyPremiumLimit = premiumLimit
	}
}

// NewUsageTracker creates a new UsageTracker with the given options.
func NewUsageTracker(opts ...UsageTrackerOption) *UsageTracker {
	// Default data path
	homeDir, _ := os.UserHomeDir()
	defaultDataPath := filepath.Join(homeDir, ".config", "mote", "usage.json")

	ut := &UsageTracker{
		dataPath: defaultDataPath,
		data: UsageData{
			Records: make([]UsageRecord, 0),
			Monthly: make(map[string]MonthlyUsage),
		},
		monthlyFreeLimit:    2000, // Default Copilot Individual free tier
		monthlyPremiumLimit: 50,   // Default Copilot Individual premium limit
	}

	for _, opt := range opts {
		opt(ut)
	}

	// Try to load existing data
	if err := ut.load(); err != nil {
		logger.Debug().Err(err).Msg("Could not load usage data, starting fresh")
	}

	return ut
}

// Record records a new usage event.
func (ut *UsageTracker) Record(modelID string, inputTokens, outputTokens int, mode Mode) error {
	record := UsageRecord{
		Timestamp:    time.Now(),
		ModelID:      modelID,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		Mode:         mode,
	}

	// Check API model registry first, then ACP model registry
	if info := GetModelInfo(modelID); info != nil {
		record.Multiplier = info.Multiplier
		record.IsPremium = !info.IsFree
	} else if acpInfo := ACPGetModelInfo(modelID); acpInfo != nil {
		record.Multiplier = int(acpInfo.Multiplier)
		record.IsPremium = true // All ACP models are premium
	}

	ut.mu.Lock()
	defer ut.mu.Unlock()

	ut.data.Records = append(ut.data.Records, record)
	ut.updateMonthly(record)

	if err := ut.save(); err != nil {
		logger.Warn().Err(err).Msg("Could not persist usage data")
	}

	return nil
}

// updateMonthly updates the monthly aggregation with a new record.
// Must be called with mu held.
func (ut *UsageTracker) updateMonthly(record UsageRecord) {
	key := monthKey(record.Timestamp)

	monthly, ok := ut.data.Monthly[key]
	if !ok {
		year, month, _ := record.Timestamp.Date()
		monthly = MonthlyUsage{
			Year:    year,
			Month:   int(month),
			ByModel: make(map[string]ModelUsage),
			ByMode:  make(map[Mode]int),
		}
	}

	monthly.TotalRequests++
	monthly.InputTokens += record.InputTokens
	monthly.OutputTokens += record.OutputTokens

	if record.IsPremium {
		monthly.PremiumUnits += record.Multiplier
	} else {
		monthly.FreeRequests++
	}

	// Update by model
	modelUsage := monthly.ByModel[record.ModelID]
	modelUsage.Requests++
	modelUsage.InputTokens += record.InputTokens
	modelUsage.OutputTokens += record.OutputTokens
	if record.IsPremium {
		modelUsage.PremiumUnits += record.Multiplier
	}
	monthly.ByModel[record.ModelID] = modelUsage

	// Update by mode
	monthly.ByMode[record.Mode]++

	monthly.LastUpdated = time.Now()
	ut.data.Monthly[key] = monthly
}

// GetCurrentMonthUsage returns usage statistics for the current month.
func (ut *UsageTracker) GetCurrentMonthUsage() MonthlyUsage {
	ut.mu.RLock()
	defer ut.mu.RUnlock()

	key := monthKey(time.Now())
	if monthly, ok := ut.data.Monthly[key]; ok {
		return monthly
	}

	now := time.Now()
	year, month, _ := now.Date()
	return MonthlyUsage{
		Year:    year,
		Month:   int(month),
		ByModel: make(map[string]ModelUsage),
		ByMode:  make(map[Mode]int),
	}
}

// GetMonthlyUsage returns usage statistics for a specific month.
func (ut *UsageTracker) GetMonthlyUsage(year int, month int) MonthlyUsage {
	ut.mu.RLock()
	defer ut.mu.RUnlock()

	key := fmt.Sprintf("%04d-%02d", year, month)
	if monthly, ok := ut.data.Monthly[key]; ok {
		return monthly
	}

	return MonthlyUsage{
		Year:    year,
		Month:   month,
		ByModel: make(map[string]ModelUsage),
		ByMode:  make(map[Mode]int),
	}
}

// GetQuotaStatus returns the current quota status.
type QuotaStatus struct {
	FreeRequestsUsed      int     `json:"free_requests_used"`
	FreeRequestsLimit     int     `json:"free_requests_limit"`
	FreeRequestsRemaining int     `json:"free_requests_remaining"`
	FreePercentUsed       float64 `json:"free_percent_used"`

	PremiumUnitsUsed      int     `json:"premium_units_used"`
	PremiumUnitsLimit     int     `json:"premium_units_limit"`
	PremiumUnitsRemaining int     `json:"premium_units_remaining"`
	PremiumPercentUsed    float64 `json:"premium_percent_used"`
}

// GetQuotaStatus returns the current month's quota status.
func (ut *UsageTracker) GetQuotaStatus() QuotaStatus {
	monthly := ut.GetCurrentMonthUsage()

	freeRemaining := ut.monthlyFreeLimit - monthly.FreeRequests
	if freeRemaining < 0 {
		freeRemaining = 0
	}

	premiumRemaining := ut.monthlyPremiumLimit - monthly.PremiumUnits
	if premiumRemaining < 0 {
		premiumRemaining = 0
	}

	var freePercent, premiumPercent float64
	if ut.monthlyFreeLimit > 0 {
		freePercent = float64(monthly.FreeRequests) / float64(ut.monthlyFreeLimit) * 100
	}
	if ut.monthlyPremiumLimit > 0 {
		premiumPercent = float64(monthly.PremiumUnits) / float64(ut.monthlyPremiumLimit) * 100
	}

	return QuotaStatus{
		FreeRequestsUsed:      monthly.FreeRequests,
		FreeRequestsLimit:     ut.monthlyFreeLimit,
		FreeRequestsRemaining: freeRemaining,
		FreePercentUsed:       freePercent,

		PremiumUnitsUsed:      monthly.PremiumUnits,
		PremiumUnitsLimit:     ut.monthlyPremiumLimit,
		PremiumUnitsRemaining: premiumRemaining,
		PremiumPercentUsed:    premiumPercent,
	}
}

// GetRecentRecords returns the most recent N usage records.
func (ut *UsageTracker) GetRecentRecords(n int) []UsageRecord {
	ut.mu.RLock()
	defer ut.mu.RUnlock()

	if n <= 0 || len(ut.data.Records) == 0 {
		return nil
	}

	start := len(ut.data.Records) - n
	if start < 0 {
		start = 0
	}

	result := make([]UsageRecord, len(ut.data.Records)-start)
	copy(result, ut.data.Records[start:])

	return result
}

// ClearOldRecords removes records older than the specified duration.
// Monthly aggregations are kept.
func (ut *UsageTracker) ClearOldRecords(olderThan time.Duration) int {
	ut.mu.Lock()
	defer ut.mu.Unlock()

	cutoff := time.Now().Add(-olderThan)
	newRecords := make([]UsageRecord, 0)
	removed := 0

	for _, record := range ut.data.Records {
		if record.Timestamp.After(cutoff) {
			newRecords = append(newRecords, record)
		} else {
			removed++
		}
	}

	ut.data.Records = newRecords

	if removed > 0 {
		if err := ut.save(); err != nil {
			logger.Warn().Err(err).Msg("Could not persist usage data after cleanup")
		}
	}

	return removed
}

// save persists usage data to disk.
func (ut *UsageTracker) save() error {
	dir := filepath.Dir(ut.dataPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}

	data, err := json.MarshalIndent(ut.data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal data: %w", err)
	}

	if err := os.WriteFile(ut.dataPath, data, 0644); err != nil {
		return fmt.Errorf("write data: %w", err)
	}

	return nil
}

// load reads usage data from disk.
func (ut *UsageTracker) load() error {
	data, err := os.ReadFile(ut.dataPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read data: %w", err)
	}

	var usageData UsageData
	if err := json.Unmarshal(data, &usageData); err != nil {
		return fmt.Errorf("unmarshal data: %w", err)
	}

	ut.data = usageData

	// Initialize maps if nil
	if ut.data.Records == nil {
		ut.data.Records = make([]UsageRecord, 0)
	}
	if ut.data.Monthly == nil {
		ut.data.Monthly = make(map[string]MonthlyUsage)
	}

	return nil
}

// monthKey returns the month key in "2024-01" format.
func monthKey(t time.Time) string {
	return fmt.Sprintf("%04d-%02d", t.Year(), int(t.Month()))
}

// Reset clears all usage data.
func (ut *UsageTracker) Reset() error {
	ut.mu.Lock()
	defer ut.mu.Unlock()

	ut.data = UsageData{
		Records: make([]UsageRecord, 0),
		Monthly: make(map[string]MonthlyUsage),
	}

	return ut.save()
}
