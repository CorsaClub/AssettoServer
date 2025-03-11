package monitoring

import (
	"context"
	"runtime"
	"sync"
	"time"

	"metrics/metrics"
	"metrics/types"
	"metrics/utils"
	"metrics/victoria"
)

type PerformanceMonitor struct {
	state    *types.ServerState
	vmClient *victoria.MetricsClient
	// Channels for asynchronous collection
	perfUpdates chan perfMetrics

	// FPS calculation fields
	frameTimes      []time.Duration
	frameTimesMutex sync.Mutex
	lastFrameTime   time.Time
	maxFrameTimes   int
}

type perfMetrics struct {
	fps      float64
	tickTime float64
}

// NewPerformanceMonitor creates a new performance monitor
func NewPerformanceMonitor(state *types.ServerState, vmClient *victoria.MetricsClient) *PerformanceMonitor {
	return &PerformanceMonitor{
		state:         state,
		vmClient:      vmClient,
		perfUpdates:   make(chan perfMetrics, 100),
		frameTimes:    make([]time.Duration, 0, 100),
		lastFrameTime: time.Now(),
		maxFrameTimes: 100,
	}
}

// Start begins monitoring performance metrics
func (pm *PerformanceMonitor) Start(ctx context.Context) {
	// Start the high-frequency metrics collection
	go pm.collectHighFrequencyMetrics(ctx)

	// Start the low-frequency metrics collection
	go pm.collectLowFrequencyMetrics(ctx)

	// Start processing metrics
	go pm.processMetrics(ctx)
}

// collectHighFrequencyMetrics collects metrics that need to be sampled frequently
func (pm *PerformanceMonitor) collectHighFrequencyMetrics(ctx context.Context) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Record frame time for FPS calculation
			pm.recordFrameTime()

			// Calculate FPS
			fps := pm.calculateFPS()

			// Get tick time
			tickTime := 0.0
			if fps > 0 {
				tickTime = 1000.0 / fps // in milliseconds
			}

			// Send metrics for processing
			pm.perfUpdates <- perfMetrics{
				fps:      fps,
				tickTime: tickTime,
			}
		}
	}
}

// collectLowFrequencyMetrics collects metrics that don't need to be sampled as frequently
func (pm *PerformanceMonitor) collectLowFrequencyMetrics(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Collect memory stats
			var memStats runtime.MemStats
			runtime.ReadMemStats(&memStats)

			// Create a batch of metrics
			metricsData := types.MetricBatch{
				Metrics: []types.Metric{
					{
						Name:      metrics.ServerMemoryUsage.Name,
						Value:     float64(memStats.HeapAlloc),
						Type:      types.Gauge,
						Timestamp: time.Now(),
						LabelValues: map[string]string{
							"server_id":   pm.state.ServerID,
							"server_name": pm.state.ServerName,
							"server_type": pm.state.ServerType,
							"memory_type": "heap_alloc",
						},
					},
					{
						Name:      metrics.ServerMemoryUsage.Name,
						Value:     float64(memStats.StackInuse),
						Type:      types.Gauge,
						Timestamp: time.Now(),
						LabelValues: map[string]string{
							"server_id":   pm.state.ServerID,
							"server_name": pm.state.ServerName,
							"server_type": pm.state.ServerType,
							"memory_type": "stack_inuse",
						},
					},
					{
						Name:      metrics.ServerMemoryUsage.Name,
						Value:     float64(memStats.Sys),
						Type:      types.Gauge,
						Timestamp: time.Now(),
						LabelValues: map[string]string{
							"server_id":   pm.state.ServerID,
							"server_name": pm.state.ServerName,
							"server_type": pm.state.ServerType,
							"memory_type": "system",
						},
					},
					{
						Name:      metrics.SystemGoRoutines.Name,
						Value:     float64(runtime.NumGoroutine()),
						Type:      types.Gauge,
						Timestamp: time.Now(),
						LabelValues: map[string]string{
							"server_id":   pm.state.ServerID,
							"server_name": pm.state.ServerName,
							"server_type": pm.state.ServerType,
						},
					},
				},
				Time: time.Now(),
			}

			// Add player-specific metrics
			pm.state.RLock()
			for _, player := range pm.state.ConnectedPlayers {
				// Add player latency metric
				metricsData.Metrics = append(metricsData.Metrics, types.Metric{
					Name:      metrics.NetworkLatency.Name,
					Value:     float64(player.Latency),
					Type:      types.Gauge,
					Timestamp: time.Now(),
					LabelValues: map[string]string{
						"server_id":   pm.state.ServerID,
						"server_name": pm.state.ServerName,
						"server_type": pm.state.ServerType,
						"player_id":   player.SteamID,
						"player_name": player.Name,
					},
				})

				// Add player packet loss metric
				metricsData.Metrics = append(metricsData.Metrics, types.Metric{
					Name:      metrics.NetworkPacketLoss.Name,
					Value:     player.PacketLoss,
					Type:      types.Gauge,
					Timestamp: time.Now(),
					LabelValues: map[string]string{
						"server_id":   pm.state.ServerID,
						"server_name": pm.state.ServerName,
						"server_type": pm.state.ServerType,
						"player_id":   player.SteamID,
						"player_name": player.Name,
					},
				})
			}
			pm.state.RUnlock()

			// Send metrics to VictoriaMetrics
			if err := pm.vmClient.SendMetrics(metricsData); err != nil {
				utils.LogError("Failed to send performance metrics: %v", err)
			}
		}
	}
}

// processMetrics processes the collected metrics
func (pm *PerformanceMonitor) processMetrics(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	// Buffers for calculating averages
	var (
		fpsValues      []float64
		tickTimeValues []float64
	)

	for {
		select {
		case <-ctx.Done():
			return
		case metrics := <-pm.perfUpdates:
			// Add values to buffers
			fpsValues = append(fpsValues, metrics.fps)
			tickTimeValues = append(tickTimeValues, metrics.tickTime)

			// Limit buffer size
			if len(fpsValues) > 10 {
				fpsValues = fpsValues[1:]
			}
			if len(tickTimeValues) > 10 {
				tickTimeValues = tickTimeValues[1:]
			}
		case <-ticker.C:
			// Calculate averages
			var avgFPS, avgTickTime float64
			if len(fpsValues) > 0 {
				for _, v := range fpsValues {
					avgFPS += v
				}
				avgFPS /= float64(len(fpsValues))
			}
			if len(tickTimeValues) > 0 {
				for _, v := range tickTimeValues {
					avgTickTime += v
				}
				avgTickTime /= float64(len(tickTimeValues))
			}

			// Update server state
			pm.state.Lock()
			pm.state.TickRate = avgFPS
			pm.state.Unlock()

			// Create labels
			labels := map[string]string{
				"server_id":   pm.state.ServerID,
				"server_name": pm.state.ServerName,
				"server_type": pm.state.ServerType,
			}

			// Send metrics
			batch := types.MetricBatch{
				Metrics: []types.Metric{
					metrics.TickRateGauge.With(labels).Set(avgFPS),
				},
				Time: time.Now(),
			}

			if err := pm.vmClient.SendMetrics(batch); err != nil {
				utils.LogError("Failed to send tick rate metrics: %v", err)
			}
		}
	}
}

// recordFrameTime records the time between frames
func (pm *PerformanceMonitor) recordFrameTime() {
	now := time.Now()
	pm.frameTimesMutex.Lock()
	defer pm.frameTimesMutex.Unlock()

	// Calculate time since last frame
	frameTime := now.Sub(pm.lastFrameTime)
	pm.lastFrameTime = now

	// Add to the slice
	pm.frameTimes = append(pm.frameTimes, frameTime)

	// Limit the size of the slice
	if len(pm.frameTimes) > pm.maxFrameTimes {
		pm.frameTimes = pm.frameTimes[1:]
	}
}

// calculateFPS calculates the current FPS based on recorded frame times
func (pm *PerformanceMonitor) calculateFPS() float64 {
	pm.frameTimesMutex.Lock()
	defer pm.frameTimesMutex.Unlock()

	if len(pm.frameTimes) == 0 {
		return 0
	}

	// Calculate average frame time
	var totalTime time.Duration
	for _, t := range pm.frameTimes {
		totalTime += t
	}
	avgFrameTime := totalTime / time.Duration(len(pm.frameTimes))

	// Calculate FPS
	if avgFrameTime <= 0 {
		return 0
	}
	return float64(time.Second) / float64(avgFrameTime)
}
