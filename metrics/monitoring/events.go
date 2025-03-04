package monitoring

import (
	"context"
	"regexp"
	"strconv"
	"sync"
	"time"

	"metrics/types"
	"metrics/utils"
	"metrics/victoria"
)

var (
	// Regex pour extraire les informations de collision
	collisionRegex = regexp.MustCompile(`Collision between (\w+) \((\d+)\) and (environment|car \w+), rel\. speed (\d+)km/h`)
	// Regex pour extraire les messages de chat
	chatRegex = regexp.MustCompile(`CHAT: (\w+) \((\d+)\): (.+)`)
)

// CollisionStats maintient les statistiques de collision pour la session en cours
type CollisionStats struct {
	mu                 sync.Mutex
	TotalCollisions    int
	EnvironmentCount   int
	CarCount           int
	CollisionsBySpeed  map[string]int // Catégories de vitesse: "low", "medium", "high"
	CollisionsByPlayer map[string]int // Nombre de collisions par joueur
	LastUpdate         time.Time
}

// NewCollisionStats crée une nouvelle instance de CollisionStats
func NewCollisionStats() *CollisionStats {
	return &CollisionStats{
		CollisionsBySpeed:  make(map[string]int),
		CollisionsByPlayer: make(map[string]int),
		LastUpdate:         time.Now(),
	}
}

// AddCollision ajoute une collision aux statistiques
func (cs *CollisionStats) AddCollision(playerID string, collisionType string, speed float64) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	cs.TotalCollisions++

	// Incrémenter le compteur par type
	if collisionType == "environment" {
		cs.EnvironmentCount++
	} else {
		cs.CarCount++
	}

	// Catégoriser par vitesse
	var speedCategory string
	switch {
	case speed < 10:
		speedCategory = "low"
	case speed < 30:
		speedCategory = "medium"
	default:
		speedCategory = "high"
	}
	cs.CollisionsBySpeed[speedCategory]++

	// Incrémenter le compteur par joueur
	cs.CollisionsByPlayer[playerID]++

	cs.LastUpdate = time.Now()
}

// GetMetrics renvoie les métriques de collision actuelles
func (cs *CollisionStats) GetMetrics(serverID string, sessionID string, sessionType string) []types.Metric {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	metrics := []types.Metric{
		{
			Name:      "assetto_server_collisions_total",
			Value:     float64(cs.TotalCollisions),
			Type:      types.Counter,
			Timestamp: time.Now(),
			LabelValues: map[string]string{
				"server_id":    serverID,
				"session_id":   sessionID,
				"session_type": sessionType,
			},
		},
		{
			Name:      "assetto_server_collisions_environment",
			Value:     float64(cs.EnvironmentCount),
			Type:      types.Counter,
			Timestamp: time.Now(),
			LabelValues: map[string]string{
				"server_id":    serverID,
				"session_id":   sessionID,
				"session_type": sessionType,
			},
		},
		{
			Name:      "assetto_server_collisions_car",
			Value:     float64(cs.CarCount),
			Type:      types.Counter,
			Timestamp: time.Now(),
			LabelValues: map[string]string{
				"server_id":    serverID,
				"session_id":   sessionID,
				"session_type": sessionType,
			},
		},
	}

	// Ajouter les métriques par catégorie de vitesse
	for category, count := range cs.CollisionsBySpeed {
		metrics = append(metrics, types.Metric{
			Name:      "assetto_server_collisions_by_speed",
			Value:     float64(count),
			Type:      types.Counter,
			Timestamp: time.Now(),
			LabelValues: map[string]string{
				"server_id":      serverID,
				"session_id":     sessionID,
				"session_type":   sessionType,
				"speed_category": category,
			},
		})
	}

	// Ajouter les métriques par joueur
	for playerID, count := range cs.CollisionsByPlayer {
		metrics = append(metrics, types.Metric{
			Name:      "assetto_server_collisions_by_player",
			Value:     float64(count),
			Type:      types.Counter,
			Timestamp: time.Now(),
			LabelValues: map[string]string{
				"server_id":    serverID,
				"session_id":   sessionID,
				"session_type": sessionType,
				"player_id":    playerID,
			},
		})
	}

	return metrics
}

// MonitorServerEvents surveille les événements du serveur (collisions, chat, etc.)
func MonitorServerEvents(ctx context.Context, vmClient *victoria.MetricsClient, logsClient victoria.LogsClient, state *types.ServerState, eventChan <-chan string) {
	utils.LogInfo("Server events monitoring : [ OK ]")

	// Initialiser les statistiques de collision
	collisionStats := NewCollisionStats()

	// Démarrer une goroutine pour envoyer périodiquement les métriques de collision
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Envoyer les métriques de collision accumulées
				state.RLock()
				sessionID := state.CurrentSession.ID
				sessionType := state.CurrentSession.Type
				state.RUnlock()

				metrics := collisionStats.GetMetrics(state.ServerID, sessionID, sessionType)
				if len(metrics) > 0 {
					batch := types.MetricBatch{
						Metrics: metrics,
						Time:    time.Now(),
					}
					vmClient.SendMetrics(batch)
				}
			}
		}
	}()

	// Traiter les événements entrants
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case event := <-eventChan:
				// Traiter les collisions
				if matches := collisionRegex.FindStringSubmatch(event); matches != nil {
					playerName := matches[1]
					playerID := matches[2]
					collisionType := matches[3]
					speed, _ := strconv.ParseFloat(matches[4], 64)

					// Mettre à jour les statistiques de collision
					collisionStats.AddCollision(playerID, collisionType, speed)

					// Envoyer comme métrique individuelle
					batch := types.MetricBatch{
						Metrics: []types.Metric{
							{
								Name:      "assetto_server_collision",
								Value:     speed,
								Type:      types.Counter,
								Timestamp: time.Now(),
								LabelValues: map[string]string{
									"server_id":      state.ServerID,
									"player_name":    playerName,
									"player_id":      playerID,
									"collision_type": collisionType,
								},
							},
						},
						Time: time.Now(),
					}
					vmClient.SendMetrics(batch)

					// Envoyer comme log
					logs := []types.Log{
						{
							Timestamp: time.Now(),
							Level:     "INFO",
							Message:   event,
							Source:    "server_events",
							Labels: map[string]string{
								"event_type":     "collision",
								"server_id":      state.ServerID,
								"player_name":    playerName,
								"player_id":      playerID,
								"collision_type": collisionType,
								"speed":          matches[4],
							},
						},
					}
					logsClient.SendLogs(logs)
				}

				// Traiter les messages de chat
				if matches := chatRegex.FindStringSubmatch(event); matches != nil {
					playerName := matches[1]
					playerID := matches[2]
					message := matches[3]

					// Envoyer comme log uniquement
					logs := []types.Log{
						{
							Timestamp: time.Now(),
							Level:     "INFO",
							Message:   message,
							Source:    "chat",
							Labels: map[string]string{
								"event_type":  "chat",
								"server_id":   state.ServerID,
								"player_name": playerName,
								"player_id":   playerID,
							},
						},
					}
					logsClient.SendLogs(logs)
				}
			}
		}
	}()
}
