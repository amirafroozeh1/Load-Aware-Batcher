package batcher

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: Config{
				InitialBatchSize: 10,
				MinBatchSize:     5,
				MaxBatchSize:     50,
				HandlerFunc:      func(ctx context.Context, batch []any) (*LoadFeedback, error) { return nil, nil },
			},
			wantErr: false,
		},
		{
			name: "zero initial batch size",
			cfg: Config{
				InitialBatchSize: 0,
				HandlerFunc:      func(ctx context.Context, batch []any) (*LoadFeedback, error) { return nil, nil },
			},
			wantErr: true,
		},
		{
			name: "nil handler",
			cfg: Config{
				InitialBatchSize: 10,
				HandlerFunc:      nil,
			},
			wantErr: true,
		},
		{
			name: "min > max",
			cfg: Config{
				InitialBatchSize: 10,
				MinBatchSize:     100,
				MaxBatchSize:     50,
				HandlerFunc:      func(ctx context.Context, batch []any) (*LoadFeedback, error) { return nil, nil },
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := New(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
			}
			if b != nil {
				b.Close(context.Background())
			}
		})
	}
}

func TestBatcher_Add(t *testing.T) {
	var processed atomic.Int64
	var batchCount atomic.Int64

	b, err := New(Config{
		InitialBatchSize: 10,
		MinBatchSize:     5,
		MaxBatchSize:     50,
		Timeout:          100 * time.Millisecond,
		HandlerFunc: func(ctx context.Context, batch []any) (*LoadFeedback, error) {
			processed.Add(int64(len(batch)))
			batchCount.Add(1)
			return &LoadFeedback{
				CPULoad:    0.5,
				QueueDepth: 0,
				ErrorRate:  0.0,
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer b.Close(context.Background())

	ctx := context.Background()

	// Add items
	for i := 0; i < 25; i++ {
		if err := b.Add(ctx, i); err != nil {
			t.Errorf("Add() error: %v", err)
		}
	}

	// Flush remaining
	b.Flush(ctx)

	if processed.Load() != 25 {
		t.Errorf("Expected 25 items processed, got %d", processed.Load())
	}
}

func TestBatcher_Flush(t *testing.T) {
	var processed atomic.Int64

	b, err := New(Config{
		InitialBatchSize: 100,
		HandlerFunc: func(ctx context.Context, batch []any) (*LoadFeedback, error) {
			processed.Add(int64(len(batch)))
			return &LoadFeedback{CPULoad: 0.5}, nil
		},
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer b.Close(context.Background())

	ctx := context.Background()

	// Add items (less than batch size)
	for i := 0; i < 10; i++ {
		b.Add(ctx, i)
	}

	// Manual flush
	if err := b.Flush(ctx); err != nil {
		t.Errorf("Flush() error: %v", err)
	}

	if processed.Load() != 10 {
		t.Errorf("Expected 10 items flushed, got %d", processed.Load())
	}
}

func TestBatcher_Timeout(t *testing.T) {
	var processed atomic.Int64

	b, err := New(Config{
		InitialBatchSize: 100,
		Timeout:          50 * time.Millisecond,
		HandlerFunc: func(ctx context.Context, batch []any) (*LoadFeedback, error) {
			processed.Add(int64(len(batch)))
			return &LoadFeedback{CPULoad: 0.3}, nil
		},
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer b.Close(context.Background())

	ctx := context.Background()

	// Add items (less than batch size)
	for i := 0; i < 5; i++ {
		b.Add(ctx, i)
	}

	// Wait for timeout
	time.Sleep(100 * time.Millisecond)

	if processed.Load() != 5 {
		t.Errorf("Expected timeout flush of 5 items, got %d", processed.Load())
	}
}

func TestBatcher_Concurrent(t *testing.T) {
	var processed atomic.Int64

	b, err := New(Config{
		InitialBatchSize: 20,
		HandlerFunc: func(ctx context.Context, batch []any) (*LoadFeedback, error) {
			processed.Add(int64(len(batch)))
			time.Sleep(10 * time.Millisecond) // Simulate processing
			return &LoadFeedback{
				CPULoad:    0.5,
				QueueDepth: 10,
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer b.Close(context.Background())

	ctx := context.Background()
	numWorkers := 10
	itemsPerWorker := 100

	var wg sync.WaitGroup
	wg.Add(numWorkers)

	for i := 0; i < numWorkers; i++ {
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < itemsPerWorker; j++ {
				if err := b.Add(ctx, workerID*itemsPerWorker+j); err != nil {
					t.Errorf("Add() error: %v", err)
				}
			}
		}(i)
	}

	wg.Wait()
	b.Flush(ctx)

	expected := int64(numWorkers * itemsPerWorker)
	if processed.Load() != expected {
		t.Errorf("Expected %d items processed, got %d", expected, processed.Load())
	}
}

func TestBatcher_Close(t *testing.T) {
	var processed atomic.Int64

	b, err := New(Config{
		InitialBatchSize: 100,
		HandlerFunc: func(ctx context.Context, batch []any) (*LoadFeedback, error) {
			processed.Add(int64(len(batch)))
			return &LoadFeedback{CPULoad: 0.5}, nil
		},
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx := context.Background()

	// Add items
	for i := 0; i < 10; i++ {
		b.Add(ctx, i)
	}

	// Close should flush
	if err := b.Close(ctx); err != nil {
		t.Errorf("Close() error: %v", err)
	}

	if processed.Load() != 10 {
		t.Errorf("Expected Close() to flush 10 items, got %d", processed.Load())
	}

	// Subsequent Add should return error
	if err := b.Add(ctx, 11); err != ErrClosed {
		t.Errorf("Expected ErrClosed, got %v", err)
	}
}

func TestBatcher_AdaptiveSizing(t *testing.T) {
	b, err := New(Config{
		InitialBatchSize:  20,
		MinBatchSize:      5,
		MaxBatchSize:      50,
		LoadCheckInterval: 100 * time.Millisecond,
		AdjustmentFactor:  0.5,
		HandlerFunc: func(ctx context.Context, batch []any) (*LoadFeedback, error) {
			// Simulate high load
			return &LoadFeedback{
				CPULoad:    0.9,
				QueueDepth: 100,
				ErrorRate:  0.5,
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer b.Close(context.Background())

	ctx := context.Background()

	initialSize := b.GetCurrentBatchSize()

	// Add enough items to trigger multiple flushes
	for i := 0; i < 200; i++ {
		b.Add(ctx, i)
	}

	// Wait for adjustment
	time.Sleep(400 * time.Millisecond)

	// Batch size should decrease due to high load
	newSize := b.GetCurrentBatchSize()
	if newSize >= initialSize {
		t.Errorf("Expected batch size to decrease from %d due to high load, got %d", initialSize, newSize)
	}
}

func TestLoadFeedback_LoadScore(t *testing.T) {
	tests := []struct {
		name     string
		feedback LoadFeedback
		wantMin  float64
		wantMax  float64
	}{
		{
			name: "low load",
			feedback: LoadFeedback{
				CPULoad:    0.1,
				QueueDepth: 5,
				ErrorRate:  0.0,
				DBLocks:    2,
			},
			wantMin: 0.0,
			wantMax: 0.3,
		},
		{
			name: "high load",
			feedback: LoadFeedback{
				CPULoad:    0.9,
				QueueDepth: 150,
				ErrorRate:  0.2,
				DBLocks:    60,
			},
			wantMin: 0.7,
			wantMax: 1.0,
		},
		{
			name: "medium load",
			feedback: LoadFeedback{
				CPULoad:    0.5,
				QueueDepth: 50,
				ErrorRate:  0.05,
				DBLocks:    20,
			},
			wantMin: 0.3,
			wantMax: 0.7,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := tt.feedback.LoadScore()
			if score < tt.wantMin || score > tt.wantMax {
				t.Errorf("LoadScore() = %v, want between %v and %v", score, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func BenchmarkBatcher_Add(b *testing.B) {
	batcher, _ := New(Config{
		InitialBatchSize: 100,
		HandlerFunc: func(ctx context.Context, batch []any) (*LoadFeedback, error) {
			return &LoadFeedback{CPULoad: 0.5}, nil
		},
	})
	defer batcher.Close(context.Background())

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		batcher.Add(ctx, i)
	}
}

func BenchmarkBatcher_Concurrent(b *testing.B) {
	batcher, _ := New(Config{
		InitialBatchSize: 100,
		HandlerFunc: func(ctx context.Context, batch []any) (*LoadFeedback, error) {
			return &LoadFeedback{CPULoad: 0.5}, nil
		},
	})
	defer batcher.Close(context.Background())

	ctx := context.Background()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			batcher.Add(ctx, i)
			i++
		}
	})
}
