// Package victoria provides clients for interacting with VictoriaMetrics and VictoriaLogs.
// This file implements a client for VictoriaMetrics time series database.
package victoria

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"bytes"
	"compress/gzip"
	"io"

	"metrics/config"
	"metrics/models"
	"metrics/types"
	"metrics/utils"
)

// MetricsClient is a client for sending metrics to VictoriaMetrics
type MetricsClient struct {
	URL            string
	Username       string
	Password       string
	client         *http.Client
	config         *config.Config
	buffer         chan types.MetricBatch
	batchSize      int
	errorRegistry  *ErrorRegistry
	circuitBreaker *utils.CircuitBreaker
	rateLimiter    *utils.RateLimiter
}

// MetricPoint represents a data point for VictoriaMetrics
type MetricPoint struct {
	Metric    string            `json:"metric"`
	Value     float64           `json:"value"`
	Timestamp int64             `json:"timestamp"`
	Labels    map[string]string `json:"labels,omitempty"`
}

// MetricError represents a structured error for metrics operations
type MetricError struct {
	Code       string    `json:"code"`
	Message    string    `json:"message"`
	Retryable  bool      `json:"retryable"`
	Timestamp  time.Time `json:"timestamp"`
	SourceFile string    `json:"source_file,omitempty"`
	Line       int       `json:"line,omitempty"`
}

// Error returns a string representation of the error
func (e *MetricError) Error() string {
	return fmt.Sprintf("[%s] %s (retryable: %v)", e.Code, e.Message, e.Retryable)
}

// Error codes
const (
	ErrCodeValidation     = "VALIDATION_ERROR"
	ErrCodeConnection     = "CONNECTION_ERROR"
	ErrCodeAuthentication = "AUTH_ERROR"
	ErrCodeRateLimit      = "RATE_LIMIT"
	ErrCodeTimeout        = "TIMEOUT"
	ErrCodeInternal       = "INTERNAL_ERROR"
)

// ErrorRegistry keeps track of errors for monitoring
type ErrorRegistry struct {
	mu     sync.RWMutex
	errors map[string][]MetricError
	config *config.MetricsConfig
}

// NewErrorRegistry creates a new error registry
func NewErrorRegistry(cfg *config.MetricsConfig) *ErrorRegistry {
	return &ErrorRegistry{
		errors: make(map[string][]MetricError),
		config: cfg,
	}
}

// AddError adds an error to the registry
func (r *ErrorRegistry) AddError(err MetricError) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.errors[err.Code] == nil {
		r.errors[err.Code] = make([]MetricError, 0)
	}
	r.errors[err.Code] = append(r.errors[err.Code], err)

	// Cleanup old errors (keep last hour only)
	r.cleanup()
}

// cleanup removes errors older than one hour
func (r *ErrorRegistry) cleanup() {
	threshold := time.Now().Add(-1 * time.Hour)
	for code := range r.errors {
		filtered := make([]MetricError, 0)
		for _, err := range r.errors[code] {
			if err.Timestamp.After(threshold) {
				filtered = append(filtered, err)
			}
		}
		r.errors[code] = filtered
	}
}

// GetErrorCount returns the number of errors for a specific code
func (r *ErrorRegistry) GetErrorCount(code string) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.errors[code])
}

// GetRecentErrors returns errors that occurred within the specified duration
func (r *ErrorRegistry) GetRecentErrors(duration time.Duration) []MetricError {
	r.mu.RLock()
	defer r.mu.RUnlock()

	threshold := time.Now().Add(-duration)
	recent := make([]MetricError, 0)

	for _, errors := range r.errors {
		for _, err := range errors {
			if err.Timestamp.After(threshold) {
				recent = append(recent, err)
			}
		}
	}

	return recent
}

// MetricPool manages a pool of metric objects for efficient reuse
var metricPool = sync.Pool{
	New: func() interface{} {
		return &types.Metric{}
	},
}

// Validation constants
const (
	MaxMetricNameLength = 200
	MaxLabelKeyLength   = 50
	MaxLabelValueLength = 100
	MaxLabelsPerMetric  = 10
	MetricNamePattern   = "^[a-zA-Z_:][a-zA-Z0-9_:]*$"
)

var (
	metricNameRegex = regexp.MustCompile(MetricNamePattern)
	metricNameCache = make(map[string]bool)
	metricCacheMu   sync.RWMutex
)

// validateMetric performs comprehensive validation of a metric
func validateMetric(metric *types.Metric, cfg *config.MetricsConfig) error {
	if metric == nil {
		return &MetricError{
			Code:      ErrCodeValidation,
			Message:   "metric cannot be nil",
			Retryable: false,
			Timestamp: time.Now(),
		}
	}

	// Validate metric name
	if err := validateMetricName(metric.Name, cfg); err != nil {
		return err
	}

	// Validate labels
	if err := validateLabels(metric.LabelValues, cfg); err != nil {
		return err
	}

	// Validate timestamp
	if metric.Timestamp.IsZero() {
		return &MetricError{
			Code:      ErrCodeValidation,
			Message:   "metric timestamp cannot be zero",
			Retryable: false,
			Timestamp: time.Now(),
		}
	}

	// Validate value
	if math.IsNaN(metric.Value) || math.IsInf(metric.Value, 0) {
		return &MetricError{
			Code:      ErrCodeValidation,
			Message:   "metric value must be a finite number",
			Retryable: false,
			Timestamp: time.Now(),
		}
	}

	return nil
}

// validateMetricName validates the metric name
func validateMetricName(name string, cfg *config.MetricsConfig) error {
	if name == "" {
		return &MetricError{
			Code:      ErrCodeValidation,
			Message:   "metric name cannot be empty",
			Retryable: false,
			Timestamp: time.Now(),
		}
	}

	// Check cache first
	metricCacheMu.RLock()
	if valid, exists := metricNameCache[name]; exists {
		metricCacheMu.RUnlock()
		if !valid {
			return &MetricError{
				Code:      ErrCodeValidation,
				Message:   fmt.Sprintf("invalid metric name format: %s", name),
				Retryable: false,
				Timestamp: time.Now(),
			}
		}
		return nil
	}
	metricCacheMu.RUnlock()

	// Validate length
	if len(name) > cfg.MaxMetricNameLength {
		metricCacheMu.Lock()
		metricNameCache[name] = false
		metricCacheMu.Unlock()
		return &MetricError{
			Code:      ErrCodeValidation,
			Message:   fmt.Sprintf("metric name too long (max %d characters)", cfg.MaxMetricNameLength),
			Retryable: false,
			Timestamp: time.Now(),
		}
	}

	// Validate format
	if !metricNameRegex.MatchString(name) {
		metricCacheMu.Lock()
		metricNameCache[name] = false
		metricCacheMu.Unlock()
		return &MetricError{
			Code:      ErrCodeValidation,
			Message:   fmt.Sprintf("invalid metric name format: %s", name),
			Retryable: false,
			Timestamp: time.Now(),
		}
	}

	// Cache successful validation
	metricCacheMu.Lock()
	metricNameCache[name] = true
	metricCacheMu.Unlock()

	return nil
}

// validateLabels validates metric labels
func validateLabels(labels map[string]string, cfg *config.MetricsConfig) error {
	if len(labels) > cfg.MaxLabelsPerMetric {
		return &MetricError{
			Code:      ErrCodeValidation,
			Message:   fmt.Sprintf("too many labels (max %d)", cfg.MaxLabelsPerMetric),
			Retryable: false,
			Timestamp: time.Now(),
		}
	}

	for k, v := range labels {
		if k == "" {
			return &MetricError{
				Code:      ErrCodeValidation,
				Message:   "label key cannot be empty",
				Retryable: false,
				Timestamp: time.Now(),
			}
		}

		if len(k) > MaxLabelKeyLength {
			return &MetricError{
				Code:      ErrCodeValidation,
				Message:   fmt.Sprintf("label key too long: %s (max %d characters)", k, MaxLabelKeyLength),
				Retryable: false,
				Timestamp: time.Now(),
			}
		}

		if len(v) > cfg.MaxLabelValueLength {
			return &MetricError{
				Code:      ErrCodeValidation,
				Message:   fmt.Sprintf("label value too long for key %s (max %d characters)", k, cfg.MaxLabelValueLength),
				Retryable: false,
				Timestamp: time.Now(),
			}
		}
	}

	return nil
}

// NewClient creates a new VictoriaMetrics client
func NewClient(cfg *config.Config) *MetricsClient {
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   cfg.Victoria.ConnectTimeout,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	}

	client := &MetricsClient{
		URL:      cfg.Victoria.URL,
		Username: cfg.Victoria.Username,
		Password: cfg.Victoria.Password,
		config:   cfg,
		client: &http.Client{
			Transport: transport,
			Timeout:   cfg.Victoria.RequestTimeout,
		},
		buffer:        make(chan types.MetricBatch, cfg.Metrics.BufferSize),
		batchSize:     cfg.Metrics.BatchSize,
		errorRegistry: NewErrorRegistry(&cfg.Metrics),
	}
	return client
}

// SendMetrics sends a batch of metrics to VictoriaMetrics
func (c *MetricsClient) SendMetrics(batch types.MetricBatch) error {
	for _, metric := range batch.Metrics {
		// Validate metric
		if err := validateMetric(&metric, &c.config.Metrics); err != nil {
			return c.handleError(err, ErrCodeValidation, false)
		}

		// Get a metric from the pool
		pooledMetric := metricPool.Get().(*types.Metric)
		*pooledMetric = metric // Copy the metric data

		// Process metric
		if err := c.processMetric(pooledMetric); err != nil {
			metricPool.Put(pooledMetric) // Return to pool on error
			return err
		}

		// Return to pool after processing
		metricPool.Put(pooledMetric)
	}

	return c.sendToVictoriaMetrics(batch)
}

// formatMetricsPrometheus converts metrics to Prometheus text format
func (c *MetricsClient) formatMetricsPrometheus(batch types.MetricBatch) ([]byte, error) {
	var builder strings.Builder

	for _, metric := range batch.Metrics {
		// Format: name{label1="value1",label2="value2"} value timestamp
		builder.WriteString(metric.Name)

		if len(metric.LabelValues) > 0 {
			builder.WriteString("{")
			first := true
			for k, v := range metric.LabelValues {
				if !first {
					builder.WriteString(",")
				}
				builder.WriteString(k)
				builder.WriteString("=\"")
				// Escape quotes in values
				escapedValue := strings.ReplaceAll(v, "\"", "\\\"")
				builder.WriteString(escapedValue)
				builder.WriteString("\"")
				first = false
			}
			builder.WriteString("}")
		}

		builder.WriteString(" ")
		builder.WriteString(fmt.Sprintf("%g", metric.Value))
		builder.WriteString(" ")
		builder.WriteString(fmt.Sprintf("%d", metric.Timestamp.UnixNano()/1000000)) // Milliseconds
		builder.WriteString("\n")
	}

	return []byte(builder.String()), nil
}

// SendMetricsImmediate sends metrics immediately without going through the buffer
func (c *MetricsClient) SendMetricsImmediate(batch types.MetricBatch) error {
	// Log the format of the metrics for debugging
	c.logMetricFormat(batch)

	// Validate metrics
	for i := range batch.Metrics {
		if err := validateMetric(&batch.Metrics[i], &c.config.Metrics); err != nil {
			return c.handleError(err, ErrCodeValidation, false)
		}
	}

	if logsClient, ok := utils.GetLogsClient(); ok {
		logsClient.LogEvent("INFO", "Sending metrics immediately (bypassing buffer)", "metrics", nil)
	}

	// Send directly to VictoriaMetrics
	return c.sendToVictoriaMetrics(batch)
}

// SendLogs sends logs to VictoriaMetrics - this method should not be used
// because it is intended for the logs client, not the metrics client
func (c *MetricsClient) SendLogs(logs []models.LogEntry) error {
	utils.LogWarning("SendLogs called on MetricsClient instead of LogsClient - logs will not be sent")
	return fmt.Errorf("method not implemented for MetricsClient, use LogsClient instead")
}

// sendToVictoriaMetrics sends data to VictoriaMetrics
func (c *MetricsClient) sendToVictoriaMetrics(batch types.MetricBatch) error {
	// Get server ID from environment or generate one
	serverID := os.Getenv("GAMESERVER_ID")
	if serverID == "" {
		serverID = "unknown"
	}

	// Create a buffer to store JSON lines
	var buf bytes.Buffer

	// Convert each metric to JSON line format
	for _, metric := range batch.Metrics {
		// Create the log entry
		entry := map[string]interface{}{
			"log": map[string]interface{}{
				"level":   "info",
				"message": fmt.Sprintf("%s=%g", metric.Name, metric.Value),
			},
			"date":      fmt.Sprintf("%d", metric.Timestamp.UnixNano()),
			"stream":    metric.Name,
			"game":      "ac",
			"server_id": serverID,
		}

		// Add labels to the entry
		for k, v := range metric.LabelValues {
			entry[k] = v
		}

		// Marshal to JSON
		jsonData, err := json.Marshal(entry)
		if err != nil {
			return fmt.Errorf("error marshaling metric: %w", err)
		}

		// Write JSON line
		buf.Write(jsonData)
		buf.WriteString("\n")
	}

	// Prepare request body
	var body io.Reader = &buf
	contentType := "application/stream+json"

	if c.config.Victoria.Compression {
		var compressedBuf bytes.Buffer
		gz := gzip.NewWriter(&compressedBuf)
		if _, err := gz.Write(buf.Bytes()); err != nil {
			return fmt.Errorf("compression error: %w", err)
		}
		if err := gz.Close(); err != nil {
			return fmt.Errorf("compression close error: %w", err)
		}
		body = &compressedBuf
		contentType = "application/stream+json+gzip"
	}

	// Create request with the correct endpoint
	url := fmt.Sprintf("%s/insert/jsonline?_stream_fields=stream&_time_field=date&_msg_field=log.message", c.URL)
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
		return fmt.Errorf("error sending metrics: %w", err)
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code: %d - %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// LogEvent creates and sends a unique log event
func (c *MetricsClient) LogEvent(level, message string, eventType string, labels map[string]string) error {
	// Send an event to VictoriaMetrics
	c.SendMetrics(types.MetricBatch{
		Metrics: []types.Metric{
			{
				Name:  "assetto_server_event",
				Value: 1,
				Type:  types.Counter,
				LabelValues: map[string]string{
					"level":      level,
					"event_type": eventType,
					"message":    message,
				},
			},
		},
		Time: time.Now(),
	})

	return nil
}

// SendEvent sends an event to VictoriaLogs
func (c *MetricsClient) SendEvent(eventType, message string, labels map[string]string) error {
	// Create an event
	event := map[string]interface{}{
		"_msg":       message,
		"_time":      time.Now().Format(time.RFC3339),
		"event_type": eventType,
	}

	// Add labels
	for k, v := range labels {
		event[k] = v
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %v", err)
	}

	// Send to VictoriaLogs
	data := url.Values{}
	data.Set("format", "json")
	data.Set("stream", "assetto_server_event")
	data.Set("data", string(jsonData))

	resp, err := c.client.PostForm(c.URL+"/api/v1/logs/insert", data)
	if err != nil {
		return fmt.Errorf("failed to send event: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code when sending event: %d", resp.StatusCode)
	}

	return nil
}

// Add flush method with retry
func (c *MetricsClient) flushMetrics(batch types.MetricBatch) error {
	for attempt := 0; attempt < c.config.Victoria.MaxRetries; attempt++ {
		if err := c.sendToVictoriaMetrics(batch); err == nil {
			return nil
		}
		time.Sleep(c.config.Victoria.RetryBackoff)
	}
	return fmt.Errorf("failed after %d retries", c.config.Victoria.MaxRetries)
}

// Add metric processing method
func (c *MetricsClient) processMetric(metric *types.Metric) error {
	// Add any metric preprocessing logic here
	return nil
}

// Add error handling methods
func (c *MetricsClient) handleError(err error, code string, retryable bool) error {
	if err == nil {
		return nil
	}

	metricErr := MetricError{
		Code:      code,
		Message:   err.Error(),
		Retryable: retryable,
		Timestamp: time.Now(),
	}

	c.errorRegistry.AddError(metricErr)

	// Record error metric
	c.SendMetrics(types.MetricBatch{
		Metrics: []types.Metric{
			{
				Name:      "assetto_server_metrics_errors_total",
				Value:     1,
				Type:      types.Counter,
				Timestamp: time.Now(),
				LabelValues: map[string]string{
					"error_code": code,
					"retryable":  fmt.Sprintf("%v", retryable),
				},
			},
		},
		Time: time.Now(),
	})

	return &metricErr
}

// Add this method to Client
func (c *MetricsClient) Buffer() chan types.MetricBatch {
	return c.buffer
}

// GetErrorCount returns the number of errors of a specific type
func (c *MetricsClient) GetErrorCount(errorCode string) int {
	return c.errorRegistry.GetErrorCount(errorCode)
}

// Add this function to debug the format of the metrics
func (c *MetricsClient) logMetricFormat(batch types.MetricBatch) {
	// Only log format if debug mode is active
	if os.Getenv("DEBUG_METRICS") != "true" {
		return
	}

	var builder strings.Builder

	// Limit to a few metrics to avoid cluttering logs
	maxSamples := 3
	sampleCount := min(maxSamples, len(batch.Metrics))

	builder.WriteString(fmt.Sprintf("Sample format (%d/%d metrics):\n", sampleCount, len(batch.Metrics)))

	for i := 0; i < sampleCount; i++ {
		metric := batch.Metrics[i]
		// Format: name{label1="value1",label2="value2"} value timestamp
		builder.WriteString(metric.Name)

		if len(metric.LabelValues) > 0 {
			builder.WriteString("{")
			first := true
			for k, v := range metric.LabelValues {
				if !first {
					builder.WriteString(",")
				}
				builder.WriteString(k)
				builder.WriteString("=\"")
				builder.WriteString(v)
				builder.WriteString("\"")
				first = false
			}
			builder.WriteString("}")
		}

		builder.WriteString(" ")
		builder.WriteString(fmt.Sprintf("%f", metric.Value))
		builder.WriteString(" ")
		builder.WriteString(fmt.Sprintf("%d", metric.Timestamp.Unix()*1000))
		builder.WriteString("\n")
	}

	if logsClient, ok := utils.GetLogsClient(); ok {
		logsClient.LogEvent("INFO", builder.String(), "metrics_format", nil)
	}
}

// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// StartMetricBuffer starts processing metrics in the background
func (c *MetricsClient) StartMetricBuffer(ctx context.Context) {
	if logsClient, ok := utils.GetLogsClient(); ok {
		logsClient.LogEvent("INFO", "Metrics system ready (OK)", "metrics", nil)
	}
	ticker := time.NewTicker(c.config.Metrics.FlushInterval)
	defer ticker.Stop()

	batch := types.MetricBatch{
		Metrics: make([]types.Metric, 0, c.batchSize),
		Time:    time.Now(),
	}

	for {
		select {
		case <-ctx.Done():
			// Flush final remaining metrics without log
			if len(batch.Metrics) > 0 {
				c.sendToVictoriaMetrics(batch)
			}
			return
		case newBatch := <-c.buffer:
			// Add metrics to current batch
			batch.Metrics = append(batch.Metrics, newBatch.Metrics...)

			// If batch reaches maximum size, send immediately
			if len(batch.Metrics) >= c.batchSize {
				c.sendToVictoriaMetrics(batch)
				batch = types.MetricBatch{
					Metrics: make([]types.Metric, 0, c.batchSize),
					Time:    time.Now(),
				}
			}
		case <-ticker.C:
			// Send current batch if it contains metrics
			if len(batch.Metrics) > 0 {
				c.sendToVictoriaMetrics(batch)
				batch = types.MetricBatch{
					Metrics: make([]types.Metric, 0, c.batchSize),
					Time:    time.Now(),
				}
			}
		}
	}
}
