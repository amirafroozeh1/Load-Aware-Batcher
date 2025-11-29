package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/amirafroozeh/load-aware-batcher"
	"github.com/amirafroozeh/load-aware-batcher/simulator"
)

func main() {
	// Parse flags
	itemCount := flag.Int("count", 1000, "number of items to process")
	initialBatchSize := flag.Int("initial-batch", 20, "initial batch size")
	minBatchSize := flag.Int("min-batch", 5, "minimum batch size")
	maxBatchSize := flag.Int("max-batch", 100, "maximum batch size")
	timeout := flag.Duration("timeout", 2*time.Second, "flush timeout")
	workers := flag.Int("workers", 4, "number of worker goroutines")
	loadPattern := flag.String("pattern", "spikes", "load pattern: constant, sinewave, spikes, gradual")
	adjustInterval := flag.Duration("adjust-interval", 3*time.Second, "batch size adjustment interval")
	adjustFactor := flag.Float64("adjust-factor", 0.3, "adjustment factor (0.1-1.0)")
	flag.Parse()

	fmt.Println("🚀 Load-Aware Batcher Demo")
	fmt.Println("=" + repeat("=", 60))
	fmt.Printf("Items: %d | Workers: %d | Pattern: %s\n", *itemCount, *workers, *loadPattern)
	fmt.Printf("Batch Size: %d (min: %d, max: %d)\n", *initialBatchSize, *minBatchSize, *maxBatchSize)
	fmt.Println("=" + repeat("=", 60))
	fmt.Println()

	startTime := time.Now()

	// Create backend simulator with chosen pattern
	pattern := parseLoadPattern(*loadPattern)
	backend := simulator.NewBackend(pattern)

	// Create load-aware batcher
	b, err := batcher.New(batcher.Config{
		InitialBatchSize:  *initialBatchSize,
		MinBatchSize:      *minBatchSize,
		MaxBatchSize:      *maxBatchSize,
		Timeout:           *timeout,
		HandlerFunc:       backend.ProcessBatch,
		AdjustmentFactor:  *adjustFactor,
		LoadCheckInterval: *adjustInterval,
	})
	if err != nil {
		log.Fatalf("Failed to create batcher: %v", err)
	}

	// Statistics
	var itemsAdded atomic.Int64
	var itemsProcessed atomic.Int64

	// Start monitoring goroutine
	stopMonitor := make(chan struct{})
	var monitorWg sync.WaitGroup
	monitorWg.Add(1)
	go func() {
		defer monitorWg.Done()
		monitor(b, backend, &itemsAdded, &itemsProcessed, stopMonitor)
	}()

	// Worker pool
	itemChan := make(chan int, *workers*10)
	var workerWg sync.WaitGroup
	workerWg.Add(*workers)

	for i := 0; i < *workers; i++ {
		go func(workerID int) {
			defer workerWg.Done()
			ctx := context.Background()

			for item := range itemChan {
				if err := b.Add(ctx, item); err != nil {
					log.Printf("Worker %d: failed to add item: %v", workerID, err)
				}
			}
		}(i)
	}

	// Generate items
	go func() {
		for i := 0; i < *itemCount; i++ {
			itemChan <- i
			itemsAdded.Add(1)
			
			// Simulate varying production rate
			if i%100 == 0 {
				time.Sleep(10 * time.Millisecond)
			}
		}
		close(itemChan)
	}()

	// Wait for workers to finish
	workerWg.Wait()

	// Final flush
	if err := b.Flush(context.Background()); err != nil {
		log.Printf("Final flush error: %v", err)
	}

	// Close batcher
	if err := b.Close(context.Background()); err != nil {
		log.Printf("Close error: %v", err)
	}

	// Stop monitoring
	close(stopMonitor)
	monitorWg.Wait()

	// Final statistics
	duration := time.Since(startTime)
	backendStats := backend.GetStats()

	fmt.Println()
	fmt.Println("=" + repeat("=", 60))
	fmt.Println("📊 Final Statistics")
	fmt.Println("=" + repeat("=", 60))
	fmt.Printf("Duration: %v\n", duration)
	fmt.Printf("Items Added: %d\n", itemsAdded.Load())
	fmt.Printf("Batches Processed: %d\n", backendStats.TotalBatches)
	fmt.Printf("Items Processed: %d\n", backendStats.TotalProcessed)
	fmt.Printf("Errors: %d (%.2f%%)\n", backendStats.TotalErrors, 
		float64(backendStats.TotalErrors)/float64(backendStats.TotalProcessed)*100)
	
	if backendStats.TotalBatches > 0 {
		avgBatchSize := float64(backendStats.TotalProcessed) / float64(backendStats.TotalBatches)
		fmt.Printf("Average Batch Size: %.1f\n", avgBatchSize)
	}
	
	throughput := float64(backendStats.TotalProcessed) / duration.Seconds()
	fmt.Printf("Throughput: %.1f items/sec\n", throughput)
	fmt.Println("=" + repeat("=", 60))
}

// monitor displays real-time statistics
func monitor(b *batcher.Batcher, backend *simulator.Backend, 
	itemsAdded, itemsProcessed *atomic.Int64, stop chan struct{}) {
	
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	iteration := 0
	for {
		select {
		case <-ticker.C:
			iteration++
			
			batcherStats := b.GetStats()
			backendStats := backend.GetStats()
			
			loadScore := 0.0
			if batcherStats.AverageLoadScore > 0 {
				loadScore = batcherStats.AverageLoadScore
			}
			
			fmt.Printf("[%2ds] Batch Size: %3d | Pending: %3d | Load: %s | Backend: %s\n",
				iteration,
				batcherStats.CurrentBatchSize,
				batcherStats.PendingItems,
				formatLoadScore(loadScore),
				formatBackendStatus(backendStats),
			)
			
		case <-stop:
			return
		}
	}
}

// parseLoadPattern converts string to LoadPattern
func parseLoadPattern(pattern string) simulator.LoadPattern {
	switch pattern {
	case "constant":
		return simulator.PatternConstant
	case "sinewave":
		return simulator.PatternSineWave
	case "spikes":
		return simulator.PatternSpikes
	case "gradual":
		return simulator.PatternGradual
	default:
		return simulator.PatternSpikes
	}
}

// formatLoadScore formats load score with color indicators
func formatLoadScore(score float64) string {
	indicator := ""
	if score < 0.3 {
		indicator = "🟢 Low "
	} else if score < 0.7 {
		indicator = "🟡 Med "
	} else {
		indicator = "🔴 High"
	}
	return fmt.Sprintf("%s %.2f", indicator, score)
}

// formatBackendStatus formats backend status concisely
func formatBackendStatus(stats simulator.BackendStats) string {
	return fmt.Sprintf("CPU: %3.0f%% | Q: %3d | Batches: %d",
		stats.CPULoad*100,
		stats.QueueDepth,
		stats.TotalBatches,
	)
}

// repeat repeats a string n times
func repeat(s string, n int) string {
	result := ""
	for i := 0; i < n; i++ {
		result += s
	}
	return result
}
