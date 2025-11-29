package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"sync"
	"time"

	batcher "github.com/amirafroozeh1/Load-Aware-Batcher"
	"github.com/amirafroozeh1/Load-Aware-Batcher/simulator"
)

type MetricsSnapshot struct {
	Timestamp        int64   `json:"timestamp"`
	BatchSize        int     `json:"batchSize"`
	PendingItems     int     `json:"pendingItems"`
	CPULoad          float64 `json:"cpuLoad"`
	QueueDepth       int     `json:"queueDepth"`
	ErrorRate        float64 `json:"errorRate"`
	ProcessingTimeMs int64   `json:"processingTimeMs"`
	LoadScore        float64 `json:"loadScore"`
	TotalProcessed   int64   `json:"totalProcessed"`
	TotalBatches     int64   `json:"totalBatches"`
}

type DashboardServer struct {
	mu               sync.RWMutex
	metrics          []MetricsSnapshot
	maxMetrics       int
	backend          *simulator.Backend
	batcher          *batcher.Batcher
	currentPattern   simulator.LoadPattern
	itemsProcessed   int64
	batchesProcessed int64
	workerCount      int
	running          bool
	stopChan         chan struct{}
	lastProcTime     time.Duration
}

func NewDashboardServer() *DashboardServer {
	return &DashboardServer{
		metrics:        make([]MetricsSnapshot, 0, 100),
		maxMetrics:     100,
		currentPattern: simulator.PatternConstant,
		workerCount:    4,
	}
}

func (ds *DashboardServer) Start(pattern simulator.LoadPattern) error {
	ds.mu.Lock()
	if ds.running {
		ds.mu.Unlock()
		return fmt.Errorf("already running")
	}
	ds.running = true
	ds.currentPattern = pattern
	ds.itemsProcessed = 0
	ds.batchesProcessed = 0
	ds.stopChan = make(chan struct{})
	ds.mu.Unlock()

	// Create backend simulator
	ds.backend = simulator.NewBackend(pattern)

	// Create batcher
	b, err := batcher.New(batcher.Config{
		InitialBatchSize:  20,
		MinBatchSize:      5,
		MaxBatchSize:      100,
		Timeout:           2 * time.Second,
		AdjustmentFactor:  0.3,
		LoadCheckInterval: 3 * time.Second,
		HandlerFunc:       ds.handleBatch,
	})
	if err != nil {
		ds.mu.Lock()
		ds.running = false
		ds.mu.Unlock()
		return err
	}
	ds.batcher = b

	// Start worker goroutines
	for i := 0; i < ds.workerCount; i++ {
		go ds.worker(i)
	}

	// Start metrics collection
	go ds.collectMetrics()

	return nil
}

func (ds *DashboardServer) Stop() {
	ds.mu.Lock()
	if !ds.running {
		ds.mu.Unlock()
		return
	}
	ds.running = false
	close(ds.stopChan)
	ds.mu.Unlock()

	if ds.batcher != nil {
		ds.batcher.Close(context.Background())
	}
}

func (ds *DashboardServer) handleBatch(ctx context.Context, batch []any) (*batcher.LoadFeedback, error) {
	feedback, err := ds.backend.ProcessBatch(ctx, batch)

	ds.mu.Lock()
	ds.itemsProcessed += int64(len(batch))
	ds.batchesProcessed++
	if feedback != nil {
		ds.lastProcTime = feedback.ProcessingTime
	}
	ds.mu.Unlock()

	return feedback, err
}

func (ds *DashboardServer) worker(id int) {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ds.stopChan:
			return
		case <-ticker.C:
			ds.mu.RLock()
			running := ds.running
			ds.mu.RUnlock()

			if !running {
				return
			}

			// Add random number of items
			count := rand.Intn(5) + 1
			for i := 0; i < count; i++ {
				ds.batcher.Add(context.Background(), fmt.Sprintf("item-%d-%d", id, i))
			}
		}
	}
}

func (ds *DashboardServer) collectMetrics() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ds.stopChan:
			return
		case <-ticker.C:
			stats := ds.batcher.GetStats()
			backendStats := ds.backend.GetStats()

			ds.mu.Lock()
			snapshot := MetricsSnapshot{
				Timestamp:        time.Now().UnixMilli(),
				BatchSize:        stats.CurrentBatchSize,
				PendingItems:     stats.PendingItems,
				CPULoad:          backendStats.CPULoad,
				QueueDepth:       backendStats.QueueDepth,
				ErrorRate:        backendStats.ErrorRate,
				ProcessingTimeMs: int64(ds.lastProcTime / time.Millisecond),
				LoadScore:        stats.AverageLoadScore,
				TotalProcessed:   ds.itemsProcessed,
				TotalBatches:     ds.batchesProcessed,
			}

			ds.metrics = append(ds.metrics, snapshot)
			if len(ds.metrics) > ds.maxMetrics {
				ds.metrics = ds.metrics[1:]
			}
			ds.mu.Unlock()
		}
	}
}

func (ds *DashboardServer) GetMetrics() []MetricsSnapshot {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	result := make([]MetricsSnapshot, len(ds.metrics))
	copy(result, ds.metrics)
	return result
}

func (ds *DashboardServer) GetStatus() map[string]interface{} {
	ds.mu.RLock()
	defer ds.mu.RUnlock()

	return map[string]interface{}{
		"running":          ds.running,
		"pattern":          ds.currentPattern.String(),
		"workerCount":      ds.workerCount,
		"itemsProcessed":   ds.itemsProcessed,
		"batchesProcessed": ds.batchesProcessed,
	}
}

var dashboard = NewDashboardServer()

func main() {
	mainSimple()
}

func serveIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, indexHTML)
}

func handleStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Pattern string `json:"pattern"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var pattern simulator.LoadPattern
	switch req.Pattern {
	case "constant":
		pattern = simulator.PatternConstant
	case "sinewave":
		pattern = simulator.PatternSineWave
	case "spikes":
		pattern = simulator.PatternSpikes
	case "gradual":
		pattern = simulator.PatternGradual
	default:
		http.Error(w, "Invalid pattern", http.StatusBadRequest)
		return
	}

	dashboard.Stop()
	time.Sleep(100 * time.Millisecond)

	if err := dashboard.Start(pattern); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"status": "started"})
}

func handleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	dashboard.Stop()
	json.NewEncoder(w).Encode(map[string]string{"status": "stopped"})
}

func handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(dashboard.GetMetrics())
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(dashboard.GetStatus())
}

const indexHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Load-Aware Batcher Dashboard</title>
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=Inter:wght@300;400;500;600;700&display=swap" rel="stylesheet">
    <script src="https://cdn.jsdelivr.net/npm/chart.js"></script>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }

        body {
            font-family: 'Inter', -apple-system, BlinkMacSystemFont, sans-serif;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            min-height: 100vh;
            padding: 20px;
            color: #fff;
        }

        .container {
            max-width: 1600px;
            margin: 0 auto;
        }

        header {
            text-align: center;
            margin-bottom: 40px;
            animation: fadeInDown 0.6s ease;
        }

        h1 {
            font-size: 3rem;
            font-weight: 700;
            margin-bottom: 10px;
            background: linear-gradient(135deg, #fff 0%, #f0f0f0 100%);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
            background-clip: text;
        }

        .subtitle {
            font-size: 1.2rem;
            opacity: 0.9;
            font-weight: 300;
        }

        .controls {
            display: flex;
            justify-content: center;
            gap: 20px;
            margin-bottom: 40px;
            flex-wrap: wrap;
            animation: fadeIn 0.8s ease 0.2s both;
        }

        .btn {
            padding: 14px 32px;
            border: none;
            border-radius: 12px;
            font-size: 1rem;
            font-weight: 600;
            cursor: pointer;
            transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
            font-family: 'Inter', sans-serif;
            text-transform: uppercase;
            letter-spacing: 0.5px;
            position: relative;
            overflow: hidden;
        }

        .btn::before {
            content: '';
            position: absolute;
            top: 50%;
            left: 50%;
            width: 0;
            height: 0;
            border-radius: 50%;
            background: rgba(255, 255, 255, 0.3);
            transform: translate(-50%, -50%);
            transition: width 0.6s, height 0.6s;
        }

        .btn:hover::before {
            width: 300px;
            height: 300px;
        }

        .btn-primary {
            background: linear-gradient(135deg, #f093fb 0%, #f5576c 100%);
            color: white;
            box-shadow: 0 10px 30px rgba(245, 87, 108, 0.4);
        }

        .btn-primary:hover {
            transform: translateY(-2px);
            box-shadow: 0 15px 40px rgba(245, 87, 108, 0.6);
        }

        .btn-secondary {
            background: rgba(255, 255, 255, 0.2);
            color: white;
            backdrop-filter: blur(10px);
            border: 1px solid rgba(255, 255, 255, 0.3);
        }

        .btn-secondary:hover {
            background: rgba(255, 255, 255, 0.3);
            transform: translateY(-2px);
        }

        .btn:active {
            transform: translateY(0);
        }

        .btn:disabled {
            opacity: 0.5;
            cursor: not-allowed;
        }

        .status-bar {
            background: rgba(255, 255, 255, 0.1);
            backdrop-filter: blur(20px);
            border-radius: 16px;
            padding: 20px 30px;
            margin-bottom: 30px;
            display: flex;
            justify-content: space-around;
            align-items: center;
            border: 1px solid rgba(255, 255, 255, 0.2);
            animation: fadeIn 0.8s ease 0.3s both;
            box-shadow: 0 8px 32px rgba(0, 0, 0, 0.1);
        }

        .status-item {
            text-align: center;
        }

        .status-label {
            font-size: 0.85rem;
            opacity: 0.8;
            text-transform: uppercase;
            letter-spacing: 1px;
            margin-bottom: 8px;
            font-weight: 500;
        }

        .status-value {
            font-size: 1.8rem;
            font-weight: 700;
            background: linear-gradient(135deg, #fff 0%, #f0f0f0 100%);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
            background-clip: text;
        }

        .status-running {
            color: #4ade80;
            animation: pulse 2s infinite;
        }

        .status-stopped {
            color: #f87171;
        }

        .dashboard-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(500px, 1fr));
            gap: 25px;
            margin-bottom: 30px;
        }

        .card {
            background: rgba(255, 255, 255, 0.1);
            backdrop-filter: blur(20px);
            border-radius: 20px;
            padding: 30px;
            border: 1px solid rgba(255, 255, 255, 0.2);
            animation: fadeInUp 0.8s ease both;
            box-shadow: 0 8px 32px rgba(0, 0, 0, 0.1);
            transition: transform 0.3s ease, box-shadow 0.3s ease;
        }

        .card:hover {
            transform: translateY(-5px);
            box-shadow: 0 12px 48px rgba(0, 0, 0, 0.2);
        }

        .card:nth-child(1) { animation-delay: 0.4s; }
        .card:nth-child(2) { animation-delay: 0.5s; }
        .card:nth-child(3) { animation-delay: 0.6s; }
        .card:nth-child(4) { animation-delay: 0.7s; }

        .card-title {
            font-size: 1.3rem;
            font-weight: 600;
            margin-bottom: 20px;
            display: flex;
            align-items: center;
            gap: 10px;
        }

        .card-icon {
            width: 32px;
            height: 32px;
            background: linear-gradient(135deg, #f093fb 0%, #f5576c 100%);
            border-radius: 8px;
            display: flex;
            align-items: center;
            justify-content: center;
            font-size: 1.2rem;
        }

        .metrics-grid {
            display: grid;
            grid-template-columns: repeat(2, 1fr);
            gap: 20px;
            margin-top: 20px;
        }

        .metric {
            background: rgba(255, 255, 255, 0.05);
            padding: 20px;
            border-radius: 12px;
            border: 1px solid rgba(255, 255, 255, 0.1);
            transition: background 0.3s ease;
        }

        .metric:hover {
            background: rgba(255, 255, 255, 0.1);
        }

        .metric-label {
            font-size: 0.85rem;
            opacity: 0.8;
            margin-bottom: 8px;
            text-transform: uppercase;
            letter-spacing: 0.5px;
        }

        .metric-value {
            font-size: 2rem;
            font-weight: 700;
            background: linear-gradient(135deg, #4ade80 0%, #22c55e 100%);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
            background-clip: text;
        }

        .metric-value.warning {
            background: linear-gradient(135deg, #fbbf24 0%, #f59e0b 100%);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
            background-clip: text;
        }

        .metric-value.danger {
            background: linear-gradient(135deg, #f87171 0%, #ef4444 100%);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
            background-clip: text;
        }

        .chart-container {
            position: relative;
            height: 300px;
            margin-top: 20px;
        }

        @keyframes fadeIn {
            from {
                opacity: 0;
            }
            to {
                opacity: 1;
            }
        }

        @keyframes fadeInDown {
            from {
                opacity: 0;
                transform: translateY(-20px);
            }
            to {
                opacity: 1;
                transform: translateY(0);
            }
        }

        @keyframes fadeInUp {
            from {
                opacity: 0;
                transform: translateY(20px);
            }
            to {
                opacity: 1;
                transform: translateY(0);
            }
        }

        @keyframes pulse {
            0%, 100% {
                opacity: 1;
            }
            50% {
                opacity: 0.5;
            }
        }

        .loading {
            display: inline-block;
            width: 20px;
            height: 20px;
            border: 3px solid rgba(255, 255, 255, 0.3);
            border-radius: 50%;
            border-top-color: white;
            animation: spin 1s linear infinite;
        }

        @keyframes spin {
            to { transform: rotate(360deg); }
        }

        @media (max-width: 768px) {
            h1 {
                font-size: 2rem;
            }
            
            .dashboard-grid {
                grid-template-columns: 1fr;
            }

            .status-bar {
                flex-direction: column;
                gap: 20px;
            }

            .controls {
                flex-direction: column;
            }
        }
    </style>
</head>
<body>
    <div class="container">
        <header>
            <h1>🔥 Load-Aware Batcher</h1>
            <p class="subtitle">Real-time adaptive batch processing visualization</p>
        </header>

        <div class="controls">
            <button class="btn btn-primary" onclick="startSim('constant')">▶ Constant Load</button>
            <button class="btn btn-primary" onclick="startSim('sinewave')">〜 Sine Wave</button>
            <button class="btn btn-primary" onclick="startSim('spikes')">⚡ Spikes</button>
            <button class="btn btn-primary" onclick="startSim('gradual')">📈 Gradual</button>
            <button class="btn btn-secondary" onclick="stopSim()">◼ Stop</button>
        </div>

        <div class="status-bar">
            <div class="status-item">
                <div class="status-label">Status</div>
                <div class="status-value" id="status">Stopped</div>
            </div>
            <div class="status-item">
                <div class="status-label">Pattern</div>
                <div class="status-value" id="pattern">-</div>
            </div>
            <div class="status-item">
                <div class="status-label">Items Processed</div>
                <div class="status-value" id="totalItems">0</div>
            </div>
            <div class="status-item">
                <div class="status-label">Batches</div>
                <div class="status-value" id="totalBatches">0</div>
            </div>
        </div>

        <div class="dashboard-grid">
            <div class="card">
                <div class="card-title">
                    <div class="card-icon">📊</div>
                    Batch Size & Load Score
                </div>
                <div class="chart-container">
                    <canvas id="batchChart"></canvas>
                </div>
            </div>

            <div class="card">
                <div class="card-title">
                    <div class="card-icon">💻</div>
                    CPU & Queue Depth
                </div>
                <div class="chart-container">
                    <canvas id="cpuChart"></canvas>
                </div>
            </div>

            <div class="card">
                <div class="card-title">
                    <div class="card-icon">⚡</div>
                    Current Metrics
                </div>
                <div class="metrics-grid">
                    <div class="metric">
                        <div class="metric-label">Batch Size</div>
                        <div class="metric-value" id="currentBatch">-</div>
                    </div>
                    <div class="metric">
                        <div class="metric-label">CPU Load</div>
                        <div class="metric-value" id="currentCPU">-</div>
                    </div>
                    <div class="metric">
                        <div class="metric-label">Queue Depth</div>
                        <div class="metric-value" id="currentQueue">-</div>
                    </div>
                    <div class="metric">
                        <div class="metric-label">Error Rate</div>
                        <div class="metric-value" id="currentError">-</div>
                    </div>
                </div>
            </div>

            <div class="card">
                <div class="card-title">
                    <div class="card-icon">⏱️</div>
                    Processing Time
                </div>
                <div class="chart-container">
                    <canvas id="timeChart"></canvas>
                </div>
            </div>
        </div>
    </div>

    <script>
        // Chart configurations
        const chartOptions = {
            responsive: true,
            maintainAspectRatio: false,
            interaction: {
                mode: 'index',
                intersect: false,
            },
            plugins: {
                legend: {
                    labels: {
                        color: 'white',
                        font: {
                            family: 'Inter',
                            size: 12
                        }
                    }
                }
            },
            scales: {
                x: {
                    display: false
                },
                y: {
                    grid: {
                        color: 'rgba(255, 255, 255, 0.1)'
                    },
                    ticks: {
                        color: 'rgba(255, 255, 255, 0.8)',
                        font: {
                            family: 'Inter'
                        }
                    }
                }
            }
        };

        // Initialize charts
        const batchChart = new Chart(document.getElementById('batchChart'), {
            type: 'line',
            data: {
                labels: [],
                datasets: [
                    {
                        label: 'Batch Size',
                        data: [],
                        borderColor: '#4ade80',
                        backgroundColor: 'rgba(74, 222, 128, 0.1)',
                        tension: 0.4,
                        fill: true,
                        yAxisID: 'y'
                    },
                    {
                        label: 'Load Score',
                        data: [],
                        borderColor: '#f59e0b',
                        backgroundColor: 'rgba(245, 158, 11, 0.1)',
                        tension: 0.4,
                        fill: true,
                        yAxisID: 'y1'
                    }
                ]
            },
            options: {
                ...chartOptions,
                scales: {
                    ...chartOptions.scales,
                    y: {
                        ...chartOptions.scales.y,
                        type: 'linear',
                        position: 'left',
                    },
                    y1: {
                        type: 'linear',
                        position: 'right',
                        grid: {
                            drawOnChartArea: false,
                        },
                        ticks: {
                            color: 'rgba(255, 255, 255, 0.8)',
                            font: {
                                family: 'Inter'
                            }
                        },
                        max: 1
                    }
                }
            }
        });

        const cpuChart = new Chart(document.getElementById('cpuChart'), {
            type: 'line',
            data: {
                labels: [],
                datasets: [
                    {
                        label: 'CPU Load',
                        data: [],
                        borderColor: '#f093fb',
                        backgroundColor: 'rgba(240, 147, 251, 0.1)',
                        tension: 0.4,
                        fill: true,
                        yAxisID: 'y'
                    },
                    {
                        label: 'Queue Depth',
                        data: [],
                        borderColor: '#3b82f6',
                        backgroundColor: 'rgba(59, 130, 246, 0.1)',
                        tension: 0.4,
                        fill: true,
                        yAxisID: 'y1'
                    }
                ]
            },
            options: {
                ...chartOptions,
                scales: {
                    ...chartOptions.scales,
                    y: {
                        ...chartOptions.scales.y,
                        type: 'linear',
                        position: 'left',
                        max: 1
                    },
                    y1: {
                        type: 'linear',
                        position: 'right',
                        grid: {
                            drawOnChartArea: false,
                        },
                        ticks: {
                            color: 'rgba(255, 255, 255, 0.8)',
                            font: {
                                family: 'Inter'
                            }
                        }
                    }
                }
            }
        });

        const timeChart = new Chart(document.getElementById('timeChart'), {
            type: 'line',
            data: {
                labels: [],
                datasets: [
                    {
                        label: 'Processing Time (ms)',
                        data: [],
                        borderColor: '#8b5cf6',
                        backgroundColor: 'rgba(139, 92, 246, 0.1)',
                        tension: 0.4,
                        fill: true
                    }
                ]
            },
            options: chartOptions
        });

        let updateInterval;

        async function startSim(pattern) {
            try {
                const response = await fetch('/api/start', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ pattern })
                });
                
                if (response.ok) {
                    if (!updateInterval) {
                        updateInterval = setInterval(updateDashboard, 500);
                    }
                }
            } catch (error) {
                console.error('Error starting simulation:', error);
            }
        }

        async function stopSim() {
            try {
                await fetch('/api/stop', { method: 'POST' });
                if (updateInterval) {
                    clearInterval(updateInterval);
                    updateInterval = null;
                }
            } catch (error) {
                console.error('Error stopping simulation:', error);
            }
        }

        async function updateDashboard() {
            try {
                const [metricsRes, statusRes] = await Promise.all([
                    fetch('/api/metrics'),
                    fetch('/api/status')
                ]);

                const metrics = await metricsRes.json();
                const status = await statusRes.json();

                // Update status bar
                document.getElementById('status').textContent = status.running ? 'Running' : 'Stopped';
                document.getElementById('status').className = status.running ? 'status-value status-running' : 'status-value status-stopped';
                document.getElementById('pattern').textContent = status.pattern || '-';
                document.getElementById('totalItems').textContent = status.itemsProcessed || 0;
                document.getElementById('totalBatches').textContent = status.batchesProcessed || 0;

                if (metrics && metrics.length > 0) {
                    const latest = metrics[metrics.length - 1];

                    // Update current metrics
                    document.getElementById('currentBatch').textContent = latest.batchSize;
                    document.getElementById('currentCPU').textContent = (latest.cpuLoad * 100).toFixed(1) + '%';
                    document.getElementById('currentQueue').textContent = latest.queueDepth;
                    document.getElementById('currentError').textContent = (latest.errorRate * 100).toFixed(1) + '%';

                    // Apply color classes based on thresholds
                    const cpuEl = document.getElementById('currentCPU');
                    cpuEl.className = 'metric-value';
                    if (latest.cpuLoad > 0.7) cpuEl.classList.add('danger');
                    else if (latest.cpuLoad > 0.4) cpuEl.classList.add('warning');

                    const errorEl = document.getElementById('currentError');
                    errorEl.className = 'metric-value';
                    if (latest.errorRate > 0.1) errorEl.classList.add('danger');
                    else if (latest.errorRate > 0.05) errorEl.classList.add('warning');

                    // Update charts
                    const maxPoints = 50;
                    const labels = metrics.slice(-maxPoints).map((_, i) => i);
                    
                    // Batch Size & Load Score chart
                    batchChart.data.labels = labels;
                    batchChart.data.datasets[0].data = metrics.slice(-maxPoints).map(m => m.batchSize);
                    batchChart.data.datasets[1].data = metrics.slice(-maxPoints).map(m => m.loadScore);
                    batchChart.update('none');

                    // CPU & Queue chart
                    cpuChart.data.labels = labels;
                    cpuChart.data.datasets[0].data = metrics.slice(-maxPoints).map(m => m.cpuLoad);
                    cpuChart.data.datasets[1].data = metrics.slice(-maxPoints).map(m => m.queueDepth);
                    cpuChart.update('none');

                    // Processing Time chart
                    timeChart.data.labels = labels;
                    timeChart.data.datasets[0].data = metrics.slice(-maxPoints).map(m => m.processingTimeMs);
                    timeChart.update('none');
                }
            } catch (error) {
                console.error('Error updating dashboard:', error);
            }
        }

        // Initial update
        updateDashboard();
    </script>
</body>
</html>
`
