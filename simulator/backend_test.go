package simulator

import (
	"context"
	"testing"
	"time"
)

func TestNewBackend(t *testing.T) {
	patterns := []LoadPattern{
		PatternConstant,
		PatternSineWave,
		PatternSpikes,
		PatternGradual,
	}

	for _, pattern := range patterns {
		t.Run(pattern.String(), func(t *testing.T) {
			backend := NewBackend(pattern)
			if backend == nil {
				t.Error("NewBackend() returned nil")
			}
			if backend.loadPattern != pattern {
				t.Errorf("Expected pattern %v, got %v", pattern, backend.loadPattern)
			}
		})
	}
}

func TestBackend_ProcessBatch(t *testing.T) {
	backend := NewBackend(PatternConstant)
	ctx := context.Background()

	// Create a batch
	batch := make([]any, 10)
	for i := 0; i < 10; i++ {
		batch[i] = i
	}

	// Process batch
	feedback, err := backend.ProcessBatch(ctx, batch)
	if err != nil {
		t.Errorf("ProcessBatch() error = %v", err)
	}

	// Verify feedback
	if feedback == nil {
		t.Fatal("ProcessBatch() returned nil feedback")
	}

	// Check feedback fields
	if feedback.CPULoad < 0 || feedback.CPULoad > 1 {
		t.Errorf("CPULoad out of range: %v", feedback.CPULoad)
	}
	if feedback.QueueDepth < 0 {
		t.Errorf("QueueDepth negative: %v", feedback.QueueDepth)
	}
	if feedback.ErrorRate < 0 || feedback.ErrorRate > 1 {
		t.Errorf("ErrorRate out of range: %v", feedback.ErrorRate)
	}
	if feedback.ProcessingTime <= 0 {
		t.Errorf("ProcessingTime invalid: %v", feedback.ProcessingTime)
	}
}

func TestBackend_LoadPatterns(t *testing.T) {
	tests := []struct {
		name    string
		pattern LoadPattern
	}{
		{"constant", PatternConstant},
		{"sinewave", PatternSineWave},
		{"spikes", PatternSpikes},
		{"gradual", PatternGradual},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := NewBackend(tt.pattern)
			ctx := context.Background()

			batch := make([]any, 5)
			for i := 0; i < 5; i++ {
				batch[i] = i
			}

			// Process multiple batches
			for i := 0; i < 10; i++ {
				feedback, err := backend.ProcessBatch(ctx, batch)
				if err != nil {
					t.Errorf("ProcessBatch() error = %v", err)
				}
				if feedback == nil {
					t.Fatal("ProcessBatch() returned nil feedback")
				}

				// Small delay between batches
				time.Sleep(10 * time.Millisecond)
			}

			stats := backend.GetStats()
			if stats.TotalBatches != 10 {
				t.Errorf("Expected 10 batches, got %d", stats.TotalBatches)
			}
		})
	}
}

func TestBackend_Stats(t *testing.T) {
	backend := NewBackend(PatternConstant)
	ctx := context.Background()

	// Initially stats should be zero
	stats := backend.GetStats()
	if stats.TotalProcessed != 0 {
		t.Errorf("Expected 0 processed, got %d", stats.TotalProcessed)
	}
	if stats.TotalBatches != 0 {
		t.Errorf("Expected 0 batches, got %d", stats.TotalBatches)
	}

	// Process some batches
	batchSize := 20
	batches := 5

	for i := 0; i < batches; i++ {
		batch := make([]any, batchSize)
		for j := 0; j < batchSize; j++ {
			batch[j] = j
		}
		backend.ProcessBatch(ctx, batch)
	}

	// Check stats
	stats = backend.GetStats()
	if stats.TotalBatches != int64(batches) {
		t.Errorf("Expected %d batches, got %d", batches, stats.TotalBatches)
	}

	// TotalProcessed should be close to batchSize * batches
	// (might be slightly less due to simulated errors)
	expectedMin := int64(batchSize * batches * 90 / 100) // Allow 10% errors
	if stats.TotalProcessed < expectedMin {
		t.Errorf("Expected at least %d processed, got %d", expectedMin, stats.TotalProcessed)
	}

	// Check other stats are valid
	if stats.CPULoad < 0 || stats.CPULoad > 1 {
		t.Errorf("CPULoad out of range: %v", stats.CPULoad)
	}
	if stats.ErrorRate < 0 || stats.ErrorRate > 1 {
		t.Errorf("ErrorRate out of range: %v", stats.ErrorRate)
	}
}

func TestBackend_QueueDepth(t *testing.T) {
	backend := NewBackend(PatternConstant)
	ctx := context.Background()

	// Queue should start at 0
	stats := backend.GetStats()
	if stats.QueueDepth != 0 {
		t.Errorf("Expected initial queue depth 0, got %d", stats.QueueDepth)
	}

	// Process a batch
	batch := make([]any, 50)
	for i := 0; i < 50; i++ {
		batch[i] = i
	}

	feedback, _ := backend.ProcessBatch(ctx, batch)

	// During processing, queue should have been > 0
	// After completion, it should be back to 0
	stats = backend.GetStats()
	if stats.QueueDepth != 0 {
		t.Errorf("Expected queue depth 0 after processing, got %d", stats.QueueDepth)
	}

	// Feedback should have recorded some queue depth
	if feedback.QueueDepth < 0 {
		t.Errorf("Feedback queue depth negative: %d", feedback.QueueDepth)
	}
}

func TestBackend_ProcessingTime(t *testing.T) {
	backend := NewBackend(PatternConstant)
	ctx := context.Background()

	batch := make([]any, 10)
	for i := 0; i < 10; i++ {
		batch[i] = i
	}

	start := time.Now()
	feedback, err := backend.ProcessBatch(ctx, batch)
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("ProcessBatch() error = %v", err)
	}

	// Processing time in feedback should be close to actual elapsed time
	if feedback.ProcessingTime <= 0 {
		t.Errorf("ProcessingTime should be positive, got %v", feedback.ProcessingTime)
	}

	if feedback.ProcessingTime > elapsed*2 {
		t.Errorf("ProcessingTime (%v) much longer than elapsed (%v)", feedback.ProcessingTime, elapsed)
	}
}

func TestBackend_GradualPattern(t *testing.T) {
	backend := NewBackend(PatternGradual)
	ctx := context.Background()

	batch := make([]any, 10)
	for i := 0; i < 10; i++ {
		batch[i] = i
	}

	var firstCPU, lastCPU float64

	// Process many batches
	for i := 0; i < 50; i++ {
		feedback, _ := backend.ProcessBatch(ctx, batch)
		if i == 0 {
			firstCPU = feedback.CPULoad
		}
		if i == 49 {
			lastCPU = feedback.CPULoad
		}
	}

	// CPU load should increase over time with gradual pattern
	if lastCPU <= firstCPU {
		t.Errorf("Expected CPU to increase from %v to %v", firstCPU, lastCPU)
	}
}

func TestBackend_CustomMetrics(t *testing.T) {
	backend := NewBackend(PatternConstant)
	ctx := context.Background()

	batch := make([]any, 5)
	for i := 0; i < 5; i++ {
		batch[i] = i
	}

	feedback, err := backend.ProcessBatch(ctx, batch)
	if err != nil {
		t.Errorf("ProcessBatch() error = %v", err)
	}

	// Check custom metrics
	if feedback.Custom == nil {
		t.Error("Custom metrics map is nil")
	}

	if batchSize, ok := feedback.Custom["batch_size"]; !ok {
		t.Error("batch_size not in custom metrics")
	} else if batchSize != 5 {
		t.Errorf("Expected batch_size 5, got %v", batchSize)
	}
}

func TestBackendStats_String(t *testing.T) {
	stats := BackendStats{
		CPULoad:        0.75,
		QueueDepth:     42,
		DBLocks:        10,
		ErrorRate:      0.05,
		TotalProcessed: 1000,
		TotalBatches:   50,
		TotalErrors:    25,
	}

	str := stats.String()
	if str == "" {
		t.Error("String() returned empty string")
	}

	// Check that string contains key information
	required := []string{"CPU", "Queue", "Locks", "Errors", "Processed"}
	for _, req := range required {
		if !contains(str, req) {
			t.Errorf("String() missing %s: %s", req, str)
		}
	}
}

func TestLoadPattern_String(t *testing.T) {
	tests := []struct {
		pattern LoadPattern
		want    string
	}{
		{PatternConstant, "constant"},
		{PatternSineWave, "sinewave"},
		{PatternSpikes, "spikes"},
		{PatternGradual, "gradual"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.pattern.String()
			if got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMath_Sin(t *testing.T) {
	// Test basic sine function
	result := Math.Sin(0)
	if result < -0.1 || result > 0.1 {
		t.Errorf("Sin(0) = %v, want ~0", result)
	}

	// Sin(π/2) should be close to 1
	result = Math.Sin(3.14159 / 2)
	if result < 0.9 || result > 1.1 {
		t.Errorf("Sin(π/2) = %v, want ~1", result)
	}
}

func TestMath_Min(t *testing.T) {
	tests := []struct {
		a, b float64
		want float64
	}{
		{1.0, 2.0, 1.0},
		{5.0, 3.0, 3.0},
		{-1.0, 0.0, -1.0},
		{2.5, 2.5, 2.5},
	}

	for _, tt := range tests {
		got := Math.Min(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("Min(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr)
}

func findSubstring(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(s) < len(substr) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Benchmark tests
func BenchmarkBackend_ProcessBatch(b *testing.B) {
	backend := NewBackend(PatternConstant)
	ctx := context.Background()

	batch := make([]any, 20)
	for i := 0; i < 20; i++ {
		batch[i] = i
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		backend.ProcessBatch(ctx, batch)
	}
}

func BenchmarkBackend_GetStats(b *testing.B) {
	backend := NewBackend(PatternConstant)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		backend.GetStats()
	}
}
