// Package types contains definitions of structures used by the server.
package types

import (
	"time"
)

// MetricType represents the type of metric
type MetricType string

const (
	// Counter is a cumulative metric that only increases
	Counter MetricType = "counter"
	// Gauge is a metric that can increase and decrease
	Gauge MetricType = "gauge"
	// Histogram is a metric that samples observations and counts them in configurable buckets
	Histogram MetricType = "histogram"
)

// Common labels for all metrics
var CommonLabels = []string{"server_id", "server_name", "server_region"}

// Metric names are organized by category for better maintainability

// Server metrics - basic server state and health information
const (
	// ServerStateMetric represents the current state of the server (0=starting, 1=ready, 2=allocated, 3=reserved, 4=shutdown)
	ServerStateMetric = "assetto_server_state"
	// ServerPlayers represents the current number of connected players
	ServerPlayers = "assetto_server_players"
	// ServerPlayersConnected represents the current number of connected players (alternative name)
	ServerPlayersConnected = "assetto_server_players_connected"
	// ServerSessionType represents the current session type (practice, qualifying, race)
	ServerSessionType = "assetto_server_session_type"
	// ServerUptime represents the total uptime of the server in seconds
	ServerUptime = "assetto_server_uptime_seconds"
	// ServerHealth represents the health status of the server (0=unhealthy, 1=healthy)
	ServerHealth = "assetto_server_health"
	// ServerErrorsTotal represents the total number of server errors
	ServerErrorsTotal = "assetto_server_errors_total"
	// ServerStartsTotal represents the total number of server starts
	ServerStartsTotal = "assetto_server_starts_total"
	// ServerEndsTotal represents the total number of server ends
	ServerEndsTotal = "assetto_server_ends_total"
	// ServerCleanExitsTotal represents the total number of clean server exits
	ServerCleanExitsTotal = "assetto_server_clean_exits_total"
	// ServerTest is a test metric used for connection testing
	ServerTest = "assetto_server_test"
)

// Performance metrics - server performance and resource usage
const (
	// ServerFPS represents the current server frames per second
	ServerFPS = "assetto_server_fps"
	// ServerTickRate represents the current server tick rate
	ServerTickRate = "assetto_server_tick_rate"
	// ServerTickTimeMs represents the time taken for each server tick in milliseconds
	ServerTickTimeMs = "assetto_server_tick_time_ms"
	// ServerMemoryUsageBytes represents the current memory usage of the server in bytes
	ServerMemoryUsageBytes = "assetto_server_memory_usage_bytes"
	// ServerMemoryBytes represents the current memory usage of the server in bytes (alternative name)
	ServerMemoryBytes = "assetto_server_memory_bytes"
	// ServerCPUUsage represents the current CPU usage of the server as a percentage
	ServerCPUUsage = "assetto_server_cpu_usage"
	// ServerGoroutinesTotal represents the total number of goroutines
	ServerGoroutinesTotal = "assetto_server_goroutines_total"
	// ServerMemoryAllocBytes represents the total allocated memory in bytes
	ServerMemoryAllocBytes = "assetto_server_memory_alloc_bytes"
	// ServerGCPauseSeconds represents the GC pause time in seconds
	ServerGCPauseSeconds = "assetto_server_gc_pause_seconds"
	// ServerMemoryDetailedBytes represents detailed memory usage in bytes
	ServerMemoryDetailedBytes = "assetto_server_memory_detailed_bytes"
	// ServerGoroutineWaitTimeMs represents the goroutine wait time in milliseconds
	ServerGoroutineWaitTimeMs = "assetto_server_goroutine_wait_time_ms"
	// ServerDiskOperationsTotal represents the total number of disk operations
	ServerDiskOperationsTotal = "assetto_server_disk_operations_total"
	// ServerSessionLoadTimeSeconds represents the time taken to load a session in seconds
	ServerSessionLoadTimeSeconds = "assetto_server_session_load_time_seconds"
	// ServerPlayerUpdateTimeMs represents the time taken to update player data in milliseconds
	ServerPlayerUpdateTimeMs = "assetto_server_player_update_time_ms"
	// ServerUpdateRateSeconds represents the server update rate in seconds
	ServerUpdateRateSeconds = "assetto_server_update_rate_seconds"
)

// Network metrics - network traffic and connectivity
const (
	// NetworkBytesReceivedTotal represents the total bytes received
	NetworkBytesReceivedTotal = "assetto_server_network_bytes_received_total"
	// NetworkBytesSentTotal represents the total bytes sent
	NetworkBytesSentTotal = "assetto_server_network_bytes_sent_total"
	// NetworkLatencyMs represents the network latency in milliseconds
	NetworkLatencyMs = "assetto_server_network_latency_ms"
	// NetworkPacketLossRatio represents the packet loss ratio
	NetworkPacketLossRatio = "assetto_server_network_packet_loss_ratio"
	// NetworkPacketLossPercent represents the packet loss percentage
	NetworkPacketLossPercent = "assetto_server_packet_loss_percent"
	// NetworkPacketsReceivedTotal represents the total packets received
	NetworkPacketsReceivedTotal = "assetto_server_network_packets_received_total"
	// NetworkPacketsSentTotal represents the total packets sent
	NetworkPacketsSentTotal = "assetto_server_network_packets_sent_total"
	// NetworkErrorsTotal represents the total network errors
	NetworkErrorsTotal = "assetto_server_network_errors_total"
	// NetworkDropsTotal represents the total network packet drops
	NetworkDropsTotal = "assetto_server_network_drops_total"
	// ConnectionAttemptsTotal represents the total connection attempts
	ConnectionAttemptsTotal = "assetto_server_connection_attempts_total"
	// ConnectionStatusTotal represents the total connection status changes
	ConnectionStatusTotal = "assetto_server_connection_status_total"
	// PortsTotal represents the total number of ports used
	PortsTotal = "assetto_server_ports_total"
)

// Player metrics - player-related metrics
const (
	// PlayerConnectsTotal represents the total player connections
	PlayerConnectsTotal = "assetto_server_player_connects_total"
	// PlayerDisconnectsTotal represents the total player disconnections
	PlayerDisconnectsTotal = "assetto_server_player_disconnects_total"
	// PlayerLatencyMs represents the player latency in milliseconds
	PlayerLatencyMs = "assetto_server_player_latency_ms"
	// PlayerPacketLoss represents the player packet loss
	PlayerPacketLoss = "assetto_server_player_packet_loss"
	// PlayerCountryConnectionsTotal represents the total connections by country
	PlayerCountryConnectionsTotal = "assetto_server_player_country_connections_total"
	// PlayerCountryActive represents the active players by country
	PlayerCountryActive = "assetto_server_player_country_active"
	// PlayerCityConnectionsTotal represents the total connections by city
	PlayerCityConnectionsTotal = "assetto_server_player_city_connections_total"
	// PlayerCityActive represents the active players by city
	PlayerCityActive = "assetto_server_player_city_active"
	// PlayerBestLapMs represents the player's best lap time in milliseconds
	PlayerBestLapMs = "assetto_server_player_best_lap_ms"
	// PlayersCount represents the current player count
	PlayersCount = "assetto_server_players_count"
)

// Session metrics - session-related metrics
const (
	// SessionDurationSeconds represents the session duration in seconds
	SessionDurationSeconds = "assetto_server_session_duration_seconds"
	// SessionDuration represents the session duration (alternative name)
	SessionDuration = "assetto_server_session_duration"
	// SessionPlayersTotal represents the total players in the session
	SessionPlayersTotal = "assetto_server_session_players_total"
	// SessionLapsTotal represents the total laps in the session
	SessionLapsTotal = "assetto_server_session_laps_total"
	// SessionIncidentsTotal represents the total incidents in the session
	SessionIncidentsTotal = "assetto_server_session_incidents_total"
	// SessionChangesTotal represents the total session changes
	SessionChangesTotal = "assetto_server_session_changes_total"
	// SessionStateChangesTotal represents the total session state changes
	SessionStateChangesTotal = "assetto_server_session_state_changes_total"
	// SessionRemainingSeconds represents the remaining time in the session in seconds
	SessionRemainingSeconds = "assetto_server_session_remaining_seconds"
	// SessionTimeLeftSeconds represents the time left in the session in seconds
	SessionTimeLeftSeconds = "assetto_server_session_time_left_seconds"
	// SessionSwitchTotal represents the total session switches
	SessionSwitchTotal = "assetto_server_session_switch_total"
	// SessionDurationDistributionSeconds represents the distribution of session durations in seconds
	SessionDurationDistributionSeconds = "assetto_server_session_duration_distribution_seconds"
)

// Track metrics - track-related metrics
const (
	// TrackGrip represents the track grip level
	TrackGrip = "assetto_server_track_grip"
	// TrackTemperature represents the track temperature
	TrackTemperature = "assetto_server_track_temperature"
	// TrackTemp represents the track temperature (alternative name)
	TrackTemp = "assetto_server_track_temp"
	// AirTemperature represents the air temperature
	AirTemperature = "assetto_server_air_temperature"
	// TrackUsageTotal represents the total track usage
	TrackUsageTotal = "assetto_server_track_usage_total"
)

// Car metrics - car-related metrics
const (
	// CarUsageTotal represents the total car usage
	CarUsageTotal = "assetto_server_car_usage_total"
)

// CSP metrics - Custom Shaders Patch related metrics
const (
	// CSPVersion represents the CSP version
	CSPVersion = "assetto_server_csp_version"
	// CSPFeatureEnabled represents whether a CSP feature is enabled
	CSPFeatureEnabled = "assetto_server_csp_feature_enabled"
)

// Collision metrics - collision-related metrics
const (
	// CollisionsTotal represents the total collisions
	CollisionsTotal = "assetto_server_collisions_total"
	// CollisionsEnvironment represents the collisions with the environment
	CollisionsEnvironment = "assetto_server_collisions_environment"
	// CollisionsCar represents the collisions with cars
	CollisionsCar = "assetto_server_collisions_car"
	// CollisionsBySpeed represents the collisions by speed
	CollisionsBySpeed = "assetto_server_collisions_by_speed"
	// CollisionsByPlayer represents the collisions by player
	CollisionsByPlayer = "assetto_server_collisions_by_player"
)

// Chat metrics - chat-related metrics
const (
	// ChatMessagesTotal represents the total chat messages
	ChatMessagesTotal = "assetto_server_chat_messages_total"
	// ChatMessagesByPlayerTotal represents the total chat messages by player
	ChatMessagesByPlayerTotal = "assetto_server_chat_messages_by_player_total"
	// ChatMessagesByTypeTotal represents the total chat messages by type
	ChatMessagesByTypeTotal = "assetto_server_chat_messages_by_type_total"
	// ChatMessagesBySessionTotal represents the total chat messages by session
	ChatMessagesBySessionTotal = "assetto_server_chat_messages_by_session_total"
	// ChatMessageLength represents the chat message length
	ChatMessageLength = "assetto_server_chat_message_length"
)

// System metrics - detailed system resource metrics
const (
	// SystemCPUUsage represents the system CPU usage
	SystemCPUUsage = "assetto_server_system_cpu_usage"
	// SystemMemoryTotalBytes represents the total system memory in bytes
	SystemMemoryTotalBytes = "assetto_server_system_memory_total_bytes"
	// SystemMemoryUsedBytes represents the used system memory in bytes
	SystemMemoryUsedBytes = "assetto_server_system_memory_used_bytes"
	// SystemMemoryFreeBytes represents the free system memory in bytes
	SystemMemoryFreeBytes = "assetto_server_system_memory_free_bytes"
	// SystemMemorySwapBytes represents the system swap memory in bytes
	SystemMemorySwapBytes = "assetto_server_system_memory_swap_bytes"
	// SystemDiskUsageBytes represents the system disk usage in bytes
	SystemDiskUsageBytes = "assetto_server_system_disk_usage_bytes"
	// SystemDiskFreeBytes represents the free system disk space in bytes
	SystemDiskFreeBytes = "assetto_server_system_disk_free_bytes"
	// SystemNetworkTrafficBytes represents the system network traffic in bytes
	SystemNetworkTrafficBytes = "assetto_server_system_network_traffic_bytes"
	// SystemGoMemoryAllocBytes represents the Go memory allocation in bytes
	SystemGoMemoryAllocBytes = "assetto_server_go_memory_alloc_bytes"
	// SystemGoMemorySysBytes represents the Go memory system in bytes
	SystemGoMemorySysBytes = "assetto_server_go_memory_sys_bytes"
	// SystemGoRoutines represents the number of Go routines
	SystemGoRoutines = "assetto_server_go_routines"
)

// Metrics system metrics - metrics about the metrics system itself
const (
	// MetricsProcessedTotal represents the total processed metrics
	MetricsProcessedTotal = "assetto_server_metrics_processed_total"
	// MetricsRetriedTotal represents the total retried metrics
	MetricsRetriedTotal = "assetto_server_metrics_retried_total"
	// MetricsDroppedTotal represents the total dropped metrics
	MetricsDroppedTotal = "assetto_server_metrics_dropped_total"
	// MetricsErrorsTotal represents the total metrics errors
	MetricsErrorsTotal = "assetto_server_metrics_errors_total"
	// MetricsBufferUsage represents the metrics buffer usage
	MetricsBufferUsage = "assetto_server_metrics_buffer_usage"
	// MetricsBatchSizeHistogram represents the metrics batch size histogram
	MetricsBatchSizeHistogram = "assetto_server_metrics_batch_size"
	// MetricsProcessingDurationSeconds represents the metrics processing duration in seconds
	MetricsProcessingDurationSeconds = "assetto_server_metrics_processing_duration_seconds"
	// MetricsValidationErrorsTotal represents the total metrics validation errors
	MetricsValidationErrorsTotal = "assetto_server_metrics_validation_errors_total"
	// MetricsSendQueueSize represents the metrics send queue size
	MetricsSendQueueSize = "assetto_server_metrics_send_queue_size"
	// MetricsRetryCountTotal represents the total metrics retry count
	MetricsRetryCountTotal = "assetto_server_metrics_retry_count_total"
	// MetricsCompressionRatio represents the metrics compression ratio
	MetricsCompressionRatio = "assetto_server_metrics_compression_ratio"
	// MetricsProcessedRate represents the metrics processed rate
	MetricsProcessedRate = "assetto_server_metrics_processed_rate"
	// MetricsDroppedRate represents the metrics dropped rate
	MetricsDroppedRate = "assetto_server_metrics_dropped_rate"
	// MetricsRetryRate represents the metrics retry rate
	MetricsRetryRate = "assetto_server_metrics_retry_rate"
)

// Miscellaneous metrics - metrics that don't fit into other categories
const (
	// AuthSuccessTotal represents the total authentication successes
	AuthSuccessTotal = "assetto_server_auth_success_total"
	// LobbyRegistrationsTotal represents the total lobby registrations
	LobbyRegistrationsTotal = "assetto_server_lobby_registrations_total"
	// LobbyRegistrationStatusTotal represents the total lobby registration status changes
	LobbyRegistrationStatusTotal = "assetto_server_lobby_registration_status_total"
	// InviteTotal represents the total invites
	InviteTotal = "assetto_server_invite_total"
	// EventMetric represents a generic event metric
	EventMetric = "assetto_server_event"
)

// Metric represents a single metric with its metadata
type Metric struct {
	Name        string            // Name of the metric
	Help        string            // Help text describing the metric
	Type        MetricType        // Type of the metric (counter, gauge, histogram)
	Labels      []string          // Label names for the metric
	Value       float64           // Current value of the metric
	Timestamp   time.Time         // Timestamp when the metric was recorded
	Buckets     []float64         // Histogram buckets (only used for histogram metrics)
	LabelValues map[string]string // Label values for the metric
}

// MetricBatch represents a collection of metrics to be sent
type MetricBatch struct {
	Metrics []Metric  // Metrics in the batch
	Time    time.Time // Timestamp of the batch
}

// NewMetric creates a new metric with the given name, help text, type, and labels
func NewMetric(name string, help string, metricType MetricType, labels []string) Metric {
	return Metric{
		Name:        name,
		Help:        help,
		Type:        metricType,
		Labels:      labels,
		Timestamp:   time.Now(),
		LabelValues: make(map[string]string),
	}
}

// With adds labels to a metric and returns a new copy
func (m Metric) With(labels map[string]string) Metric {
	newMetric := m
	if newMetric.LabelValues == nil {
		newMetric.LabelValues = make(map[string]string)
	}

	for k, v := range labels {
		newMetric.LabelValues[k] = v
	}

	return newMetric
}

// Set sets the value of a gauge metric
func (m Metric) Set(value float64) Metric {
	newMetric := m
	newMetric.Value = value
	newMetric.Timestamp = time.Now()
	return newMetric
}

// Inc increments a counter metric by 1
func (m Metric) Inc() Metric {
	return m.Add(1)
}

// Add adds a value to a counter metric
func (m Metric) Add(value float64) Metric {
	newMetric := m
	newMetric.Value += value
	newMetric.Timestamp = time.Now()
	return newMetric
}

// Observe adds an observation to a histogram metric
func (m Metric) Observe(value float64) Metric {
	if m.Type != Histogram {
		return m
	}

	newMetric := m
	newMetric.Value = value
	newMetric.Timestamp = time.Now()
	return newMetric
}
