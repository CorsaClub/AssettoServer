package metrics

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// MetricPoint représente un point de données pour VictoriaMetrics
type MetricPoint struct {
	Metric    string            `json:"metric"`
	Value     float64           `json:"value"`
	Timestamp int64             `json:"timestamp"`
	Labels    map[string]string `json:"labels,omitempty"`
}

// VictoriaMetricsClient gère l'envoi des métriques à VictoriaMetrics
type VictoriaMetricsClient struct {
	endpoint     string
	username     string
	password     string
	client       *http.Client
	commonLabels map[string]string
	maxRetries   int
	backoff      time.Duration
	buffer       chan MetricBatch
}

// MetricBatch représente un lot de métriques à envoyer
type MetricBatch struct {
	Metrics []Metric
	Time    time.Time
}

// Options pour la configuration du client
type Option func(*VictoriaMetricsClient)

func WithRetries(retries int) Option {
	return func(c *VictoriaMetricsClient) {
		c.maxRetries = retries
	}
}

func WithBatchSize(size int) Option {
	return func(c *VictoriaMetricsClient) {
		c.buffer = make(chan MetricBatch, size)
	}
}

func WithTimeout(timeout time.Duration) Option {
	return func(c *VictoriaMetricsClient) {
		c.client.Timeout = timeout
	}
}

// NewVictoriaMetricsClient crée un nouveau client
func NewVictoriaMetricsClient(endpoint, username, password string, commonLabels map[string]string, opts ...Option) *VictoriaMetricsClient {
	client := &VictoriaMetricsClient{
		endpoint:     endpoint,
		username:     username,
		password:     password,
		commonLabels: commonLabels,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		maxRetries: 3,
		backoff:    1 * time.Second,
		buffer:     make(chan MetricBatch, 100),
	}

	for _, opt := range opts {
		opt(client)
	}

	return client
}

// SendMetrics envoie un lot de métriques
func (v *VictoriaMetricsClient) SendMetrics(batch MetricBatch) error {
	start := time.Now()
	retries := 0

	for attempt := 0; attempt < v.maxRetries; attempt++ {
		err := v.doSendMetrics(batch)
		if err == nil {
			v.recordMetrics(start, len(batch.Metrics), nil, retries)
			return nil
		}

		if !isRetryableError(err) {
			v.recordMetrics(start, len(batch.Metrics), err, retries)
			return err
		}

		retries++
		time.Sleep(v.backoff * time.Duration(attempt+1))
	}

	err := fmt.Errorf("failed to send metrics after %d attempts", v.maxRetries)
	v.recordMetrics(start, len(batch.Metrics), err, retries)
	return err
}

// doSendMetrics effectue l'envoi des métriques
func (v *VictoriaMetricsClient) doSendMetrics(batch MetricBatch) error {
	points := make([]MetricPoint, 0, len(batch.Metrics))

	for _, metric := range batch.Metrics {
		point := MetricPoint{
			Metric:    metric.Name,
			Value:     metric.Value,
			Timestamp: metric.Timestamp.Unix(),
			Labels:    mergeLabels(v.commonLabels, metric.LabelValues),
		}
		points = append(points, point)

		// Gestion spéciale pour les histogrammes
		if metric.Type == Histogram {
			points = appendHistogramPoints(points, metric)
		}
	}

	data, err := json.Marshal(points)
	if err != nil {
		return fmt.Errorf("error marshaling metrics: %w", err)
	}

	return v.sendToVictoriaMetrics(data)
}

// SendLogs envoie les logs à VictoriaLogs
func (v *VictoriaMetricsClient) SendLogs(logs []LogEntry) error {
	if len(logs) == 0 {
		return nil
	}

	// Préparer les données pour VictoriaLogs
	var lines []string
	for _, log := range logs {
		// Fusionner les labels communs avec les labels spécifiques
		allLabels := make(map[string]string)
		for k, v := range v.commonLabels {
			allLabels[k] = v
		}
		for k, v := range log.Labels {
			allLabels[k] = v
		}

		// Convertir l'entrée de log en format JSON
		logData := map[string]interface{}{
			"timestamp":  log.Timestamp.UnixNano(),
			"level":      log.Level,
			"message":    log.Message,
			"labels":     allLabels,
			"server_id":  log.ServerID,
			"session_id": log.SessionID,
		}

		// Ajouter les champs optionnels s'ils sont présents
		if log.PlayerID != "" {
			logData["player_id"] = log.PlayerID
		}
		if log.PlayerName != "" {
			logData["player_name"] = log.PlayerName
		}
		if log.EventType != "" {
			logData["event_type"] = log.EventType
		}
		if log.Error != "" {
			logData["error"] = log.Error
		}

		// Convertir en JSON
		jsonData, err := json.Marshal(logData)
		if err != nil {
			continue
		}
		lines = append(lines, string(jsonData))
	}

	// Envoyer les logs à VictoriaLogs
	data := url.Values{}
	data.Set("format", "jsonl")
	data.Set("data", strings.Join(lines, "\n"))

	resp, err := v.client.PostForm(v.endpoint+"/api/v1/logs/insert", data)
	if err != nil {
		return fmt.Errorf("failed to send logs: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code when sending logs: %d", resp.StatusCode)
	}

	return nil
}

// LogEvent crée et envoie un événement de log unique
func (v *VictoriaMetricsClient) LogEvent(level, message string, eventType string, labels map[string]string) error {
	log := LogEntry{
		Timestamp: time.Now(),
		Level:     level,
		Message:   message,
		EventType: eventType,
		Labels:    labels,
		ServerID:  v.commonLabels["server_id"],
	}

	return v.SendLogs([]LogEntry{log})
}

// sendToVictoriaMetrics envoie les données à VictoriaMetrics
func (v *VictoriaMetricsClient) sendToVictoriaMetrics(data []byte) error {
	req, err := http.NewRequest("POST", v.endpoint+"/api/v1/import", bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

	if v.username != "" && v.password != "" {
		req.SetBasicAuth(v.username, v.password)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := v.client.Do(req)
	if err != nil {
		return fmt.Errorf("error sending metrics: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

// StartMetricBuffer démarre le buffer de métriques
func (v *VictoriaMetricsClient) StartMetricBuffer(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	batch := MetricBatch{
		Metrics: make([]Metric, 0),
		Time:    time.Now(),
	}

	for {
		select {
		case <-ctx.Done():
			if len(batch.Metrics) > 0 {
				v.SendMetrics(batch)
			}
			return
		case metric := <-v.buffer:
			batch.Metrics = append(batch.Metrics, metric.Metrics...)
		case <-ticker.C:
			if len(batch.Metrics) > 0 {
				v.SendMetrics(batch)
				batch = MetricBatch{
					Metrics: make([]Metric, 0),
					Time:    time.Now(),
				}
			}
		}
	}
}

// Add this function to determine if an error is retryable
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	// Consider network errors and 5xx responses as retryable
	return strings.Contains(err.Error(), "connection refused") ||
		strings.Contains(err.Error(), "timeout") ||
		strings.Contains(err.Error(), "5") // 5xx errors
}

// Ajouter ces métriques internes
const (
	MetricSendDuration = "assetto_metrics_send_duration_seconds"
	MetricBatchSize    = "assetto_metrics_batch_size"
	MetricErrorCount   = "assetto_metrics_error_total"
	MetricRetryCount   = "assetto_metrics_retry_total"
)

func (v *VictoriaMetricsClient) recordMetrics(start time.Time, batchSize int, err error, retries int) {
	duration := time.Since(start).Seconds()

	batch := MetricBatch{
		Metrics: []Metric{
			{
				Name:        MetricSendDuration,
				Value:       duration,
				Type:        Gauge,
				Timestamp:   time.Now(),
				LabelValues: v.commonLabels,
			},
			{
				Name:        MetricBatchSize,
				Value:       float64(batchSize),
				Type:        Gauge,
				Timestamp:   time.Now(),
				LabelValues: v.commonLabels,
			},
		},
		Time: time.Now(),
	}

	if err != nil {
		batch.Metrics = append(batch.Metrics, Metric{
			Name:        MetricErrorCount,
			Value:       1,
			Type:        Counter,
			Timestamp:   time.Now(),
			LabelValues: v.commonLabels,
		})
	}

	if retries > 0 {
		batch.Metrics = append(batch.Metrics, Metric{
			Name:        MetricRetryCount,
			Value:       float64(retries),
			Type:        Counter,
			Timestamp:   time.Now(),
			LabelValues: v.commonLabels,
		})
	}

	v.SendMetrics(batch)
}

// Nouvelles méthodes utilitaires
func mergeLabels(common, specific map[string]string) map[string]string {
	result := make(map[string]string, len(common)+len(specific))
	for k, v := range common {
		result[k] = v
	}
	for k, v := range specific {
		result[k] = v
	}
	return result
}

func appendHistogramPoints(points []MetricPoint, metric Metric) []MetricPoint {
	// Ajouter les points spécifiques aux histogrammes
	// (sum, count, buckets)
	return points
}
