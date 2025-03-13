// Package victoria provides clients for interacting with VictoriaMetrics and VictoriaLogs.
// This file implements a JSON Stream format client for VictoriaLogs.

package victoria

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"metrics/config"
	"metrics/env"
	"metrics/types"
)

// LogType définit les différents types de logs
type LogType string

const (
	// LogTypeServer représente les logs originaux d'AssettoServer
	LogTypeServer LogType = "server"
	// LogTypeWrapper représente les logs générés par le wrapper metrics
	LogTypeWrapper LogType = "wrapper"
	// LogTypeChat représente les messages du chat d'AssettoServer
	LogTypeChat LogType = "chat"
)

// LogsClient is the client for sending logs to VictoriaLogs
type LogsClient struct {
	URL         string
	Username    string
	Password    string
	client      *http.Client
	config      config.VictoriaLogsConfig
	Compression bool
	Timeout     time.Duration
}

// NewLogsClient creates a new VictoriaLogs client
func NewLogsClient(cfg *config.VictoriaLogsConfig) LogsClient {
	client := &http.Client{
		Timeout: cfg.Timeout,
	}

	return LogsClient{
		URL:         cfg.URL,
		Username:    cfg.Username,
		Password:    cfg.Password,
		client:      client,
		config:      *cfg,
		Compression: cfg.Compression,
		Timeout:     cfg.Timeout,
	}
}

// SendLogs sends logs to VictoriaLogs using the JSON Stream API
func (c LogsClient) SendLogs(logs []types.Log) error {
	if len(logs) == 0 {
		return nil
	}

	envVars := env.GetEnv()

	// Get server ID from environment or generate one
	serverID := envVars.GameServerID
	if serverID == "" {
		serverID = "unknown"
	}

	// Log debug information
	if envVars.DebugLogs {
		fmt.Printf("[DEBUG] Sending %d logs to VictoriaLogs at %s\n", len(logs), c.URL)
	}

	// Create a buffer to store JSON lines
	var buf bytes.Buffer

	// Convert each log to JSON line format according to VictoriaLogs JSON Stream API
	// https://docs.victoriametrics.com/victorialogs/data-ingestion/#json-stream-api
	for _, log := range logs {
		// Create the log entry
		entry := map[string]interface{}{
			"date": log.Timestamp.Format(time.RFC3339Nano),
			"log": map[string]interface{}{
				"level":   log.Level,
				"message": log.Message,
			},
			"source": log.Source,
			"type":   log.LogType,
			"server": serverID,
			"game":   "ac",
			"stream": fmt.Sprintf("%s-%s", log.Source, serverID),
		}

		// Add any additional labels
		for k, v := range log.Labels {
			// Avoid overwriting existing fields
			if k != "log" && k != "date" && k != "stream" && k != "source" && k != "type" && k != "server" && k != "game" {
				entry[k] = v
			}
		}

		// Marshal to JSON
		jsonData, err := json.Marshal(entry)
		if err != nil {
			continue
		}

		// Write JSON line
		buf.Write(jsonData)
		buf.WriteString("\n")
	}

	// Prepare request body
	var body io.Reader = &buf
	contentType := "application/stream+json"

	if c.Compression {
		var compressedBuf bytes.Buffer
		gzipWriter := gzip.NewWriter(&compressedBuf)
		if _, err := io.Copy(gzipWriter, &buf); err != nil {
			return fmt.Errorf("error compressing logs: %w", err)
		}
		if err := gzipWriter.Close(); err != nil {
			return fmt.Errorf("error closing gzip writer: %w", err)
		}
		body = &compressedBuf
		contentType = "application/stream+json+gzip"
	}

	// Create request with the correct endpoint for VictoriaLogs JSON Stream API
	url := fmt.Sprintf("%s/insert/jsonline?_msg_field=log.message&_time_field=date&_stream_fields=stream", c.URL)
	if envVars.DebugLogs {
		fmt.Printf("[DEBUG] Using endpoint for logs: %s\n", url)

		// Log a sample of the data being sent
		if buf.Len() > 0 {
			sample := buf.String()
			if len(sample) > 500 {
				sample = sample[:500] + "..." // Truncate to avoid too long logs
			}
			fmt.Printf("[DEBUG] Sample log data: %s\n", sample)
		}
	}

	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", contentType)

	// Set authentication if provided
	if c.Username != "" && c.Password != "" {
		req.SetBasicAuth(c.Username, c.Password)
	}

	// Send request
	resp, err := c.client.Do(req)
	if err != nil {
		if envVars.DebugLogs {
			fmt.Printf("[DEBUG] Error sending logs to VictoriaLogs: %v\n", err)
		}
		return fmt.Errorf("error sending logs: %w", err)
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		bodyBytes, _ := io.ReadAll(resp.Body)
		if envVars.DebugLogs {
			fmt.Printf("[DEBUG] VictoriaLogs returned status %d: %s\n", resp.StatusCode, string(bodyBytes))
		}
		return fmt.Errorf("error from VictoriaLogs: %s - %s", resp.Status, string(bodyBytes))
	}

	if envVars.DebugLogs {
		fmt.Printf("[DEBUG] Successfully sent %d logs to VictoriaLogs\n", len(logs))
	}

	return nil
}

// LogEvent sends a single log event to VictoriaLogs
func (c LogsClient) LogEvent(level, message string, eventType string, labels map[string]string) error {
	// Create a log entry
	log := types.Log{
		Timestamp: time.Now(),
		Level:     level,
		Message:   message,
		Source:    "wrapper",
		LogType:   string(LogTypeWrapper),
		Labels:    make(map[string]string),
	}

	// Add labels
	if labels != nil {
		for k, v := range labels {
			log.Labels[k] = v
		}
	}

	// Add event type if provided
	if eventType != "" {
		log.Labels["event_type"] = eventType
	}

	// Send the log
	return c.SendLogs([]types.Log{log})
}

// LogServerEvent send a log from AssettoServer
func (c LogsClient) LogServerEvent(level, message string, eventType string, labels map[string]string) error {
	// Create a log entry
	log := types.Log{
		Timestamp: time.Now(),
		Level:     level,
		Message:   message,
		Source:    "assetto_server",
		LogType:   string(LogTypeServer),
		Labels:    make(map[string]string),
	}

	// Add labels
	if labels != nil {
		for k, v := range labels {
			log.Labels[k] = v
		}
	}

	// Add event type if provided
	if eventType != "" {
		log.Labels["event_type"] = eventType
	}

	// Send the log
	return c.SendLogs([]types.Log{log})
}

// LogChatMessage send a chat message
func (c LogsClient) LogChatMessage(playerName, message string, labels map[string]string) error {
	// Create a log entry
	log := types.Log{
		Timestamp: time.Now(),
		Level:     "INFO",
		Message:   message,
		Source:    "chat",
		LogType:   string(LogTypeChat),
		Labels:    make(map[string]string),
	}

	// Add player name
	log.Labels["player_name"] = playerName

	// Add additional labels
	if labels != nil {
		for k, v := range labels {
			log.Labels[k] = v
		}
	}

	// Send the log
	return c.SendLogs([]types.Log{log})
}

// TestConnection tests the connection to VictoriaLogs
func (c LogsClient) TestConnection() error {
	// Create test logs for each type
	testLogs := []types.Log{
		{
			Timestamp: time.Now(),
			Level:     "info",
			Message:   "Test log from wrapper",
			Source:    "wrapper",
			LogType:   string(LogTypeWrapper),
			Labels: map[string]string{
				"test": "true",
			},
		},
		{
			Timestamp: time.Now(),
			Level:     "info",
			Message:   "Test log from server",
			Source:    "assetto_server",
			LogType:   string(LogTypeServer),
			Labels: map[string]string{
				"test": "true",
			},
		},
		{
			Timestamp: time.Now(),
			Level:     "info",
			Message:   "Test chat message",
			Source:    "chat",
			LogType:   string(LogTypeChat),
			Labels: map[string]string{
				"test":        "true",
				"player_name": "test_player",
			},
		},
	}

	// Send test logs
	if err := c.SendLogs(testLogs); err != nil {
		// Try to determine the cause of the error
		if strings.Contains(err.Error(), "unsupported path") || strings.Contains(err.Error(), "404") {
			return fmt.Errorf("VictoriaLogs API endpoint may be incorrect (should be /insert/jsonline): %w", err)
		} else if strings.Contains(err.Error(), "connection refused") {
			return fmt.Errorf("connection to VictoriaLogs was refused, check if the service is running on %s: %w", c.URL, err)
		} else if strings.Contains(err.Error(), "no such host") {
			return fmt.Errorf("VictoriaLogs host could not be resolved (%s): %w", c.URL, err)
		} else if strings.Contains(err.Error(), "timeout") {
			return fmt.Errorf("connection to VictoriaLogs timed out (%s): %w", c.URL, err)
		}
		return fmt.Errorf("failed to send test logs to VictoriaLogs: %w", err)
	}

	return nil
}

// GetConfig returns the current configuration
func (c LogsClient) GetConfig() config.VictoriaLogsConfig {
	return c.config
}

// IsConfigured returns true if the client is configured with a valid URL
func (c LogsClient) IsConfigured() bool {
	return c.URL != ""
}
