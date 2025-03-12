package monitoring

import (
	"context"
	"regexp"
	"strings"
	"sync"
	"time"

	"metrics/types"
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

// MonitorServerEvents surveille les événements du serveur
func MonitorServerEvents(ctx context.Context, metricsClient *victoria.MetricsClient, logsClient victoria.LogsClient, serverState *types.ServerState, eventChan <-chan string) {
	logsClient.LogEvent("INFO", "Starting event monitoring", "monitoring", nil)

	for {
		select {
		case <-ctx.Done():
			return
		case event := <-eventChan:
			// Envoyer tous les événements à VictoriaLogs
			logsClient.LogServerEvent("INFO", event, "server_event", map[string]string{
				"server_id": serverState.ServerID,
				"raw_event": "true",
			})

			// Traitement spécifique selon le type d'événement
			if strings.Contains(event, "CHAT:") {
				// Traitement des messages de chat
				// ...
			} else if strings.Contains(event, "Collision between") {
				// Traitement des collisions
				// ...
			}
		}
	}
}
