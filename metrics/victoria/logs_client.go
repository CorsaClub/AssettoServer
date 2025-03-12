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
	"time"

	"metrics/config"
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

	// Convert logs to JSON lines format (ndjson)
	var buf bytes.Buffer
	for _, log := range logs {
		// Create a map for the log entry
		logMap := map[string]interface{}{
			"_time":    log.Timestamp.Format(time.RFC3339),
			"_msg":     log.Message,
			"level":    log.Level,
			"source":   log.Source,
			"log_type": log.LogType,
		}

		// Add labels as individual fields
		for k, v := range log.Labels {
			logMap[k] = v
		}

		// Encode as JSON
		if err := json.NewEncoder(&buf).Encode(logMap); err != nil {
			return fmt.Errorf("error encoding log: %w", err)
		}
	}

	// Compress if enabled
	var body io.Reader = &buf
	var contentType string = "application/json"
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
		contentType = "application/json+gzip"
	}

	// Create request with the correct endpoint for JSON Stream API
	req, err := http.NewRequest("POST", c.URL+"/insert/jsonline", body)
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", contentType)

	// Set query parameters for JSON Stream API
	q := req.URL.Query()
	q.Add("_stream_fields", "log_type,source")
	q.Add("_time_field", "_time")
	q.Add("_msg_field", "_msg")
	req.URL.RawQuery = q.Encode()

	// Set authentication if provided
	if c.Username != "" && c.Password != "" {
		req.SetBasicAuth(c.Username, c.Password)
	}

	// Send request
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("error sending logs: %w", err)
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("error from VictoriaLogs: %s - %s", resp.Status, string(bodyBytes))
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
	// Create a test log
	testLog := types.Log{
		Timestamp: time.Now(),
		Level:     "info",
		Message:   "Connection test",
		Source:    "wrapper",
		LogType:   string(LogTypeWrapper),
		Labels: map[string]string{
			"test": "true",
		},
	}

	// Send the test log
	if err := c.SendLogs([]types.Log{testLog}); err != nil {
		return fmt.Errorf("error testing connection to VictoriaLogs: %w", err)
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
