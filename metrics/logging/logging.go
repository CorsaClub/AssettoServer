package logging

import (
	"fmt"
	"os"
	"strings"
	"time"

	"metrics/config"
	"metrics/types"
	"metrics/victoria"
)

// LogManager handles all logging operations
type LogManager struct {
	client victoria.LogsClient
}

// Client returns the underlying VictoriaLogs client
func (m *LogManager) Client() victoria.LogsClient {
	return m.client
}

// NewLogManager creates a new log manager
func NewLogManager(cfg *config.VictoriaLogsConfig) (*LogManager, error) {
	// Configure VictoriaLogs URL and credentials
	if url := os.Getenv("VICTORIA_LOGS_URL"); url != "" {
		if port := os.Getenv("VICTORIA_LOGS_PORT"); port != "" {
			cfg.URL = fmt.Sprintf("http://%s:%s", url, port)
		} else {
			cfg.URL = fmt.Sprintf("http://%s:%s", url, config.DefaultVictoriaLogsPort)
		}
		fmt.Printf("[DEBUG] VictoriaLogs URL set from environment: %s\n", cfg.URL)
	} else {
		fmt.Printf("[DEBUG] Using default VictoriaLogs URL: %s\n", cfg.URL)
	}

	// Configure credentials
	if user := os.Getenv("VICTORIA_LOGS_USERNAME"); user != "" {
		cfg.Username = user
		fmt.Println("[DEBUG] VictoriaLogs username set from environment")
	}
	if pass := os.Getenv("VICTORIA_LOGS_PASSWORD"); pass != "" {
		cfg.Password = pass
		fmt.Println("[DEBUG] VictoriaLogs password set from environment")
	}

	// Create the client
	client := victoria.NewLogsClient(cfg)

	// Create log manager
	mgr := &LogManager{
		client: client,
	}

	// Test connection
	if err := mgr.TestConnection(); err != nil {
		fmt.Printf("[ERROR] Failed to connect to VictoriaLogs: %v\n", err)
		return nil, fmt.Errorf("failed to connect to VictoriaLogs: %v", err)
	}

	fmt.Println("[INFO] Successfully connected to VictoriaLogs")
	return mgr, nil
}

// TestConnection tests the connection to VictoriaLogs
func (m *LogManager) TestConnection() error {
	// Test connection with a simple event
	err := m.client.LogEvent("INFO", "Testing connection to VictoriaLogs", "test_connection", map[string]string{
		"test": "true",
	})

	if err != nil {
		m.client.LogEvent("WARNING", fmt.Sprintf("Failed to connect to VictoriaLogs: %v", err), "test_connection", nil)
		m.client.LogEvent("INFO", "Logs will be buffered and retried later", "test_connection", nil)

		// Try to determine the cause of the error
		if strings.Contains(err.Error(), "unsupported path") || strings.Contains(err.Error(), "404") {
			m.client.LogEvent("INFO", "VictoriaLogs API endpoint may be incorrect", "test_connection", nil)
		} else if strings.Contains(err.Error(), "connection refused") {
			m.client.LogEvent("INFO", "Connection to VictoriaLogs was refused", "test_connection", nil)
		} else if strings.Contains(err.Error(), "no such host") {
			m.client.LogEvent("INFO", "VictoriaLogs host could not be resolved", "test_connection", nil)
		} else if strings.Contains(err.Error(), "timeout") {
			m.client.LogEvent("INFO", "Connection to VictoriaLogs timed out", "test_connection", nil)
		}

		return err
	}

	m.client.LogEvent("INFO", "Successfully connected to VictoriaLogs", "test_connection", nil)
	return nil
}

// LogServerOutput logs server output with appropriate level and type
func (m *LogManager) LogServerOutput(output string, state *types.ServerState) {
	str := strings.TrimSpace(output)

	// Determine log level and type
	logLevel := "INFO"
	eventType := "server_output"

	// Detect errors and warnings
	if strings.Contains(str, "ERROR") {
		logLevel = "ERROR"
		eventType = "error"
	} else if strings.Contains(str, "Warning") || strings.Contains(str, "WARNING") {
		logLevel = "WARNING"
		eventType = "warning"
	}

	// Log to VictoriaLogs
	m.client.LogServerEvent(logLevel, str, eventType, map[string]string{
		"server_id":  state.ServerID,
		"session_id": state.CurrentSession.ID,
	})
}

// LogServerError logs server errors
func (m *LogManager) LogServerError(err error, state *types.ServerState) {
	m.client.LogServerEvent("ERROR", err.Error(), "server_error", map[string]string{
		"server_id":  state.ServerID,
		"session_id": state.CurrentSession.ID,
	})
}

// LogChatMessage logs chat messages
func (m *LogManager) LogChatMessage(playerName, message string, state *types.ServerState) {
	m.client.LogChatMessage(playerName, message, map[string]string{
		"server_id":  state.ServerID,
		"session_id": state.CurrentSession.ID,
		"timestamp":  time.Now().Format(time.RFC3339),
	})
}

// LogEvent logs a general event
func (m *LogManager) LogEvent(level, message, eventType string, labels map[string]string) {
	m.client.LogEvent(level, message, eventType, labels)
}
