package metrics

import (
	"context"
	"metrics/types"
	"metrics/victoria"
	"runtime"
	"time"
)

// Performance metrics
var (
	// Server Performance
	ServerFPSGauge = types.NewMetric(
		"assetto_server_fps",
		"Current server FPS",
		types.Gauge,
		types.CommonLabels,
	)

	// Server Tick Time
	ServerTickTimeHistogram = WithBuckets(
		types.NewMetric(
			"assetto_server_tick_time_ms",
			"Server tick processing time in milliseconds",
			types.Histogram,
			types.CommonLabels,
		),
		LinearBuckets(0, 5, 20), // 0-100ms in 5ms steps
	)

	// Network Performance
	NetworkLatencyHistogram = WithBuckets(
		types.NewMetric(
			"assetto_server_network_latency_ms",
			"Network latency per player in milliseconds",
			types.Histogram,
			append(types.CommonLabels, "player_id"),
		),
		ExponentialBuckets(10, 1.5, 10), // 10ms to ~400ms
	)

	// Network Packet Loss
	NetworkPacketLossGauge = types.NewMetric(
		"assetto_server_packet_loss_percent",
		"Packet loss percentage per player",
		types.Gauge,
		append(types.CommonLabels, "player_id"),
	)

	// Resource Usage
	CPUUsagePerThreadGauge = types.NewMetric(
		"assetto_server_cpu_usage",
		"CPU usage per thread percentage",
		types.Gauge,
		append(types.CommonLabels, "thread_id"),
	)

	// Goroutines
	GoroutinesGauge = types.NewMetric(
		"assetto_server_goroutines_total",
		"Number of goroutines",
		types.Gauge,
		types.CommonLabels,
	)

	// Memory Allocation
	MemoryAllocGauge = types.NewMetric(
		"assetto_server_memory_alloc_bytes",
		"Memory allocation in bytes",
		types.Gauge,
		types.CommonLabels,
	)

	// GC Stats
	GCPauseHistogram = WithBuckets(
		types.NewMetric(
			"assetto_server_gc_pause_seconds",
			"GC pause time in seconds",
			types.Histogram,
			types.CommonLabels,
		),
		ExponentialBuckets(0.001, 2, 10), // 1ms to ~1s
	)

	// Memory Usage
	MemoryDetailedGauge = types.NewMetric(
		"assetto_server_memory_detailed_bytes",
		"Detailed memory usage in bytes",
		types.Gauge,
		append(types.CommonLabels, "type"), // heap, stack, etc.
	)

	// Goroutine Wait Time
	GoroutineWaitTimeHistogram = WithBuckets(
		types.NewMetric(
			"assetto_server_goroutine_wait_time_ms",
			"Time goroutines spend waiting",
			types.Histogram,
			types.CommonLabels,
		),
		ExponentialBuckets(0.1, 2, 10),
	)

	// Disk I/O
	DiskOperationsCounter = types.NewMetric(
		"assetto_server_disk_operations_total",
		"Number of disk operations",
		types.Counter,
		append(types.CommonLabels, "operation"), // read, write
	)

	// Session Performance
	SessionLoadTimeHistogram = WithBuckets(
		types.NewMetric(
			"assetto_server_session_load_time_seconds",
			"Time taken to load sessions",
			types.Histogram,
			append(types.CommonLabels, "session_type"),
		),
		LinearBuckets(0, 1, 10),
	)

	// Player Performance
	PlayerUpdateTimeHistogram = WithBuckets(
		types.NewMetric(
			"assetto_server_player_update_time_ms",
			"Time taken to process player updates",
			types.Histogram,
			append(types.CommonLabels, "update_type"),
		),
		ExponentialBuckets(0.1, 2, 10),
	)

	MetricsSendDuration = WithBuckets(
		types.NewMetric(
			"assetto_metrics_send_duration_seconds",
			"Time taken to send metrics to VictoriaMetrics",
			types.Histogram,
			[]string{"status"},
		),
		[]float64{0.01, 0.05, 0.1, 0.5, 1, 2, 5},
	)

	MetricsBufferSize = types.NewMetric(
		"assetto_metrics_buffer_size",
		"Current size of metrics buffer",
		types.Gauge,
		nil,
	)

	// Metrics Processing
	MetricsProcessed = types.NewMetric(
		"assetto_server_metrics_processed_total",
		"Number of metrics processed",
		types.Counter,
		nil,
	)

	MetricsRetried = types.NewMetric(
		"assetto_server_metrics_retried_total",
		"Number of metrics retried",
		types.Counter,
		nil,
	)

	MetricsDropped = types.NewMetric(
		"assetto_server_metrics_dropped_total",
		"Number of metrics dropped due to buffer full",
		types.Counter,
		[]string{"reason"},
	)

	MemoryUsage = types.NewMetric(
		"assetto_server_memory_usage_bytes",
		"Current memory usage",
		types.Gauge,
		[]string{"type"},
	)
)

// LinearBuckets creates linearly spaced buckets
func LinearBuckets(start, width float64, count int) []float64 {
	buckets := make([]float64, count)
	for i := 0; i < count; i++ {
		buckets[i] = start + (float64(i) * width)
	}
	return buckets
}

// ExponentialBuckets creates exponentially spaced buckets
func ExponentialBuckets(start, factor float64, count int) []float64 {
	buckets := make([]float64, count)
	buckets[0] = start
	for i := 1; i < count; i++ {
		buckets[i] = buckets[i-1] * factor
	}
	return buckets
}

// WithBuckets adds bucket configuration to a histogram metric
func WithBuckets(m types.Metric, buckets []float64) types.Metric {
	if m.Type == types.Histogram {
		m.Buckets = buckets
	}
	return m
}

// StartPerformanceMonitoring starts monitoring performance metrics
func StartPerformanceMonitoring(ctx context.Context, client *victoria.MetricsClient) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Collect runtime stats
			var memStats runtime.MemStats
			runtime.ReadMemStats(&memStats)

			// Create metrics batch
			batch := types.MetricBatch{
				Metrics: []types.Metric{
					{
						Name:      "assetto_server_memory_usage_bytes",
						Value:     float64(memStats.Alloc),
						Type:      types.Gauge,
						Timestamp: time.Now(),
						LabelValues: map[string]string{
							"type": "heap",
						},
					},
					{
						Name:      "assetto_server_goroutines_total",
						Value:     float64(runtime.NumGoroutine()),
						Type:      types.Gauge,
						Timestamp: time.Now(),
					},
				},
				Time: time.Now(),
			}

			// Send metrics
			client.SendMetrics(batch)
		}
	}
}

// Ajouter un pool de workers pour le traitement des métriques
type MetricsProcessor struct {
	workers  int
	jobQueue chan MetricBatch
}

// Ajouter une méthode de monitoring des performances du système de métriques
func (mp *MetricsProcessor) monitorPerformance() {
	// Implémentation du monitoring
}
