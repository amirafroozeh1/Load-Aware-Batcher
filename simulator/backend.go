package simulator

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/amirafroozeh1/Load-Aware-Batcher"
)

// Backend simulates a backend service with varying load
type Backend struct {
	mu sync.Mutex

	// Current state
	cpuLoad      float64
	queueDepth   int
	dbLocks      int
	errorRate    float64
	
	// Config
	maxQueueDepth int
	loadPattern   LoadPattern
	
	// Stats
	totalProcessed int64
	totalBatches   int64
	totalErrors    int64
}

// LoadPattern defines how backend load varies over time
type LoadPattern int

const (
	// PatternConstant maintains steady load
	PatternConstant LoadPattern = iota
	
	// PatternSineWave creates periodic load variations
	PatternSineWave
	
	// PatternSpikes creates random load spikes
	PatternSpikes
	
	// PatternGradual gradually increases load over time
	PatternGradual
)

// String returns the string representation of LoadPattern
func (lp LoadPattern) String() string {
	switch lp {
	case PatternConstant:
		return "constant"
	case PatternSineWave:
		return "sinewave"
	case PatternSpikes:
		return "spikes"
	case PatternGradual:
		return "gradual"
	default:
		return "unknown"
	}
}

// NewBackend creates a new backend simulator
func NewBackend(pattern LoadPattern) *Backend {
	return &Backend{
		cpuLoad:       0.3,
		queueDepth:    0,
		dbLocks:       0,
		errorRate:     0.01,
		maxQueueDepth: 200,
		loadPattern:   pattern,
	}
}

// ProcessBatch simulates processing a batch and returns load feedback
func (b *Backend) ProcessBatch(ctx context.Context, batch []any) (*batcher.LoadFeedback, error) {
	startTime := time.Now()
	
	b.mu.Lock()
	
	// Add to queue
	batchSize := len(batch)
	b.queueDepth += batchSize
	
	// Update load based on pattern
	b.updateLoad()
	
	// Simulate processing time based on queue depth and CPU load
	processingTime := b.calculateProcessingTime(batchSize)
	
	b.mu.Unlock()
	
	// Simulate actual processing
	time.Sleep(processingTime)
	
	b.mu.Lock()
	defer b.mu.Unlock()
	
	// Remove from queue
	b.queueDepth -= batchSize
	if b.queueDepth < 0 {
		b.queueDepth = 0
	}
	
	// Update stats
	b.totalBatches++
	
	// Simulate errors based on load
	errors := 0
	for i := 0; i < batchSize; i++ {
		if rand.Float64() < b.errorRate {
			errors++
			b.totalErrors++
		} else {
			b.totalProcessed++
		}
	}
	
	currentErrorRate := 0.0
	if batchSize > 0 {
		currentErrorRate = float64(errors) / float64(batchSize)
	}
	
	// Create feedback
	feedback := &batcher.LoadFeedback{
		CPULoad:        b.cpuLoad,
		QueueDepth:     b.queueDepth,
		ProcessingTime: time.Since(startTime),
		ErrorRate:      currentErrorRate,
		DBLocks:        b.dbLocks,
		Custom: map[string]interface{}{
			"batch_size": batchSize,
		},
	}
	
	return feedback, nil
}

// updateLoad updates backend load based on the pattern
func (b *Backend) updateLoad() {
	switch b.loadPattern {
	case PatternConstant:
		// Keep load constant
		b.cpuLoad = 0.5
		b.errorRate = 0.01
		
	case PatternSineWave:
		// Sine wave pattern (period ~60 seconds)
		t := float64(time.Now().Unix())
		b.cpuLoad = 0.5 + 0.4*Math.Sin(t/10.0)
		b.errorRate = 0.01 + 0.05*Math.Sin(t/10.0)
		if b.errorRate < 0 {
			b.errorRate = 0
		}
		
	case PatternSpikes:
		// Random spikes
		if rand.Float64() < 0.1 { // 10% chance of spike
			b.cpuLoad = 0.9 + rand.Float64()*0.1
			b.errorRate = 0.1
			b.dbLocks = 30 + rand.Intn(40)
		} else {
			b.cpuLoad = 0.2 + rand.Float64()*0.3
			b.errorRate = 0.01
			b.dbLocks = rand.Intn(10)
		}
		
	case PatternGradual:
		// Gradually increase load
		increase := float64(b.totalBatches) * 0.001
		b.cpuLoad = Math.Min(0.2+increase, 0.95)
		b.errorRate = Math.Min(0.01+increase*0.05, 0.2)
	}
	
	// Adjust DB locks based on queue depth
	if b.queueDepth > 100 {
		b.dbLocks = 20 + rand.Intn(30)
	} else {
		b.dbLocks = rand.Intn(10)
	}
}

// calculateProcessingTime calculates how long processing should take
func (b *Backend) calculateProcessingTime(batchSize int) time.Duration {
	// Base processing time per item
	baseTime := 1 * time.Millisecond
	
	// Adjust based on CPU load (higher load = slower processing)
	loadMultiplier := 1.0 + b.cpuLoad*2
	
	// Adjust based on queue depth (deeper queue = more contention)
	queueMultiplier := 1.0
	if b.queueDepth > 50 {
		queueMultiplier = 1.5
	}
	if b.queueDepth > 100 {
		queueMultiplier = 2.0
	}
	
	totalTime := float64(baseTime) * float64(batchSize) * loadMultiplier * queueMultiplier
	
	// Add some randomness
	jitter := 0.8 + rand.Float64()*0.4 // 80% to 120%
	totalTime *= jitter
	
	return time.Duration(totalTime)
}

// GetStats returns current backend statistics
func (b *Backend) GetStats() BackendStats {
	b.mu.Lock()
	defer b.mu.Unlock()
	
	return BackendStats{
		CPULoad:        b.cpuLoad,
		QueueDepth:     b.queueDepth,
		DBLocks:        b.dbLocks,
		ErrorRate:      b.errorRate,
		TotalProcessed: b.totalProcessed,
		TotalBatches:   b.totalBatches,
		TotalErrors:    b.totalErrors,
	}
}

// BackendStats holds backend statistics
type BackendStats struct {
	CPULoad        float64
	QueueDepth     int
	DBLocks        int
	ErrorRate      float64
	TotalProcessed int64
	TotalBatches   int64
	TotalErrors    int64
}

// String formats backend stats as a string
func (s BackendStats) String() string {
	return fmt.Sprintf(
		"CPU: %.1f%% | Queue: %d | Locks: %d | Errors: %.1f%% | Processed: %d batches (%d items, %d errors)",
		s.CPULoad*100,
		s.QueueDepth,
		s.DBLocks,
		s.ErrorRate*100,
		s.TotalBatches,
		s.TotalProcessed,
		s.TotalErrors,
	)
}

// Math helpers (since we can't import math in some contexts)
type MathHelper struct{}

var Math = MathHelper{}

func (MathHelper) Sin(x float64) float64 {
	// Simple sine approximation using Taylor series
	// For demo purposes only
	x = x - float64(int(x/(2*3.14159)))*2*3.14159
	result := x
	term := x
	for i := 1; i < 10; i++ {
		term *= -x * x / float64((2*i)*(2*i+1))
		result += term
	}
	return result
}

func (MathHelper) Min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
