.PHONY: help test bench demo clean

help:
	@echo "Load-Aware Batcher - Available Commands:"
	@echo ""
	@echo "  make test          - Run all tests"
	@echo "  make test-verbose  - Run tests with verbose output"
	@echo "  make test-cover    - Run tests with coverage report"
	@echo "  make bench         - Run benchmarks"
	@echo "  make demo          - Run demo with default settings"
	@echo "  make demo-spikes   - Run demo with spike pattern"
	@echo "  make demo-sinewave - Run demo with sine wave pattern"
	@echo "  make demo-gradual  - Run demo with gradual load increase"
	@echo "  make clean         - Clean build artifacts"
	@echo "  make deps          - Download dependencies"
	@echo ""

deps:
	go mod download
	go mod tidy

test:
	go test ./...

test-verbose:
	go test -v ./...

test-cover:
	go test -cover ./...
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

bench:
	go test -bench=. -benchmem ./...

demo:
	go run ./cmd/demo -count=1000 -pattern=spikes -workers=4

demo-spikes:
	go run ./cmd/demo -count=2000 -pattern=spikes -workers=8 -initial-batch=30

demo-sinewave:
	go run ./cmd/demo -count=2000 -pattern=sinewave -workers=4 -initial-batch=20

demo-gradual:
	go run ./cmd/demo -count=3000 -pattern=gradual -workers=6 -initial-batch=25

demo-constant:
	go run ./cmd/demo -count=1000 -pattern=constant -workers=4 -initial-batch=20

clean:
	rm -f coverage.out coverage.html
	go clean
