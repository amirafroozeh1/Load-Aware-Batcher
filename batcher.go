package batcher

import (
	"context"
	"errors"
	"math"
	"sync"
	"time"
)

// LoadFeedback represents backend load metrics returned by the handler
type LoadFeedback struct {
	// CPULoad is the CPU usage percentage (0.0 to 1.0)
	CPULoad float64

	// QueueDepth is the number of pending items in backend queue
	QueueDepth int

	// ProcessingTime is how long it took to process the batch
	ProcessingTime time.Duration

	// ErrorRate is the percentage of errors in this batch (0.0 to 1.0)
	ErrorRate float64

	// DBLocks is the number of database lock contentions
	DBLocks int

	// Custom can hold any additional metrics
	Custom map[string]interface{}
}

// LoadScore calculates a normalized load score (0.0 = idle, 1.0 = overloaded)
func (lf *LoadFeedback) LoadScore() float64 {
	// Weighted combination of different metrics
	score := 0.0

	// CPU load (60% weight) - increased from 40% to be more responsive to CPU pressure
	score += lf.CPULoad * 0.6

	// Queue depth normalized (15% weight)
	// Assume queue depth > 100 is critical
	queueScore := math.Min(float64(lf.QueueDepth)/100.0, 1.0)
	score += queueScore * 0.15

	// Error rate (15% weight)
	score += lf.ErrorRate * 0.15

	// DB locks normalized (10% weight)
	// Assume > 50 locks is critical
	lockScore := math.Min(float64(lf.DBLocks)/50.0, 1.0)
	score += lockScore * 0.1

	return math.Min(score, 1.0)
}

// HandlerFunc processes a batch and returns load feedback
// The batch slice must be treated as read-only and not retained.
type HandlerFunc func(ctx context.Context, batch []any) (*LoadFeedback, error)

// Config holds the configuration for the load-aware batcher
type Config struct {
	// InitialBatchSize is the starting batch size
	InitialBatchSize int

	// MinBatchSize is the minimum allowed batch size
	MinBatchSize int

	// MaxBatchSize is the maximum allowed batch size
	MaxBatchSize int

	// Timeout is the maximum time items can sit in the batch
	// before a flush is triggered. If Timeout <= 0, only size-based
	// flushing is used.
	Timeout time.Duration

	// HandlerFunc is called with each flushed batch
	HandlerFunc HandlerFunc

	// AdjustmentFactor controls how aggressively batch size changes (default: 0.2)
	// Higher values = more aggressive adjustments
	AdjustmentFactor float64

	// LoadCheckInterval is how often to recalculate optimal batch size
	// based on recent load feedback (default: 5 seconds)
	LoadCheckInterval time.Duration
}

var (
	// ErrClosed is returned when Add is called after Close
	ErrClosed = errors.New("batcher: closed")

	// ErrInvalidConfig is returned when configuration is invalid
	ErrInvalidConfig = errors.New("batcher: invalid configuration")
)

// Batcher accumulates items in memory and flushes them based on
// dynamic batch size adjusted by backend load
type Batcher struct {
	mu     sync.Mutex
	batch  []any
	cfg    Config
	timer  *time.Timer
	closed bool

	// Load tracking
	currentBatchSize int
	recentFeedback   []LoadFeedback
	maxFeedbackLen   int
	adjustTicker     *time.Ticker
	stopAdjust       chan struct{}
	wg               sync.WaitGroup
}

// New creates a new load-aware Batcher with the given configuration
func New(cfg Config) (*Batcher, error) {
	// Validate configuration
	if cfg.InitialBatchSize <= 0 {
		return nil, ErrInvalidConfig
	}
	if cfg.MinBatchSize <= 0 {
		cfg.MinBatchSize = 1
	}
	if cfg.MaxBatchSize <= 0 {
		cfg.MaxBatchSize = 1000
	}
	if cfg.MinBatchSize > cfg.MaxBatchSize {
		return nil, ErrInvalidConfig
	}
	if cfg.InitialBatchSize < cfg.MinBatchSize {
		cfg.InitialBatchSize = cfg.MinBatchSize
	}
	if cfg.InitialBatchSize > cfg.MaxBatchSize {
		cfg.InitialBatchSize = cfg.MaxBatchSize
	}
	if cfg.HandlerFunc == nil {
		return nil, ErrInvalidConfig
	}
	if cfg.AdjustmentFactor <= 0 {
		cfg.AdjustmentFactor = 0.2
	}
	if cfg.LoadCheckInterval <= 0 {
		cfg.LoadCheckInterval = 5 * time.Second
	}

	b := &Batcher{
		batch:            make([]any, 0, cfg.InitialBatchSize),
		cfg:              cfg,
		currentBatchSize: cfg.InitialBatchSize,
		recentFeedback:   make([]LoadFeedback, 0, 10),
		maxFeedbackLen:   10,
		stopAdjust:       make(chan struct{}),
	}

	// Start background goroutine to adjust batch size based on load
	b.adjustTicker = time.NewTicker(cfg.LoadCheckInterval)
	b.wg.Add(1)
	go b.adjustBatchSizeLoop()

	return b, nil
}

// Add adds one item to the batch
func (b *Batcher) Add(ctx context.Context, item any) error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return ErrClosed
	}

	wasEmpty := len(b.batch) == 0
	b.batch = append(b.batch, item)

	// Check if we've reached the current dynamic batch size
	if len(b.batch) >= b.currentBatchSize {
		batch := b.detachBatchLocked()
		b.stopTimerLocked()
		b.mu.Unlock()

		// Process batch and get feedback
		return b.processBatch(ctx, batch)
	}

	// Only schedule a timeout when we transition from empty -> non-empty
	if wasEmpty && b.cfg.Timeout > 0 && b.timer == nil {
		b.startTimerLocked()
	}

	b.mu.Unlock()
	return nil
}

// Flush flushes the current batch, if any
func (b *Batcher) Flush(ctx context.Context) error {
	b.mu.Lock()
	if len(b.batch) == 0 {
		b.mu.Unlock()
		return nil
	}

	batch := b.detachBatchLocked()
	b.stopTimerLocked()
	b.mu.Unlock()

	return b.processBatch(ctx, batch)
}

// Close marks the batcher as closed and flushes any remaining items
func (b *Batcher) Close(ctx context.Context) error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil
	}
	b.closed = true
	b.mu.Unlock()

	// Stop adjustment goroutine
	close(b.stopAdjust)
	b.adjustTicker.Stop()
	b.wg.Wait()

	return b.Flush(ctx)
}

// GetCurrentBatchSize returns the current dynamic batch size
func (b *Batcher) GetCurrentBatchSize() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.currentBatchSize
}

// GetStats returns current statistics
func (b *Batcher) GetStats() Stats {
	b.mu.Lock()
	defer b.mu.Unlock()

	avgLoad := 0.0
	if len(b.recentFeedback) > 0 {
		for _, f := range b.recentFeedback {
			avgLoad += f.LoadScore()
		}
		avgLoad /= float64(len(b.recentFeedback))
	}

	return Stats{
		CurrentBatchSize:   b.currentBatchSize,
		PendingItems:       len(b.batch),
		AverageLoadScore:   avgLoad,
		RecentFeedbackSize: len(b.recentFeedback),
	}
}

// Stats holds batcher statistics
type Stats struct {
	CurrentBatchSize   int
	PendingItems       int
	AverageLoadScore   float64
	RecentFeedbackSize int
}

// --- Internal methods ---

func (b *Batcher) processBatch(ctx context.Context, batch []any) error {
	feedback, err := b.cfg.HandlerFunc(ctx, batch)

	// Store feedback for batch size adjustment
	if feedback != nil {
		b.mu.Lock()
		b.recordFeedback(*feedback)
		b.mu.Unlock()
	}

	return err
}

func (b *Batcher) recordFeedback(feedback LoadFeedback) {
	b.recentFeedback = append(b.recentFeedback, feedback)
	if len(b.recentFeedback) > b.maxFeedbackLen {
		b.recentFeedback = b.recentFeedback[1:]
	}
}

func (b *Batcher) adjustBatchSizeLoop() {
	defer b.wg.Done()

	for {
		select {
		case <-b.adjustTicker.C:
			b.adjustBatchSize()
		case <-b.stopAdjust:
			return
		}
	}
}

func (b *Batcher) adjustBatchSize() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.recentFeedback) == 0 {
		return
	}

	// Calculate average load score
	avgLoad := 0.0
	for _, f := range b.recentFeedback {
		avgLoad += f.LoadScore()
	}
	avgLoad /= float64(len(b.recentFeedback))

	// Adjust batch size based on load
	// Low load (< 0.25) -> increase batch size
	// Medium load (0.25 - 0.55) -> keep current size
	// High load (> 0.55) -> decrease batch size

	newSize := b.currentBatchSize

	if avgLoad < 0.25 {
		// Backend is idle, increase batch size
		increase := float64(b.currentBatchSize) * b.cfg.AdjustmentFactor
		newSize = b.currentBatchSize + int(math.Max(increase, 1))
	} else if avgLoad > 0.55 {
		// Backend is overloaded, decrease batch size
		decrease := float64(b.currentBatchSize) * b.cfg.AdjustmentFactor
		newSize = b.currentBatchSize - int(math.Max(decrease, 1))
	}

	// Clamp to min/max
	if newSize < b.cfg.MinBatchSize {
		newSize = b.cfg.MinBatchSize
	}
	if newSize > b.cfg.MaxBatchSize {
		newSize = b.cfg.MaxBatchSize
	}

	b.currentBatchSize = newSize
}

func (b *Batcher) detachBatchLocked() []any {
	if len(b.batch) == 0 {
		return nil
	}
	batch := b.batch
	b.batch = make([]any, 0, b.currentBatchSize)
	return batch
}

func (b *Batcher) stopTimerLocked() {
	if b.timer != nil {
		b.timer.Stop()
		b.timer = nil
	}
}

func (b *Batcher) startTimerLocked() {
	timeout := b.cfg.Timeout
	b.timer = time.AfterFunc(timeout, func() {
		_ = b.Flush(context.Background())
	})
}
