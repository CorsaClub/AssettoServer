package monitoring

import (
	"context"
	"runtime"
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
}

type perfMetrics struct {
	fps      float64
	tickTime float64
}

// NewPerformanceMonitor creates a new PerformanceMonitor instance
func NewPerformanceMonitor(state *types.ServerState, vmClient *metrics.VictoriaMetricsClient) *PerformanceMonitor {
	return &PerformanceMonitor{
		state:       state,
		vmClient:    vmClient,
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
			var memStats runtime.MemStats
			runtime.ReadMemStats(&memStats)

			metricsData := metrics.MetricBatch{
				Metrics: []metrics.Metric{
					{
						Name:      "assetto_server_memory_heap_bytes",
						Value:     float64(memStats.HeapAlloc),
						Type:      metrics.Gauge,
						Timestamp: time.Now(),
						LabelValues: map[string]string{
							"server_id":    pm.state.ServerID,
							"session_id":   pm.state.CurrentSession.ID,
							"session_type": pm.state.CurrentSession.Type,
						},
					},
					{
						Name:      "assetto_server_memory_stack_bytes",
						Value:     float64(memStats.StackInuse),
						Type:      metrics.Gauge,
						Timestamp: time.Now(),
						LabelValues: map[string]string{
							"server_id":    pm.state.ServerID,
							"session_id":   pm.state.CurrentSession.ID,
							"session_type": pm.state.CurrentSession.Type,
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
						Name:      "assetto_server_fps",
						Value:     perfData.fps,
						Type:      metrics.Gauge,
						Timestamp: time.Now(),
						LabelValues: map[string]string{
							"server_id":    pm.state.ServerID,
							"session_id":   pm.state.CurrentSession.ID,
							"session_type": pm.state.CurrentSession.Type,
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
						Name:      "assetto_server_goroutines",
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
						Name:      "assetto_server_uptime",
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

// Calculates FPS
func (pm *PerformanceMonitor) calculateFPS() float64 {
	// Implementation of FPS calculation based on server tick rate
	return pm.state.TickRate
}
