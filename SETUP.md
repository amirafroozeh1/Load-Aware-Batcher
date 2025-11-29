# Load-Aware Batcher - Quick Setup Guide

## Prerequisites

Ensure you have Go installed (version 1.21 or later):
```bash
go version
```

If not installed, download from: https://go.dev/dl/

## Installation

1. **Initialize the module:**
```bash
cd "/Users/amirafroozeh/Documents/Startup/Load-aware Batcher"
go mod tidy
```

2. **Run tests:**
```bash
go test -v
```

3. **Run benchmarks:**
```bash
go test -bench=. -benchmem
```

## Running the Demo

### Basic demo with spike pattern:
```bash
go run ./cmd/demo
```

### Custom configuration:
```bash
go run ./cmd/demo \
  -count=2000 \
  -initial-batch=30 \
  -min-batch=10 \
  -max-batch=200 \
  -pattern=spikes \
  -workers=8 \
  -adjust-interval=2s \
  -adjust-factor=0.4
```

## Available Load Patterns

1. **spikes** (default) - Random load spikes, best for testing adaptation
2. **sinewave** - Periodic load variations
3. **constant** - Steady load
4. **gradual** - Gradually increasing load

## Expected Output

You should see real-time monitoring like:

```
🚀 Load-Aware Batcher Demo
============================================================
Items: 1000 | Workers: 4 | Pattern: spikes
Batch Size: 20 (min: 5, max: 100)
============================================================

[ 1s] Batch Size:  20 | Pending:   5 | Load: 🟢 Low  0.25 | Backend: CPU:  30% | Q:   0 | Batches: 12
[ 2s] Batch Size:  26 | Pending:   3 | Load: 🟢 Low  0.22 | Backend: CPU:  25% | Q:   2 | Batches: 25
[ 3s] Batch Size:  34 | Pending:   8 | Load: 🟢 Low  0.28 | Backend: CPU:  35% | Q:   5 | Batches: 40
[ 4s] Batch Size:  20 | Pending:  15 | Load: 🔴 High 0.85 | Backend: CPU:  95% | Q:  80 | Batches: 48
[ 5s] Batch Size:  14 | Pending:   4 | Load: 🔴 High 0.78 | Backend: CPU:  85% | Q:  35 | Batches: 58
```

Notice how the batch size:
- **Increases** (20 → 34) when load is low 🟢
- **Decreases** (34 → 14) when load spikes 🔴

## Project Structure

```
.
├── README.md              # Main documentation
├── SETUP.md              # This file
├── LICENSE               # MIT License
├── go.mod                # Go module definition
├── .gitignore            # Git ignore rules
├── batcher.go            # Core load-aware batcher
├── batcher_test.go       # Comprehensive tests
├── cmd/
│   └── demo/
│       └── main.go       # Demo application
└── simulator/
    └── backend.go        # Backend load simulator
```

## Using in Your Project

```go
import "github.com/amirafroozeh1/Load-Aware-Batcher"

// Create batcher
b, err := batcher.New(batcher.Config{
    InitialBatchSize:  20,
    MinBatchSize:      5,
    MaxBatchSize:      100,
    Timeout:           2 * time.Second,
    
    HandlerFunc: func(ctx context.Context, batch []any) (*batcher.LoadFeedback, error) {
        // Your processing logic
        startTime := time.Now()
        err := processBatch(batch)
        
        // Return load feedback
        return &batcher.LoadFeedback{
            CPULoad:        getCurrentCPU(),
            QueueDepth:     getQueueDepth(),
            ProcessingTime: time.Since(startTime),
            ErrorRate:      calculateErrors(err),
        }, err
    },
})

// Add items
for item := range items {
    b.Add(ctx, item)
}

// Cleanup
b.Close(ctx)
```

## Troubleshooting

### "command not found: go"
Install Go from https://go.dev/dl/

### Module errors
Run `go mod tidy` in the project directory

### Import errors
Make sure you're in the correct directory and go.mod exists

## Next Steps

1. Read the full README.md for detailed documentation
2. Check out the test file (batcher_test.go) for usage examples
3. Experiment with different load patterns in the demo
4. Integrate into your own project!

## Performance Tips

- Start with `AdjustmentFactor` around 0.3-0.5
- Set `LoadCheckInterval` to 3-5 seconds for most use cases
- Use smaller `MinBatchSize` for latency-sensitive workloads
- Use larger `MaxBatchSize` for throughput-oriented workloads
- Monitor the load scores and adjust thresholds if needed

## Contributing

Issues and PRs welcome! This is a learning project demonstrating adaptive batching concepts.
