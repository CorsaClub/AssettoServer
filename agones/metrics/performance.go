package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Performance metrics
var (
	// Server Performance
	ServerFPSGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "assetto_server_fps",
			Help: "Current server FPS",
		},
		ServerLabels,
	)

	// Server Tick Time
	ServerTickTimeHistogram = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "assetto_server_tick_time_ms",
			Help:    "Server tick processing time in milliseconds",
			Buckets: prometheus.LinearBuckets(0, 5, 20), // 0-100ms in 5ms steps
		},
		ServerLabels,
	)

	// Network Performance
	NetworkLatencyHistogram = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "assetto_server_network_latency_ms",
			Help:    "Network latency per player in milliseconds",
			Buckets: prometheus.ExponentialBuckets(10, 1.5, 10), // 10ms to ~400ms
		},
		append(ServerLabels, "player_id"),
	)

	// Network Packet Loss
	NetworkPacketLossGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "assetto_server_packet_loss_percent",
			Help: "Packet loss percentage per player",
		},
		append(ServerLabels, "player_id"),
	)

	// Resource Usage
	CPUUsagePerThreadGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "assetto_server_cpu_usage_per_thread",
			Help: "CPU usage per thread percentage",
		},
		append(ServerLabels, "thread_id"),
	)

	// Memory Usage
	MemoryDetailedGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "assetto_server_memory_detailed_bytes",
			Help: "Detailed memory usage in bytes",
		},
		append(ServerLabels, "type"), // heap, stack, etc.
	)

	// Goroutine Wait Time
	GoroutineWaitTimeHistogram = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "assetto_server_goroutine_wait_time_ms",
			Help:    "Time goroutines spend waiting",
			Buckets: prometheus.ExponentialBuckets(0.1, 2, 10),
		},
		ServerLabels,
	)

	// Disk I/O
	DiskOperationsCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "assetto_server_disk_operations_total",
			Help: "Number of disk operations",
		},
		append(ServerLabels, "operation"), // read, write
	)

	// Session Performance
	SessionLoadTimeHistogram = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "assetto_server_session_load_time_seconds",
			Help:    "Time taken to load sessions",
			Buckets: prometheus.LinearBuckets(0, 1, 10),
		},
		append(ServerLabels, "session_type"),
	)

	// Player Performance
	PlayerUpdateTimeHistogram = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "assetto_server_player_update_time_ms",
			Help:    "Time taken to process player updates",
			Buckets: prometheus.ExponentialBuckets(0.1, 2, 10),
		},
		append(ServerLabels, "update_type"),
	)
)
