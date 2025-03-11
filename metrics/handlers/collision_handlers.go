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

// handleCollision processes collision events and updates relevant metrics.
func handleCollision(output string, state *types.ServerState, labels map[string]string) {
	// Extract collision information
	collisionType := "environment"
	if strings.Contains(output, "Collision between") && strings.Contains(output, "and") {
		collisionType = "car"
	}

	// Extract player information
	playerName := utils.ExtractName(output)
	steamID := utils.ExtractSteamID(output)

	// Extract speed if available
	speed := 0.0
	speedMatch := regexp.MustCompile(`speed (\d+\.?\d*)km/h`).FindStringSubmatch(output)
	if len(speedMatch) > 1 {
		speed, _ = strconv.ParseFloat(speedMatch[1], 64)
	}

	// Create collision labels
	collisionLabels := map[string]string{
		"server_id":      labels["server_id"],
		"server_name":    labels["server_name"],
		"server_type":    labels["server_type"],
		"collision_type": collisionType,
		"player_name":    playerName,
		"player_id":      steamID,
	}

	// Increment collision counter
	metrics.CollisionCounter.With(collisionLabels).Inc()

	// Increment incident counter
	metrics.IncidentCounter.With(map[string]string{
		"server_id":     labels["server_id"],
		"server_name":   labels["server_name"],
		"server_type":   labels["server_type"],
		"incident_type": "collision",
		"player_name":   playerName,
		"player_id":     steamID,
	}).Inc()

	// Increment session incidents counter
	if state.CurrentSession != nil {
		sessionLabels := map[string]string{
			"server_id":    labels["server_id"],
			"server_name":  labels["server_name"],
			"server_type":  labels["server_type"],
			"session_id":   state.CurrentSession.ID,
			"session_type": state.CurrentSession.Type,
		}
		metrics.SessionIncidentsCounter.With(sessionLabels).Inc()
	}

	// Log the collision event
	logEvent("collision", fmt.Sprintf("Collision detected: %s (Speed: %.1f km/h)", collisionType, speed), state, collisionLabels)
}
