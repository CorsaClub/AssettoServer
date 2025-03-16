package handlers

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"metrics/metrics"
	"metrics/types"
	"metrics/utils"
	"metrics/victoria"
)

// startNewSession initiates a new game session with the specified type and track.
func startNewSession(state *types.ServerState, sessionType, track string) {
	state.Lock()
	defer state.Unlock()

	state.CurrentSession = &types.Session{
		Type:      sessionType,
		StartTime: time.Now(),
		Track:     track,
	}
}

// handleSessionEnd handles the end of a game session by kicking all players and initiating a graceful shutdown.
func handleSessionEnd(vmClient *victoria.MetricsClient, state *types.ServerState, labels map[string]string, cancel context.CancelFunc) {
	state.Lock()
	if state.ShuttingDown {
		state.Unlock()
		return
	}
	state.ShuttingDown = true

	// Clear connected players on session end
	for steamID, player := range state.ConnectedPlayers {
		utils.LogSDK("Player %s (Steam ID: %s) disconnected due to session end", player.Name, steamID)
		delete(state.ConnectedPlayers, steamID)
	}
	state.Players = 0
	state.Unlock()

	utils.LogSDK("Session ended, initiating server shutdown")
	vmClient.SendMetrics(types.MetricBatch{
		Metrics: []types.Metric{
			{
				Name:        "assetto_server_state",
				Value:       float64(metrics.ServerStateShutdown),
				Type:        types.Gauge,
				LabelValues: labels,
				Timestamp:   time.Now(),
			},
		},
		Time: time.Now(),
	})
}

// handleSessionChange manages changes to the game session, such as switching tracks or session types.
func handleSessionChange(state *types.ServerState, output string, labels map[string]string) {
	sessionType := utils.ExtractSessionType(output)
	if sessionType == "" {
		return
	}

	oldSessionType := "unknown"
	if state.CurrentSession != nil {
		oldSessionType = state.CurrentSession.Type
	}

	// Update session state
	state.Lock()
	state.SessionType = sessionType
	if state.CurrentSession == nil {
		state.CurrentSession = &types.Session{
			Type:      sessionType,
			StartTime: time.Now(),
			ID:        fmt.Sprintf("session_%d", time.Now().Unix()),
		}
	} else {
		state.CurrentSession.Type = sessionType
	}
	state.Unlock()

	// Increment session change counter
	metrics.SessionChangeCounter.With(labels).Inc()

	// Increment session state changes counter
	metrics.SessionStateChangesCounter.With(map[string]string{
		"server_id":   labels["server_id"],
		"server_name": labels["server_name"],
		"server_type": labels["server_type"],
		"from_state":  oldSessionType,
		"to_state":    sessionType,
	}).Inc()

	// Increment session switch counter
	metrics.SessionSwitchCounter.With(map[string]string{
		"server_id":   labels["server_id"],
		"server_name": labels["server_name"],
		"server_type": labels["server_type"],
		"from_type":   oldSessionType,
		"to_type":     sessionType,
		"track":       state.CurrentTrack,
		"config":      state.CurrentLayout,
	}).Inc()

	// Update track usage counter
	if state.CurrentTrack != "" {
		metrics.TrackUsageCounter.With(map[string]string{
			"server_id":   labels["server_id"],
			"server_name": labels["server_name"],
			"server_type": labels["server_type"],
			"track_name":  state.CurrentTrack,
		}).Inc()
	}

	// Reset session players counter
	sessionLabels := map[string]string{
		"server_id":    labels["server_id"],
		"server_name":  labels["server_name"],
		"server_type":  labels["server_type"],
		"session_id":   state.CurrentSession.ID,
		"session_type": sessionType,
	}

	// Set initial session players count
	state.RLock()
	metrics.SessionPlayersCounter.With(sessionLabels).Add(float64(len(state.ConnectedPlayers)))
	state.RUnlock()

	// Log the session change
	logEvent("session_change", fmt.Sprintf("Session changed from %s to %s", oldSessionType, sessionType), state, map[string]string{
		"from_type": oldSessionType,
		"to_type":   sessionType,
		"track":     state.CurrentTrack,
		"config":    state.CurrentLayout,
	})
}

// handleSessionSwitch handles session switch-related events and updates metrics accordingly.
func handleSessionSwitch(output string, state *types.ServerState, labels map[string]string) {
	fromType := state.SessionType // ancien type
	toType := utils.ExtractSessionType(output)
	track := state.CurrentTrack
	config := state.CurrentLayout

	// Extract session ID if available
	sessionID := extractSessionID(output)

	// Incrémenter le compteur de changements
	metrics.SessionSwitchCounter.With(map[string]string{
		"server_id": labels["server_id"],
		"from_type": fromType,
		"to_type":   toType,
		"track":     track,
		"config":    config,
	}).Inc()

	// Mettre à jour le type de session dans l'état
	state.Lock()
	state.SessionType = toType
	if sessionID != "" && state.CurrentSession != nil {
		state.CurrentSession.ID = sessionID
	}
	state.Unlock()
}

// handleSessionTime handles session time-related events and updates metrics accordingly.
func handleSessionTime(output string, state *types.ServerState, labels map[string]string) {
	remainingTime := utils.ExtractSessionTime(output)
	sessionType := state.SessionType

	// Mettre à jour la gauge de temps restant
	metrics.SessionRemainingTimeGauge.With(map[string]string{
		"server_id":    labels["server_id"],
		"session_type": sessionType,
	}).Set(float64(remainingTime))
}

// handleLobbyRegistration processes lobby registration events and updates relevant metrics.
func handleLobbyRegistration(output string, state *types.ServerState, labels map[string]string) {
	// Extract registration details
	details := "unknown"
	if strings.Contains(output, "successful") {
		details = "success"
	} else if strings.Contains(output, "failed") {
		details = "failure"
	}

	// Increment lobby registration counter
	metrics.LobbyRegistrationCounter.With(labels).Inc()

	// Increment detailed lobby registration status counter
	metrics.LobbyRegistrationStatusCounter.With(map[string]string{
		"server_id":   labels["server_id"],
		"server_name": labels["server_name"],
		"server_type": labels["server_type"],
		"status":      details,
		"details":     output,
	}).Inc()

	// Log the lobby registration event
	logEvent("lobby_registration", fmt.Sprintf("Lobby registration: %s", details), state, map[string]string{
		"status":  details,
		"details": output,
	})
}

// handleUpdateLoop handles update loop-related events
func handleUpdateLoop(output string, _ *types.ServerState, labels map[string]string) {
	rate := strings.Split(output, "rate of")[1]
	metrics.ServerUpdateRateGauge.With(labels).Set(parseUpdateRate(rate))
}

// handleLobbySuccess handles lobby success-related events
func handleLobbySuccess(_ string, state *types.ServerState, labels map[string]string) {
	utils.LogInfo("Lobby registration successful")

	// Update server health metric to indicate server is healthy
	metrics.ServerHealth.With(labels).Set(1)

	// Update server state if needed
	state.Lock()
	state.Ready = true
	state.Unlock()
}

// extractSessionID extracts the session ID from the output string.
func extractSessionID(output string) string {
	return strings.TrimSpace(strings.Split(output, "id")[1])
}

// parseUpdateRate extracts and parses the update rate value
func parseUpdateRate(rate string) float64 {
	r := strings.TrimSpace(strings.Split(rate, "hz")[0])
	f, err := strconv.ParseFloat(r, 64)
	if err != nil {
		utils.LogWarning("Failed to parse update rate: %v", err)
		return 0
	}
	return f
}

// handleServerInvite handles server invite-related events
func handleServerInvite(output string, state *types.ServerState, labels map[string]string) {
	inviteURL := utils.ExtractInviteURL(output)
	urlHash := utils.HashString(inviteURL) // Pour éviter de stocker l'URL complète

	metrics.ServerInviteCounter.With(map[string]string{
		"server_id": labels["server_id"],
		"url_hash":  urlHash,
	}).Inc()
}

// handleTCPServer handles TCP server-related events
func handleTCPServer(output string, _ *types.ServerState, _ map[string]string) {
	port := strings.Split(output, "port")[1]
	//utils.LogSDK("Starting TCP server on port%s", port)
	metrics.ServerPortsGauge.With(map[string]string{
		"port_type": "tcp",
		"port":      strings.TrimSpace(port),
	}).Set(1)
}

// handleUDPServer handles UDP server-related events
func handleUDPServer(output string, _ *types.ServerState, _ map[string]string) {
	port := strings.Split(output, "port")[1]
	//utils.LogSDK("Starting UDP server on port%s", port)
	metrics.ServerPortsGauge.With(map[string]string{
		"port_type": "udp",
		"port":      strings.TrimSpace(port),
	}).Set(1)
}
