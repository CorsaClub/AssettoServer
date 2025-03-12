package metrics

import (
	"fmt"
	"os"
	"strings"
	"time"

	"metrics/config"
	"metrics/geoip"
	"metrics/types"
	"metrics/victoria"
)

// InitGeoIPService initializes the GeoIP service if enabled
func InitGeoIPService(geoipConfig config.GeoIPConfig, logsClient victoria.LogsClient) *geoip.GeoIPService {
	// Check environment variable to override default config
	if geoipEnabled := os.Getenv("GEOIP_ENABLED"); geoipEnabled == "true" {
		geoipConfig.Enabled = true
		logsClient.LogEvent("INFO", "GeoIP service enabled via environment variable", "geoip", nil)
	}

	// Check for database path override
	if dbPath := os.Getenv("GEOIP_DATABASE_PATH"); dbPath != "" {
		geoipConfig.DatabasePath = dbPath
		logsClient.LogEvent("INFO", fmt.Sprintf("Using GeoIP database path from environment: %s", dbPath), "geoip", nil)
	}

	if !geoipConfig.Enabled {
		logsClient.LogEvent("INFO", "GeoIP service is disabled", "geoip", nil)
		return nil
	}

	logsClient.LogEvent("INFO", fmt.Sprintf("Initializing GeoIP service with database: %s", geoipConfig.DatabasePath), "geoip", nil)
	geoipService, err := geoip.InitGeoIPService(&geoipConfig)
	if err != nil {
		logsClient.LogEvent("WARNING", fmt.Sprintf("Failed to initialize GeoIP service: %v", err), "geoip", nil)
		return nil
	}

	logsClient.LogEvent("INFO", "GeoIP service initialized successfully", "geoip", nil)
	return geoipService
}

// TestVictoriaMetricsConnection tests the connection to VictoriaMetrics
func TestVictoriaMetricsConnection(client *victoria.MetricsClient, serverID string, logsClient victoria.LogsClient) error {
	// Create a test metric
	testMetric := types.MetricBatch{
		Metrics: []types.Metric{
			{
				Name:      "assetto_server_test",
				Value:     1.0,
				Timestamp: time.Now(),
				LabelValues: map[string]string{
					"server_id": serverID,
					"test":      "true",
				},
			},
		},
	}

	// Send the test metric
	err := client.SendMetricsImmediate(testMetric)
	if err != nil {
		logsClient.LogEvent("WARNING", fmt.Sprintf("Failed to connect to VictoriaMetrics: %v", err), "metrics_test", nil)
		logsClient.LogEvent("INFO", "Metrics will be buffered and retried later", "metrics_test", nil)
		return err
	}

	logsClient.LogEvent("INFO", "Successfully connected to VictoriaMetrics", "metrics_test", nil)
	return nil
}

// TestVictoriaLogsConnection tests the connection to VictoriaLogs
func TestVictoriaLogsConnection(client victoria.LogsClient) error {
	// Test connection with a simple event
	err := client.LogEvent("INFO", "Testing connection to VictoriaLogs", "test_connection", map[string]string{
		"test": "true",
	})

	if err != nil {
		client.LogEvent("WARNING", fmt.Sprintf("Failed to connect to VictoriaLogs: %v", err), "test_connection", nil)
		client.LogEvent("INFO", "Logs will be buffered and retried later", "test_connection", nil)

		// Try to determine the cause of the error
		if strings.Contains(err.Error(), "unsupported path") || strings.Contains(err.Error(), "404") {
			client.LogEvent("INFO", "VictoriaLogs API endpoint may be incorrect", "test_connection", nil)
		} else if strings.Contains(err.Error(), "connection refused") {
			client.LogEvent("INFO", "Connection to VictoriaLogs was refused", "test_connection", nil)
		} else if strings.Contains(err.Error(), "no such host") {
			client.LogEvent("INFO", "VictoriaLogs host could not be resolved", "test_connection", nil)
		} else if strings.Contains(err.Error(), "timeout") {
			client.LogEvent("INFO", "Connection to VictoriaLogs timed out", "test_connection", nil)
		}

		return err
	}

	client.LogEvent("INFO", "Successfully connected to VictoriaLogs", "test_connection", nil)
	return nil
}
