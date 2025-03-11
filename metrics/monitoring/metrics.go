package monitoring

import (
	"context"
	"metrics/types"
	"metrics/utils"
	"metrics/victoria"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// Add new monitoring metrics
const (
	// Existing metrics...

	// Enhanced monitoring metrics
	MetricDroppedTotal       = "assetto_server_metrics_dropped_total"
	MetricBufferUsage        = "assetto_server_metrics_buffer_usage"
	MetricBatchSizeHistogram = "assetto_server_metrics_batch_size"
	MetricProcessingDuration = "assetto_server_metrics_processing_duration_seconds"
	MetricValidationErrors   = "assetto_server_metrics_validation_errors_total"
	MetricSendQueueSize      = "assetto_server_metrics_send_queue_size"
	MetricRetryCount         = "assetto_server_metrics_retry_count_total"
	MetricCompressionRatio   = "assetto_server_metrics_compression_ratio"
)

// Variables for metrics tracking
var (
	processedMetrics int64
	droppedMetrics   int64
	retryCount       int64
)

func MonitorMetrics(ctx context.Context, vmClient *victoria.MetricsClient, state *types.ServerState) {
	// Start internal monitoring
	go MonitorMetricsInternals(ctx, vmClient)

	// Start performance monitoring
	go MonitorMetricsPerformance(ctx, vmClient)

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			start := time.Now()

			// Collecter les métriques du serveur
			metrics := []types.Metric{
				{
					Name:      "assetto_server_players_connected",
					Value:     float64(state.Players),
					Type:      types.Gauge,
					Timestamp: time.Now(),
					LabelValues: map[string]string{
						"server_id":   state.ServerID,
						"server_name": state.ServerName,
					},
				},
				{
					Name:      "assetto_server_session_duration",
					Value:     time.Since(state.SessionStart).Seconds(),
					Type:      types.Gauge,
					Timestamp: time.Now(),
					LabelValues: map[string]string{
						"server_id":    state.ServerID,
						"session_type": state.SessionType,
					},
				},
			}

			// Ajouter les métriques des joueurs
			state.RLock()
			for _, player := range state.ConnectedPlayers {
				metrics = append(metrics,
					types.Metric{
						Name:      "assetto_server_network_latency_ms",
						Value:     float64(player.Latency),
						Type:      types.Gauge,
						Timestamp: time.Now(),
						LabelValues: map[string]string{
							"player_id":   player.SteamID,
							"player_name": player.Name,
							"server_id":   state.ServerID,
							"server_name": state.ServerName,
						},
					},
					types.Metric{
						Name:      "assetto_server_player_packet_loss",
						Value:     player.PacketLoss,
						Type:      types.Gauge,
						Timestamp: time.Now(),
						LabelValues: map[string]string{
							"player_id":   player.SteamID,
							"player_name": player.Name,
						},
					},
				)
			}
			state.RUnlock()

			// Add processing duration metric
			metrics = append(metrics, types.Metric{
				Name:      MetricProcessingDuration,
				Value:     time.Since(start).Seconds(),
				Type:      types.Gauge,
				Timestamp: time.Now(),
			})

			// Send metrics and track
			if err := vmClient.SendMetrics(types.MetricBatch{
				Metrics: metrics,
				Time:    time.Now(),
			}); err != nil {
				incrementDroppedMetrics()
			} else {
				incrementProcessedMetrics()
			}
		}
	}
}

func MonitorMetricsSystem(ctx context.Context, vmClient *victoria.MetricsClient) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Métriques du wrapper
			metrics := []types.Metric{
				{
					Name:      "assetto_server_wrapper_goroutines",
					Value:     float64(runtime.NumGoroutine()),
					Type:      types.Gauge,
					Timestamp: time.Now(),
				},
				{
					Name:      "assetto_server_wrapper_memory_alloc_bytes",
					Value:     float64(getMemoryStats()),
					Type:      types.Gauge,
					Timestamp: time.Now(),
				},
				{
					Name:      "assetto_server_wrapper_metrics_send_queue",
					Value:     float64(len(vmClient.Buffer())),
					Type:      types.Gauge,
					Timestamp: time.Now(),
				},
			}

			// Envoyer les métriques système
			vmClient.SendMetrics(types.MetricBatch{
				Metrics: metrics,
				Time:    time.Now(),
			})
		}
	}
}

// Fonction utilitaire pour obtenir les stats mémoire
func getMemoryStats() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.Alloc
}

// Ajouter un moniteur d'erreurs
type ErrorMonitor struct {
	sync.RWMutex
	errors     map[string]int
	thresholds map[string]int
	client     *victoria.MetricsClient
}

func NewErrorMonitor(client *victoria.MetricsClient) *ErrorMonitor {
	return &ErrorMonitor{
		errors: make(map[string]int),
		thresholds: map[string]int{
			"metric_validation": 100,
			"send_failure":      50,
			"connection":        10,
		},
		client: client,
	}
}

func (em *ErrorMonitor) RecordError(errorType string, err error) {
	em.Lock()
	defer em.Unlock()

	em.errors[errorType]++
	count := em.errors[errorType]
	threshold := em.thresholds[errorType]

	// Envoyer une métrique d'erreur
	em.client.SendMetrics(types.MetricBatch{
		Metrics: []types.Metric{
			{
				Name:  "assetto_server_wrapper_errors_total",
				Value: float64(count),
				Type:  types.Counter,
				LabelValues: map[string]string{
					"error_type": errorType,
					"error":      err.Error(),
				},
				Timestamp: time.Now(),
			},
		},
	})

	// Vérifier le seuil
	if count >= threshold {
		utils.LogError("Error threshold reached for %s: %d errors", errorType, count)
		// Réinitialiser le compteur
		em.errors[errorType] = 0
	}
}

// Add monitoring functions
func MonitorMetricsInternals(ctx context.Context, vmClient *victoria.MetricsClient) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Monitor internal metrics
			metrics := []types.Metric{
				{
					Name:      MetricBufferUsage,
					Value:     float64(len(vmClient.Buffer())) / float64(cap(vmClient.Buffer())),
					Type:      types.Gauge,
					Timestamp: time.Now(),
					LabelValues: map[string]string{
						"type": "send_buffer",
					},
				},
				{
					Name:      MetricSendQueueSize,
					Value:     float64(len(vmClient.Buffer())),
					Type:      types.Gauge,
					Timestamp: time.Now(),
				},
			}

			// Add error metrics from registry
			for _, errType := range []string{
				victoria.ErrCodeValidation,
				victoria.ErrCodeConnection,
				victoria.ErrCodeAuthentication,
				victoria.ErrCodeRateLimit,
				victoria.ErrCodeTimeout,
			} {
				metrics = append(metrics, types.Metric{
					Name:      MetricValidationErrors,
					Value:     float64(vmClient.GetErrorCount(errType)),
					Type:      types.Counter,
					Timestamp: time.Now(),
					LabelValues: map[string]string{
						"error_type": errType,
					},
				})
			}

			// Send internal monitoring metrics
			vmClient.SendMetrics(types.MetricBatch{
				Metrics: metrics,
				Time:    time.Now(),
			})
		}
	}
}

// Add performance monitoring
func MonitorMetricsPerformance(ctx context.Context, vmClient *victoria.MetricsClient) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	var (
		lastProcessedCount int64
		lastDroppedCount   int64
		lastRetryCount     int64
	)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Get current counts
			currentProcessed := atomic.LoadInt64(&processedMetrics)
			currentDropped := atomic.LoadInt64(&droppedMetrics)
			currentRetries := atomic.LoadInt64(&retryCount)

			// Calculate rates
			metrics := []types.Metric{
				{
					Name:      "assetto_server_metrics_processed_rate",
					Value:     float64(currentProcessed - lastProcessedCount),
					Type:      types.Gauge,
					Timestamp: time.Now(),
				},
				{
					Name:      "assetto_server_metrics_dropped_rate",
					Value:     float64(currentDropped - lastDroppedCount),
					Type:      types.Gauge,
					Timestamp: time.Now(),
				},
				{
					Name:      "assetto_server_metrics_retry_rate",
					Value:     float64(currentRetries - lastRetryCount),
					Type:      types.Gauge,
					Timestamp: time.Now(),
				},
			}

			// Update last counts
			lastProcessedCount = currentProcessed
			lastDroppedCount = currentDropped
			lastRetryCount = currentRetries

			// Send performance metrics
			vmClient.SendMetrics(types.MetricBatch{
				Metrics: metrics,
				Time:    time.Now(),
			})
		}
	}
}

// Add helper functions to increment counters
func incrementProcessedMetrics() {
	atomic.AddInt64(&processedMetrics, 1)
}

func incrementDroppedMetrics() {
	atomic.AddInt64(&droppedMetrics, 1)
}

func incrementRetryCount() {
	atomic.AddInt64(&retryCount, 1)
}

// Wrapper metrics
