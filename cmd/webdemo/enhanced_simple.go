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

// EnhancedDemo is a simple demo with basic charting
type EnhancedDemo struct {
	mu             sync.RWMutex
	batcher        *batcher.Batcher
	currentLoad    float64 // 0.0 to 1.0
	history        []DataPoint
	maxHistory     int
	itemsProcessed int64
	running        bool
}

type DataPoint struct {
	Timestamp int64   `json:"timestamp"`
	BatchSize int     `json:"batchSize"`
	Load      float64 `json:"load"`
}

func NewEnhancedDemo() *EnhancedDemo {
	return &EnhancedDemo{
		currentLoad: 0.3,
		history:     make([]DataPoint, 0, 50),
		maxHistory:  50,
		running:     false,
	}
}

func (ed *EnhancedDemo) Start() error {
	ed.mu.Lock()
	if ed.running {
		ed.mu.Unlock()
		return fmt.Errorf("already running")
	}
	ed.running = true
	ed.itemsProcessed = 0
	ed.history = make([]DataPoint, 0, ed.maxHistory)
	ed.mu.Unlock()

	// Create batcher
	b, err := batcher.New(batcher.Config{
		InitialBatchSize:  20,
		MinBatchSize:      5,
		MaxBatchSize:      100,
		Timeout:           2 * time.Second,
		AdjustmentFactor:  0.5,
		LoadCheckInterval: 1 * time.Second,
		HandlerFunc:       ed.handleBatch,
	})
	if err != nil {
		ed.mu.Lock()
		ed.running = false
		ed.mu.Unlock()
		return err
	}
	ed.batcher = b

	// Start background worker
	go ed.worker()
	go ed.collectHistory()

	return nil
}

func (ed *EnhancedDemo) Stop() {
	ed.mu.Lock()
	if !ed.running {
		ed.mu.Unlock()
		return
	}
	ed.running = false
	ed.mu.Unlock()

	if ed.batcher != nil {
		ed.batcher.Close(context.Background())
	}
}

func (ed *EnhancedDemo) SetLoad(load float64) {
	ed.mu.Lock()
	defer ed.mu.Unlock()

	if load < 0 {
		load = 0
	}
	if load > 1 {
		load = 1
	}
	ed.currentLoad = load
}

func (ed *EnhancedDemo) handleBatch(ctx context.Context, batch []any) (*batcher.LoadFeedback, error) {
	ed.mu.RLock()
	load := ed.currentLoad
	ed.mu.RUnlock()

	// Simulate processing based on load
	processingTime := time.Duration(float64(len(batch)) * (1 + load*3) * float64(time.Millisecond))
	time.Sleep(processingTime)

	ed.mu.Lock()
	ed.itemsProcessed += int64(len(batch))
	ed.mu.Unlock()

	errorRate := load * 0.2
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

func (ed *EnhancedDemo) worker() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		ed.mu.RLock()
		running := ed.running
		ed.mu.RUnlock()

		if !running {
			return
		}

		<-ticker.C
		ed.batcher.Add(context.Background(), "item")
	}
}

func (ed *EnhancedDemo) collectHistory() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		ed.mu.RLock()
		running := ed.running
		ed.mu.RUnlock()

		if !running {
			return
		}

		<-ticker.C
		stats := ed.batcher.GetStats()

		ed.mu.Lock()
		dataPoint := DataPoint{
			Timestamp: time.Now().UnixMilli(),
			BatchSize: stats.CurrentBatchSize,
			Load:      ed.currentLoad,
		}

		ed.history = append(ed.history, dataPoint)
		if len(ed.history) > ed.maxHistory {
			ed.history = ed.history[1:]
		}
		ed.mu.Unlock()
	}
}

func (ed *EnhancedDemo) GetStatus() map[string]interface{} {
	ed.mu.RLock()
	defer ed.mu.RUnlock()

	return map[string]interface{}{
		"running":        ed.running,
		"currentLoad":    ed.currentLoad,
		"itemsProcessed": ed.itemsProcessed,
		"history":        ed.history,
	}
}

var enhancedDemo = NewEnhancedDemo()

func mainEnhanced() {
	http.HandleFunc("/", serveEnhancedIndex)
	http.HandleFunc("/api/enhanced/start", handleEnhancedStart)
	http.HandleFunc("/api/enhanced/stop", handleEnhancedStop)
	http.HandleFunc("/api/enhanced/setload", handleEnhancedSetLoad)
	http.HandleFunc("/api/enhanced/status", handleEnhancedStatus)

	port := ":8080"
	log.Printf("🚀 Enhanced Load-Aware Batcher Demo at http://localhost%s", port)
	log.Fatal(http.ListenAndServe(port, nil))
}

func serveEnhancedIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, enhancedHTML)
}

func handleEnhancedStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := enhancedDemo.Start(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"status": "started"})
}

func handleEnhancedStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	enhancedDemo.Stop()
	json.NewEncoder(w).Encode(map[string]string{"status": "stopped"})
}

func handleEnhancedSetLoad(w http.ResponseWriter, r *http.Request) {
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

	enhancedDemo.SetLoad(req.Load)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func handleEnhancedStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(enhancedDemo.GetStatus())
}

const enhancedHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>🔥 Load-Aware Batcher - Enhanced Demo</title>
    <link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;600;700;900&display=swap" rel="stylesheet">
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }

        body {
            font-family: 'Inter', sans-serif;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            min-height: 100vh;
            padding: 20px;
            color: white;
        }

        .container {
            max-width: 1000px;
            margin: 0 auto;
        }

        h1 {
            text-align: center;
            font-size: 3rem;
            margin-bottom: 10px;
            font-weight: 900;
            text-shadow: 0 2px 10px rgba(0,0,0,0.2);
        }

        .subtitle {
            text-align: center;
            font-size: 1.2rem;
            margin-bottom: 40px;
            opacity: 0.9;
        }

        .card {
            background: rgba(255, 255, 255, 0.15);
            backdrop-filter: blur(20px);
            border-radius: 24px;
            padding: 30px;
            border: 1px solid rgba(255, 255, 255, 0.3);
            box-shadow: 0 20px 60px rgba(0, 0, 0, 0.3);
            margin-bottom: 20px;
            transition: transform 0.3s ease;
        }

        .card:hover {
            transform: translateY(-2px);
        }

        .controls {
            display: grid;
            grid-template-columns: 1fr 1fr;
            gap: 15px;
            margin-bottom: 20px;
        }

        .btn {
            padding: 18px 30px;
            border: none;
            border-radius: 14px;
            font-size: 1.1rem;
            font-weight: 700;
            cursor: pointer;
            transition: all 0.3s;
            font-family: 'Inter', sans-serif;
            text-transform: uppercase;
            letter-spacing: 1px;
            box-shadow: 0 8px 20px rgba(0, 0, 0, 0.2);
        }

        .btn:hover {
            transform: translateY(-2px);
            box-shadow: 0 12px 30px rgba(0, 0, 0, 0.3);
        }

        .btn:active {
            transform: translateY(0);
        }

        .btn-start {
            grid-column: 1 / -1;
            background: linear-gradient(135deg, #4ade80 0%, #22c55e 100%);
            color: white;
        }

        .btn-low {
            background: linear-gradient(135deg, #4ade80 0%, #22c55e 100%);
            color: white;
        }

        .btn-high {
            background: linear-gradient(135deg, #f87171 0%, #ef4444 100%);
            color: white;
        }

        .btn-stop {
            grid-column: 1 / -1;
            background: linear-gradient(135deg, #6b7280 0%, #4b5563 100%);
            color: white;
        }

        .status-bar {
            text-align: center;
            font-size: 1.2rem;
            padding: 20px;
            border-radius: 12px;
            margin-bottom: 20px;
            font-weight: 600;
            background: rgba(255, 255, 255, 0.1);
        }

        .metrics {
            display: grid;
            grid-template-columns: repeat(3, 1fr);
            gap: 15px;
            margin-bottom: 20px;
        }

        .metric {
            text-align: center;
            padding: 20px;
            background: rgba(255, 255, 255, 0.1);
            border-radius: 12px;
        }

        .metric-label {
            font-size: 0.9rem;
            opacity: 0.8;
            margin-bottom: 10px;
            text-transform: uppercase;
            letter-spacing: 1px;
        }

        .metric-value {
            font-size: 2.5rem;
            font-weight: 900;
        }

        .chart-container {
            background: rgba(255, 255, 255, 0.1);
            border-radius: 16px;
            padding: 20px;
            min-height: 250px;
            position: relative;
        }

        .chart-title {
            font-size: 1.2rem;
            font-weight: 700;
            margin-bottom: 20px;
            text-align: center;
        }

        svg {
            width: 100%;
            height: 200px;
        }

        @keyframes pulse {
            0%, 100% { opacity: 1; }
            50% { opacity: 0.7; }
        }

        .pulsing {
            animation: pulse 2s infinite;
        }

        .legend {
            display: flex;
            justify-content: center;
            gap: 30px;
            margin-top: 15px;
        }

        .legend-item {
            display: flex;
            align-items: center;
            gap: 8px;
            font-size: 0.9rem;
        }

        .legend-color {
            width: 20px;
            height: 3px;
            border-radius: 2px;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>🔥 Load-Aware Batcher</h1>
        <p class="subtitle">نمودار ساده و کاربردی - Simple and functional chart</p>

        <div class="status-bar" id="statusBar">Click START to begin</div>

        <div class="card">
            <div class="controls">
                <button class="btn btn-start" id="startBtn" onclick="start()">▶ START</button>
                <button class="btn btn-low" onclick="setLoad(0.2)">🟢 LOW LOAD</button>
                <button class="btn btn-high" onclick="setLoad(0.9)">🔴 HIGH LOAD</button>
                <button class="btn btn-stop" onclick="stop()">◼ STOP</button>
            </div>
        </div>

        <div class="card">
            <div class="metrics">
                <div class="metric">
                    <div class="metric-label">Batch Size</div>
                    <div class="metric-value" id="batchSize">-</div>
                </div>
                <div class="metric">
                    <div class="metric-label">CPU Load</div>
                    <div class="metric-value" id="cpuLoad">-</div>
                </div>
                <div class="metric">
                    <div class="metric-label">Processed</div>
                    <div class="metric-value" id="itemsProcessed">0</div>
                </div>
            </div>
        </div>

        <div class="card">
            <div class="chart-container">
                <div class="chart-title">📊 Real-time Adaptation</div>
                <svg id="chart" viewBox="0 0 800 200"></svg>
                <div class="legend">
                    <div class="legend-item">
                        <div class="legend-color" style="background: #4ade80;"></div>
                        <span>Batch Size</span>
                    </div>
                    <div class="legend-item">
                        <div class="legend-color" style="background: #f59e0b;"></div>
                        <span>CPU Load</span>
                    </div>
                </div>
            </div>
        </div>
    </div>

    <script>
        let updateInterval;

        async function start() {
            try {
                const response = await fetch('/api/enhanced/start', { method: 'POST' });
                if (response.ok) {
                    document.getElementById('startBtn').disabled = true;
                    if (!updateInterval) {
                        updateInterval = setInterval(updateStatus, 500);
                    }
                }
            } catch (error) {
                console.error('Error starting demo:', error);
            }
        }

        async function stop() {
            try {
                await fetch('/api/enhanced/stop', { method: 'POST' });
                document.getElementById('startBtn').disabled = false;
                if (updateInterval) {
                    clearInterval(updateInterval);
                    updateInterval = null;
                }
            } catch (error) {
                console.error('Error stopping demo:', error);
            }
        }

        async function setLoad(load) {
            try {
                await fetch('/api/enhanced/setload', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ load: load })
                });
            } catch (error) {
                console.error('Error setting load:', error);
            }
        }

        function drawChart(history) {
            const svg = document.getElementById('chart');
            svg.innerHTML = '';

            if (!history || history.length < 2) return;

            const width = 800;
            const height = 200;
            const padding = 20;
            const chartWidth = width - 2 * padding;
            const chartHeight = height - 2 * padding;

            // Find max values
            const maxBatchSize = Math.max(...history.map(d => d.batchSize), 100);
            const maxLoad = 1;

            // Draw grid lines
            for (let i = 0; i <= 4; i++) {
                const y = padding + (chartHeight * i / 4);
                const line = document.createElementNS('http://www.w3.org/2000/svg', 'line');
                line.setAttribute('x1', padding);
                line.setAttribute('y1', y);
                line.setAttribute('x2', width - padding);
                line.setAttribute('y2', y);
                line.setAttribute('stroke', 'rgba(255, 255, 255, 0.1)');
                line.setAttribute('stroke-width', '1');
                svg.appendChild(line);
            }

            // Draw batch size line
            let batchPath = '';
            history.forEach((point, i) => {
                const x = padding + (chartWidth * i / (history.length - 1));
                const y = height - padding - (chartHeight * point.batchSize / maxBatchSize);
                batchPath += (i === 0 ? 'M' : 'L') + x + ',' + y;
            });

            const batchLine = document.createElementNS('http://www.w3.org/2000/svg', 'path');
            batchLine.setAttribute('d', batchPath);
            batchLine.setAttribute('fill', 'none');
            batchLine.setAttribute('stroke', '#4ade80');
            batchLine.setAttribute('stroke-width', '3');
            svg.appendChild(batchLine);

            // Draw load line
            let loadPath = '';
            history.forEach((point, i) => {
                const x = padding + (chartWidth * i / (history.length - 1));
                const y = height - padding - (chartHeight * point.load / maxLoad);
                loadPath += (i === 0 ? 'M' : 'L') + x + ',' + y;
            });

            const loadLine = document.createElementNS('http://www.w3.org/2000/svg', 'path');
            loadLine.setAttribute('d', loadPath);
            loadLine.setAttribute('fill', 'none');
            loadLine.setAttribute('stroke', '#f59e0b');
            loadLine.setAttribute('stroke-width', '3');
            loadLine.setAttribute('stroke-dasharray', '5,5');
            svg.appendChild(loadLine);
        }

        async function updateStatus() {
            try {
                const response = await fetch('/api/enhanced/status');
                const status = await response.json();

                // Update metrics
                const latestData = status.history && status.history.length > 0 
                    ? status.history[status.history.length - 1] 
                    : null;

                if (latestData) {
                    document.getElementById('batchSize').textContent = latestData.batchSize;
                    document.getElementById('cpuLoad').textContent = 
                        Math.round(status.currentLoad * 100) + '%';
                }

                document.getElementById('itemsProcessed').textContent = 
                    status.itemsProcessed || 0;

                // Update status bar
                const statusBar = document.getElementById('statusBar');
                if (status.running) {
                    statusBar.textContent = '🟢 Running - Try changing the load!';
                    statusBar.className = 'status-bar pulsing';
                } else {
                    statusBar.textContent = '⭕ Stopped';
                    statusBar.className = 'status-bar';
                }

                // Draw chart
                if (status.history && status.history.length > 0) {
                    drawChart(status.history);
                }
            } catch (error) {
                console.error('Error updating status:', error);
            }
        }

        // Initial update
        updateStatus();
    </script>
</body>
</html>
`
