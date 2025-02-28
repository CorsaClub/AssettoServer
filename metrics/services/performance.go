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
	ServerFPSGauge = NewMetric(
		"assetto_server_fps",
		"Current server FPS",
		Gauge,
		ServerLabels,
	)

	// Server Tick Time
	ServerTickTimeHistogram = NewMetric(
		"assetto_server_tick_time_ms",
		"Server tick processing time in milliseconds",
		Histogram,
		ServerLabels,
	).WithBuckets(LinearBuckets(0, 5, 20)) // 0-100ms in 5ms steps

	// Network Performance
	NetworkLatencyHistogram = NewMetric(
		"assetto_server_network_latency_ms",
		"Network latency per player in milliseconds",
		Histogram,
		append(ServerLabels, "player_id"),
	).WithBuckets(ExponentialBuckets(10, 1.5, 10)) // 10ms to ~400ms

	// Network Packet Loss
	NetworkPacketLossGauge = NewMetric(
		"assetto_server_packet_loss_percent",
		"Packet loss percentage per player",
		Gauge,
		append(ServerLabels, "player_id"),
	)

	// Resource Usage
	CPUUsagePerThreadGauge = NewMetric(
		"assetto_server_cpu_usage_per_thread",
		"CPU usage per thread percentage",
		Gauge,
		append(ServerLabels, "thread_id"),
	)

	// Memory Usage
	MemoryDetailedGauge = NewMetric(
		"assetto_server_memory_detailed_bytes",
		"Detailed memory usage in bytes",
		Gauge,
		append(ServerLabels, "type"), // heap, stack, etc.
	)

	// Goroutine Wait Time
	GoroutineWaitTimeHistogram = NewMetric(
		"assetto_server_goroutine_wait_time_ms",
		"Time goroutines spend waiting",
		Histogram,
		ServerLabels,
	).WithBuckets(ExponentialBuckets(0.1, 2, 10))

	// Disk I/O
	DiskOperationsCounter = NewMetric(
		"assetto_server_disk_operations_total",
		"Number of disk operations",
		Counter,
		append(ServerLabels, "operation"), // read, write
	)

	// Session Performance
	SessionLoadTimeHistogram = NewMetric(
		"assetto_server_session_load_time_seconds",
		"Time taken to load sessions",
		Histogram,
		append(ServerLabels, "session_type"),
	).WithBuckets(LinearBuckets(0, 1, 10))

	// Player Performance
	PlayerUpdateTimeHistogram = NewMetric(
		"assetto_server_player_update_time_ms",
		"Time taken to process player updates",
		Histogram,
		append(ServerLabels, "update_type"),
	).WithBuckets(ExponentialBuckets(0.1, 2, 10))

	MetricsSendDuration = NewMetric(
		"assetto_metrics_send_duration_seconds",
		"Time taken to send metrics to VictoriaMetrics",
		Histogram,
		[]string{"status"},
	).WithBuckets([]float64{0.01, 0.05, 0.1, 0.5, 1, 2, 5})

	MetricsBufferSize = NewMetric(
		"assetto_metrics_buffer_size",
		"Current size of metrics buffer",
		Gauge,
		nil,
	)

	MetricsDropped = NewMetric(
		"assetto_metrics_dropped_total",
		"Number of metrics dropped due to buffer full",
		Counter,
		[]string{"reason"},
	)

	GoroutineCount = NewMetric(
		"assetto_goroutines_total",
		"Number of running goroutines",
		Gauge,
		nil,
	)

	MemoryUsage = NewMetric(
		"assetto_memory_usage_bytes",
		"Current memory usage",
		Gauge,
		[]string{"type"},
	)
)

// Helper functions for creating buckets
func LinearBuckets(start, width float64, count int) []float64 {
	buckets := make([]float64, count)
	for i := 0; i < count; i++ {
		buckets[i] = start + (float64(i) * width)
	}
	return buckets
}

func ExponentialBuckets(start, factor float64, count int) []float64 {
	buckets := make([]float64, count)
	value := start
	for i := 0; i < count; i++ {
		buckets[i] = value
		value *= factor
	}
	return buckets
}

// WithBuckets adds bucket configuration to a histogram metric
func (m Metric) WithBuckets(buckets []float64) Metric {
	if m.Type == Histogram {
		m.Buckets = buckets
	}
	return m
}

// StartPerformanceMonitoring démarre la surveillance des performances internes
func StartPerformanceMonitoring(ctx context.Context, client *victoria.Client) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			var memStats runtime.MemStats
			runtime.ReadMemStats(&memStats)

			client.SendMetrics(types.MetricBatch{
				Metrics: []types.Metric{
					{
						Name:      "assetto_memory_usage_bytes",
						Value:     float64(memStats.Alloc),
						Type:      types.MetricType(Gauge),
						Timestamp: time.Now(),
						LabelValues: map[string]string{
							"type": "heap",
						},
					},
					{
						Name:      "assetto_goroutines_total",
						Value:     float64(runtime.NumGoroutine()),
						Type:      types.MetricType(Gauge),
						Timestamp: time.Now(),
					},
				},
				Time: time.Now(),
			})
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
