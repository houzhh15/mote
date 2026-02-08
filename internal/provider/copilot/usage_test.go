package copilot

import (
	"path/filepath"
	"testing"
	"time"
)

func TestNewUsageTracker(t *testing.T) {
	ut := NewUsageTracker(WithDataPath(filepath.Join(t.TempDir(), "usage.json")))

	if ut == nil {
		t.Fatal("NewUsageTracker returned nil")
	}

	if ut.monthlyFreeLimit != 2000 {
		t.Errorf("monthlyFreeLimit = %d, want 2000", ut.monthlyFreeLimit)
	}

	if ut.monthlyPremiumLimit != 50 {
		t.Errorf("monthlyPremiumLimit = %d, want 50", ut.monthlyPremiumLimit)
	}
}

func TestNewUsageTracker_WithOptions(t *testing.T) {
	customPath := "/tmp/test-usage.json"

	ut := NewUsageTracker(
		WithDataPath(customPath),
		WithMonthlyLimits(1000, 100),
	)

	if ut.dataPath != customPath {
		t.Errorf("dataPath = %s, want %s", ut.dataPath, customPath)
	}

	if ut.monthlyFreeLimit != 1000 {
		t.Errorf("monthlyFreeLimit = %d, want 1000", ut.monthlyFreeLimit)
	}

	if ut.monthlyPremiumLimit != 100 {
		t.Errorf("monthlyPremiumLimit = %d, want 100", ut.monthlyPremiumLimit)
	}
}

func TestUsageTracker_Record(t *testing.T) {
	tmpDir := t.TempDir()
	ut := NewUsageTracker(WithDataPath(filepath.Join(tmpDir, "usage.json")))

	err := ut.Record("gpt-4.1", 100, 50, ModeAgent)
	if err != nil {
		t.Fatalf("Record failed: %v", err)
	}

	monthly := ut.GetCurrentMonthUsage()

	if monthly.TotalRequests != 1 {
		t.Errorf("TotalRequests = %d, want 1", monthly.TotalRequests)
	}

	if monthly.FreeRequests != 1 {
		t.Errorf("FreeRequests = %d, want 1", monthly.FreeRequests)
	}

	if monthly.InputTokens != 100 {
		t.Errorf("InputTokens = %d, want 100", monthly.InputTokens)
	}

	if monthly.OutputTokens != 50 {
		t.Errorf("OutputTokens = %d, want 50", monthly.OutputTokens)
	}

	// Check by model
	modelUsage, ok := monthly.ByModel["gpt-4.1"]
	if !ok {
		t.Fatal("Model usage not recorded")
	}

	if modelUsage.Requests != 1 {
		t.Errorf("Model requests = %d, want 1", modelUsage.Requests)
	}

	// Check by mode
	modeCount := monthly.ByMode[ModeAgent]
	if modeCount != 1 {
		t.Errorf("Mode count = %d, want 1", modeCount)
	}
}

func TestUsageTracker_Record_Premium(t *testing.T) {
	tmpDir := t.TempDir()
	ut := NewUsageTracker(WithDataPath(filepath.Join(tmpDir, "usage.json")))

	// gpt-5 has multiplier of 5
	err := ut.Record("gpt-5", 200, 100, ModeAgent)
	if err != nil {
		t.Fatalf("Record failed: %v", err)
	}

	monthly := ut.GetCurrentMonthUsage()

	if monthly.TotalRequests != 1 {
		t.Errorf("TotalRequests = %d, want 1", monthly.TotalRequests)
	}

	if monthly.FreeRequests != 0 {
		t.Errorf("FreeRequests = %d, want 0 (premium model)", monthly.FreeRequests)
	}

	if monthly.PremiumUnits != 5 {
		t.Errorf("PremiumUnits = %d, want 5", monthly.PremiumUnits)
	}
}

func TestUsageTracker_GetQuotaStatus(t *testing.T) {
	tmpDir := t.TempDir()
	ut := NewUsageTracker(
		WithDataPath(filepath.Join(tmpDir, "usage.json")),
		WithMonthlyLimits(100, 10),
	)

	// Record some usage
	_ = ut.Record("gpt-4.1", 100, 50, ModeAgent) // Free, no multiplier
	_ = ut.Record("gpt-4.1", 100, 50, ModeAgent) // Free, no multiplier
	_ = ut.Record("gpt-5", 100, 50, ModeAgent)   // Premium, 5x multiplier

	status := ut.GetQuotaStatus()

	if status.FreeRequestsUsed != 2 {
		t.Errorf("FreeRequestsUsed = %d, want 2", status.FreeRequestsUsed)
	}

	if status.FreeRequestsLimit != 100 {
		t.Errorf("FreeRequestsLimit = %d, want 100", status.FreeRequestsLimit)
	}

	if status.FreeRequestsRemaining != 98 {
		t.Errorf("FreeRequestsRemaining = %d, want 98", status.FreeRequestsRemaining)
	}

	if status.FreePercentUsed != 2.0 {
		t.Errorf("FreePercentUsed = %f, want 2.0", status.FreePercentUsed)
	}

	if status.PremiumUnitsUsed != 5 {
		t.Errorf("PremiumUnitsUsed = %d, want 5", status.PremiumUnitsUsed)
	}

	if status.PremiumUnitsLimit != 10 {
		t.Errorf("PremiumUnitsLimit = %d, want 10", status.PremiumUnitsLimit)
	}

	if status.PremiumUnitsRemaining != 5 {
		t.Errorf("PremiumUnitsRemaining = %d, want 5", status.PremiumUnitsRemaining)
	}

	if status.PremiumPercentUsed != 50.0 {
		t.Errorf("PremiumPercentUsed = %f, want 50.0", status.PremiumPercentUsed)
	}
}

func TestUsageTracker_GetRecentRecords(t *testing.T) {
	tmpDir := t.TempDir()
	ut := NewUsageTracker(WithDataPath(filepath.Join(tmpDir, "usage.json")))

	// Record multiple entries
	for i := 0; i < 5; i++ {
		_ = ut.Record("gpt-4.1", 100, 50, ModeAgent)
	}

	records := ut.GetRecentRecords(3)

	if len(records) != 3 {
		t.Errorf("Got %d records, want 3", len(records))
	}

	// Request more than available
	records = ut.GetRecentRecords(10)
	if len(records) != 5 {
		t.Errorf("Got %d records, want 5", len(records))
	}

	// Request 0 or negative
	records = ut.GetRecentRecords(0)
	if records != nil {
		t.Errorf("Got %v records, want nil", records)
	}
}

func TestUsageTracker_ClearOldRecords(t *testing.T) {
	tmpDir := t.TempDir()
	ut := NewUsageTracker(WithDataPath(filepath.Join(tmpDir, "usage.json")))

	// Add records
	for i := 0; i < 3; i++ {
		_ = ut.Record("gpt-4.1", 100, 50, ModeAgent)
	}

	// Clear records older than 1 hour (should clear none)
	removed := ut.ClearOldRecords(1 * time.Hour)
	if removed != 0 {
		t.Errorf("Removed %d records, want 0", removed)
	}

	// Clear records older than 0 (should clear all)
	removed = ut.ClearOldRecords(0)
	if removed != 3 {
		t.Errorf("Removed %d records, want 3", removed)
	}

	records := ut.GetRecentRecords(10)
	if len(records) != 0 {
		t.Errorf("After clear, got %d records, want 0", len(records))
	}
}

func TestUsageTracker_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	dataPath := filepath.Join(tmpDir, "usage.json")

	// Create tracker and record usage
	ut1 := NewUsageTracker(WithDataPath(dataPath))
	_ = ut1.Record("gpt-4.1", 100, 50, ModeAgent)

	// Create new tracker and verify data was loaded
	ut2 := NewUsageTracker(WithDataPath(dataPath))
	monthly := ut2.GetCurrentMonthUsage()

	if monthly.TotalRequests != 1 {
		t.Errorf("Loaded TotalRequests = %d, want 1", monthly.TotalRequests)
	}

	if monthly.InputTokens != 100 {
		t.Errorf("Loaded InputTokens = %d, want 100", monthly.InputTokens)
	}
}

func TestUsageTracker_Reset(t *testing.T) {
	tmpDir := t.TempDir()
	ut := NewUsageTracker(WithDataPath(filepath.Join(tmpDir, "usage.json")))

	// Record some usage
	_ = ut.Record("gpt-4.1", 100, 50, ModeAgent)

	monthly := ut.GetCurrentMonthUsage()
	if monthly.TotalRequests != 1 {
		t.Fatalf("Pre-reset TotalRequests = %d, want 1", monthly.TotalRequests)
	}

	// Reset
	err := ut.Reset()
	if err != nil {
		t.Fatalf("Reset failed: %v", err)
	}

	// Verify data is cleared
	monthly = ut.GetCurrentMonthUsage()
	if monthly.TotalRequests != 0 {
		t.Errorf("Post-reset TotalRequests = %d, want 0", monthly.TotalRequests)
	}

	records := ut.GetRecentRecords(10)
	if len(records) != 0 {
		t.Errorf("Post-reset records = %d, want 0", len(records))
	}
}

func TestUsageTracker_GetMonthlyUsage(t *testing.T) {
	tmpDir := t.TempDir()
	ut := NewUsageTracker(WithDataPath(filepath.Join(tmpDir, "usage.json")))

	// Record usage
	_ = ut.Record("gpt-4.1", 100, 50, ModeAgent)

	now := time.Now()
	monthly := ut.GetMonthlyUsage(now.Year(), int(now.Month()))

	if monthly.TotalRequests != 1 {
		t.Errorf("TotalRequests = %d, want 1", monthly.TotalRequests)
	}

	// Request non-existent month
	monthly = ut.GetMonthlyUsage(2020, 1)
	if monthly.TotalRequests != 0 {
		t.Errorf("Non-existent month TotalRequests = %d, want 0", monthly.TotalRequests)
	}

	if monthly.Year != 2020 || monthly.Month != 1 {
		t.Errorf("Non-existent month = %d-%d, want 2020-1", monthly.Year, monthly.Month)
	}
}

func TestMonthKey(t *testing.T) {
	tests := []struct {
		time time.Time
		want string
	}{
		{time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC), "2024-01"},
		{time.Date(2024, 12, 31, 23, 59, 59, 0, time.UTC), "2024-12"},
		{time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC), "2025-06"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := monthKey(tt.time)
			if got != tt.want {
				t.Errorf("monthKey(%v) = %s, want %s", tt.time, got, tt.want)
			}
		})
	}
}

func TestUsageTracker_UnknownModel(t *testing.T) {
	tmpDir := t.TempDir()
	ut := NewUsageTracker(WithDataPath(filepath.Join(tmpDir, "usage.json")))

	// Record with unknown model (should not panic)
	err := ut.Record("unknown-model", 100, 50, ModeAgent)
	if err != nil {
		t.Fatalf("Record with unknown model failed: %v", err)
	}

	monthly := ut.GetCurrentMonthUsage()

	if monthly.TotalRequests != 1 {
		t.Errorf("TotalRequests = %d, want 1", monthly.TotalRequests)
	}

	// Unknown model should be treated as free (multiplier 0)
	if monthly.FreeRequests != 1 {
		t.Errorf("FreeRequests = %d, want 1 (unknown model treated as free)", monthly.FreeRequests)
	}
}
