package metrics

import (
	"time"
)

// LogEntry représente une entrée de log structurée
type LogEntry struct {
	Timestamp  time.Time         `json:"timestamp"`
	Level      string            `json:"level"`
	Message    string            `json:"message"`
	Labels     map[string]string `json:"labels"`
	ServerID   string            `json:"server_id"`
	SessionID  string            `json:"session_id"`
	PlayerID   string            `json:"player_id,omitempty"`
	PlayerName string            `json:"player_name,omitempty"`
	EventType  string            `json:"event_type,omitempty"`
	Error      string            `json:"error,omitempty"`
}

// LogLevel définit les niveaux de log
const (
	LogLevelInfo    = "info"
	LogLevelWarning = "warning"
	LogLevelError   = "error"
	LogLevelDebug   = "debug"
)

// EventType définit les types d'événements importants
const (
	EventServerStart      = "server_start"
	EventServerStop       = "server_stop"
	EventPlayerConnect    = "player_connect"
	EventPlayerDisconnect = "player_disconnect"
	EventSessionChange    = "session_change"
	EventError            = "error"
	EventHealthCheck      = "health_check"
	EventCSPHandshake     = "csp_handshake"
)
