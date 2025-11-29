# Load-Aware Batcher Web Dashboard

A beautiful real-time visualization dashboard for the Load-Aware Batcher, showcasing how batch sizes dynamically adapt to backend load in real-time.

## 🎨 Features

- **Real-time Charts**: Live visualization of batch sizes, CPU load, queue depth, and processing time
- **Multiple Load Patterns**: Test with constant, sine wave, spike, and gradual load patterns
- **Interactive Controls**: Start/stop simulation with different patterns on the fly
- **Beautiful UI**: Modern, responsive design with animations and gradients
- **Live Metrics**: Monitor current batch size, CPU load, queue depth, and error rate
- **Performance Dashboard**: Track total items processed and batches completed

## 🚀 Quick Start

1. **Start the Dashboard Server**:
   ```bash
   go run cmd/webdemo/main.go
   ```

2. **Open in Browser**:
   Navigate to [http://localhost:8080](http://localhost:8080)

3. **Start a Simulation**:
   Click any of the load pattern buttons:
   - **▶ Constant Load**: Steady, predictable load
   - **〜 Sine Wave**: Periodic load variation
   - **⚡ Spikes**: Random load spikes (most dramatic!)
   - **📈 Gradual**: Gradually increasing load

4. **Watch the Magic**:
   - The **Batch Size & Load Score** chart shows how the batcher adapts
   - **CPU & Queue Depth** chart shows backend pressure
   - **Current Metrics** displays real-time values
   - **Processing Time** shows response time trends

## 📊 What You'll See

### When Load is Low (Green Zone)
- 🟢 Batch size **increases** automatically
- Throughput maximizes
- CPU and queue stay low

### When Load is High (Red Zone)
- 🔴 Batch size **decreases** automatically
- Prevents overload
- Error rate stays controlled

### Spike Pattern (Most Impressive!)
Watch as random load spikes occur and the batcher:
1. Detects the spike in CPU/Queue
2. Immediately reduces batch size
3. Prevents system overload
4. Gradually recovers when spike passes
5. Increases batch size again

## 🎯 Technical Details

The dashboard consists of:

- **Backend Server** (Go): Serves the dashboard and provides REST API
- **Metrics API**: Real-time metrics endpoint updated every 500ms
- **Simulator**: Generates various load patterns
- **Batcher**: Adapts batch size based on feedback
- **Frontend**: Vanilla HTML/CSS/JS with Chart.js

### API Endpoints

- `GET /` - Dashboard UI
- `POST /api/start` - Start simulation with pattern
- `POST /api/stop` - Stop simulation
- `GET /api/metrics` - Get metrics history
- `GET /api/status` - Get current status

### Configuration

The demo runs with these settings:
- **Initial Batch Size**: 20
- **Min Batch Size**: 5
- **Max Batch Size**: 100
- **Adjustment Factor**: 0.3 (30% per cycle)
- **Load Check Interval**: 3 seconds
- **Workers**: 4 concurrent workers

## 🎨 Design Highlights

- **Glassmorphism**: Frosted glass effects with backdrop blur
- **Gradient Backgrounds**: Beautiful purple-to-pink gradients
- **Smooth Animations**: Fade-in, hover effects, and micro-interactions
- **Responsive Design**: Works on desktop and mobile
- **Inter Font**: Modern, professional typography
- **Color-Coded Metrics**: Green (good), yellow (warning), red (danger)

## 💡 Use Cases

Perfect for:
- **Demos**: Show adaptive batching to stakeholders
- **Development**: Test and tune batcher parameters
- **Learning**: Understand how load-aware batching works
- **Debugging**: Visualize performance issues
- **Presentations**: Impressive real-time visualization

## 📝 Code Structure

```
cmd/webdemo/
  main.go          # HTTP server + embedded HTML dashboard
```

The main.go file contains:
1. **DashboardServer**: Manages batcher and simulator
2. **HTTP Handlers**: Serve UI and API endpoints
3. **Embedded HTML**: Complete dashboard UI in a string constant
4. **Worker Goroutines**: Generate continuous load
5. **Metrics Collection**: Track and store snapshots

## 🔧 Customization

Want to modify the visualization? The HTML is embedded in `main.go` as a constant string. Look for `const indexHTML = ...` and edit:

- **Colors**: Search for color values like `#667eea`, `#764ba2`
- **Chart Settings**: Modify `chartOptions` in JavaScript
- **Update Frequency**: Change ticker intervals
- **Load Patterns**: Add new patterns in `simulator/backend.go`

## 🌟 Pro Tips

1. **Spike Pattern** is the most visually impressive
2. Let simulations run for 30+ seconds to see full adaptation
3. Watch the batch size number change in real-time in the metrics card
4. The charts show the last 50 data points
5. You can stop and switch patterns without restarting the server

---

Made with ❤️ to showcase adaptive batch processing
