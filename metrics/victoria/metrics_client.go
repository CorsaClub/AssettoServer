// Package victoria provides clients for interacting with VictoriaMetrics and VictoriaLogs.
// This file implements a client for VictoriaMetrics time series database.
package victoria

import (
	"context"
	"fmt"
	"math"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"bytes"
	"compress/gzip"
	"io"

	"metrics/config"
	"metrics/env"
	"metrics/models"
	"metrics/types"
	"metrics/utils"
)

// MetricsClient is a client for sending metrics to VictoriaMetrics
type MetricsClient struct {
	URL           string
	Username      string
	Password      string
	client        *http.Client
	config        *config.Config
	buffer        chan types.MetricBatch
	batchSize     int
	errorRegistry *ErrorRegistry
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
	// Get environment variables
	envVars := env.GetEnv()

	// Configure VictoriaMetrics URL from environment variables if available
	if envVars.VictoriaMetricsURL != "" {
		url := envVars.GetVictoriaMetricsURL()
		cfg.Victoria.URL = url
		fmt.Printf("[DEBUG] VictoriaMetrics URL set from environment: %s\n", url)
	} else if url := envVars.VictoriaMetricsURL; url != "" {
		// Legacy support
		if port := envVars.VictoriaMetricsPort; port != "" {
			cfg.Victoria.URL = fmt.Sprintf("http://%s:%s", url, port)
		} else {
			cfg.Victoria.URL = fmt.Sprintf("http://%s:%s", url, config.DefaultVictoriaPort)
		}
		fmt.Printf("[DEBUG] VictoriaMetrics URL set from legacy environment variables: %s\n", cfg.Victoria.URL)
	} else {
		fmt.Printf("[DEBUG] Using default VictoriaMetrics URL: %s\n", cfg.Victoria.URL)
	}

	// Configure credentials from environment variables if available
	if envVars.VictoriaMetricsUsername != "" {
		cfg.Victoria.Username = envVars.VictoriaMetricsUsername
		fmt.Println("[DEBUG] VictoriaMetrics username set from environment")
	} else if user := envVars.VictoriaMetricsUsername; user != "" {
		// Legacy support
		cfg.Victoria.Username = user
		fmt.Println("[DEBUG] VictoriaMetrics username set from legacy environment variables")
	}

	if envVars.VictoriaMetricsPassword != "" {
		cfg.Victoria.Password = envVars.VictoriaMetricsPassword
		fmt.Println("[DEBUG] VictoriaMetrics password set from environment")
	} else if pass := envVars.VictoriaMetricsPassword; pass != "" {
		// Legacy support
		cfg.Victoria.Password = pass
		fmt.Println("[DEBUG] VictoriaMetrics password set from legacy environment variables")
	}

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

	// Use flushMetrics instead of direct send for retry capability
	return c.flushMetrics(batch)
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
	envVars := env.GetEnv()

	ctx, cancel := context.WithTimeout(context.Background(), c.config.Victoria.RequestTimeout)
	defer cancel()

	// Get server ID from environment or generate one
	serverID := envVars.GameServerID
	if serverID == "" {
		serverID = "unknown"
	}

	// Log debug information
	if envVars.DebugLogs {
		fmt.Printf("[DEBUG] Sending %d metrics to VictoriaMetrics at %s using Prometheus exposition format\n", len(batch.Metrics), c.URL)
	}

	// Format metrics in Prometheus exposition format
	promData, err := c.formatMetricsPrometheus(batch)
	if err != nil {
		return fmt.Errorf("error formatting metrics: %w", err)
	}

	// Prepare request body
	var body io.Reader = bytes.NewReader(promData)
	contentType := "text/plain"

	if c.config.Victoria.Compression {
		var compressedBuf bytes.Buffer
		gz := gzip.NewWriter(&compressedBuf)
		if _, err := gz.Write(promData); err != nil {
			return fmt.Errorf("compression error: %w", err)
		}
		if err := gz.Close(); err != nil {
			return fmt.Errorf("compression close error: %w", err)
		}
		body = &compressedBuf
		contentType = "text/plain; charset=utf-8"
	}

	// Create request with the correct endpoint for Prometheus exposition format
	url := fmt.Sprintf("%s/api/v1/import/prometheus", c.URL)
	if envVars.DebugLogs {
		fmt.Printf("[DEBUG] Using endpoint for metrics: %s\n", url)

		// Log a sample of the data being sent
		sample := string(promData)
		if len(sample) > 500 {
			sample = sample[:500] + "..." // Truncate to avoid too long logs
		}
		fmt.Printf("[DEBUG] Sample metrics data: %s\n", sample)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, body)
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", contentType)
	if c.config.Victoria.Compression {
		req.Header.Set("Content-Encoding", "gzip")
	}

	// Set authentication if provided
	if c.Username != "" && c.Password != "" {
		req.SetBasicAuth(c.Username, c.Password)
	}

	// Log request details
	if envVars.DebugLogs {
		fmt.Printf("[DEBUG] HTTP Request details:\n")
		fmt.Printf("  Method: %s\n", req.Method)
		fmt.Printf("  URL: %s\n", req.URL.String())
		fmt.Printf("  Headers:\n")
		for key, values := range req.Header {
			for _, value := range values {
				fmt.Printf("    %s: %s\n", key, value)
			}
		}
		if c.Username != "" {
			fmt.Printf("  Authentication: Basic (username: %s)\n", c.Username)
		}
	}

	// Send request
	startTime := time.Now()
	resp, err := c.client.Do(req)
	requestDuration := time.Since(startTime)

	if err != nil {
		if envVars.DebugLogs {
			fmt.Printf("[DEBUG] Error sending metrics to VictoriaMetrics: %v (took %v)\n", err, requestDuration)
		}
		return fmt.Errorf("error sending metrics: %w", err)
	}
	defer resp.Body.Close()

	// Log response details
	if envVars.DebugLogs {
		fmt.Printf("[DEBUG] HTTP Response details:\n")
		fmt.Printf("  Status: %s\n", resp.Status)
		fmt.Printf("  Status Code: %d\n", resp.StatusCode)
		fmt.Printf("  Request Duration: %v\n", requestDuration)
		fmt.Printf("  Headers:\n")
		for key, values := range resp.Header {
			for _, value := range values {
				fmt.Printf("    %s: %s\n", key, value)
			}
		}
	}

	// Check response
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		if envVars.DebugLogs {
			fmt.Printf("[DEBUG] VictoriaMetrics returned status %d: %s\n", resp.StatusCode, string(respBody))
		}
		return fmt.Errorf("unexpected status code: %d - %s", resp.StatusCode, string(respBody))
	}

	// Read response body for debugging
	if envVars.DebugLogs {
		respBody, _ := io.ReadAll(resp.Body)
		if len(respBody) > 0 {
			fmt.Printf("[DEBUG] Response body: %s\n", string(respBody))
		} else {
			fmt.Printf("[DEBUG] Response body is empty\n")
		}
		fmt.Printf("[DEBUG] Successfully sent %d metrics to VictoriaMetrics (took %v)\n", len(batch.Metrics), requestDuration)
	}

	return nil
}

// LogEvent creates and sends a metric event
func (c *MetricsClient) LogEvent(level, message string, eventType string, labels map[string]string) error {
	// Create a metric for the event
	eventMetric := types.MetricBatch{
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
	}

	// Add additional labels if provided
	if labels != nil {
		for k, v := range labels {
			eventMetric.Metrics[0].LabelValues[k] = v
		}
	}

	// Send the event metric
	return c.SendMetrics(eventMetric)
}

// flushMetrics sends metrics with retry capability
func (c *MetricsClient) flushMetrics(batch types.MetricBatch) error {
	for attempt := 0; attempt < c.config.Victoria.MaxRetries; attempt++ {
		if err := c.sendToVictoriaMetrics(batch); err == nil {
			return nil
		}
		time.Sleep(c.config.Victoria.RetryBackoff)
	}
	return fmt.Errorf("failed after %d retries", c.config.Victoria.MaxRetries)
}

// processMetric processes a metric before sending
func (c *MetricsClient) processMetric(metric *types.Metric) error {
	envVars := env.GetEnv()

	// Get server ID from environment
	serverID := envVars.GameServerID
	if serverID == "" {
		serverID = "unknown"
	}

	// Add common labels if they don't exist
	if metric.LabelValues == nil {
		metric.LabelValues = make(map[string]string)
	}

	// Add required labels if not present
	if _, exists := metric.LabelValues["game"]; !exists {
		metric.LabelValues["game"] = "ac"
	}
	if _, exists := metric.LabelValues["server_id"]; !exists {
		metric.LabelValues["server_id"] = serverID
	}

	// Ensure timestamp is set
	if metric.Timestamp.IsZero() {
		metric.Timestamp = time.Now()
	}

	return nil
}

// handleError handles and records metric errors
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

// Buffer returns the metrics buffer channel
func (c *MetricsClient) Buffer() chan types.MetricBatch {
	return c.buffer
}

// GetErrorCount returns the number of errors of a specific type
func (c *MetricsClient) GetErrorCount(errorCode string) int {
	return c.errorRegistry.GetErrorCount(errorCode)
}

// logMetricFormat logs the format of metrics for debugging
func (c *MetricsClient) logMetricFormat(batch types.MetricBatch) {
	// Only log format if debug mode is active
	envVars := env.GetEnv()
	if !envVars.DebugMetrics {
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
			// Flush final remaining metrics with retry
			if len(batch.Metrics) > 0 {
				if err := c.flushMetrics(batch); err != nil {
					if logsClient, ok := utils.GetLogsClient(); ok {
						logsClient.LogEvent("ERROR", fmt.Sprintf("Failed to flush final metrics: %v", err), "metrics", nil)
					}
				}
			}
			return
		case newBatch := <-c.buffer:
			// Add metrics to current batch
			batch.Metrics = append(batch.Metrics, newBatch.Metrics...)

			// If batch reaches maximum size, send immediately with retry
			if len(batch.Metrics) >= c.batchSize {
				if err := c.flushMetrics(batch); err != nil {
					if logsClient, ok := utils.GetLogsClient(); ok {
						logsClient.LogEvent("ERROR", fmt.Sprintf("Failed to flush metrics batch: %v", err), "metrics", nil)
					}
				}
				batch = types.MetricBatch{
					Metrics: make([]types.Metric, 0, c.batchSize),
					Time:    time.Now(),
				}
			}
		case <-ticker.C:
			// Send current batch if it contains metrics with retry
			if len(batch.Metrics) > 0 {
				if err := c.flushMetrics(batch); err != nil {
					if logsClient, ok := utils.GetLogsClient(); ok {
						logsClient.LogEvent("ERROR", fmt.Sprintf("Failed to flush metrics on ticker: %v", err), "metrics", nil)
					}
				}
				batch = types.MetricBatch{
					Metrics: make([]types.Metric, 0, c.batchSize),
					Time:    time.Now(),
				}
			}
		}
	}
}

// TestConnection tests the connection to VictoriaMetrics
func (c *MetricsClient) TestConnection() error {
	envVars := env.GetEnv()

	// Create a test metric
	testMetric := types.Metric{
		Name:      "test_connection",
		Value:     1.0,
		Type:      types.Gauge,
		Timestamp: time.Now(),
		LabelValues: map[string]string{
			"test": "true",
		},
	}

	// Create a batch with the test metric
	batch := types.MetricBatch{
		Metrics: []types.Metric{testMetric},
		Time:    time.Now(),
	}

	// Send the test metric
	err := c.SendMetricsImmediate(batch)
	if err != nil {
		// Try to determine the cause of the error
		if strings.Contains(err.Error(), "404") {
			return fmt.Errorf("VictoriaMetrics API endpoint may be incorrect (should be /api/v1/import/prometheus): %w", err)
		} else if strings.Contains(err.Error(), "connection refused") {
			return fmt.Errorf("connection to VictoriaMetrics was refused, check if the service is running on %s: %w", c.URL, err)
		} else if strings.Contains(err.Error(), "no such host") {
			return fmt.Errorf("VictoriaMetrics host could not be resolved (%s): %w", c.URL, err)
		} else if strings.Contains(err.Error(), "timeout") {
			return fmt.Errorf("connection to VictoriaMetrics timed out (%s): %w", c.URL, err)
		}
		return fmt.Errorf("failed to send test metric to VictoriaMetrics: %w", err)
	}

	if envVars.DebugLogs {
		fmt.Printf("[DEBUG] Successfully tested connection to VictoriaMetrics at %s\n", c.URL)
	}

	return nil
}
