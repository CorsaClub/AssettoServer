// Package monitoring handles the monitoring and health checks of the server.
package monitoring

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	metrics "metrics/services"
	"metrics/types"
	"metrics/utils"
	"metrics/victoria"
)

// DoHealth performs periodic health checks of the server.
// It pings the Agones SDK and updates relevant metrics based on the health status.
// If a health check fails, it initiates a graceful shutdown of the server.
func DoHealth(ctx context.Context, vmClient *victoria.MetricsClient, state *types.ServerState, cancel context.CancelFunc) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			state.Lock()
			state.LastPing = time.Now()
			state.Unlock()

			// Perform health check
			if !isHealthy(state) {
				utils.LogWarning("Health check failed")
				// Créer une métrique pour les échecs de ping
				vmClient.SendMetrics(types.MetricBatch{
					Metrics: []types.Metric{
						{
							Name:      metrics.ServerErrorsCounter.Name,
							Value:     1,
							Type:      types.Counter,
							Timestamp: time.Now(),
							LabelValues: map[string]string{
								"server_id":    state.ServerID,
								"session_id":   state.CurrentSession.ID,
								"session_type": state.CurrentSession.Type,
								"error_type":   "health_ping_failure",
							},
						},
					},
					Time: time.Now(),
				})

				// Log the current system state
				state.RLock()
				utils.LogSDK("System state - Players: %d, Ready: %v", state.Players, state.Ready)
				state.RUnlock()

				// Initiate a graceful shutdown
				gracefulShutdown(cancel, state)
				return
			}

			// Update health metrics
			state.RLock()
			vmClient.SendMetrics(types.MetricBatch{
				Metrics: []types.Metric{
					{
						Name:      metrics.ServerUptime,
						Value:     time.Since(state.LastPing).Seconds(),
						Type:      types.Gauge,
						Timestamp: time.Now(),
						LabelValues: map[string]string{
							"server_id":    state.ServerID,
							"session_id":   state.CurrentSession.ID,
							"session_type": state.CurrentSession.Type,
							"metric_type":  "last_health_ping",
						},
					},
				},
				Time: time.Now(),
			})
			state.RUnlock()

			// Log health status periodically
			if time.Now().Second()%30 == 0 {
				state.RLock()
				utils.LogSDK("Health status: Ready=%v, LastPing=%v ago, ShuttingDown=%v",
					state.Ready,
					time.Since(state.LastPing),
					state.ShuttingDown)
				state.RUnlock()
			}
		}
	}
}

// isHealthy vérifie si le serveur est en bonne santé
func isHealthy(state *types.ServerState) bool {
	state.RLock()
	defer state.RUnlock()

	return state.Ready &&
		time.Since(state.LastPing) < 5*time.Second &&
		!state.ShuttingDown
}

// MonitorHealthMetrics surveille et met à jour les métriques de santé du serveur
func MonitorHealthMetrics(ctx context.Context, vmClient *victoria.MetricsClient, state *types.ServerState) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Create a clean set of labels without any JSON escaping issues
			serverID := state.ServerID
			serverName := state.ServerName
			serverRegion := state.ServerRegion

			// Ensure labels are properly formatted
			labels := map[string]string{
				"server_id": serverID,
				"name":      serverName,
				"region":    serverRegion,
				"type":      state.ServerType,
			}

			// Send metrics with explicit naming and clean labels
			batch := types.MetricBatch{
				Metrics: []types.Metric{
					{
						Name:        metrics.ServerHealth,
						Value:       1,
						Type:        types.Gauge,
						Timestamp:   time.Now(),
						LabelValues: labels,
					},
					{
						Name:        "assetto_server_players_count",
						Value:       float64(state.Players),
						Type:        types.Gauge,
						Timestamp:   time.Now(),
						LabelValues: labels,
					},
					{
						Name:      metrics.ServerUptime,
						Value:     float64(time.Now().Unix()),
						Type:      types.Gauge,
						Timestamp: time.Now(),
						LabelValues: map[string]string{
							"server_id":    state.ServerID,
							"session_id":   state.CurrentSession.ID,
							"session_type": state.CurrentSession.Type,
							"metric_type":  "health_timestamp",
						},
					},
				},
				Time: time.Now(),
			}

			// Ajouter les métriques de base
			batch.Metrics = append(batch.Metrics, types.Metric{
				Name:      metrics.ServerPlayersConnected,
				Value:     float64(len(state.ConnectedPlayers)),
				Type:      types.Gauge,
				Timestamp: time.Now(),
				LabelValues: map[string]string{
					"server_id":    state.ServerID,
					"session_id":   state.CurrentSession.ID,
					"session_type": state.CurrentSession.Type,
				},
			})

			// Pas de logs détaillés avant l'envoi
			if err := vmClient.SendMetrics(batch); err != nil {
				utils.LogError("Échec de l'envoi des métriques de santé: %v", err)
			}
		}
	}
}

func collectMetrics(state *types.ServerState) types.MetricBatch {
	state.RLock()
	defer state.RUnlock()

	metrics := types.MetricBatch{
		Metrics: []types.Metric{
			{
				Name:      "assetto_server_players",
				Value:     float64(state.Players),
				Type:      types.Gauge,
				Timestamp: time.Now(),
				LabelValues: map[string]string{
					"server_id":    state.ServerID,
					"session_id":   state.CurrentSession.ID,
					"session_type": state.CurrentSession.Type,
				},
			},
			{
				Name:      "assetto_server_tick_rate",
				Value:     state.TickRate,
				Type:      types.Gauge,
				Timestamp: time.Now(),
				LabelValues: map[string]string{
					"server_id":    state.ServerID,
					"session_id":   state.CurrentSession.ID,
					"session_type": state.CurrentSession.Type,
				},
			},
			{
				Name:      metrics.TrackGrip,
				Value:     state.TrackGrip,
				Type:      types.Gauge,
				Timestamp: time.Now(),
				LabelValues: map[string]string{
					"server_id":    state.ServerID,
					"session_id":   state.CurrentSession.ID,
					"session_type": state.CurrentSession.Type,
				},
			},
			{
				Name:      metrics.TrackTemperature,
				Value:     state.TrackTemp,
				Type:      types.Gauge,
				Timestamp: time.Now(),
				LabelValues: map[string]string{
					"server_id":    state.ServerID,
					"session_id":   state.CurrentSession.ID,
					"session_type": state.CurrentSession.Type,
				},
			},
			{
				Name:      "assetto_server_players",
				Value:     float64(state.Players),
				Type:      types.Gauge,
				Timestamp: time.Now(),
				LabelValues: map[string]string{
					"server_id":    state.ServerID,
					"session_id":   state.CurrentSession.ID,
					"session_type": state.CurrentSession.Type,
				},
			},
			{
				Name:      metrics.ServerTickRate,
				Value:     float64(state.TickRate),
				Type:      types.Gauge,
				Timestamp: time.Now(),
				LabelValues: map[string]string{
					"server_id":    state.ServerID,
					"session_id":   state.CurrentSession.ID,
					"session_type": state.CurrentSession.Type,
				},
			},
			{
				Name:      "assetto_server_track_grip",
				Value:     state.TrackGrip,
				Type:      types.Gauge,
				Timestamp: time.Now(),
				LabelValues: map[string]string{
					"server_id":    state.ServerID,
					"session_id":   state.CurrentSession.ID,
					"session_type": state.CurrentSession.Type,
				},
			},
			{
				Name:      "assetto_server_track_temp",
				Value:     state.TrackTemp,
				Type:      types.Gauge,
				Timestamp: time.Now(),
				LabelValues: map[string]string{
					"server_id":    state.ServerID,
					"session_id":   state.CurrentSession.ID,
					"session_type": state.CurrentSession.Type,
				},
			},
			{
				Name:      metrics.AirTemperature,
				Value:     state.AirTemp,
				Type:      types.Gauge,
				Timestamp: time.Now(),
				LabelValues: map[string]string{
					"server_id":    state.ServerID,
					"session_id":   state.CurrentSession.ID,
					"session_type": state.CurrentSession.Type,
				},
			},
			{
				Name:      metrics.SessionDuration,
				Value:     float64(time.Since(state.CurrentSession.StartTime).Seconds()),
				Type:      types.Gauge,
				Timestamp: time.Now(),
				LabelValues: map[string]string{
					"server_id":    state.ServerID,
					"session_id":   state.CurrentSession.ID,
					"session_type": state.CurrentSession.Type,
				},
			},
			{
				Name:      metrics.ServerSessionType,
				Value:     sessionTypeToValue(state.CurrentSession.Type),
				Type:      types.Gauge,
				Timestamp: time.Now(),
				LabelValues: map[string]string{
					"server_id":    state.ServerID,
					"session_id":   state.CurrentSession.ID,
					"session_type": state.CurrentSession.Type,
				},
			},
			{
				Name:      metrics.ServerPlayersConnected,
				Value:     float64(len(state.ConnectedPlayers)),
				Type:      types.Gauge,
				Timestamp: time.Now(),
				LabelValues: map[string]string{
					"server_id":    state.ServerID,
					"session_id":   state.CurrentSession.ID,
					"session_type": state.CurrentSession.Type,
				},
			},
		},
		Time: time.Now(),
	}

	return metrics
}

// MonitorSystemResources monitors the system resource usage (CPU and Memory).
// It updates the relevant metrics at regular intervals.
// A pool is used to limit the number of concurrent goroutines performing the updates.
func MonitorSystemResources(ctx context.Context, state *types.ServerState) {
	// Use a goroutine pool to limit the number of concurrent system metric updates
	metricsPool := make(chan struct{}, 2) // Limit to 2 concurrent goroutines

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			select {
			case metricsPool <- struct{}{}:
				go func() {
					defer func() { <-metricsPool }()
					updateSystemMetrics(state)
				}()
			default:
				// Skip this update if the pool is full to avoid overwhelming the system
				utils.LogDebug("Skipping metrics update - too busy")
			}
		}
	}
}

// gracefulShutdown performs a graceful shutdown of the server
func gracefulShutdown(cancel context.CancelFunc, state *types.ServerState) {
	state.Lock()
	state.ShuttingDown = true
	state.Unlock()

	time.Sleep(time.Second)
	cancel()
}

// monitorGameServerState logs the GameServer state for debugging purposes.
func monitorGameServerState(gameServer interface{}) {
	// Don't log anymore the GameServer details
}

// updateMetrics updates the basic metrics such as the number of players and session duration.
func updateMetrics(state *types.ServerState) {
	baseLabels := map[string]string{
		"server_id":   state.ServerID,
		"server_name": state.ServerName,
		"server_type": state.ServerType,
	}

	metrics.PlayersGauge.With(baseLabels).Set(float64(state.Players))
	if state.CurrentSession != nil {
		sessionLabels := map[string]string{
			"server_id":    state.ServerID,
			"server_name":  state.ServerName,
			"server_type":  state.ServerType,
			"session_type": state.SessionType,
		}
		metrics.SessionDurationGauge.With(sessionLabels).Set(time.Since(state.SessionStart).Seconds())
	}
}

// updateDetailedMetrics updates more detailed metrics
func updateDetailedMetrics(state *types.ServerState) {
	baseLabels := map[string]string{
		"server_id":   state.ServerID,
		"server_name": state.ServerName,
		"server_type": state.ServerType,
	}

	metrics.SessionTimeLeftGauge.With(baseLabels).Set(float64(state.SessionTimeLeft))
	metrics.TrackGripGauge.With(baseLabels).Set(state.TrackGrip)
	metrics.TrackTemperatureGauge.With(baseLabels).Set(state.TrackTemp)
	metrics.AirTemperatureGauge.With(baseLabels).Set(state.AirTemp)
	metrics.TickRateGauge.With(baseLabels).Set(state.TickRate)

	for _, player := range state.ConnectedPlayers {
		updatePlayerMetrics(player, baseLabels)
	}
}

// updatePlayerMetrics updates metrics related to individual players
func updatePlayerMetrics(player *types.Player, baseLabels map[string]string) {
	playerLabels := copyLabels(baseLabels)
	playerLabels["player_name"] = player.Name
	playerLabels["steam_id"] = player.SteamID

	metrics.PlayerLatencyGauge.With(playerLabels).Set(float64(player.Latency))
	metrics.PacketLossGauge.With(playerLabels).Set(player.PacketLoss)

	if player.BestLap > 0 {
		metrics.PlayerBestLapGauge.With(playerLabels).Set(float64(player.BestLap))
	}
}

// copyLabels creates and returns a copy of the provided labels
func copyLabels(labels map[string]string) map[string]string {
	newLabels := make(map[string]string)
	for k, v := range labels {
		newLabels[k] = v
	}
	return newLabels
}

// updateSystemMetrics updates system resource usage metrics
func updateSystemMetrics(state *types.ServerState) {
	labels := map[string]string{
		"server_id":   state.ServerID,
		"server_name": state.ServerName,
		"server_type": state.ServerType,
	}

	if cpu, err := getProcessCPUUsage(); err == nil {
		metrics.CpuUsageGauge.With(labels).Set(cpu)
	} else {
		utils.LogWarning("%v", err)
	}

	if mem, err := getProcessMemoryUsage(); err == nil {
		metrics.MemoryUsageGauge.With(labels).Set(float64(mem))
	} else {
		utils.LogWarning("%v", err)
	}
}

// getProcessCPUUsage returns the CPU usage of the current process as a percentage.
// It reads directly from /proc/self/stat.
func getProcessCPUUsage() (float64, error) {
	// Lire directement depuis /proc/self/stat
	data, err := os.ReadFile("/proc/self/stat")
	if err != nil {
		return 0, fmt.Errorf("failed to read CPU usage from /proc: %v", err)
	}

	fields := strings.Fields(string(data))
	if len(fields) < 14 {
		return 0, fmt.Errorf("invalid /proc/self/stat format")
	}

	utime, _ := strconv.ParseFloat(fields[13], 64)
	stime, _ := strconv.ParseFloat(fields[14], 64)

	return (utime + stime) / float64(os.Getpagesize()), nil
}

// getProcessMemoryUsage returns the memory usage of the current process in bytes.
// It reads directly from /proc/self/status.
func getProcessMemoryUsage() (uint64, error) {
	// Lire directement depuis /proc/self/status
	data, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0, fmt.Errorf("failed to read memory usage from /proc: %v", err)
	}

	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "VmRSS:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				memKB, err := strconv.ParseUint(fields[1], 10, 64)
				if err != nil {
					return 0, fmt.Errorf("failed to parse memory usage: %v", err)
				}
				return memKB * 1024, nil
			}
		}
	}

	return 0, fmt.Errorf("VmRSS not found in /proc/self/status")
}

// MonitorDetailedMetrics envoie régulièrement les métriques détaillées à VictoriaMetrics
func MonitorDetailedMetrics(ctx context.Context, vmClient *victoria.MetricsClient, state *types.ServerState) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	utils.LogInfo("Monitoring detailed metrics : [ OK ] - Interval: 10s")

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Collecter et envoyer les métriques détaillées sans log
			batch := collectMetrics(state)

			// Pas de log avant ou après l'envoi
			if err := vmClient.SendMetrics(batch); err != nil {
				utils.LogError("Failed to send detailed metrics: %v", err)
			}
		}
	}
}

// sessionTypeToValue converts a session type to a corresponding value
func sessionTypeToValue(sessionType string) float64 {
	switch sessionType {
	case "practice":
		return 1
	case "qualifying":
		return 2
	case "race":
		return 3
	default:
		return 0
	}
}
