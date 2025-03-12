// Package monitoring handles the monitoring and health checks of the server.
package monitoring

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"metrics/metrics"
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

				// Update health metric
				labels := map[string]string{
					"server_id":   state.ServerID,
					"server_name": state.ServerName,
					"server_type": state.ServerType,
				}

				batch := types.MetricBatch{
					Metrics: []types.Metric{
						metrics.ServerHealth.With(labels).Set(0),
					},
					Time: time.Now(),
				}

				// Send metrics
				err := vmClient.SendMetrics(batch)
				if err != nil {
					utils.LogError("Failed to send health metrics", err)
				}

				// Initiate graceful shutdown
				gracefulShutdown(cancel, state)
				return
			}

			// Update metrics
			updateMetrics(state)

			// Collect metrics
			batch := collectMetrics(state)

			// Send metrics
			err := vmClient.SendMetrics(batch)
			if err != nil {
				utils.LogError("Failed to send metrics", err)
			}
		}
	}
}

// isHealthy checks if the server is healthy
func isHealthy(state *types.ServerState) bool {
	state.RLock()
	defer state.RUnlock()

	// Check if the server is ready
	if !state.Ready {
		return false
	}

	// Check if the last ping was recent enough
	if time.Since(state.LastPing) > 10*time.Second {
		return false
	}

	// Check if the server is shutting down
	if state.ShuttingDown {
		return false
	}

	// Additional health checks can be added here

	return true
}

// collectMetrics collects metrics from the server state
func collectMetrics(state *types.ServerState) types.MetricBatch {
	state.RLock()
	defer state.RUnlock()

	// Create base labels
	baseLabels := map[string]string{
		"server_id":   state.ServerID,
		"server_name": state.ServerName,
		"server_type": state.ServerType,
	}

	// Create metrics batch
	batch := types.MetricBatch{
		Metrics: []types.Metric{
			// Server state metrics
			{
				Name:        metrics.ServerStateGauge.Name,
				Value:       float64(types.ServerStateReady),
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: baseLabels,
			},
			// Player count metrics
			{
				Name:        metrics.PlayersGauge.Name,
				Value:       float64(state.Players),
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: baseLabels,
			},
			// Server uptime metrics
			{
				Name:        metrics.ServerUptime.Name,
				Value:       time.Since(state.StartTime).Seconds(),
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: baseLabels,
			},
		},
		Time: time.Now(),
	}

	// Add session metrics if available
	if state.CurrentSession != nil {
		// Add session type to labels
		sessionLabels := copyLabels(baseLabels)
		sessionLabels["session_type"] = state.CurrentSession.Type
		sessionLabels["track"] = state.CurrentSession.Track

		// Add session metrics
		batch.Metrics = append(batch.Metrics, types.Metric{
			Name:        metrics.SessionDurationGauge.Name,
			Value:       time.Since(state.CurrentSession.StartTime).Seconds(),
			Type:        types.Gauge,
			Timestamp:   time.Now(),
			LabelValues: sessionLabels,
		})

		// Add track metrics if available
		if state.TrackGrip > 0 {
			batch.Metrics = append(batch.Metrics, types.Metric{
				Name:        metrics.TrackGrip.Name,
				Value:       state.TrackGrip,
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: baseLabels,
			})
		}

		if state.TrackTemp > 0 {
			batch.Metrics = append(batch.Metrics, types.Metric{
				Name:        metrics.TrackTemperature.Name,
				Value:       state.TrackTemp,
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: baseLabels,
			})
		}

		if state.AirTemp > 0 {
			batch.Metrics = append(batch.Metrics, types.Metric{
				Name:        metrics.AirTemperature.Name,
				Value:       state.AirTemp,
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: baseLabels,
			})
		}
	}

	// Add session time metrics if available
	if state.SessionTimeLeft > 0 {
		batch.Metrics = append(batch.Metrics, types.Metric{
			Name:        metrics.SessionTimeLeftGauge.Name,
			Value:       float64(state.SessionTimeLeft),
			Type:        types.Gauge,
			Timestamp:   time.Now(),
			LabelValues: baseLabels,
		})
	}

	return batch
}

// MonitorSystemResources monitors system resources like CPU, memory, etc.
func MonitorSystemResources(ctx context.Context, state *types.ServerState) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Update system metrics
			updateSystemMetrics(state)
		}
	}
}

// gracefulShutdown initiates a graceful shutdown of the server
func gracefulShutdown(cancel context.CancelFunc, state *types.ServerState) {
	if logsClient, ok := utils.GetLogsClient(); ok {
		logsClient.LogEvent("INFO", "Initiating graceful shutdown", "shutdown", nil)
	}

	// Update server state
	state.Lock()
	state.Ready = false
	state.Unlock()

	// Cancel the context to signal shutdown
	cancel()
}

// monitorGameServerState monitors the state of the game server
func monitorGameServerState(gameServer interface{}) {
	// This is a placeholder for future implementation
	// It will monitor the state of the game server and update metrics accordingly
}

// updateMetrics updates metrics based on the server state
func updateMetrics(state *types.ServerState) {
	state.RLock()
	defer state.RUnlock()

	// Update server state metrics
	metrics.ServerStateGauge.With(map[string]string{
		"server_id":   state.ServerID,
		"server_name": state.ServerName,
		"server_type": state.ServerType,
	}).Set(float64(types.ServerStateReady))

	// Update player count metrics
	metrics.PlayersGauge.With(map[string]string{
		"server_id":   state.ServerID,
		"server_name": state.ServerName,
		"server_type": state.ServerType,
	}).Set(float64(state.Players))

	// Update server uptime metrics
	metrics.ServerUptime.With(map[string]string{
		"server_id":   state.ServerID,
		"server_name": state.ServerName,
		"server_type": state.ServerType,
	}).Set(time.Since(state.StartTime).Seconds())
}

// updateDetailedMetrics updates detailed metrics
func updateDetailedMetrics(state *types.ServerState) {
	state.RLock()
	defer state.RUnlock()

	// Base labels
	baseLabels := map[string]string{
		"server_id":   state.ServerID,
		"server_name": state.ServerName,
		"server_type": state.ServerType,
	}

	// Update player metrics
	for id, player := range state.ConnectedPlayers {
		// Create player-specific labels
		playerLabels := copyLabels(baseLabels)
		playerLabels["player_id"] = id
		playerLabels["player_name"] = player.Name

		// Update player-specific metrics
		updatePlayerMetrics(player, playerLabels)
	}
}

// updatePlayerMetrics updates player-specific metrics
func updatePlayerMetrics(player *types.Player, baseLabels map[string]string) {
	// Add car-specific labels
	labels := copyLabels(baseLabels)
	labels["car_model"] = player.CarModel

	// TODO: Implement player-specific metrics
}

// copyLabels creates a copy of a labels map
func copyLabels(labels map[string]string) map[string]string {
	copy := make(map[string]string)
	for k, v := range labels {
		copy[k] = v
	}
	return copy
}

// updateSystemMetrics updates system metrics
func updateSystemMetrics(state *types.ServerState) {
	// Get CPU usage
	cpuUsage, err := getProcessCPUUsage()
	if err != nil {
		utils.LogError("Failed to get CPU usage", err)
	}

	// Get memory usage
	memUsage, err := getProcessMemoryUsage()
	if err != nil {
		utils.LogError("Failed to get memory usage", err)
	}

	// Update metrics
	metrics.CpuUsageGauge.With(map[string]string{
		"server_id":   state.ServerID,
		"server_name": state.ServerName,
		"server_type": state.ServerType,
	}).Set(cpuUsage)

	metrics.MemoryUsageGauge.With(map[string]string{
		"server_id":   state.ServerID,
		"server_name": state.ServerName,
		"server_type": state.ServerType,
	}).Set(float64(memUsage))
}

// getProcessCPUUsage gets the CPU usage of the current process
func getProcessCPUUsage() (float64, error) {
	// This is a simplified implementation
	// In a real-world scenario, you would use a library like gopsutil
	// to get accurate CPU usage

	// For now, we'll parse /proc/self/stat on Linux
	// or use a placeholder value on other platforms
	if _, err := os.Stat("/proc/self/stat"); os.IsNotExist(err) {
		// Not on Linux, return a placeholder
		return 0.0, nil
	}

	// Read /proc/self/stat
	data, err := os.ReadFile("/proc/self/stat")
	if err != nil {
		return 0.0, err
	}

	// Parse the stat file
	fields := strings.Fields(string(data))
	if len(fields) < 15 {
		return 0.0, fmt.Errorf("invalid stat file format")
	}

	// Extract utime and stime
	utime, err := strconv.ParseFloat(fields[13], 64)
	if err != nil {
		return 0.0, err
	}

	stime, err := strconv.ParseFloat(fields[14], 64)
	if err != nil {
		return 0.0, err
	}

	// Calculate CPU usage
	// This is a simplified calculation
	// In a real-world scenario, you would need to account for
	// the number of cores and the time elapsed
	return (utime + stime) / 100.0, nil
}

// getProcessMemoryUsage gets the memory usage of the current process
func getProcessMemoryUsage() (uint64, error) {
	// This is a simplified implementation
	// In a real-world scenario, you would use a library like gopsutil
	// to get accurate memory usage

	// For now, we'll parse /proc/self/status on Linux
	// or use a placeholder value on other platforms
	if _, err := os.Stat("/proc/self/status"); os.IsNotExist(err) {
		// Not on Linux, return a placeholder
		return 0, nil
	}

	// Read /proc/self/status
	data, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0, err
	}

	// Parse the status file
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "VmRSS:") {
			// Extract the memory usage
			fields := strings.Fields(line)
			if len(fields) < 2 {
				return 0, fmt.Errorf("invalid status file format")
			}

			// Parse the memory usage
			memUsage, err := strconv.ParseUint(fields[1], 10, 64)
			if err != nil {
				return 0, err
			}

			// Convert from KB to bytes
			return memUsage * 1024, nil
		}
	}

	return 0, fmt.Errorf("memory usage not found in status file")
}

// MonitorDetailedMetrics monitors detailed metrics
func MonitorDetailedMetrics(ctx context.Context, vmClient *victoria.MetricsClient, state *types.ServerState) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Update detailed metrics
			updateDetailedMetrics(state)

			// Collect system metrics
			cpuUsage, _ := getProcessCPUUsage()
			memUsage, _ := getProcessMemoryUsage()

			// Create base labels
			baseLabels := map[string]string{
				"server_id":   state.ServerID,
				"server_name": state.ServerName,
				"server_type": state.ServerType,
			}

			// Create metrics batch
			batch := types.MetricBatch{
				Metrics: []types.Metric{
					{
						Name:        metrics.CpuUsageGauge.Name,
						Value:       cpuUsage,
						Type:        types.Gauge,
						Timestamp:   time.Now(),
						LabelValues: baseLabels,
					},
					{
						Name:        metrics.MemoryUsageGauge.Name,
						Value:       float64(memUsage),
						Type:        types.Gauge,
						Timestamp:   time.Now(),
						LabelValues: baseLabels,
					},
					{
						Name:        metrics.ServerUpdateRateGauge.Name,
						Value:       state.TickRate,
						Type:        types.Gauge,
						Timestamp:   time.Now(),
						LabelValues: baseLabels,
					},
				},
				Time: time.Now(),
			}

			// Send metrics
			err := vmClient.SendMetrics(batch)
			if err != nil {
				utils.LogError("Failed to send detailed metrics", err)
			}
		}
	}
}

// sessionTypeToValue converts a session type string to a numeric value
func sessionTypeToValue(sessionType string) float64 {
	switch sessionType {
	case types.SessionTypePractice:
		return 0
	case types.SessionTypeQualifying:
		return 1
	case types.SessionTypeRace:
		return 2
	default:
		return -1
	}
}

// MonitorSessionMetrics monitors session-specific metrics
func MonitorSessionMetrics(ctx context.Context, vmClient *victoria.MetricsClient, state *types.ServerState) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			state.RLock()
			if state.CurrentSession != nil {
				// Create base labels
				baseLabels := map[string]string{
					"server_id":    state.ServerID,
					"server_name":  state.ServerName,
					"server_type":  state.ServerType,
					"session_type": state.CurrentSession.Type,
					"track":        state.CurrentSession.Track,
				}

				// Create metrics batch
				batch := types.MetricBatch{
					Metrics: []types.Metric{
						{
							Name:        metrics.SessionDurationGauge.Name,
							Value:       time.Since(state.CurrentSession.StartTime).Seconds(),
							Type:        types.Gauge,
							Timestamp:   time.Now(),
							LabelValues: baseLabels,
						},
					},
					Time: time.Now(),
				}

				// Add time left metric if available
				if state.SessionTimeLeft > 0 {
					batch.Metrics = append(batch.Metrics, types.Metric{
						Name:        metrics.SessionTimeLeftGauge.Name,
						Value:       float64(state.SessionTimeLeft),
						Type:        types.Gauge,
						Timestamp:   time.Now(),
						LabelValues: baseLabels,
					})
				}

				state.RUnlock()

				// Send metrics
				err := vmClient.SendMetrics(batch)
				if err != nil {
					utils.LogError("Failed to send session metrics", err)
				}
			} else {
				state.RUnlock()
			}
		}
	}
}
