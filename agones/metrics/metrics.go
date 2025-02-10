// Package metrics provides Prometheus metrics for the Assetto Corsa server.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// ServerLabels defines common labels for all server metrics
var ServerLabels = []string{"server_id", "server_name", "server_type"}

// Basic server metrics
var (
	// ServerStateGauge tracks the current state of the server
	ServerStateGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "assetto_server_state",
		Help: "Current state of the server (0=starting, 1=ready, 2=allocated, 3=reserved, 4=shutdown)",
	}, ServerLabels)

	// PlayersGauge tracks the current number of connected players
	PlayersGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "assetto_server_players",
		Help: "Current number of connected players",
	}, ServerLabels)

	// ServerErrorsCounter tracks the number of server errors
	ServerErrorsCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "assetto_server_errors_total",
		Help: "Total number of server errors",
	}, append(ServerLabels, "error_type"))
)

// Health and performance metrics
var (
	// HealthPingFailuresCounter tracks failed health checks
	HealthPingFailuresCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "assetto_server_health_ping_failures_total",
		Help: "Total number of failed health pings",
	}, ServerLabels)

	// LastHealthPingGauge tracks the time since last successful health check
	LastHealthPingGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "assetto_server_last_health_ping_seconds",
		Help: "Time since last successful health ping in seconds",
	}, ServerLabels)

	// TickRateGauge tracks the current server tick rate
	TickRateGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "assetto_server_tick_rate",
		Help: "Current server tick rate",
	}, ServerLabels)
)

// Resource usage metrics
var (
	// CpuUsageGauge tracks CPU usage
	CpuUsageGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "assetto_server_cpu_usage",
		Help: "Current CPU usage percentage",
	}, ServerLabels)

	// MemoryUsageGauge tracks memory usage
	MemoryUsageGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "assetto_server_memory_usage_bytes",
		Help: "Current memory usage in bytes",
	}, ServerLabels)
)

// Session metrics
var (
	// SessionDurationHistogram tracks session duration distribution
	SessionDurationHistogram = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "assetto_server_session_duration_distribution_seconds",
			Help:    "Distribution of session durations in seconds",
			Buckets: prometheus.ExponentialBuckets(60, 2, 10), // Starting from 1 minute
		},
		append(ServerLabels, "session_type"),
	)

	// SessionDurationGauge tracks the current session duration
	SessionDurationGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "assetto_server_session_duration_seconds",
		Help: "Duration of the current session in seconds",
	}, append(ServerLabels, "session_type"))

	// SessionTimeLeftGauge tracks remaining session time
	SessionTimeLeftGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "assetto_server_session_time_left_seconds",
		Help: "Time remaining in the current session in seconds",
	}, ServerLabels)

	// SessionChangeCounter tracks session changes
	SessionChangeCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "assetto_server_session_changes_total",
		Help: "Total number of session changes",
	}, ServerLabels)
)

// Track condition metrics
var (
	// TrackGripGauge tracks track grip level
	TrackGripGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "assetto_server_track_grip",
		Help: "Current track grip level percentage",
	}, ServerLabels)

	// TrackTemperatureGauge tracks track temperature
	TrackTemperatureGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "assetto_server_track_temperature",
		Help: "Current track temperature in Celsius",
	}, ServerLabels)

	// AirTemperatureGauge tracks air temperature
	AirTemperatureGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "assetto_server_air_temperature",
		Help: "Current air temperature in Celsius",
	}, ServerLabels)
)

// Track and car usage metrics
var (
	// TrackUsageCounter tracks how many times each track is used
	TrackUsageCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "assetto_server_track_usage_total",
		Help: "Total number of times each track has been used",
	}, append(ServerLabels, "track_name"))

	// CarUsageCounter tracks how many times each car is used
	CarUsageCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "assetto_server_car_usage_total",
		Help: "Total number of times each car has been used",
	}, append(ServerLabels, "car_name"))
)

// Player metrics
var (
	// PlayerConnectCounter tracks player connections
	PlayerConnectCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "assetto_server_player_connects_total",
		Help: "Total number of player connections",
	}, ServerLabels)

	// PlayerLatencyGauge tracks player latency
	PlayerLatencyGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "assetto_server_player_latency_ms",
		Help: "Current player latency in milliseconds",
	}, append(ServerLabels, "player_name", "steam_id"))

	// PacketLossGauge tracks player packet loss
	PacketLossGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "assetto_server_player_packet_loss",
		Help: "Current player packet loss percentage",
	}, append(ServerLabels, "player_name", "steam_id"))

	// PlayerBestLapGauge tracks player best lap times
	PlayerBestLapGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "assetto_server_player_best_lap_ms",
		Help: "Player best lap time in milliseconds",
	}, append(ServerLabels, "player_name", "steam_id"))

	// PlayerDisconnectCounter tracks player disconnections
	PlayerDisconnectCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "assetto_server_player_disconnects_total",
		Help: "Total number of player disconnections",
	}, ServerLabels)

	// AuthSuccessCounter tracks successful authentications
	AuthSuccessCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "assetto_server_auth_success_total",
		Help: "Total number of successful authentications",
	}, ServerLabels)
)

// Server operation metrics
var (
	// ServerPortsGauge tracks server ports usage
	ServerPortsGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "assetto_server_ports_total",
		Help: "Current number of ports used by the server",
	}, []string{"port_type", "port"})

	// ServerUpdateRateGauge tracks server update rate
	ServerUpdateRateGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "assetto_server_update_rate_seconds",
		Help: "Current server update rate in seconds",
	}, ServerLabels)

	// LobbyRegistrationCounter tracks lobby registrations
	LobbyRegistrationCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "assetto_server_lobby_registrations_total",
		Help: "Total number of lobby registrations",
	}, ServerLabels)

	// ServerStartCounter tracks server starts
	ServerStartCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "assetto_server_starts_total",
		Help: "Total number of server starts",
	}, ServerLabels)

	// SessionEndCounter tracks session ends
	SessionEndCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "assetto_server_ends_total",
		Help: "Total number of server ends",
	}, ServerLabels)
)

// Debug metrics
var (
	// CommandProcessingTimeHistogram tracks command processing times
	CommandProcessingTimeHistogram = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "assetto_server_command_processing_seconds",
			Help:    "Time spent processing server commands",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 10),
		},
		append(ServerLabels, "command_type"),
	)

	// PlayerLatencyHistogram tracks player latency distribution
	PlayerLatencyHistogram = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "assetto_server_player_latency_distribution_ms",
			Help:    "Distribution of player latencies",
			Buckets: prometheus.LinearBuckets(0, 50, 20),
		},
		append(ServerLabels, "player_name"),
	)
)

// Network metrics
var (
	// NetworkBytesReceivedCounter tracks received network traffic
	NetworkBytesReceivedCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "assetto_server_network_bytes_received_total",
		Help: "Total number of bytes received",
	}, ServerLabels)

	// NetworkBytesSentCounter tracks sent network traffic
	NetworkBytesSentCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "assetto_server_network_bytes_sent_total",
		Help: "Total number of bytes sent",
	}, ServerLabels)
)

// CSP related metrics
var (
	// CSPVersionGauge tracks CSP version of connected players
	CSPVersionGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "assetto_server_csp_version",
		Help: "CSP version of connected players",
	}, append(ServerLabels, "player_name"))
)

// Chat metrics
var (
	// ChatMessagesCounter tracks total number of chat messages
	ChatMessagesCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "assetto_server_chat_messages_total",
		Help: "Total number of chat messages",
	}, ServerLabels)
)
