// Package metrics contains metric definitions for the server
package metrics

import (
	"errors"
	"time"
)

// Common labels for all metrics
var ServerLabels = []string{"server_id", "server_name", "server_region"}

// Metric names
const (
	// Server metrics
	ServerPlayersConnected = "assetto_server_players_connected"
	ServerSessionType      = "assetto_server_session_type"
	ServerUptime           = "assetto_server_uptime_seconds"
	ServerHealth           = "assetto_server_health"

	// Network metrics
	NetworkBytesReceived   = "assetto_server_network_bytes_received_total"
	NetworkBytesSent       = "assetto_server_network_bytes_sent_total"
	NetworkLatency         = "assetto_server_network_latency_ms"
	NetworkPacketLoss      = "assetto_server_network_packet_loss_ratio"
	NetworkPacketsReceived = "assetto_server_network_packets_received_total"
	NetworkPacketsSent     = "assetto_server_network_packets_sent_total"
	NetworkErrors          = "assetto_server_network_errors_total"
	NetworkDrops           = "assetto_server_network_drops_total"

	// CSP metrics
	CSPVersion        = "assetto_server_csp_version"
	CSPFeatureEnabled = "assetto_server_csp_feature_enabled"

	// Performance metrics
	ServerFPS         = "assetto_server_fps"
	ServerTickRate    = "assetto_server_tick_rate"
	ServerMemoryUsage = "assetto_server_memory_bytes"
	ServerCPUUsage    = "assetto_server_cpu_usage"

	// Detailed system metrics
	SystemCPUUsage       = "assetto_server_system_cpu_usage"
	SystemMemoryTotal    = "assetto_server_system_memory_total_bytes"
	SystemMemoryUsed     = "assetto_server_system_memory_used_bytes"
	SystemMemoryFree     = "assetto_server_system_memory_free_bytes"
	SystemMemorySwap     = "assetto_server_system_memory_swap_bytes"
	SystemDiskUsage      = "assetto_server_system_disk_usage_bytes"
	SystemDiskFree       = "assetto_server_system_disk_free_bytes"
	SystemNetworkTraffic = "assetto_server_system_network_traffic_bytes"
	SystemGoMemoryAlloc  = "assetto_server_go_memory_alloc_bytes"
	SystemGoMemorySys    = "assetto_server_go_memory_sys_bytes"
	SystemGoRoutines     = "assetto_server_go_routines"

	// Session metrics
	SessionDuration  = "assetto_server_session_duration_seconds"
	SessionPlayers   = "assetto_server_session_players_total"
	SessionLaps      = "assetto_server_session_laps_total"
	SessionIncidents = "assetto_server_session_incidents_total"

	// Track metrics
	TrackGrip        = "assetto_server_track_grip"
	TrackTemperature = "assetto_server_track_temperature"
	AirTemperature   = "assetto_server_air_temperature"
)

// MetricType represents the type of metric
type MetricType string

const (
	Counter   MetricType = "counter"
	Gauge     MetricType = "gauge"
	Histogram MetricType = "histogram"
)

// Metric represents a single metric with its metadata
type Metric struct {
	Name        string
	Help        string
	Type        MetricType
	Labels      []string
	Value       float64
	Timestamp   time.Time
	Buckets     []float64 // Used for histograms
	LabelValues map[string]string
}

// MetricsBatch represents a collection of metrics to be sent
type MetricsBatch struct {
	Metrics []Metric
	Time    time.Time
}

// With adds labels to a metric and returns a new copy
func (m Metric) With(labels map[string]string) Metric {
	newMetric := m
	newMetric.LabelValues = make(map[string]string)
	// Copy existing labels
	for k, v := range m.LabelValues {
		newMetric.LabelValues[k] = v
	}
	// Add new labels
	for k, v := range labels {
		newMetric.LabelValues[k] = v
	}
	return newMetric
}

// Set sets the value for a gauge metric
func (m Metric) Set(value float64) {
	if m.Type != Gauge {
		// Log error or panic - this operation is only valid for gauges
		return
	}
	m.Value = value
	m.Timestamp = time.Now()
}

// Inc increments a counter metric by 1
func (m Metric) Inc() {
	if m.Type != Counter {
		// Log error or panic - this operation is only valid for counters
		return
	}
	m.Value++
	m.Timestamp = time.Now()
}

// Add adds a value to a counter metric
func (m Metric) Add(value float64) {
	if m.Type != Counter {
		// Log error or panic - this operation is only valid for counters
		return
	}
	m.Value += value
	m.Timestamp = time.Now()
}

// Observe adds a value to a histogram metric
func (m Metric) Observe(value float64) {
	if m.Type != Histogram {
		// Log error or panic - this operation is only valid for histograms
		return
	}
	m.Value = value
	m.Timestamp = time.Now()
}

// Predefined metrics
var (
	// Server state metrics
	ServerStateGauge = NewMetric(
		"assetto_server_state",
		"Current state of the server",
		Gauge,
		ServerLabels,
	)

	PlayersGauge = NewMetric(
		"assetto_server_players",
		"Current number of connected players",
		Gauge,
		ServerLabels,
	)

	ServerErrorsCounter = NewMetric(
		"assetto_server_errors_total",
		"Total number of server errors",
		Counter,
		append(ServerLabels, "error_type"),
	)

	// Server operations metrics
	ServerStartCounter = NewMetric(
		"assetto_server_starts_total",
		"Total number of server starts",
		Counter,
		ServerLabels,
	)

	SessionEndCounter = NewMetric(
		"assetto_server_ends_total",
		"Total number of server ends",
		Counter,
		ServerLabels,
	)

	SessionChangeCounter = NewMetric(
		"assetto_server_session_changes_total",
		"Total number of session changes",
		Counter,
		ServerLabels,
	)

	// Player metrics
	PlayerConnectCounter = NewMetric(
		"assetto_server_player_connects_total",
		"Total number of player connections",
		Counter,
		ServerLabels,
	)

	PlayerDisconnectCounter = NewMetric(
		"assetto_server_player_disconnects_total",
		"Total number of player disconnections",
		Counter,
		ServerLabels,
	)

	PlayerLatencyGauge = NewMetric(
		"assetto_server_player_latency_ms",
		"Current player latency in milliseconds",
		Gauge,
		append(ServerLabels, "player_name", "steam_id"),
	)

	// Track usage metrics
	TrackUsageCounter = NewMetric(
		"assetto_server_track_usage_total",
		"Total number of times each track has been used",
		Counter,
		append(ServerLabels, "track_name"),
	)

	// Car usage metrics
	CarUsageCounter = NewMetric(
		"assetto_server_car_usage_total",
		"Total number of times each car has been used",
		Counter,
		append(ServerLabels, "car_name"),
	)

	// Authentication metrics
	AuthSuccessCounter = NewMetric(
		"assetto_server_auth_success_total",
		"Total number of successful authentications",
		Counter,
		ServerLabels,
	)

	// Network metrics
	NetworkBytesReceivedCounter = NewMetric(
		NetworkBytesReceived,
		"Total number of bytes received",
		Counter,
		ServerLabels,
	)

	NetworkBytesSentCounter = NewMetric(
		NetworkBytesSent,
		"Total number of bytes sent",
		Counter,
		ServerLabels,
	)

	// Server ports metrics
	ServerPortsGauge = NewMetric(
		"assetto_server_ports_total",
		"Current number of ports used by the server",
		Gauge,
		[]string{"port_type", "port"},
	)

	// Server update rate metrics
	ServerUpdateRateGauge = NewMetric(
		"assetto_server_update_rate_seconds",
		"Current server update rate in seconds",
		Gauge,
		ServerLabels,
	)

	// Lobby metrics
	LobbyRegistrationCounter = NewMetric(
		"assetto_server_lobby_registrations_total",
		"Total number of lobby registrations",
		Counter,
		ServerLabels,
	)

	// Session duration metrics
	SessionDurationHistogram = NewMetric(
		"assetto_server_session_duration_distribution_seconds",
		"Distribution of session durations in seconds",
		Histogram,
		append(ServerLabels, "session_type"),
	)

	// CSP metrics
	CSPVersionGauge = NewMetric(
		CSPVersion,
		"CSP version of connected players",
		Gauge,
		append(ServerLabels, "player_name"),
	)

	// Chat metrics
	ChatMessagesCounter = NewMetric(
		"assetto_server_chat_messages_total",
		"Total number of chat messages",
		Counter,
		ServerLabels,
	)

	ChatMessagesByPlayerCounter = NewMetric(
		"assetto_server_chat_messages_by_player_total",
		"Total number of chat messages by player",
		Counter,
		append(ServerLabels, "player_name", "player_id"),
	)

	ChatMessagesByTypeCounter = NewMetric(
		"assetto_server_chat_messages_by_type_total",
		"Total number of chat messages by type",
		Counter,
		append(ServerLabels, "player_name", "message_type", "content"),
	)

	ChatMessagesBySessionCounter = NewMetric(
		"assetto_server_chat_messages_by_session_total",
		"Total number of chat messages by session type",
		Counter,
		append(ServerLabels, "session_type", "session_id"),
	)

	ChatMessageLengthHistogram = NewMetric(
		"assetto_server_chat_message_length",
		"Distribution of chat message lengths in characters",
		Histogram,
		append(ServerLabels, "player_name", "message_type"),
	).WithBuckets([]float64{10, 20, 30, 50, 100, 200, 500})

	// Session metrics
	SessionDurationGauge = NewMetric(
		SessionDuration,
		"Duration of the current session in seconds",
		Gauge,
		append(ServerLabels, "session_type"),
	)

	SessionTimeLeftGauge = NewMetric(
		"assetto_server_session_time_left_seconds",
		"Time remaining in the current session in seconds",
		Gauge,
		ServerLabels,
	)

	SessionIncidentsCounter = NewMetric(
		SessionIncidents,
		"Total number of incidents in the session",
		Counter,
		append(ServerLabels, "session_id", "session_type"),
	)

	SessionLapsCounter = NewMetric(
		SessionLaps,
		"Total number of laps in the session",
		Counter,
		append(ServerLabels, "session_id", "session_type"),
	)

	SessionPlayersCounter = NewMetric(
		SessionPlayers,
		"Total number of players in the session",
		Counter,
		append(ServerLabels, "session_id", "session_type"),
	)

	// Track condition metrics
	TrackGripGauge = NewMetric(
		TrackGrip,
		"Current track grip level percentage",
		Gauge,
		ServerLabels,
	)

	TrackTemperatureGauge = NewMetric(
		TrackTemperature,
		"Current track temperature in Celsius",
		Gauge,
		ServerLabels,
	)

	AirTemperatureGauge = NewMetric(
		AirTemperature,
		"Current air temperature in Celsius",
		Gauge,
		ServerLabels,
	)

	TickRateGauge = NewMetric(
		ServerTickRate,
		"Current server tick rate",
		Gauge,
		ServerLabels,
	)

	// Player metrics
	PacketLossGauge = NewMetric(
		NetworkPacketLoss,
		"Current player packet loss percentage",
		Gauge,
		append(ServerLabels, "player_name", "steam_id"),
	)

	PlayerBestLapGauge = NewMetric(
		"assetto_server_player_best_lap_ms",
		"Player best lap time in milliseconds",
		Gauge,
		append(ServerLabels, "player_name", "steam_id"),
	)

	// Resource usage metrics
	CpuUsageGauge = NewMetric(
		ServerCPUUsage,
		"Current CPU usage percentage",
		Gauge,
		ServerLabels,
	)

	MemoryUsageGauge = NewMetric(
		ServerMemoryUsage,
		"Current memory usage in bytes",
		Gauge,
		ServerLabels,
	)

	// New metrics
	CollisionCounter = NewMetric(
		"assetto_collision_total",
		"Total number of collisions",
		Counter,
		append(ServerLabels, "collision_type", "player_id", "player_name"),
	)

	LapTimeHistogram = NewMetric(
		"assetto_lap_time_seconds",
		"Lap time distribution",
		Histogram,
		append(ServerLabels, "player_id", "player_name", "car_model", "track"),
	).WithBuckets([]float64{60, 90, 120, 150, 180, 210, 240})

	IncidentCounter = NewMetric(
		"assetto_race_incident_total",
		"Total number of race incidents",
		Counter,
		append(ServerLabels, "incident_type", "player_id", "player_name"),
	)

	// Métriques de connexion
	ConnectionAttemptsCounter = NewMetric(
		"assetto_server_connection_attempts_total",
		"Total number of connection attempts",
		Counter,
		append(ServerLabels, "status"),
	)

	// Métriques CSP
	CSPFeaturesGauge = NewMetric(
		CSPFeatureEnabled,
		"CSP features enabled per player",
		Gauge,
		append(ServerLabels, "player_name", "feature"),
	)

	// Métriques de session
	SessionStateChangesCounter = NewMetric(
		"assetto_server_session_state_changes_total",
		"Total number of session state changes",
		Counter,
		append(ServerLabels, "from_state", "to_state"),
	)

	// Métriques de clean exit
	CleanExitsCounter = NewMetric(
		"assetto_server_clean_exits_total",
		"Total number of clean player exits",
		Counter,
		append(ServerLabels, "player_name", "steam_id"),
	)

	// Connection metrics détaillées
	ConnectionStatusCounter = NewMetric(
		"assetto_server_connection_status_total",
		"Connection attempts and results",
		Counter,
		append(ServerLabels, "player_name", "steam_id", "status"),
	)

	// Lobby registration confirmation
	LobbyRegistrationStatusCounter = NewMetric(
		"assetto_server_lobby_registration_status_total",
		"Lobby registration confirmation status",
		Counter,
		append(ServerLabels, "status", "details"),
	)

	// Session time remaining
	SessionRemainingTimeGauge = NewMetric(
		"assetto_server_session_remaining_seconds",
		"Remaining time in current session",
		Gauge,
		append(ServerLabels, "session_type"),
	)

	// Session switch events
	SessionSwitchCounter = NewMetric(
		"assetto_server_session_switch_total",
		"Session type changes",
		Counter,
		append(ServerLabels, "from_type", "to_type", "track", "config"),
	)

	// Server invites
	ServerInviteCounter = NewMetric(
		"assetto_server_invite_total",
		"Server invitation URLs generated",
		Counter,
		append(ServerLabels, "url_hash"),
	)
)

// NewMetric creates a new metric with the given parameters
func NewMetric(name string, help string, metricType MetricType, labels []string) Metric {
	return Metric{
		Name:        name,
		Help:        help,
		Type:        metricType,
		Labels:      labels,
		LabelValues: make(map[string]string),
		Timestamp:   time.Now(),
	}
}

// SetValue sets the value for the metric
func (m *Metric) SetValue(value float64) {
	m.Value = value
	m.Timestamp = time.Now()
}

// AddLabel adds a label key-value pair to the metric
func (m *Metric) AddLabel(key, value string) {
	m.LabelValues[key] = value
}

// SetLabels sets multiple labels at once
func (m *Metric) SetLabels(labels map[string]string) {
	for k, v := range labels {
		m.LabelValues[k] = v
	}
}

func (m *Metric) Validate() error {
	if m.Name == "" {
		return errors.New("metric name cannot be empty")
	}
	if m.Type == Histogram && len(m.Buckets) == 0 {
		return errors.New("histogram must have buckets")
	}
	return nil
}
