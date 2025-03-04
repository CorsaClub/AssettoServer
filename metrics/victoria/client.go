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

// Renommer LogsClientImpl en LogsClient (le type concret)
type LogsClientImpl struct {
	config     *config.VictoriaLogsConfig
	httpClient *http.Client
}

// Renommer l'interface en LogsClient (au lieu de LogsClientInterface)
type LogsClient interface {
	SendLogs(logs []types.Log) error
}

// Client existant renommé pour plus de clarté
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

// MetricPoint représente un point de données pour VictoriaMetrics
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

func NewErrorRegistry(cfg *config.MetricsConfig) *ErrorRegistry {
	return &ErrorRegistry{
		errors: make(map[string][]MetricError),
		config: cfg,
	}
}

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

func (r *ErrorRegistry) GetErrorCount(code string) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.errors[code])
}

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

// MetricPool manages a pool of metric objects
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

// formatMetricsPrometheus convertit les métriques au format texte Prometheus
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
				// Échapper les guillemets dans les valeurs
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
		builder.WriteString(fmt.Sprintf("%d", metric.Timestamp.UnixNano()/1000000)) // Millisecondes
		builder.WriteString("\n")
	}

	return []byte(builder.String()), nil
}

// SendMetricsImmediate envoie immédiatement des métriques sans passer par le buffer
func (c *MetricsClient) SendMetricsImmediate(batch types.MetricBatch) error {
	// Log le format des métriques pour le débogage
	c.logMetricFormat(batch)

	// Validation des métriques
	for i := range batch.Metrics {
		if err := validateMetric(&batch.Metrics[i], &c.config.Metrics); err != nil {
			return c.handleError(err, ErrCodeValidation, false)
		}
	}

	// Envoi direct à VictoriaMetrics
	utils.LogInfo("Sending metrics immediately (bypassing buffer)")
	return c.sendToVictoriaMetrics(batch)
}

// SendLogs envoie les logs à VictoriaMetrics
func (c *MetricsClient) SendLogs(logs []models.LogEntry) error {
	// Implémentation de l'envoi des logs
	return nil
}

func (c *MetricsClient) sendToVictoriaMetrics(batch types.MetricBatch) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.Victoria.RequestTimeout)
	defer cancel()

	data, err := c.formatMetricsPrometheus(batch)
	if err != nil {
		return fmt.Errorf("error formatting metrics: %w", err)
	}

	var body io.Reader = bytes.NewBuffer(data)
	if c.config.Victoria.Compression {
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		if _, err := gz.Write(data); err != nil {
			return fmt.Errorf("compression error: %w", err)
		}
		if err := gz.Close(); err != nil {
			return fmt.Errorf("compression close error: %w", err)
		}
		body = &buf
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.URL+"/api/v1/import/prometheus", body)
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

	if c.Username != "" && c.Password != "" {
		req.SetBasicAuth(c.Username, c.Password)
	}

	if c.config.Victoria.Compression {
		req.Header.Set("Content-Encoding", "gzip")
	}
	req.Header.Set("Content-Type", "text/plain")

	resp, err := c.client.Do(req)
	if err != nil {
		utils.LogError("Échec de l'envoi de métriques: %v", err)
		return fmt.Errorf("error sending metrics: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		utils.LogError("Échec de l'envoi de métriques (code %d): %s",
			resp.StatusCode, string(respBody))
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Aucun log en cas de succès
	return nil
}

// LogEvent envoie un événement de log à VictoriaMetrics
func (c *MetricsClient) LogEvent(level string, message string, eventType string, labels map[string]string) {
	c.SendMetrics(types.MetricBatch{
		Metrics: []types.Metric{
			{
				Name:  "assetto_event",
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
}

// Ajouter une méthode de flush avec retry
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
				Name:      "assetto_metrics_errors_total",
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

// Mettre à jour la signature
func NewLogsClient(cfg *config.VictoriaLogsConfig) LogsClient {
	return &LogsClientImpl{
		config: cfg,
		httpClient: &http.Client{
			Timeout: cfg.RequestTimeout,
		},
	}
}

func (c *LogsClientImpl) SendLogs(logs []types.Log) error {
	if len(logs) == 0 {
		return nil
	}

	// Convert logs to JSON lines format
	var lines []string
	for _, log := range logs {
		// Create log entry with all fields
		logData := map[string]interface{}{
			"timestamp": log.Timestamp.UnixNano(),
			"level":     log.Level,
			"message":   log.Message,
			"source":    log.Source,
		}

		// Add labels if present
		if len(log.Labels) > 0 {
			logData["labels"] = log.Labels
		}

		// Convert to JSON
		jsonData, err := json.Marshal(logData)
		if err != nil {
			continue
		}
		lines = append(lines, string(jsonData))
	}

	// Prepare request data
	data := url.Values{}
	data.Set("format", "jsonl")
	data.Set("data", strings.Join(lines, "\n"))

	// Create request
	req, err := http.NewRequest("POST", c.config.URL+"/api/v1/logs/insert", strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if c.config.Username != "" && c.config.Password != "" {
		req.SetBasicAuth(c.config.Username, c.config.Password)
	}

	// Send request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send logs: %v", err)
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code when sending logs: %d", resp.StatusCode)
	}

	return nil
}

// GetErrorCount returns the number of errors of a specific type
func (c *MetricsClient) GetErrorCount(errorCode string) int {
	return c.errorRegistry.GetErrorCount(errorCode)
}

// Ajoutez cette fonction pour déboguer le format des métriques
func (c *MetricsClient) logMetricFormat(batch types.MetricBatch) {
	// Ne logger le format que si le mode debug est activé
	if os.Getenv("DEBUG_METRICS") != "true" {
		return
	}

	var builder strings.Builder

	// Limiter à quelques métriques pour éviter de polluer les logs
	maxSamples := 3
	sampleCount := min(maxSamples, len(batch.Metrics))

	builder.WriteString(fmt.Sprintf("Échantillon de format (%d/%d métriques):\n", sampleCount, len(batch.Metrics)))

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

	utils.LogInfo("%s", builder.String())
}

// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// StartMetricBuffer démarre le traitement des métriques en arrière-plan
func (c *MetricsClient) StartMetricBuffer(ctx context.Context) {
	utils.LogInfo("Système de métriques prêt (OK)")
	ticker := time.NewTicker(c.config.Metrics.FlushInterval)
	defer ticker.Stop()

	batch := types.MetricBatch{
		Metrics: make([]types.Metric, 0, c.batchSize),
		Time:    time.Now(),
	}

	for {
		select {
		case <-ctx.Done():
			// Flush final des métriques restantes sans log
			if len(batch.Metrics) > 0 {
				c.sendToVictoriaMetrics(batch)
			}
			return
		case newBatch := <-c.buffer:
			// Ajouter les métriques au lot courant
			batch.Metrics = append(batch.Metrics, newBatch.Metrics...)

			// Si le lot atteint la taille maximale, l'envoyer immédiatement
			if len(batch.Metrics) >= c.batchSize {
				c.sendToVictoriaMetrics(batch)
				batch = types.MetricBatch{
					Metrics: make([]types.Metric, 0, c.batchSize),
					Time:    time.Now(),
				}
			}
		case <-ticker.C:
			// Envoyer le lot actuel s'il contient des métriques
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
