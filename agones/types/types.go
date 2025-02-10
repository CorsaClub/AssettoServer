// Package types contains definitions of structures used by the server.
package types

import (
	"sync"
	"time"
)

// ServerState represents the current state of the Assetto Corsa server.
type ServerState struct {
	sync.RWMutex
	Ready            bool               // Indicates if the server is ready to accept connections
	Players          int                // Current number of connected players
	LastPing         time.Time          // Timestamp of the last successful health check
	Allocated        bool               // Indicates if the server is currently allocated
	ServerID         string             // Unique identifier of the server
	ServerName       string             // Name of the server
	ServerType       string             // Type of the server
	SessionType      string             // Type of the current session
	SessionStart     time.Time          // Start time of the session
	SessionTimeLeft  int                // Time left in the session (seconds)
	CurrentTrack     string             // Current track name
	CurrentLayout    string             // Current track layout
	TrackTemp        float64            // Track temperature
	AirTemp          float64            // Air temperature
	TrackGrip        float64            // Track grip level
	ConnectedPlayers map[string]*Player // Map of connected players
	ActiveCars       map[string]int     // Map of active cars
	TickRate         float64            // Current tick rate
	CurrentSession   *Session           // Current active session
	ShuttingDown     bool               // Indicates if the server is shutting down
}

// Player represents a player connected to the server.
type Player struct {
	Name       string  // Player's name
	SteamID    string  // Player's Steam ID
	CarModel   string  // Car model used by the player
	BestLap    int64   // Player's best lap time (ms)
	LastLap    int64   // Player's latest lap time (ms)
	Latency    int     // Player's latency (ms)
	PacketLoss float64 // Player's packet loss percentage
}

// Session represents a game session.
type Session struct {
	Type          string
	StartTime     time.Time
	EndTime       time.Time
	Track         string
	ID            string
	RemainingTime string
}

// TrackConditions represents the conditions of the track.
type TrackConditions struct {
	GripLevel   float64 // Grip level percentage
	Temperature float64 // Track temperature in Celsius
	Weather     string  // Weather conditions
	TimeOfDay   string  // Time of day
}

// Constants for server states.
const (
	ServerStateStarting  = 0 // Server is starting
	ServerStateReady     = 1 // Server is ready
	ServerStateAllocated = 2 // Server is allocated
	ServerStateReserved  = 3 // Server is reserved
	ServerStateShutdown  = 4 // Server is shutting down
)

// Constants for session types.
const (
	SessionTypePractice   = "practice"   // Practice session
	SessionTypeQualifying = "qualifying" // Qualifying session
	SessionTypeRace       = "race"       // Race session
	SessionTypeUnknown    = "unknown"    // Unknown session type
)

// Config provides flexible configuration options for the server.
type Config struct {
	ServerScript    string        `json:"server_script"`     // Path to the server script
	ServerArgs      string        `json:"server_args"`       // Arguments for the server script
	ShutdownTimeout time.Duration `json:"shutdown_timeout"`  // Timeout for server shutdown
	ReserveDuration time.Duration `json:"reserve_duration"`  // Duration to reserve the server
	HealthCheckRate time.Duration `json:"health_check_rate"` // Rate for health checks
	MetricsPort     int           `json:"metrics_port"`      // Port for exposing metrics
	HealthPort      int           `json:"health_port"`       // Port for health checks
	Debug           bool          `json:"debug"`             // Enable debug mode
}

// LogEvent represents a structured log event with contextual information.
type LogEvent struct {
	Timestamp   time.Time `json:"timestamp"`       // Time of the event
	Level       string    `json:"level"`           // Log level (e.g., INFO, ERROR)
	Event       string    `json:"event"`           // Event type
	ServerID    string    `json:"server_id"`       // ID of the server
	ServerName  string    `json:"server_name"`     // Name of the server
	Message     string    `json:"message"`         // Descriptive message
	Players     int       `json:"players"`         // Number of players
	SessionType string    `json:"session_type"`    // Type of session
	Error       string    `json:"error,omitempty"` // Error message, if any
}

// GameServerSDK defines the interface for interacting with the game server.
type GameServerSDK interface {
	Health() error                                      // Perform a health check
	Ready() error                                       // Mark the server as ready
	Shutdown() error                                    // Shutdown the server
	SetLabel(key, value string) error                   // Set a label on the server
	SetAnnotation(key, value string) error              // Set an annotation on the server
	GameServer() (interface{}, error)                   // Retrieve the game server information
	Reserve(seconds int64) error                        // Reserve the server for a duration
	Allocate() error                                    // Allocate the server
	WatchGameServer(func(gameServer interface{})) error // Watch for game server updates
	Alpha() interface{}                                 // Access to experimental features
	Connect() error                                     // Connect to the SDK
	Close() error                                       // Close the SDK connection
}
