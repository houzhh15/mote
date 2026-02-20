package context

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"mote/internal/compaction"
	"mote/internal/provider"
	"mote/internal/storage"
)

// setupBenchManager creates a Manager suitable for benchmark/performance tests.
func setupBenchManager(b *testing.B) (*Manager, *storage.DB) {
	b.Helper()
	tmpDir := b.TempDir()
	db, err := storage.Open(filepath.Join(tmpDir, "bench.db"))
	if err != nil {
		b.Fatalf("Open failed: %v", err)
	}

	compactorConfig := compaction.DefaultConfig()
	compactorConfig.KeepRecentCount = 10
	compactorConfig.MaxContextTokens = 10000
	compactorConfig.TriggerThreshold = 0.8

	c := compaction.NewCompactor(compactorConfig, nil)

	config := Config{
		MaxContextTokens:       10000,
		TriggerThreshold:       0.8,
		KeepRecentCount:        10,
		TargetCompressionRatio: 0.3,
	}

	mgr := NewManager(db, c, nil, config)
	return mgr, db
}

// BenchmarkBuildContext_NoCompression benchmarks BuildContext when no previous
// compressed context exists and the messages are below threshold.
func BenchmarkBuildContext_NoCompression(b *testing.B) {
	mgr, db := setupBenchManager(b)
	defer db.Close()

	session, _ := db.CreateSession(nil)
	for i := 0; i < 20; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		db.AppendMessage(session.ID, role, fmt.Sprintf("Msg %d", i), nil, "")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := mgr.BuildContext(context.Background(), session.ID, "System.", "Input.")
		if err != nil {
			b.Fatalf("BuildContext failed: %v", err)
		}
	}
}

// BenchmarkBuildContext_WithCompressedContext benchmarks BuildContext when a
// compressed context already exists (the common "restart recovery" path).
func BenchmarkBuildContext_WithCompressedContext(b *testing.B) {
	mgr, db := setupBenchManager(b)
	defer db.Close()

	session, _ := db.CreateSession(nil)
	for i := 0; i < 50; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		db.AppendMessage(session.ID, role, fmt.Sprintf("Old msg %d - padding to simulate real content.", i), nil, "")
	}

	// Pre-save a compressed context
	db.SaveContext(&storage.Context{
		SessionID:      session.ID,
		Version:        1,
		Summary:        "Previous conversation covered various topics including setup and configuration.",
		KeptMessageIDs: nil,
		TotalTokens:    200,
		OriginalTokens: 2000,
		CreatedAt:      time.Now().Add(-10 * time.Second),
	})

	// Add a few messages after compression
	time.Sleep(time.Millisecond)
	for i := 0; i < 5; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		db.AppendMessage(session.ID, role, fmt.Sprintf("New msg %d", i), nil, "")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := mgr.BuildContext(context.Background(), session.ID, "System.", "Input.")
		if err != nil {
			b.Fatalf("BuildContext failed: %v", err)
		}
	}
}

// BenchmarkLoadLatestContext benchmarks loading the latest compressed context
// from the database, validating the NFR that context loading < 100ms.
func BenchmarkLoadLatestContext(b *testing.B) {
	mgr, db := setupBenchManager(b)
	defer db.Close()

	session, _ := db.CreateSession(nil)
	for i := 0; i < 100; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		db.AppendMessage(session.ID, role, fmt.Sprintf("Msg %d with some real content padding here.", i), nil, "")
	}

	// Save multiple context versions
	for v := 1; v <= 5; v++ {
		db.SaveContext(&storage.Context{
			SessionID:      session.ID,
			Version:        v,
			Summary:        fmt.Sprintf("Summary version %d covering topics A, B, C.", v),
			KeptMessageIDs: nil,
			TotalTokens:    200,
			OriginalTokens: 5000,
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := mgr.LoadLatestContext(session.ID)
		if err != nil {
			b.Fatalf("LoadLatestContext failed: %v", err)
		}
	}
}

// BenchmarkSaveContext benchmarks persisting a compressed context.
func BenchmarkSaveContext(b *testing.B) {
	mgr, db := setupBenchManager(b)
	defer db.Close()

	session, _ := db.CreateSession(nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := mgr.SaveContext(context.Background(), session.ID,
			fmt.Sprintf("Benchmark summary iteration %d", i), nil, 500, 5000)
		if err != nil {
			b.Fatalf("SaveContext failed: %v", err)
		}
	}
}

// BenchmarkNeedsCompression benchmarks token estimation + threshold check.
func BenchmarkNeedsCompression(b *testing.B) {
	mgr, db := setupBenchManager(b)
	defer db.Close()

	msgs := make([]provider.Message, 100)
	for i := range msgs {
		msgs[i] = provider.Message{
			Role:    provider.RoleUser,
			Content: fmt.Sprintf("Message %d with enough content for realistic token estimation.", i),
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mgr.NeedsCompression(msgs)
	}
}

// TestPerformance_ContextLoadTime verifies the non-functional requirement
// that context loading completes within 100ms.
func TestPerformance_ContextLoadTime(t *testing.T) {
	mgr, db := setupIntegrationManager(t)
	defer db.Close()

	session, _ := db.CreateSession(nil)
	// Populate with a realistic number of messages
	for i := 0; i < 100; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		db.AppendMessage(session.ID, role,
			fmt.Sprintf("Message %d with realistic content to simulate a real conversation turn.", i), nil, "")
	}

	// Save a compressed context with kept messages
	msgs, _ := db.GetMessages(session.ID, 0)
	var keptIDs []string
	if len(msgs) >= 5 {
		for _, m := range msgs[len(msgs)-5:] {
			keptIDs = append(keptIDs, m.ID)
		}
	}
	db.SaveContext(&storage.Context{
		SessionID:      session.ID,
		Version:        1,
		Summary:        "Comprehensive summary of 100 messages discussing various topics.",
		KeptMessageIDs: keptIDs,
		TotalTokens:    300,
		OriginalTokens: 5000,
	})

	// Measure load time
	start := time.Now()
	result, err := mgr.LoadLatestContext(session.ID)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("LoadLatestContext failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil context")
	}

	const maxAllowed = 100 * time.Millisecond
	if elapsed > maxAllowed {
		t.Errorf("context loading took %v, exceeds NFR limit of %v", elapsed, maxAllowed)
	} else {
		t.Logf("context loading completed in %v (limit: %v)", elapsed, maxAllowed)
	}
}

// TestPerformance_BuildContextTime verifies that BuildContext with a saved
// compressed context completes within a reasonable time.
func TestPerformance_BuildContextTime(t *testing.T) {
	mgr, db := setupIntegrationManager(t)
	defer db.Close()

	session, _ := db.CreateSession(nil)
	for i := 0; i < 50; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		db.AppendMessage(session.ID, role,
			fmt.Sprintf("Message %d content for build context perf test.", i), nil, "")
	}

	db.SaveContext(&storage.Context{
		SessionID:      session.ID,
		Version:        1,
		Summary:        "Summary of the initial conversation.",
		KeptMessageIDs: nil,
		TotalTokens:    100,
		OriginalTokens: 2000,
		CreatedAt:      time.Now().Add(-5 * time.Second),
	})

	// Add some new messages after context save
	time.Sleep(time.Millisecond)
	for i := 0; i < 5; i++ {
		db.AppendMessage(session.ID, "user", fmt.Sprintf("New msg %d", i), nil, "")
	}

	start := time.Now()
	msgs, err := mgr.BuildContext(context.Background(), session.ID, "System.", "Input.")
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("BuildContext failed: %v", err)
	}
	if len(msgs) == 0 {
		t.Fatal("expected non-empty messages")
	}

	const maxAllowed = 200 * time.Millisecond
	if elapsed > maxAllowed {
		t.Errorf("BuildContext took %v, exceeds limit of %v", elapsed, maxAllowed)
	} else {
		t.Logf("BuildContext completed in %v (limit: %v)", elapsed, maxAllowed)
	}
}
