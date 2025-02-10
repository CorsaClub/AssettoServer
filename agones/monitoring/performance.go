package monitoring

import (
	"context"
	"runtime"
	"time"

	"agones/metrics"
	"agones/types"
	"agones/utils"

	"github.com/prometheus/client_golang/prometheus"
)

type PerformanceMonitor struct {
	state *types.ServerState
	// Channels for asynchronous collection
	perfUpdates chan perfMetrics
}

type perfMetrics struct {
	fps      float64
	tickTime float64
}

// NewPerformanceMonitor creates a new PerformanceMonitor instance
func NewPerformanceMonitor(state *types.ServerState) *PerformanceMonitor {
	return &PerformanceMonitor{
		state:       state,
		perfUpdates: make(chan perfMetrics, 100),
	}
}

// Start starts the performance monitor
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
			// Collect memory metrics
			var memStats runtime.MemStats
			runtime.ReadMemStats(&memStats)

			labels := prometheus.Labels{
				"server_id":   pm.state.ServerID,
				"server_name": pm.state.ServerName,
				"server_type": pm.state.ServerType,
			}

			// Detailed memory metrics
			metrics.MemoryDetailedGauge.With(prometheus.Labels{
				"server_id": pm.state.ServerID,
				"type":      "heap",
			}).Set(float64(memStats.HeapAlloc))

			metrics.MemoryDetailedGauge.With(prometheus.Labels{
				"server_id": pm.state.ServerID,
				"type":      "stack",
			}).Set(float64(memStats.StackInuse))

			// Goroutine metrics
			metrics.GoroutineWaitTimeHistogram.With(labels).Observe(
				float64(memStats.PauseTotalNs) / 1000000.0,
			)

			// Collect network metrics for each player
			pm.state.RLock()
			for _, player := range pm.state.ConnectedPlayers {
				playerLabels := prometheus.Labels{
					"server_id": pm.state.ServerID,
					"player_id": player.SteamID,
				}

				metrics.NetworkLatencyHistogram.With(playerLabels).Observe(float64(player.Latency))
				if player.PacketLoss > 0 {
					metrics.NetworkPacketLossGauge.With(playerLabels).Set(player.PacketLoss)
				}
			}
			pm.state.RUnlock()
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
			labels := prometheus.Labels{
				"server_id":   pm.state.ServerID,
				"server_name": pm.state.ServerName,
				"server_type": pm.state.ServerType,
			}

			// Update metrics
			metrics.ServerFPSGauge.With(labels).Set(perfData.fps)
			metrics.ServerTickTimeHistogram.With(labels).Observe(perfData.tickTime)
		}
	}
}

// Calculates FPS
func (pm *PerformanceMonitor) calculateFPS() float64 {
	// Implementation of FPS calculation based on server tick rate
	return pm.state.TickRate
}
