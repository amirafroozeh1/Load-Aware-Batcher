package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	batcher "github.com/amirafroozeh1/Load-Aware-Batcher"
)

// SimpleDemo is a simplified version for demonstration
type SimpleDemo struct {
	mu             sync.RWMutex
	batcher        *batcher.Batcher
	currentLoad    float64 // 0.0 to 1.0
	batchSize      int
	itemsProcessed int64
	running        bool
}

func NewSimpleDemo() *SimpleDemo {
	return &SimpleDemo{
		currentLoad: 0.3,
		running:     false,
	}
}

func (sd *SimpleDemo) Start() error {
	sd.mu.Lock()
	if sd.running {
		sd.mu.Unlock()
		return fmt.Errorf("already running")
	}
	sd.running = true
	sd.itemsProcessed = 0
	sd.mu.Unlock()

	// Create batcher
	b, err := batcher.New(batcher.Config{
		InitialBatchSize:  20,
		MinBatchSize:      5,
		MaxBatchSize:      100,
		Timeout:           2 * time.Second,
		AdjustmentFactor:  0.5, // More aggressive for demo
		LoadCheckInterval: 1 * time.Second,
		HandlerFunc:       sd.handleBatch,
	})
	if err != nil {
		sd.mu.Lock()
		sd.running = false
		sd.mu.Unlock()
		return err
	}
	sd.batcher = b

	// Start background worker
	go sd.worker()
	go sd.updateBatchSize()

	return nil
}

func (sd *SimpleDemo) Stop() {
	sd.mu.Lock()
	if !sd.running {
		sd.mu.Unlock()
		return
	}
	sd.running = false
	sd.mu.Unlock()

	if sd.batcher != nil {
		sd.batcher.Close(context.Background())
	}
}

func (sd *SimpleDemo) SetLoad(load float64) {
	sd.mu.Lock()
	defer sd.mu.Unlock()

	if load < 0 {
		load = 0
	}
	if load > 1 {
		load = 1
	}
	sd.currentLoad = load
}

func (sd *SimpleDemo) handleBatch(ctx context.Context, batch []any) (*batcher.LoadFeedback, error) {
	sd.mu.RLock()
	load := sd.currentLoad
	sd.mu.RUnlock()

	// Simulate processing based on load
	processingTime := time.Duration(float64(len(batch)) * (1 + load*3) * float64(time.Millisecond))
	time.Sleep(processingTime)

	sd.mu.Lock()
	sd.itemsProcessed += int64(len(batch))
	sd.mu.Unlock()

	// Return feedback based on current load
	errorRate := load * 0.2 // Higher load = more errors
	queueDepth := int(load * 50)
	dbLocks := int(load * 30)

	return &batcher.LoadFeedback{
		CPULoad:        load,
		QueueDepth:     queueDepth,
		ProcessingTime: processingTime,
		ErrorRate:      errorRate,
		DBLocks:        dbLocks,
	}, nil
}

func (sd *SimpleDemo) worker() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		sd.mu.RLock()
		running := sd.running
		sd.mu.RUnlock()

		if !running {
			return
		}

		<-ticker.C
		sd.batcher.Add(context.Background(), "item")
	}
}

func (sd *SimpleDemo) updateBatchSize() {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		sd.mu.RLock()
		running := sd.running
		sd.mu.RUnlock()

		if !running {
			return
		}

		<-ticker.C
		stats := sd.batcher.GetStats()
		sd.mu.Lock()
		sd.batchSize = stats.CurrentBatchSize
		sd.mu.Unlock()
	}
}

func (sd *SimpleDemo) GetStatus() map[string]interface{} {
	sd.mu.RLock()
	defer sd.mu.RUnlock()

	return map[string]interface{}{
		"running":        sd.running,
		"currentLoad":    sd.currentLoad,
		"batchSize":      sd.batchSize,
		"itemsProcessed": sd.itemsProcessed,
	}
}

var simpleDemo = NewSimpleDemo()

func mainSimple() {
	http.HandleFunc("/", serveSimpleIndex)
	http.HandleFunc("/api/simple/start", handleSimpleStart)
	http.HandleFunc("/api/simple/stop", handleSimpleStop)
	http.HandleFunc("/api/simple/setload", handleSetLoad)
	http.HandleFunc("/api/simple/status", handleSimpleStatus)

	port := ":8080"
	log.Printf("🚀 Simple Load-Aware Batcher Demo at http://localhost%s", port)
	log.Fatal(http.ListenAndServe(port, nil))
}

func serveSimpleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, simpleHTML)
}

func handleSimpleStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := simpleDemo.Start(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"status": "started"})
}

func handleSimpleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	simpleDemo.Stop()
	json.NewEncoder(w).Encode(map[string]string{"status": "stopped"})
}

func handleSetLoad(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Load float64 `json:"load"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	simpleDemo.SetLoad(req.Load)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func handleSimpleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(simpleDemo.GetStatus())
}

const simpleHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Load-Aware Batcher</title>
    <link rel="preconnect" href="https://fonts.googleapis.com">
    <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600&family=JetBrains+Mono:wght@400;700&display=swap" rel="stylesheet">
    <script src="https://cdn.jsdelivr.net/npm/chart.js"></script>
    <style>
        :root {
            --bg-body: #101217;
            --bg-panel: #181b1f;
            --border-panel: #22252b;
            --text-primary: #d8d9da;
            --text-secondary: #8e8e8e;
            --accent-green: #73bf69;
            --accent-yellow: #f2cc0c;
            --accent-red: #f2495c;
            --accent-blue: #5794f2;
        }

        * { margin: 0; padding: 0; box-sizing: border-box; }

        body {
            font-family: 'Inter', sans-serif;
            background-color: var(--bg-body);
            color: var(--text-primary);
            height: 100vh;
            display: flex;
            flex-direction: column;
            padding: 20px;
        }

        .header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 20px;
            padding-bottom: 10px;
            border-bottom: 1px solid var(--border-panel);
        }

        .title {
            font-size: 16px;
            font-weight: 600;
            color: #fff;
            display: flex;
            align-items: center;
            gap: 8px;
        }

        .status-badge {
            font-size: 11px;
            padding: 2px 6px;
            border-radius: 3px;
            background: rgba(115, 191, 105, 0.1);
            color: var(--accent-green);
            border: 1px solid rgba(115, 191, 105, 0.2);
            font-family: 'JetBrains Mono', monospace;
            text-transform: uppercase;
        }

        .status-badge.stopped {
            background: rgba(242, 73, 92, 0.1);
            color: var(--accent-red);
            border-color: rgba(242, 73, 92, 0.2);
        }

        .grid {
            display: grid;
            grid-template-columns: 3fr 1fr;
            gap: 16px;
            flex: 1;
        }

        .panel {
            background: var(--bg-panel);
            border: 1px solid var(--border-panel);
            border-radius: 2px;
            padding: 16px;
            display: flex;
            flex-direction: column;
        }

        .panel-header {
            font-size: 12px;
            font-weight: 500;
            color: var(--text-secondary);
            margin-bottom: 12px;
            text-transform: uppercase;
            letter-spacing: 0.5px;
        }

        /* Hero Metric */
        .hero-container {
            display: flex;
            flex-direction: column;
            justify-content: center;
            align-items: center;
            position: relative;
            margin-bottom: 20px;
        }

        .hero-value {
            font-family: 'JetBrains Mono', monospace;
            font-size: 60px;
            font-weight: 700;
            color: var(--accent-blue);
            line-height: 1;
            z-index: 2;
        }

        .hero-label {
            margin-top: 8px;
            font-size: 13px;
            color: var(--text-secondary);
            z-index: 2;
        }

        /* Chart Container */
        .chart-wrapper {
            flex: 1;
            position: relative;
            width: 100%;
            min-height: 0;
        }

        /* Controls */
        .control-section {
            margin-bottom: 20px;
        }

        .control-label {
            font-size: 11px;
            color: var(--text-secondary);
            margin-bottom: 6px;
            text-transform: uppercase;
        }

        .btn {
            width: 100%;
            padding: 8px 12px; /* Small buttons */
            background: #262626;
            border: 1px solid #333;
            color: var(--text-primary);
            font-family: 'Inter', sans-serif;
            font-size: 12px;
            font-weight: 500;
            cursor: pointer;
            border-radius: 2px;
            margin-bottom: 6px;
            transition: all 0.1s;
            text-align: center;
        }

        .btn:hover { background: #303030; border-color: #444; }
        .btn:active { transform: translateY(1px); }

        .btn-primary { background: var(--accent-blue); border-color: var(--accent-blue); color: #fff; }
        .btn-primary:hover { background: #3274d9; border-color: #3274d9; }
        
        .btn-danger { color: var(--accent-red); border-color: rgba(242, 73, 92, 0.3); }
        .btn-danger:hover { background: rgba(242, 73, 92, 0.1); }

        .btn-success { color: var(--accent-green); border-color: rgba(115, 191, 105, 0.3); }
        .btn-success:hover { background: rgba(115, 191, 105, 0.1); }

        /* Mini Stats */
        .mini-stat {
            display: flex;
            justify-content: space-between;
            align-items: center;
            padding: 8px 0;
            border-bottom: 1px solid rgba(255,255,255,0.05);
        }
        
        .mini-stat:last-child { border-bottom: none; }

        .mini-label { font-size: 12px; color: var(--text-secondary); }
        .mini-value { font-family: 'JetBrains Mono', monospace; font-size: 13px; color: #fff; }

    </style>
</head>
<body>
    <div class="header">
        <div class="title">
            <span>⚡</span> Load-Aware Batcher
        </div>
        <div id="statusBadge" class="status-badge stopped">STOPPED</div>
    </div>

    <div class="grid">
        <!-- Main Panel -->
        <div class="panel">
            <div class="panel-header">Real-time Batch Size</div>
            <div class="hero-container">
                <div id="batchSize" class="hero-value">--</div>
                <div class="hero-label">Items per Batch</div>
            </div>
            <div class="chart-wrapper">
                <canvas id="batchChart"></canvas>
            </div>
        </div>

        <!-- Sidebar Panel -->
        <div class="panel">
            <div class="control-section">
                <div class="panel-header">Actions</div>
                <button id="startBtn" class="btn btn-primary" onclick="start()">Start Simulation</button>
                <button class="btn" onclick="stop()">Stop</button>
            </div>

            <div class="control-section">
                <div class="panel-header">Load Injection</div>
                <button class="btn btn-success" onclick="setLoad(0.2)">Low Load (20%)</button>
                <button class="btn btn-danger" onclick="setLoad(0.9)">High Load (90%)</button>
            </div>

            <div class="control-section" style="margin-top: auto;">
                <div class="panel-header">Metrics</div>
                <div class="mini-stat">
                    <span class="mini-label">CPU Load</span>
                    <span id="cpuLoad" class="mini-value">--%</span>
                </div>
                <div class="mini-stat">
                    <span class="mini-label">Processed</span>
                    <span id="itemsProcessed" class="mini-value">0</span>
                </div>
            </div>
        </div>
    </div>

    <script>
        let updateInterval;
        let chart;

        // Initialize Chart
        function initChart() {
            const ctx = document.getElementById('batchChart').getContext('2d');
            
            // Gradient
            const gradient = ctx.createLinearGradient(0, 0, 0, 400);
            gradient.addColorStop(0, 'rgba(87, 148, 242, 0.2)');
            gradient.addColorStop(1, 'rgba(87, 148, 242, 0)');

            chart = new Chart(ctx, {
                type: 'line',
                data: {
                    labels: Array(30).fill(''),
                    datasets: [
                        {
                            label: 'Batch Size',
                            data: Array(30).fill(null),
                            borderColor: '#5794f2',
                            backgroundColor: gradient,
                            borderWidth: 2,
                            pointRadius: 0,
                            fill: true,
                            tension: 0.4
                        },
                        {
                            label: 'CPU Load (%)',
                            data: Array(30).fill(null),
                            borderColor: '#f2495c',
                            backgroundColor: 'rgba(242, 73, 92, 0.1)',
                            borderWidth: 2,
                            pointRadius: 0,
                            fill: true,
                            tension: 0.4
                        }
                    ]
                },
                options: {
                    responsive: true,
                    maintainAspectRatio: false,
                    animation: false,
                    plugins: {
                        legend: { 
                            display: true,
                            labels: { color: '#8e8e8e', font: { family: 'Inter', size: 11 } }
                        }
                    },
                    scales: {
                        y: {
                            beginAtZero: true,
                            max: 100,
                            grid: {
                                color: '#22252b'
                            },
                            ticks: {
                                color: '#8e8e8e',
                                font: { family: 'JetBrains Mono', size: 10 }
                            }
                        },
                        x: {
                            display: false
                        }
                    }
                }
            });
        }

        async function start() {
            try {
                const res = await fetch('/api/simple/start', { method: 'POST' });
                if (res.ok) {
                    document.getElementById('startBtn').disabled = true;
                    document.getElementById('startBtn').style.opacity = '0.5';
                    if (!updateInterval) updateInterval = setInterval(updateStatus, 200);
                }
            } catch (e) { console.error(e); }
        }

        async function stop() {
            try {
                await fetch('/api/simple/stop', { method: 'POST' });
                document.getElementById('startBtn').disabled = false;
                document.getElementById('startBtn').style.opacity = '1';
                if (updateInterval) { clearInterval(updateInterval); updateInterval = null; }
                updateStatus();
            } catch (e) { console.error(e); }
        }

        async function setLoad(load) {
            try {
                await fetch('/api/simple/setload', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ load: load })
                });
            } catch (e) { console.error(e); }
        }

        async function updateStatus() {
            try {
                const res = await fetch('/api/simple/status');
                const data = await res.json();

                // Batch Size
                const batchSize = data.batchSize || 0;
                const batchEl = document.getElementById('batchSize');
                batchEl.textContent = batchSize;

                // Metrics
                const cpu = Math.round((data.currentLoad || 0) * 100);
                
                // Update Chart
                if (chart) {
                    // Batch Size
                    const batchData = chart.data.datasets[0].data;
                    batchData.shift();
                    batchData.push(batchSize);

                    // CPU Load
                    const loadData = chart.data.datasets[1].data;
                    loadData.shift();
                    loadData.push(cpu);

                    chart.update('none'); // 'none' mode for performance
                }

                // Color Logic
                if (batchSize < 30) {
                    batchEl.style.color = 'var(--accent-green)';
                    if(chart) chart.data.datasets[0].borderColor = '#73bf69';
                } else if (batchSize < 70) {
                    batchEl.style.color = 'var(--accent-yellow)';
                    if(chart) chart.data.datasets[0].borderColor = '#f2cc0c';
                } else {
                    batchEl.style.color = 'var(--accent-blue)';
                    if(chart) chart.data.datasets[0].borderColor = '#5794f2';
                }
                const cpuEl = document.getElementById('cpuLoad');
                cpuEl.textContent = cpu + '%';
                cpuEl.style.color = cpu > 80 ? 'var(--accent-red)' : '#fff';

                document.getElementById('itemsProcessed').textContent = data.itemsProcessed || 0;

                // Status
                const badge = document.getElementById('statusBadge');
                if (data.running) {
                    badge.textContent = 'RUNNING';
                    badge.className = 'status-badge';
                } else {
                    badge.textContent = 'STOPPED';
                    badge.className = 'status-badge stopped';
                }

            } catch (e) { console.error(e); }
        }

        initChart();
        updateStatus();
    </script>
</body>
</html>
`
