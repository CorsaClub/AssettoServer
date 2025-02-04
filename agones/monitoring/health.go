// Package monitoring handles the monitoring and health checks of the server.
package monitoring

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	sdk "agones.dev/agones/sdks/go"
	"github.com/prometheus/client_golang/prometheus"

	"agones/metrics"
	"agones/types"
)

// DoHealth performs periodic health checks of the server.
// It pings the Agones SDK and updates relevant metrics based on the health status.
// If a health check fails, it initiates a graceful shutdown of the server.
func DoHealth(ctx context.Context, s *sdk.SDK, state *types.ServerState, cancel context.CancelFunc) {
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

			// Perform health check with Agones SDK
			if err := s.Health(); err != nil {
				log.Printf(">>> Agones health check failed: %v", err)

				// Increment the health ping failure counter
				metrics.HealthPingFailuresCounter.With(prometheus.Labels{
					"server_id":   state.ServerID,
					"server_name": state.ServerName,
					"server_type": state.ServerType,
				}).Inc()

				// Retrieve and log the GameServer state during health failure
				if gameServer, gsErr := s.GameServer(); gsErr == nil {
					log.Printf(">>> GameServer state during health failure: %v", gameServer.Status.State)
				}

				// Log the current system state
				log.Printf(">>> System state - Players: %d, Ready: %v", state.Players, state.Ready)

				// Initiate a graceful shutdown
				gracefulShutdown(s, cancel, state)
				return
			}

			// Update health metrics
			state.RLock()
			metrics.LastHealthPingGauge.With(prometheus.Labels{
				"server_id":   state.ServerID,
				"server_name": state.ServerName,
				"server_type": state.ServerType,
			}).Set(time.Since(state.LastPing).Seconds())
			state.RUnlock()

			// Log health status periodically every 30 seconds
			if time.Now().Second()%30 == 0 {
				state.RLock()
				log.Printf(">>> Health status: Ready=%v, LastPing=%v ago, ShuttingDown=%v",
					state.Ready,
					time.Since(state.LastPing),
					state.ShuttingDown)
				state.RUnlock()
			}
		}
	}
}

// MonitorMetrics monitors and updates the server's metrics periodically.
// It retrieves the GameServer status and updates annotations and detailed metrics.
func MonitorMetrics(ctx context.Context, s *sdk.SDK, state *types.ServerState) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			state.RLock()
			// Retrieve the current GameServer status from Agones
			if gameServer, err := s.GameServer(); err != nil {
				log.Printf(">>> Warning: Failed to get GameServer status: %v", err)
			} else {
				monitorGameServerState(gameServer)
				log.Printf(">>> GameServer Status: %v, Players: %d, Ready: %v, Last Ping: %v",
					gameServer.Status.State,
					state.Players,
					state.Ready,
					time.Since(state.LastPing).Seconds())

				// Update server annotations and metrics based on the current state
				updateServerAnnotations(s, state)
				updateMetrics(s, state)
				updateDetailedMetrics(s, state)
			}
			state.RUnlock()
		}
	}
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
				log.Println(">>> Skipping metrics update - too busy")
			}
		}
	}
}

// gracefulShutdown performs a graceful shutdown of the server by updating the state and notifying the SDK.
// It sets the ShuttingDown flag, sends a shutdown message to Agones, waits for a second, and then cancels the context.
func gracefulShutdown(s *sdk.SDK, cancel context.CancelFunc, state *types.ServerState) {
	state.Lock()
	state.ShuttingDown = true
	state.Unlock()

	if err := s.Shutdown(); err != nil {
		log.Printf(">>> Warning: Could not send shutdown message: %v", err)
	}
	time.Sleep(time.Second)
	cancel()
}

// monitorGameServerState logs detailed information about the GameServer's state.
// It is useful for debugging purposes.
func monitorGameServerState(gameServer interface{}) {
	log.Printf(">>> GameServer Details: %+v", gameServer)
}

// updateServerAnnotations updates the server's annotations with the current player count, readiness, and allocation status.
// Annotations are key-value pairs stored in Agones to provide additional information about the GameServer.
func updateServerAnnotations(s *sdk.SDK, state *types.ServerState) {
	annotations := map[string]string{
		"players":   fmt.Sprintf("%d", state.Players),
		"ready":     fmt.Sprintf("%v", state.Ready),
		"allocated": fmt.Sprintf("%v", state.Allocated),
	}

	for key, value := range annotations {
		if err := s.SetAnnotation(key, value); err != nil {
			log.Printf(">>> Warning: Failed to set %s annotation: %v", key, err)
		}
	}
}

// updateMetrics updates the basic metrics such as the number of players and session duration.
// It uses Prometheus labels to categorize the metrics.
func updateMetrics(s *sdk.SDK, state *types.ServerState) {
	labels := prometheus.Labels{
		"server_id":   state.ServerID,
		"server_name": state.ServerName,
		"server_type": state.ServerType,
	}

	metrics.PlayersGauge.With(labels).Set(float64(state.Players))
	if state.CurrentSession != nil {
		sessionLabels := prometheus.Labels{
			"server_id":    state.ServerID,
			"server_name":  state.ServerName,
			"server_type":  state.ServerType,
			"session_type": state.SessionType,
		}
		metrics.SessionDurationGauge.With(sessionLabels).Set(time.Since(state.SessionStart).Seconds())
	}
}

// updateDetailedMetrics updates more detailed metrics, including session time left, track conditions, and per-player metrics.
func updateDetailedMetrics(s *sdk.SDK, state *types.ServerState) {
	labels := prometheus.Labels{
		"server_id":   state.ServerID,
		"server_name": state.ServerName,
		"server_type": state.ServerType,
	}

	// Update session time left metric
	metrics.SessionTimeLeftGauge.With(labels).Set(float64(state.SessionTimeLeft))

	// Update track condition metrics
	metrics.TrackGripGauge.With(labels).Set(state.TrackGrip)
	metrics.TrackTemperatureGauge.With(labels).Set(state.TrackTemp)
	metrics.AirTemperatureGauge.With(labels).Set(state.AirTemp)
	metrics.TickRateGauge.With(labels).Set(state.TickRate)

	// Update per-player metrics
	for _, player := range state.ConnectedPlayers {
		updatePlayerMetrics(player, labels)
	}
}

// updatePlayerMetrics updates metrics related to individual players, such as latency and packet loss.
// It creates a separate set of labels for each player to track their specific metrics.
func updatePlayerMetrics(player *types.Player, baseLabels prometheus.Labels) {
	playerLabels := copyLabels(baseLabels)
	playerLabels["player_name"] = player.Name
	playerLabels["steam_id"] = player.SteamID

	metrics.PlayerLatencyGauge.With(playerLabels).Set(float64(player.Latency))
	metrics.PacketLossGauge.With(playerLabels).Set(player.PacketLoss)

	if player.BestLap > 0 {
		metrics.PlayerBestLapGauge.With(playerLabels).Set(float64(player.BestLap))
	}
}

// copyLabels creates and returns a copy of the provided Prometheus labels.
// This is useful to avoid mutating the original labels when adding new ones.
func copyLabels(labels prometheus.Labels) prometheus.Labels {
	newLabels := make(prometheus.Labels)
	for k, v := range labels {
		newLabels[k] = v
	}
	return newLabels
}

// updateSystemMetrics retrieves and updates system resource usage metrics such as CPU and Memory usage.
func updateSystemMetrics(state *types.ServerState) {
	labels := prometheus.Labels{
		"server_id":   state.ServerID,
		"server_name": state.ServerName,
		"server_type": state.ServerType,
	}

	// Retrieve and update CPU usage metric
	if cpu, err := getProcessCPUUsage(); err == nil {
		metrics.CpuUsageGauge.With(labels).Set(cpu)
	} else {
		log.Printf(">>> Warning: %v", err)
	}

	// Retrieve and update Memory usage metric
	if mem, err := getProcessMemoryUsage(); err == nil {
		metrics.MemoryUsageGauge.With(labels).Set(float64(mem))
	} else {
		log.Printf(">>> Warning: %v", err)
	}
}

// getProcessCPUUsage returns the CPU usage of the current process as a percentage.
// It executes the 'ps' command to retrieve the CPU usage.
func getProcessCPUUsage() (float64, error) {
	cmd := exec.Command("ps", "-p", fmt.Sprintf("%d", os.Getpid()), "-o", "%cpu")
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("failed to get CPU usage: %v", err)
	}

	// Parse the output, skipping the header line
	lines := strings.Split(string(output), "\n")
	if len(lines) < 2 {
		return 0, fmt.Errorf("unexpected ps output format")
	}

	cpu := strings.TrimSpace(lines[1])
	cpuUsage, err := strconv.ParseFloat(cpu, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse CPU usage: %v", err)
	}

	return cpuUsage, nil
}

// getProcessMemoryUsage returns the memory usage of the current process in bytes.
// It executes the 'ps' command to retrieve the RSS (Resident Set Size) and converts it from KB to bytes.
func getProcessMemoryUsage() (uint64, error) {
	cmd := exec.Command("ps", "-p", fmt.Sprintf("%d", os.Getpid()), "-o", "rss=")
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("failed to get memory usage: %v", err)
	}

	// Convert RSS from KB to bytes
	memKB, err := strconv.ParseUint(strings.TrimSpace(string(output)), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse memory usage: %v", err)
	}

	return memKB * 1024, nil
}
