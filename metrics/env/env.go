// Package env provides utilities for managing environment variables
// used throughout the application.
package env

import (
	"fmt"
	"os"
	"sync"

	"metrics/config"
)

// Variables structure holds all environment variables used by the application
type Variables struct {
	// Authentication
	AuthSteamID string
	AuthUserID  string

	// Server identification
	GameServerID     string
	GameServerRegion string
	ServerName       string
	ServerType       string

	// VictoriaMetrics configuration
	VictoriaMetricsURL      string
	VictoriaMetricsPort     string
	VictoriaMetricsUsername string
	VictoriaMetricsPassword string

	// VictoriaLogs configuration
	VictoriaLogsURL      string
	VictoriaLogsPort     string
	VictoriaLogsUsername string
	VictoriaLogsPassword string

	// Logging configuration
	EnableConsoleLogs bool
	DisableServerLogs bool
	DebugLogs         bool
	DebugMetrics      bool

	// GeoIP configuration
	GeoIPEnabled      bool
	GeoIPDatabasePath string

	// Test mode
	TestMode bool
}

var (
	// instance is the singleton instance of Variables
	instance *Variables
	// once ensures the singleton is initialized only once
	once sync.Once
)

// GetEnv returns the singleton instance of Variables
func GetEnv() *Variables {
	once.Do(func() {
		instance = &Variables{}
		instance.initialize()
	})
	return instance
}

// initialize loads all environment variables
func (v *Variables) initialize() {
	// Authentication
	v.AuthSteamID = os.Getenv("AUTH_STEAM_ID")
	v.AuthUserID = os.Getenv("AUTH_USER_ID")

	// Server identification
	v.GameServerID = os.Getenv("GAMESERVER_ID")
	v.GameServerRegion = os.Getenv("GAMESERVER_REGION")
	v.ServerName = os.Getenv("SERVER_NAME")
	v.ServerType = os.Getenv("SERVER_TYPE")

	// VictoriaMetrics configuration
	v.VictoriaMetricsURL = os.Getenv("VICTORIA_METRICS_URL")
	v.VictoriaMetricsPort = os.Getenv("VICTORIA_METRICS_PORT")
	v.VictoriaMetricsUsername = os.Getenv("VICTORIA_METRICS_USERNAME")
	v.VictoriaMetricsPassword = os.Getenv("VICTORIA_METRICS_PASSWORD")

	// VictoriaLogs configuration
	v.VictoriaLogsURL = os.Getenv("VICTORIA_LOGS_URL")
	v.VictoriaLogsPort = os.Getenv("VICTORIA_LOGS_PORT")
	v.VictoriaLogsUsername = os.Getenv("VICTORIA_LOGS_USERNAME")
	v.VictoriaLogsPassword = os.Getenv("VICTORIA_LOGS_PASSWORD")

	// Logging configuration
	v.EnableConsoleLogs = os.Getenv("ENABLE_CONSOLE_LOGS") == "true"
	v.DisableServerLogs = os.Getenv("DISABLE_SERVER_LOGS") == "true"
	v.DebugLogs = os.Getenv("DEBUG_LOGS") == "true"
	v.DebugMetrics = os.Getenv("DEBUG_METRICS") == "true"

	// GeoIP configuration
	v.GeoIPEnabled = os.Getenv("GEOIP_ENABLED") == "true"
	v.GeoIPDatabasePath = os.Getenv("GEOIP_DATABASE_PATH")

	// Test mode
	v.TestMode = os.Getenv("TEST_MODE") == "true"

	// Set legacy environment variables for backward compatibility
	v.setLegacyVariables()
}

// setLegacyVariables sets environment variables for backward compatibility
func (v *Variables) setLegacyVariables() {
	// Set VICTORIA_URL and VICTORIA_PORT for backward compatibility
	if v.VictoriaMetricsURL != "" {
		os.Setenv("VICTORIA_URL", v.VictoriaMetricsURL)
	}
	if v.VictoriaMetricsPort != "" {
		os.Setenv("VICTORIA_PORT", v.VictoriaMetricsPort)
	}
	if v.VictoriaMetricsUsername != "" {
		os.Setenv("VICTORIA_USERNAME", v.VictoriaMetricsUsername)
	}
	if v.VictoriaMetricsPassword != "" {
		os.Setenv("VICTORIA_PASSWORD", v.VictoriaMetricsPassword)
	}

	// Test mode
	if v.TestMode {
		os.Setenv("TEST_MODE", "true")
	}

	// Enable debug logs if needed
	if v.DebugLogs {
		os.Setenv("DEBUG_LOGS", "true")
	}
	if v.DebugMetrics {
		os.Setenv("DEBUG_METRICS", "true")
	}
}

// LogEnvironmentVariables logs all environment variables for debugging
func (v *Variables) LogEnvironmentVariables() {
	fmt.Println("=== Environment Variables ===")

	// Authentication
	fmt.Printf("AUTH_STEAM_ID: %s\n", maskIfNotEmpty(v.AuthSteamID))
	fmt.Printf("AUTH_USER_ID: %s\n", maskIfNotEmpty(v.AuthUserID))

	// Server identification
	fmt.Printf("GAMESERVER_ID: %s\n", v.GameServerID)
	fmt.Printf("GAMESERVER_REGION: %s\n", v.GameServerRegion)
	fmt.Printf("SERVER_NAME: %s\n", v.ServerName)
	fmt.Printf("SERVER_TYPE: %s\n", v.ServerType)

	// VictoriaMetrics configuration
	fmt.Printf("VICTORIA_METRICS_URL: %s\n", v.VictoriaMetricsURL)
	fmt.Printf("VICTORIA_METRICS_PORT: %s\n", v.VictoriaMetricsPort)
	fmt.Printf("VICTORIA_METRICS_USERNAME: %s\n", maskIfNotEmpty(v.VictoriaMetricsUsername))
	fmt.Printf("VICTORIA_METRICS_PASSWORD: %s\n", maskIfPresent(v.VictoriaMetricsPassword))

	// VictoriaLogs configuration
	fmt.Printf("VICTORIA_LOGS_URL: %s\n", v.VictoriaLogsURL)
	fmt.Printf("VICTORIA_LOGS_PORT: %s\n", v.VictoriaLogsPort)
	fmt.Printf("VICTORIA_LOGS_USERNAME: %s\n", maskIfNotEmpty(v.VictoriaLogsUsername))
	fmt.Printf("VICTORIA_LOGS_PASSWORD: %s\n", maskIfPresent(v.VictoriaLogsPassword))

	// Logging configuration
	fmt.Printf("ENABLE_CONSOLE_LOGS: %v\n", v.EnableConsoleLogs)
	fmt.Printf("DISABLE_SERVER_LOGS: %v\n", v.DisableServerLogs)
	fmt.Printf("DEBUG_LOGS: %v\n", v.DebugLogs)
	fmt.Printf("DEBUG_METRICS: %v\n", v.DebugMetrics)

	// GeoIP configuration
	fmt.Printf("GEOIP_ENABLED: %v\n", v.GeoIPEnabled)
	fmt.Printf("GEOIP_DATABASE_PATH: %s\n", v.GeoIPDatabasePath)

	// Test mode
	fmt.Printf("TEST_MODE: %v\n", v.TestMode)

	fmt.Println("============================")
}

// Helper functions for masking sensitive information
func maskIfNotEmpty(value string) string {
	if value != "" {
		return "[REDACTED]"
	}
	return ""
}

func maskIfPresent(value string) string {
	if value != "" {
		return "[REDACTED]"
	}
	return ""
}

// GetVictoriaMetricsURL returns the full URL for VictoriaMetrics
func (v *Variables) GetVictoriaMetricsURL() string {
	if v.VictoriaMetricsURL == "" {
		return ""
	}

	port := v.VictoriaMetricsPort
	if port == "" {
		port = "8428" // Default port
	}

	return fmt.Sprintf("http://%s:%s", v.VictoriaMetricsURL, port)
}

// GetVictoriaLogsURL returns the full URL for VictoriaLogs
func (v *Variables) GetVictoriaLogsURL() string {
	if v.VictoriaLogsURL == "" {
		return ""
	}

	port := v.VictoriaLogsPort
	if port == "" {
		port = "9428" // Default port
	}

	return fmt.Sprintf("http://%s:%s", v.VictoriaLogsURL, port)
}

// NewAuthConfig is for websocket authentication
func (v *Variables) GetAuthConfig() *config.AuthConfig {
	return &config.AuthConfig{
		SteamID: v.AuthSteamID,
		UserID:  v.AuthUserID,
	}
}
