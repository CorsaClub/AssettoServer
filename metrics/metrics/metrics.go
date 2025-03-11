// Package metrics contient toutes les définitions de métriques utilisées dans le projet
package metrics

import (
	"metrics/types"
)

// Common labels for all metrics
var CommonLabels = []string{"server_id", "server_name", "server_region"}

// Server state constants
const (
	ServerStateStarting  = 0
	ServerStateReady     = 1
	ServerStateAllocated = 2
	ServerStateReserved  = 3
	ServerStateShutdown  = 4
)

// Event types for logging
const (
	EventServerStart       = "server_start"
	EventServerStop        = "server_stop"
	EventPlayerConnect     = "player_connect"
	EventPlayerDisconnect  = "player_disconnect"
	EventSessionChange     = "session_change"
	EventSessionEnd        = "session_end"
	EventError             = "error"
	EventHealthCheck       = "health_check"
	EventCSPHandshake      = "csp_handshake"
	EventConnectionAttempt = "connection_attempt"
)

// Log levels
const (
	LogLevelInfo    = "info"
	LogLevelWarning = "warning"
	LogLevelError   = "error"
	LogLevelDebug   = "debug"
)

// Server metrics - basic server state and health information
var (
	// ServerStateGauge represents the current state of the server
	ServerStateGauge = types.NewMetric("assetto_server_state", "Current server state", types.Gauge, CommonLabels)

	// ServerStartCounter counts the number of server starts
	ServerStartCounter = types.NewMetric("assetto_server_starts_total", "Total number of server starts", types.Counter, CommonLabels)

	// PlayersGauge represents the current number of connected players
	PlayersGauge = types.NewMetric("assetto_server_players", "Current number of connected players", types.Gauge, CommonLabels)

	// ServerPlayersConnected represents the current number of connected players (alternative name)
	ServerPlayersConnected = types.NewMetric("assetto_server_players_connected", "Current number of connected players", types.Gauge, CommonLabels)

	// ServerHealth represents the health status of the server
	ServerHealth = types.NewMetric("assetto_server_health", "Server health status (0=unhealthy, 1=healthy)", types.Gauge, CommonLabels)

	// ServerErrorsCounter counts the number of server errors
	ServerErrorsCounter = types.NewMetric("assetto_server_errors_total", "Total number of server errors", types.Counter, append(CommonLabels, "error_type"))
)

// Session metrics
var (
	// SessionDurationGauge represents the duration of the current session
	SessionDurationGauge = types.NewMetric("assetto_server_session_duration_seconds", "Session duration in seconds", types.Gauge, append(CommonLabels, "session_id", "session_type"))

	// SessionTimeLeftGauge represents the time left in the current session
	SessionTimeLeftGauge = types.NewMetric("assetto_server_session_time_left_seconds", "Time left in the session in seconds", types.Gauge, CommonLabels)

	// SessionRemainingTimeGauge represents the remaining time in a session
	SessionRemainingTimeGauge = types.NewMetric("assetto_server_session_remaining_seconds", "Remaining time in a session in seconds", types.Gauge, append(CommonLabels, "session_type"))

	// ServerUptime represents the total uptime of the server
	ServerUptime = types.NewMetric("assetto_server_uptime_seconds", "Server uptime in seconds", types.Gauge, CommonLabels)

	// SessionChangeCounter counts the number of session changes
	SessionChangeCounter = types.NewMetric("assetto_server_session_changes_total", "Total number of session changes", types.Counter, CommonLabels)

	// SessionStateChangesCounter counts the number of session state changes
	SessionStateChangesCounter = types.NewMetric("assetto_server_session_state_changes_total", "Total number of session state changes", types.Counter, append(CommonLabels, "from_state", "to_state"))

	// SessionSwitchCounter counts the number of session switches
	SessionSwitchCounter = types.NewMetric("assetto_server_session_switches_total", "Total number of session switches", types.Counter, append(CommonLabels, "from_type", "to_type"))

	// SessionPlayersCounter counts the number of players in a session
	SessionPlayersCounter = types.NewMetric("assetto_server_session_players_total", "Total number of players in a session", types.Counter, append(CommonLabels, "session_id", "session_type"))

	// SessionLapsCounter counts the number of laps in a session
	SessionLapsCounter = types.NewMetric("assetto_server_session_laps_total", "Total number of laps in a session", types.Counter, append(CommonLabels, "session_id", "session_type"))

	// SessionIncidentsCounter counts the number of incidents in a session
	SessionIncidentsCounter = types.NewMetric("assetto_server_session_incidents_total", "Total number of incidents in a session", types.Counter, append(CommonLabels, "session_id", "session_type"))
)

// Track metrics
var (
	// TrackGrip represents the current track grip level
	TrackGrip = types.NewMetric("assetto_server_track_grip", "Track grip level", types.Gauge, CommonLabels)

	// TrackTemperature represents the current track temperature
	TrackTemperature = types.NewMetric("assetto_server_track_temperature", "Track temperature", types.Gauge, CommonLabels)

	// AirTemperature represents the current air temperature
	AirTemperature = types.NewMetric("assetto_server_air_temperature", "Air temperature", types.Gauge, CommonLabels)

	// TrackUsageCounter counts the number of times a track is used
	TrackUsageCounter = types.NewMetric("assetto_server_track_usage_total", "Total number of times a track is used", types.Counter, append(CommonLabels, "track", "layout"))
)

// Player metrics
var (
	// PlayerLatencyGauge represents the current latency of a player
	PlayerLatencyGauge = types.NewMetric("assetto_server_player_latency_ms", "Player latency in milliseconds", types.Gauge, append(CommonLabels, "player_id", "player_name"))

	// PacketLossGauge represents the current packet loss
	PacketLossGauge = types.NewMetric("assetto_server_packet_loss", "Current packet loss", types.Gauge, append(CommonLabels, "player_id", "player_name"))

	// PlayerBestLapGauge represents the best lap time of a player
	PlayerBestLapGauge = types.NewMetric("assetto_server_player_best_lap_seconds", "Player best lap time in seconds", types.Gauge, append(CommonLabels, "player_id", "player_name"))

	// PlayerConnectCounter counts the number of player connections
	PlayerConnectCounter = types.NewMetric("assetto_server_player_connects_total", "Total number of player connections", types.Counter, CommonLabels)

	// PlayerDisconnectCounter counts the number of player disconnections
	PlayerDisconnectCounter = types.NewMetric("assetto_server_player_disconnects_total", "Total number of player disconnections", types.Counter, CommonLabels)

	// PlayerCountryActiveGauge represents the number of active players from a country
	PlayerCountryActiveGauge = types.NewMetric("assetto_server_player_country_active", "Number of active players from a country", types.Gauge, append(CommonLabels, "country", "country_code"))

	// PlayerCityActiveGauge represents the number of active players from a city
	PlayerCityActiveGauge = types.NewMetric("assetto_server_player_city_active", "Number of active players from a city", types.Gauge, append(CommonLabels, "country", "country_code", "city"))

	// CarUsageCounter counts the number of times a car is used
	CarUsageCounter = types.NewMetric("assetto_server_car_usage_total", "Total number of times a car is used", types.Counter, append(CommonLabels, "car_model", "car_name"))
)

// System metrics
var (
	// CpuUsageGauge represents the current CPU usage
	CpuUsageGauge = types.NewMetric("assetto_server_cpu_usage", "CPU usage percentage", types.Gauge, CommonLabels)

	// MemoryUsageGauge represents the current memory usage
	MemoryUsageGauge = types.NewMetric("assetto_server_memory_usage_bytes", "Memory usage in bytes", types.Gauge, CommonLabels)

	// TickRateGauge represents the current server tick rate
	TickRateGauge = types.NewMetric("assetto_server_tick_rate", "Server tick rate", types.Gauge, CommonLabels)

	// ServerCPUUsage represents the current CPU usage of the server
	ServerCPUUsage = types.NewMetric("assetto_server_cpu_usage", "Server CPU usage percentage", types.Gauge, CommonLabels)

	// SystemCPUUsage represents the system CPU usage
	SystemCPUUsage = types.NewMetric("assetto_system_cpu_usage", "System CPU usage percentage", types.Gauge, append(CommonLabels, "cpu"))

	// ServerMemoryUsage represents the current memory usage of the server
	ServerMemoryUsage = types.NewMetric("assetto_server_memory_usage_bytes", "Server memory usage in bytes", types.Gauge, CommonLabels)

	// SystemMemoryTotal represents the total system memory
	SystemMemoryTotal = types.NewMetric("assetto_system_memory_total_bytes", "Total system memory in bytes", types.Gauge, CommonLabels)

	// SystemMemoryUsed represents the used system memory
	SystemMemoryUsed = types.NewMetric("assetto_system_memory_used_bytes", "Used system memory in bytes", types.Gauge, CommonLabels)

	// SystemMemoryFree represents the free system memory
	SystemMemoryFree = types.NewMetric("assetto_system_memory_free_bytes", "Free system memory in bytes", types.Gauge, CommonLabels)

	// SystemMemorySwap represents the swap memory usage
	SystemMemorySwap = types.NewMetric("assetto_system_memory_swap_bytes", "Swap memory usage in bytes", types.Gauge, CommonLabels)

	// SystemDiskUsage represents the disk usage
	SystemDiskUsage = types.NewMetric("assetto_system_disk_usage_bytes", "Disk usage in bytes", types.Gauge, append(CommonLabels, "path"))

	// SystemDiskFree represents the free disk space
	SystemDiskFree = types.NewMetric("assetto_system_disk_free_bytes", "Free disk space in bytes", types.Gauge, append(CommonLabels, "path"))

	// SystemGoMemoryAlloc represents the Go memory allocations
	SystemGoMemoryAlloc = types.NewMetric("assetto_system_go_memory_alloc_bytes", "Go memory allocations in bytes", types.Gauge, CommonLabels)

	// SystemGoMemorySys represents the Go memory system
	SystemGoMemorySys = types.NewMetric("assetto_system_go_memory_sys_bytes", "Go memory system in bytes", types.Gauge, CommonLabels)

	// SystemGoRoutines represents the number of goroutines
	SystemGoRoutines = types.NewMetric("assetto_system_go_routines", "Number of goroutines", types.Gauge, CommonLabels)
)

// Network metrics
var (
	// NetworkLatency represents the network latency
	NetworkLatency = types.NewMetric("assetto_server_network_latency_ms", "Network latency in milliseconds", types.Gauge, CommonLabels)

	// NetworkPacketLoss represents the network packet loss
	NetworkPacketLoss = types.NewMetric("assetto_server_network_packet_loss", "Network packet loss percentage", types.Gauge, CommonLabels)

	// NetworkBytesReceived represents the total bytes received
	NetworkBytesReceived = types.NewMetric("assetto_server_network_bytes_received_total", "Total bytes received", types.Counter, append(CommonLabels, "interface"))

	// NetworkBytesSent represents the total bytes sent
	NetworkBytesSent = types.NewMetric("assetto_server_network_bytes_sent_total", "Total bytes sent", types.Counter, append(CommonLabels, "interface"))

	// NetworkBytesReceivedCounter counts the number of bytes received
	NetworkBytesReceivedCounter = types.NewMetric("assetto_server_network_bytes_received_total", "Total number of bytes received", types.Counter, CommonLabels)

	// NetworkBytesSentCounter counts the number of bytes sent
	NetworkBytesSentCounter = types.NewMetric("assetto_server_network_bytes_sent_total", "Total number of bytes sent", types.Counter, CommonLabels)

	// NetworkPacketsReceived represents the total packets received
	NetworkPacketsReceived = types.NewMetric("assetto_server_network_packets_received_total", "Total packets received", types.Counter, append(CommonLabels, "interface"))

	// NetworkPacketsSent represents the total packets sent
	NetworkPacketsSent = types.NewMetric("assetto_server_network_packets_sent_total", "Total packets sent", types.Counter, append(CommonLabels, "interface"))

	// NetworkErrors represents the total network errors
	NetworkErrors = types.NewMetric("assetto_server_network_errors_total", "Total network errors", types.Counter, append(CommonLabels, "interface"))

	// NetworkDrops represents the total network drops
	NetworkDrops = types.NewMetric("assetto_server_network_drops_total", "Total network drops", types.Counter, append(CommonLabels, "interface"))
)

// Authentication metrics
var (
	// AuthSuccessCounter counts the number of successful authentications
	AuthSuccessCounter = types.NewMetric("assetto_server_auth_success_total", "Total number of successful authentications", types.Counter, CommonLabels)

	// ServerInviteCounter counts the number of server invites
	ServerInviteCounter = types.NewMetric("assetto_server_invites_total", "Total number of server invites", types.Counter, append(CommonLabels, "url_hash"))

	// ServerPortsGauge represents the current server ports
	ServerPortsGauge = types.NewMetric("assetto_server_ports", "Current server ports", types.Gauge, append(CommonLabels, "port_type", "port"))

	// LobbyRegistrationCounter counts the number of lobby registrations
	LobbyRegistrationCounter = types.NewMetric("assetto_server_lobby_registrations_total", "Total number of lobby registrations", types.Counter, CommonLabels)

	// LobbyRegistrationStatusCounter counts the number of lobby registrations by status
	LobbyRegistrationStatusCounter = types.NewMetric("assetto_server_lobby_registration_status_total", "Total number of lobby registrations by status", types.Counter, append(CommonLabels, "status"))

	// ConnectionAttemptsCounter counts the number of connection attempts
	ConnectionAttemptsCounter = types.NewMetric("assetto_server_connection_attempts_total", "Total number of connection attempts", types.Counter, append(CommonLabels, "player_id", "player_name"))

	// ConnectionStatusCounter counts the number of connection status changes
	ConnectionStatusCounter = types.NewMetric("assetto_server_connection_status_total", "Total number of connection status changes", types.Counter, append(CommonLabels, "status", "player_id", "player_name"))

	// CSPVersionGauge represents the CSP version of a player
	CSPVersionGauge = types.NewMetric("assetto_server_csp_version", "CSP version of a player", types.Gauge, append(CommonLabels, "player_id", "player_name"))
)

// Chat metrics
var (
	// ChatMessagesCounter counts the number of chat messages
	ChatMessagesCounter = types.NewMetric("assetto_server_chat_messages_total", "Total number of chat messages", types.Counter, CommonLabels)

	// ChatMessagesByPlayerCounter counts the number of chat messages by player
	ChatMessagesByPlayerCounter = types.NewMetric("assetto_server_chat_messages_by_player_total", "Total number of chat messages by player", types.Counter, append(CommonLabels, "player_id", "player_name"))

	// ChatMessagesBySessionCounter counts the number of chat messages by session
	ChatMessagesBySessionCounter = types.NewMetric("assetto_server_chat_messages_by_session_total", "Total number of chat messages by session", types.Counter, append(CommonLabels, "session_id", "session_type"))

	// ChatMessagesByTypeCounter counts the number of chat messages by type
	ChatMessagesByTypeCounter = types.NewMetric("assetto_server_chat_messages_by_type_total", "Total number of chat messages by type", types.Counter, append(CommonLabels, "message_type", "content_hash"))

	// ChatMessageLengthHistogram represents the distribution of chat message lengths
	ChatMessageLengthHistogram = types.NewMetric("assetto_server_chat_message_length", "Distribution of chat message lengths", types.Histogram, append(CommonLabels, "message_type"))
)

// Collision metrics
var (
	// CollisionCounter counts the number of collisions
	CollisionCounter = types.NewMetric("assetto_server_collisions_total", "Total number of collisions", types.Counter, append(CommonLabels, "car1", "car2", "speed", "force"))

	// IncidentCounter counts the number of incidents
	IncidentCounter = types.NewMetric("assetto_server_incidents_total", "Total number of incidents", types.Counter, append(CommonLabels, "type", "severity"))
)

// Performance metrics
var (
	// ServerUpdateRateGauge represents the current server update rate
	ServerUpdateRateGauge = types.NewMetric("assetto_server_update_rate", "Current server update rate", types.Gauge, CommonLabels)
)

// Lap metrics
var (
	// LapTimeHistogram represents the distribution of lap times
	LapTimeHistogram = types.NewMetric("assetto_server_lap_time_seconds", "Distribution of lap times in seconds", types.Histogram, append(CommonLabels, "player_id", "player_name", "car_model", "track"))
)

// Helper functions for creating metrics
func CreateServerMetric(name string, help string, metricType types.MetricType) types.Metric {
	return types.NewMetric(name, help, metricType, CommonLabels)
}

func CreatePlayerMetric(name string, help string, metricType types.MetricType) types.Metric {
	return types.NewMetric(name, help, metricType, append(CommonLabels, "player_id", "player_name"))
}

func CreateSessionMetric(name string, help string, metricType types.MetricType) types.Metric {
	return types.NewMetric(name, help, metricType, append(CommonLabels, "session_id", "session_type"))
}

func CreateTrackMetric(name string, help string, metricType types.MetricType) types.Metric {
	return types.NewMetric(name, help, metricType, append(CommonLabels, "track", "layout"))
}

func CreateCarMetric(name string, help string, metricType types.MetricType) types.Metric {
	return types.NewMetric(name, help, metricType, append(CommonLabels, "car_model", "car_name"))
}

func CreateNetworkMetric(name string, help string, metricType types.MetricType) types.Metric {
	return types.NewMetric(name, help, metricType, append(CommonLabels, "interface"))
}

func CreateSystemMetric(name string, help string, metricType types.MetricType) types.Metric {
	return types.NewMetric(name, help, metricType, CommonLabels)
}

func CreateMetricsSystemMetric(name string, help string, metricType types.MetricType) types.Metric {
	return types.NewMetric(name, help, metricType, CommonLabels)
}

func CreateCollisionMetric(name string, help string, metricType types.MetricType) types.Metric {
	return types.NewMetric(name, help, metricType, append(CommonLabels, "car1", "car2"))
}

func CreateChatMetric(name string, help string, metricType types.MetricType) types.Metric {
	return types.NewMetric(name, help, metricType, append(CommonLabels, "player_id", "player_name", "message_type"))
}

// Helper functions for working with metrics
func WithLabels(m types.Metric, labels map[string]string) types.Metric {
	return m.With(labels)
}

func SetValue(m types.Metric, value float64) types.Metric {
	return m.Set(value)
}

func IncrementCounter(m types.Metric) types.Metric {
	return m.Inc()
}

func AddToCounter(m types.Metric, value float64) types.Metric {
	return m.Add(value)
}

func ObserveHistogram(m types.Metric, value float64) types.Metric {
	return m.Observe(value)
}
