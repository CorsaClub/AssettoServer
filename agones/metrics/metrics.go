// Package metrics contains Prometheus metric definitions for the server
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// ServerLabels defines the common labels for all metrics
var ServerLabels = []string{"server_id", "server_name", "server_type"}

// Server state metrics
var (
	ServerStateGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "assetto_server_state",
		Help: "Current state of the server (0: Starting, 1: Ready, 2: Allocated, 3: Reserved, 4: Shutdown)",
	}, ServerLabels)

	PlayersGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "assetto_server_players_total",
		Help: "Current number of players on the server",
	}, ServerLabels)

	PlayerConnectCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "assetto_server_player_connects_total",
		Help: "Total number of player connections since server start",
	}, ServerLabels)

	PlayerDisconnectCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "assetto_server_player_disconnects_total",
		Help: "Total number of player disconnections since server start",
	}, ServerLabels)
)

// Session metrics
var (
	SessionChangeCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "assetto_server_session_changes_total",
		Help: "Total number of session changes",
	}, ServerLabels)

	SessionDurationGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "assetto_server_session_duration_seconds",
		Help: "Duration of the current session in seconds",
	}, append(ServerLabels, "session_type"))

	SessionStartTimeGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "assetto_server_session_start_timestamp",
		Help: "Start timestamp of the current session",
	}, append(ServerLabels, "session_type"))

	SessionTimeLeftGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "assetto_server_session_time_left_seconds",
		Help: "Time remaining in current session in seconds",
	}, ServerLabels)
)

// Health metrics
var (
	LastHealthPingGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "assetto_server_last_health_ping_seconds",
		Help: "Time since last successful health ping in seconds",
	}, ServerLabels)

	HealthPingFailuresCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "assetto_server_health_ping_failures_total",
		Help: "Total number of failed health pings",
	}, ServerLabels)
)

// Error and authentication metrics
var (
	ServerErrorsCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "assetto_server_errors_total",
		Help: "Total number of server errors detected",
	}, append(ServerLabels, "error_type"))

	AuthSuccessCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "assetto_server_auth_successes_total",
		Help: "Total number of successful Steam authentications",
	}, ServerLabels)
)

// Network metrics
var (
	NetworkBytesReceivedCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "assetto_server_network_bytes_received_total",
		Help: "Total number of bytes received by the server",
	}, ServerLabels)

	NetworkBytesSentCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "assetto_server_network_bytes_sent_total",
		Help: "Total number of bytes sent by the server",
	}, ServerLabels)

	NetworkLatencyGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "assetto_server_network_latency_ms",
		Help: "Average network latency in milliseconds",
	}, ServerLabels)
)

// Performance metrics
var (
	CpuUsageGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "assetto_server_cpu_usage_percent",
		Help: "CPU usage percentage of the server process",
	}, ServerLabels)

	MemoryUsageGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "assetto_server_memory_usage_bytes",
		Help: "Memory usage in bytes of the server process",
	}, ServerLabels)
)

// Usage metrics
var (
	// Car metrics
	CarUsageCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "assetto_server_car_usage_total",
		Help: "Number of times each car has been used",
	}, append(ServerLabels, "car_name"))

	CarCountGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "assetto_server_car_count",
		Help: "Number of cars per model currently in use",
	}, append(ServerLabels, "car_model"))

	// Track metrics
	TrackUsageCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "assetto_server_track_usage_total",
		Help: "Number of times each track has been used",
	}, append(ServerLabels, "track_name"))

	// Detailed player metrics
	PlayerLatencyGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "assetto_server_player_latency_ms",
		Help: "Player latency in milliseconds",
	}, append(ServerLabels, "player_name", "steam_id"))

	PlayerBestLapGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "assetto_server_player_best_lap_ms",
		Help: "Player best lap time in milliseconds",
	}, append(ServerLabels, "player_name", "steam_id"))

	PacketLossGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "assetto_server_packet_loss_percent",
		Help: "Current packet loss percentage",
	}, append(ServerLabels, "player_name"))

	// Track condition metrics
	TrackGripGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "assetto_server_track_grip_level",
		Help: "Current track grip level percentage",
	}, ServerLabels)

	TrackTemperatureGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "assetto_server_track_temperature_celsius",
		Help: "Current track temperature in Celsius",
	}, ServerLabels)

	AirTemperatureGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "assetto_server_air_temperature_celsius",
		Help: "Current air temperature in Celsius",
	}, ServerLabels)

	TickRateGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "assetto_server_tick_rate",
		Help: "Current server tick rate",
	}, ServerLabels)
)

// Add more metrics useful for debugging
var (
	// Metric to track command processing times
	CommandProcessingTimeHistogram = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "assetto_server_command_processing_seconds",
			Help:    "Time spent processing server commands",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 10),
		},
		append(ServerLabels, "command_type"),
	)

	// Metric to track player latency
	PlayerLatencyHistogram = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "assetto_server_player_latency_distribution_ms",
			Help:    "Distribution of player latencies",
			Buckets: prometheus.LinearBuckets(0, 50, 20),
		},
		append(ServerLabels, "player_name"),
	)
)

var (
	ServerPortsGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "assetto_server_ports_total",
		Help: "Current number of ports used by the server",
	}, []string{"port_type", "port"})

	ServerUpdateRateGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "assetto_server_update_rate_seconds",
		Help: "Current server update rate in seconds",
	}, ServerLabels)

	LobbyRegistrationCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "assetto_server_lobby_registrations_total",
		Help: "Total number of lobby registrations",
	}, ServerLabels)
)

var (
	ServerStartCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "assetto_server_starts_total",
		Help: "Total number of server starts",
	}, ServerLabels)

	SessionEndCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "assetto_server_ends_total",
		Help: "Total number of server ends",
	}, ServerLabels)

	SessionDurationHistogram = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "assetto_server_session_duration_seconds",
			Help:    "Duration of the current session in seconds",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 10),
		},
		append(ServerLabels, "session_type"),
	)
)
