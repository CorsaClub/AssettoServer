package monitoring

import (
	"context"
	"runtime"
	"sync"
	"time"

	metrics "metrics/services"
	"metrics/types"
	"metrics/utils"
)

type PerformanceMonitor struct {
	state    *types.ServerState
	vmClient *metrics.VictoriaMetricsClient
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

// NewPerformanceMonitor creates a new PerformanceMonitor instance
func NewPerformanceMonitor(state *types.ServerState, vmClient *metrics.VictoriaMetricsClient) *PerformanceMonitor {
	return &PerformanceMonitor{
		state:         state,
		vmClient:      vmClient,
		perfUpdates:   make(chan perfMetrics, 100),
		frameTimes:    make([]time.Duration, 0, 100),
		lastFrameTime: time.Now(),
		maxFrameTimes: 100, // Keep track of the last 100 frames
	}
}

// Start begins monitoring performance metrics
func (pm *PerformanceMonitor) Start(ctx context.Context) {
	// High frequency collection (every 100ms)
	go pm.collectHighFrequencyMetrics(ctx)

	// Low frequency collection (every 5 seconds)
	go pm.collectLowFrequencyMetrics(ctx)

	// Process metrics
	go pm.processMetrics(ctx)
}

// Collects high frequency metrics
func (pm *PerformanceMonitor) collectHighFrequencyMetrics(ctx context.Context) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			start := time.Now()

			// Record frame time
			pm.recordFrameTime()

			// Collect FPS and tick time metrics
			metrics := perfMetrics{
				fps:      pm.calculateFPS(),
				tickTime: float64(time.Since(start).Microseconds()) / 1000.0,
			}

			select {
			case pm.perfUpdates <- metrics:
			default:
				utils.LogWarning("Performance metrics channel full, dropping update")
			}
		}
	}
}

// Collects low frequency metrics
func (pm *PerformanceMonitor) collectLowFrequencyMetrics(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			var memStats runtime.MemStats
			runtime.ReadMemStats(&memStats)

			metricsData := metrics.MetricBatch{
				Metrics: []metrics.Metric{
					{
						Name:      metrics.ServerMemoryUsage,
						Value:     float64(memStats.HeapAlloc),
						Type:      metrics.Gauge,
						Timestamp: time.Now(),
						LabelValues: map[string]string{
							"server_id":   pm.state.ServerID,
							"server_name": pm.state.ServerName,
							"memory_type": "heap",
						},
					},
					{
						Name:      metrics.ServerMemoryUsage,
						Value:     float64(memStats.StackInuse),
						Type:      metrics.Gauge,
						Timestamp: time.Now(),
						LabelValues: map[string]string{
							"server_id":   pm.state.ServerID,
							"server_name": pm.state.ServerName,
							"memory_type": "stack",
						},
					},
				},
				Time: time.Now(),
			}

			pm.state.RLock()
			for _, player := range pm.state.ConnectedPlayers {
				metricsData.Metrics = append(metricsData.Metrics, metrics.Metric{
					Name:      "assetto_server_player_latency_ms",
					Value:     float64(player.Latency),
					Type:      metrics.Gauge,
					Timestamp: time.Now(),
					LabelValues: map[string]string{
						"server_id":   pm.state.ServerID,
						"session_id":  pm.state.CurrentSession.ID,
						"player_id":   player.SteamID,
						"player_name": player.Name,
					},
				})
				metricsData.Metrics = append(metricsData.Metrics, metrics.Metric{
					Name:      "assetto_server_player_packet_loss",
					Value:     player.PacketLoss,
					Type:      metrics.Gauge,
					Timestamp: time.Now(),
					LabelValues: map[string]string{
						"server_id":   pm.state.ServerID,
						"session_id":  pm.state.CurrentSession.ID,
						"player_id":   player.SteamID,
						"player_name": player.Name,
					},
				})
			}
			pm.state.RUnlock()

			pm.vmClient.SendMetrics(metricsData)
		}
	}
}

// Processes metrics
func (pm *PerformanceMonitor) processMetrics(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case perfData := <-pm.perfUpdates:
			// Envoyer les métriques à VictoriaMetrics
			pm.vmClient.SendMetrics(metrics.MetricBatch{
				Metrics: []metrics.Metric{
					{
						Name:      metrics.ServerFPS,
						Value:     perfData.fps,
						Type:      metrics.Gauge,
						Timestamp: time.Now(),
						LabelValues: map[string]string{
							"server_id":   pm.state.ServerID,
							"server_name": pm.state.ServerName,
							"server_type": pm.state.ServerType,
						},
					},
					{
						Name:      "assetto_server_tick_time_ms",
						Value:     perfData.tickTime,
						Type:      metrics.Gauge,
						Timestamp: time.Now(),
						LabelValues: map[string]string{
							"server_id":    pm.state.ServerID,
							"session_id":   pm.state.CurrentSession.ID,
							"session_type": pm.state.CurrentSession.Type,
						},
					},
					{
						Name:      metrics.DebugGoroutines,
						Value:     float64(runtime.NumGoroutine()),
						Type:      metrics.Gauge,
						Timestamp: time.Now(),
						LabelValues: map[string]string{
							"server_id":   pm.state.ServerID,
							"server_name": pm.state.ServerName,
							"server_type": pm.state.ServerType,
						},
					},
					{
						Name:      metrics.ServerUptime,
						Value:     float64(time.Since(pm.state.StartTime).Seconds()),
						Type:      metrics.Gauge,
						Timestamp: time.Now(),
						LabelValues: map[string]string{
							"server_id":   pm.state.ServerID,
							"server_name": pm.state.ServerName,
							"server_type": pm.state.ServerType,
						},
					},
				},
				Time: time.Now(),
			})
		}
	}
}

// recordFrameTime records the time between frames
func (pm *PerformanceMonitor) recordFrameTime() {
	now := time.Now()
	frameTime := now.Sub(pm.lastFrameTime)
	pm.lastFrameTime = now

	// Only record reasonable frame times (between 1ms and 1s)
	if frameTime >= time.Millisecond && frameTime <= time.Second {
		pm.frameTimesMutex.Lock()
		defer pm.frameTimesMutex.Unlock()

		// Add the new frame time
		pm.frameTimes = append(pm.frameTimes, frameTime)

		// Keep only the most recent frame times
		if len(pm.frameTimes) > pm.maxFrameTimes {
			pm.frameTimes = pm.frameTimes[len(pm.frameTimes)-pm.maxFrameTimes:]
		}
	}
}

// Calculates FPS based on the average frame time
func (pm *PerformanceMonitor) calculateFPS() float64 {
	pm.frameTimesMutex.Lock()
	defer pm.frameTimesMutex.Unlock()

	// If we don't have enough frame times, fall back to tick rate
	if len(pm.frameTimes) < 10 {
		return pm.state.TickRate
	}

	// Calculate the average frame time
	var totalTime time.Duration
	for _, frameTime := range pm.frameTimes {
		totalTime += frameTime
	}

	avgFrameTime := totalTime / time.Duration(len(pm.frameTimes))

	// Convert to FPS (frames per second)
	if avgFrameTime <= 0 {
		return pm.state.TickRate // Fallback to tick rate if we have invalid data
	}

	fps := float64(time.Second) / float64(avgFrameTime)

	// Apply some smoothing to avoid wild fluctuations
	// Blend with the tick rate (80% new value, 20% tick rate)
	smoothedFPS := (fps * 0.8) + (pm.state.TickRate * 0.2)

	return smoothedFPS
}
