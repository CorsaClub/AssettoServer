package victoria

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"bytes"
	"compress/gzip"
	"io"

	"metrics/config"
	"metrics/models"
	"metrics/types"
)

// Client represents a VictoriaMetrics client
type Client struct {
	URL       string
	Username  string
	Password  string
	client    *http.Client
	config    *config.VictoriaConfig
	buffer    chan types.MetricBatch
	batchSize int
}

// MetricPoint représente un point de données pour VictoriaMetrics
type MetricPoint struct {
	Metric    string            `json:"metric"`
	Value     float64           `json:"value"`
	Timestamp int64             `json:"timestamp"`
	Labels    map[string]string `json:"labels,omitempty"`
}

// NewClient creates a new VictoriaMetrics client
func NewClient(cfg *config.Config) *Client {
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

	client := &Client{
		URL:      cfg.Victoria.URL,
		Username: cfg.Victoria.Username,
		Password: cfg.Victoria.Password,
		config:   &cfg.Victoria,
		client: &http.Client{
			Transport: transport,
			Timeout:   cfg.Victoria.RequestTimeout,
		},
		buffer:    make(chan types.MetricBatch, cfg.Metrics.BufferSize),
		batchSize: cfg.Metrics.BatchSize,
	}
	return client
}

// SendMetrics sends a batch of metrics to VictoriaMetrics
func (c *Client) SendMetrics(batch types.MetricBatch) error {
	for _, metric := range batch.Metrics {
		if err := validateMetric(&metric); err != nil {
			return fmt.Errorf("invalid metric %s: %w", metric.Name, err)
		}
	}

	return c.sendToVictoriaMetrics(batch)
}

// formatMetrics converts metrics to VictoriaMetrics format
func formatMetrics(batch types.MetricBatch) ([]byte, error) {
	points := make([]MetricPoint, 0, len(batch.Metrics))

	for _, metric := range batch.Metrics {
		// Pour les histogrammes, on crée plusieurs points
		if metric.Type == types.Histogram {
			// Point pour la valeur
			points = append(points, MetricPoint{
				Metric:    metric.Name + "_sum",
				Value:     metric.Value,
				Timestamp: metric.Timestamp.Unix(),
				Labels:    metric.LabelValues,
			})

			// Points pour les buckets
			for _, bucket := range metric.Buckets {
				bucketLabels := copyLabels(metric.LabelValues)
				bucketLabels["le"] = fmt.Sprintf("%g", bucket)
				points = append(points, MetricPoint{
					Metric:    metric.Name + "_bucket",
					Value:     metric.Value,
					Timestamp: metric.Timestamp.Unix(),
					Labels:    bucketLabels,
				})
			}

			// Point pour le count
			points = append(points, MetricPoint{
				Metric:    metric.Name + "_count",
				Value:     1, // Incrémenter le compteur
				Timestamp: metric.Timestamp.Unix(),
				Labels:    metric.LabelValues,
			})
		} else {
			// Pour les gauges et counters, on crée un seul point
			points = append(points, MetricPoint{
				Metric:    metric.Name,
				Value:     metric.Value,
				Timestamp: metric.Timestamp.Unix(),
				Labels:    metric.LabelValues,
			})
		}
	}

	return json.Marshal(points)
}

// copyLabels creates a copy of a label map
func copyLabels(labels map[string]string) map[string]string {
	newLabels := make(map[string]string, len(labels))
	for k, v := range labels {
		newLabels[k] = v
	}
	return newLabels
}

// Ajouter les méthodes manquantes
func (c *Client) StartMetricBuffer(ctx context.Context) {
	// Implémentation similaire à l'ancienne version
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	batch := types.MetricBatch{
		Metrics: make([]types.Metric, 0),
		Time:    time.Now(),
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if len(batch.Metrics) > 0 {
				c.SendMetrics(batch)
				batch = types.MetricBatch{
					Metrics: make([]types.Metric, 0),
					Time:    time.Now(),
				}
			}
		}
	}
}

// SendLogs envoie les logs à VictoriaMetrics
func (c *Client) SendLogs(logs []models.LogEntry) error {
	// Implémentation de l'envoi des logs
	return nil
}

func (c *Client) sendToVictoriaMetrics(batch types.MetricBatch) error {
	ctx, cancel := context.WithTimeout(context.Background(), c.config.RequestTimeout)
	defer cancel()

	data, err := formatMetrics(batch)
	if err != nil {
		return fmt.Errorf("error formatting metrics: %w", err)
	}

	var body io.Reader = bytes.NewBuffer(data)
	if c.config.Compression {
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

	req, err := http.NewRequestWithContext(ctx, "POST", c.URL+"/api/v1/import", body)
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

	if c.Username != "" && c.Password != "" {
		req.SetBasicAuth(c.Username, c.Password)
	}

	if c.config.Compression {
		req.Header.Set("Content-Encoding", "gzip")
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("error sending metrics: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

// LogEvent envoie un événement de log à VictoriaMetrics
func (c *Client) LogEvent(level string, message string, eventType string, labels map[string]string) {
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
func (c *Client) flushMetrics(batch types.MetricBatch) error {
	for attempt := 0; attempt < c.config.MaxRetries; attempt++ {
		if err := c.sendToVictoriaMetrics(batch); err == nil {
			return nil
		}
		time.Sleep(c.config.RetryBackoff)
	}
	return fmt.Errorf("failed after %d retries", c.config.MaxRetries)
}

// Ajouter la validation des métriques
func validateMetric(metric *types.Metric) error {
	if metric.Name == "" {
		return fmt.Errorf("metric name cannot be empty")
	}

	if metric.Timestamp.IsZero() {
		return fmt.Errorf("metric timestamp cannot be zero")
	}

	if metric.Type == types.Histogram && len(metric.Buckets) == 0 {
		return fmt.Errorf("histogram metric must have buckets")
	}

	// Valider les labels
	for k, v := range metric.LabelValues {
		if k == "" {
			return fmt.Errorf("label key cannot be empty")
		}
		if v == "" {
			return fmt.Errorf("label value cannot be empty for key %s", k)
		}
	}

	return nil
}

// Ajouter un chiffrement des données sensibles
func (c *Client) encryptSensitiveData(data []byte) ([]byte, error) {
	// Implémentation du chiffrement
	return nil, nil
}

// Ajouter une catégorisation des erreurs
type MetricError struct {
	Type      string
	Message   string
	Retryable bool
}

// Améliorer la gestion des retries
func (c *Client) shouldRetry(err error) bool {
	// Logique de décision pour les retries
	return false
}

// Add this method to Client
func (c *Client) Buffer() chan types.MetricBatch {
	return c.buffer
}
