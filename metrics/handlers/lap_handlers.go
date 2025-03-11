package handlers

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"metrics/metrics"
	"metrics/types"
	"metrics/utils"
)

// handleLap processes lap completion events and updates relevant metrics.
func handleLap(output string, state *types.ServerState, labels map[string]string) {
	// Extract player information
	playerName := utils.ExtractName(output)
	steamID := utils.ExtractSteamID(output)

	// Extract lap time if available
	lapTimeMs := int64(0)
	lapTimeMatch := regexp.MustCompile(`LAP (\d+:\d+\.\d+)`).FindStringSubmatch(output)
	if len(lapTimeMatch) > 1 {
		// Convert lap time format (e.g., "1:23.456") to milliseconds
		parts := strings.Split(lapTimeMatch[1], ":")
		if len(parts) == 2 {
			minutes, _ := strconv.ParseInt(parts[0], 10, 64)
			secondsParts := strings.Split(parts[1], ".")
			seconds, _ := strconv.ParseInt(secondsParts[0], 10, 64)
			milliseconds := int64(0)
			if len(secondsParts) > 1 {
				// Handle milliseconds, padding if necessary
				msStr := secondsParts[1]
				for len(msStr) < 3 {
					msStr += "0"
				}
				milliseconds, _ = strconv.ParseInt(msStr[:3], 10, 64)
			}

			lapTimeMs = (minutes * 60 * 1000) + (seconds * 1000) + milliseconds
		}
	}

	// Create lap labels
	lapLabels := map[string]string{
		"server_id":   labels["server_id"],
		"server_name": labels["server_name"],
		"server_type": labels["server_type"],
		"player_name": playerName,
		"player_id":   steamID,
	}

	// Add car model and track if available
	if state.CurrentSession != nil {
		lapLabels["track"] = state.CurrentTrack

		// Find car model for the player
		state.RLock()
		if player, ok := state.ConnectedPlayers[steamID]; ok {
			lapLabels["car_model"] = player.CarModel
		}
		state.RUnlock()
	}

	// Record lap time in histogram
	if lapTimeMs > 0 {
		metrics.LapTimeHistogram.With(lapLabels).Observe(float64(lapTimeMs) / 1000.0) // Convert to seconds
	}

	// Increment session laps counter
	if state.CurrentSession != nil {
		sessionLabels := map[string]string{
			"server_id":    labels["server_id"],
			"server_name":  labels["server_name"],
			"server_type":  labels["server_type"],
			"session_id":   state.CurrentSession.ID,
			"session_type": state.CurrentSession.Type,
		}
		metrics.SessionLapsCounter.With(sessionLabels).Inc()
	}

	// Log the lap event
	logEvent("lap_completed", fmt.Sprintf("Lap completed by %s (Time: %d ms)", playerName, lapTimeMs), state, lapLabels)
}
