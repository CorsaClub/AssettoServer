package monitoring

import (
	"context"
	"metrics/types"
	"metrics/utils"
	"metrics/victoria"
	"runtime"
	"sync"
	"time"
)

func MonitorMetrics(ctx context.Context, vmClient *victoria.Client, state *types.ServerState) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
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
						Name:      "assetto_server_player_latency",
						Value:     float64(player.Latency),
						Type:      types.Gauge,
						Timestamp: time.Now(),
						LabelValues: map[string]string{
							"player_id":   player.SteamID,
							"player_name": player.Name,
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

			// Envoyer les métriques
			vmClient.SendMetrics(types.MetricBatch{
				Metrics: metrics,
				Time:    time.Now(),
			})
		}
	}
}

func MonitorMetricsSystem(ctx context.Context, vmClient *victoria.Client) {
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
					Name:      "assetto_wrapper_goroutines",
					Value:     float64(runtime.NumGoroutine()),
					Type:      types.Gauge,
					Timestamp: time.Now(),
				},
				{
					Name:      "assetto_wrapper_memory_alloc_bytes",
					Value:     float64(getMemoryStats()),
					Type:      types.Gauge,
					Timestamp: time.Now(),
				},
				{
					Name:      "assetto_wrapper_metrics_send_queue",
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
	client     *victoria.Client
}

func NewErrorMonitor(client *victoria.Client) *ErrorMonitor {
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
				Name:  "assetto_wrapper_errors_total",
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
