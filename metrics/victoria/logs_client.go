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
	"os"
	"strings"
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

// SendLogs sends logs to VictoriaLogs using the Loki API
func (c LogsClient) SendLogs(logs []types.Log) error {
	if len(logs) == 0 {
		return nil
	}

	// Get server ID from environment or generate one
	serverID := os.Getenv("GAMESERVER_ID")
	if serverID == "" {
		serverID = "unknown"
	}

	// Group logs by stream (combination of log_type and source)
	streams := make(map[string][]types.Log)
	for _, log := range logs {
		streamKey := fmt.Sprintf("%s_%s", log.LogType, log.Source)
		streams[streamKey] = append(streams[streamKey], log)
	}

	// Create Loki push request format
	request := struct {
		Streams []struct {
			Stream map[string]string `json:"stream"`
			Values [][]string        `json:"values"`
		} `json:"streams"`
	}{
		Streams: make([]struct {
			Stream map[string]string `json:"stream"`
			Values [][]string        `json:"values"`
		}, 0, len(streams)),
	}

	// Convert each stream group to Loki format
	for streamKey, streamLogs := range streams {
		stream := struct {
			Stream map[string]string `json:"stream"`
			Values [][]string        `json:"values"`
		}{
			Stream: map[string]string{
				"log_type":  strings.Split(streamKey, "_")[0],
				"source":    strings.Split(streamKey, "_")[1],
				"game":      "ac",
				"server_id": serverID,
			},
			Values: make([][]string, 0, len(streamLogs)),
		}

		// Add common labels from the first log
		if len(streamLogs) > 0 {
			for k, v := range streamLogs[0].Labels {
				stream.Stream[k] = v
			}
		}

		// Add values for each log
		for _, log := range streamLogs {
			// Format the log entry
			logEntry := map[string]interface{}{
				"level":   log.Level,
				"message": log.Message,
			}
			// Add any additional labels as fields
			for k, v := range log.Labels {
				if _, exists := stream.Stream[k]; !exists {
					logEntry[k] = v
				}
			}

			// Convert to JSON
			jsonData, err := json.Marshal(logEntry)
			if err != nil {
				continue
			}

			stream.Values = append(stream.Values, []string{
				fmt.Sprintf("%d", log.Timestamp.UnixNano()),
				string(jsonData),
			})
		}

		request.Streams = append(request.Streams, stream)
	}

	// Convert request to JSON
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(request); err != nil {
		return fmt.Errorf("error encoding request: %w", err)
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

	// Create request with the correct endpoint for Loki API
	req, err := http.NewRequest("POST", c.URL+"/insert/loki/api/v1/push", body)
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
		return fmt.Errorf("error sending logs: %w", err)
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
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
			return fmt.Errorf("VictoriaLogs API endpoint may be incorrect (should be /loki/api/v1/push): %w", err)
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
