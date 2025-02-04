// Package handlers manages interactions with the Assetto Corsa server
package handlers

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	sdk "agones.dev/agones/sdks/go"
	"github.com/prometheus/client_golang/prometheus"

	"agones/metrics"
	"agones/types"
	"agones/utils"
)

// HandleServerOutput processes server output and updates metrics.
// It handles various server events based on the output string.
func HandleServerOutput(output string, s *sdk.SDK, state *types.ServerState, serverReady chan struct{}, cancel context.CancelFunc) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf(">>> Recovered from panic in HandleServerOutput: %v", r)
			// Notify metrics of a critical error
			metrics.ServerErrorsCounter.With(prometheus.Labels{
				"server_id":   state.ServerID,
				"server_name": state.ServerName,
				"server_type": state.ServerType,
				"error_type":  "panic",
			}).Inc()
		}
	}()

	// Validate input length to prevent excessive memory usage
	if len(output) > 8192 { // Limit input size
		log.Printf(">>> Warning: Large output received (%d bytes)", len(output))
		output = output[:8192]
	}

	if output == "" {
		return
	}

	// Common labels for all metrics
	baseLabels := prometheus.Labels{
		"server_id":   state.ServerID,
		"server_name": state.ServerName,
		"server_type": state.ServerType,
	}

	switch {
	case strings.Contains(output, "Starting Assetto Corsa Server..."):
		handleServerStarting(state, baseLabels)
	case strings.Contains(output, "Lobby registration successful"):
		handleServerReady(state, baseLabels, serverReady)
	case strings.Contains(output, "End of session"):
		handleSessionEnd(s, state, baseLabels, cancel)
	case strings.Contains(output, "has connected"):
		handlePlayerConnect(s, state, output, baseLabels)
	case strings.Contains(output, "has disconnected"):
		handlePlayerDisconnect(s, state, output, baseLabels)
	case strings.Contains(output, "Next session:"):
		handleSessionChange(state, output, baseLabels)
	case strings.Contains(output, "[ERR]"):
		handleError(fmt.Errorf(output), "server_error", state, baseLabels)
	case strings.Contains(output, "Steam authentication succeeded"):
		handleSteamAuth(state, baseLabels)
	case strings.Contains(output, "Network stats"):
		handleNetworkStats(output, baseLabels)
	case strings.Contains(output, "steamclient.so") || strings.Contains(output, "SteamAPI"):
		handleSteamError(output, state, baseLabels)
	case strings.Contains(output, "AssettoServer"):
		handleServerVersion(output, state, baseLabels)
	case strings.Contains(output, "Loading") && strings.Contains(output, ".ini"):
		handleConfigLoading(output, state, baseLabels)
	case strings.Contains(output, "Loaded plugin"):
		handlePluginLoading(output, state, baseLabels)
	case strings.Contains(output, "AI Slot"):
		handleAISlotUpdate(output, state, baseLabels)
	case strings.Contains(output, "Added checksum"):
		handleChecksumUpdate(output, state, baseLabels)
	default:
		log.Printf(">>> Unhandled server output: %s", output)
	}
}

// StartNewSession initiates a new game session with the specified type and track.
func StartNewSession(state *types.ServerState, sessionType, track string) {
	state.Lock()
	defer state.Unlock()

	state.CurrentSession = &types.Session{
		Type:      sessionType,
		StartTime: time.Now(),
		Track:     track,
	}
}

// handleServerStarting manages the server startup process and updates metrics accordingly.
func handleServerStarting(state *types.ServerState, labels prometheus.Labels) {
	log.Println(">>> Server starting up...")
	metrics.ServerStateGauge.With(labels).Set(types.ServerStateStarting) // Starting state
}

// handleServerReady updates the server state to ready and signals readiness.
func handleServerReady(state *types.ServerState, labels prometheus.Labels, serverReady chan struct{}) {
	log.Println(">>> Server is ready")
	state.Lock()
	state.Ready = true
	state.Unlock()
	metrics.ServerStateGauge.With(labels).Set(types.ServerStateReady) // Ready state
	serverReady <- struct{}{}
}

// handleSessionEnd handles the end of a game session by initiating a graceful shutdown.
func handleSessionEnd(s *sdk.SDK, state *types.ServerState, labels prometheus.Labels, cancel context.CancelFunc) {
	log.Println(">>> Session ended, initiating server shutdown")
	metrics.ServerStateGauge.With(labels).Set(types.ServerStateShutdown) // Shutdown state
	gracefulShutdown(s, cancel, state)
}

// handlePlayerConnect processes a player's connection, updates player counts, and increments relevant metrics.
func handlePlayerConnect(s *sdk.SDK, state *types.ServerState, output string, labels prometheus.Labels) {
	player := utils.ExtractPlayerInfo(output)
	addPlayer(state, player)

	metrics.PlayersGauge.With(labels).Set(float64(state.Players))
	metrics.PlayerConnectCounter.With(labels).Inc()
	updatePlayerCount(s, state.Players)

	// Increment the car usage counter based on the player's car model
	carLabels := copyLabels(labels)
	carLabels["car_name"] = player.CarModel
	metrics.CarUsageCounter.With(carLabels).Inc()
}

// handlePlayerDisconnect processes a player's disconnection and updates relevant metrics.
func handlePlayerDisconnect(s *sdk.SDK, state *types.ServerState, output string, labels prometheus.Labels) {
	steamID := utils.ExtractSteamID(output)
	removePlayer(state, steamID)

	metrics.PlayersGauge.With(labels).Set(float64(state.Players))
	metrics.PlayerDisconnectCounter.With(labels).Inc()
	updatePlayerCount(s, state.Players)
}

// handleSessionChange manages changes to the game session, such as switching tracks or session types.
func handleSessionChange(state *types.ServerState, output string, labels prometheus.Labels) {
	logEvent("SESSION_CHANGE", "Session change detected", state)
	sessionType := utils.ExtractSessionType(output)
	track := utils.ExtractTrackName(output)
	StartNewSession(state, sessionType, track)

	metrics.SessionChangeCounter.With(labels).Inc()

	trackLabels := copyLabels(labels)
	trackLabels["track_name"] = track
	metrics.TrackUsageCounter.With(trackLabels).Inc()
}

// handleSteamAuth records successful Steam authentication events.
func handleSteamAuth(state *types.ServerState, labels prometheus.Labels) {
	log.Println(">>> Steam authentication successful for player")
	metrics.AuthSuccessCounter.With(labels).Inc()
}

// handleNetworkStats updates network-related metrics based on the server output.
func handleNetworkStats(output string, labels prometheus.Labels) {
	if bytesReceived := utils.ExtractBytesReceived(output); bytesReceived > 0 {
		metrics.NetworkBytesReceivedCounter.With(labels).Add(float64(bytesReceived))
	}
	if bytesSent := utils.ExtractBytesSent(output); bytesSent > 0 {
		metrics.NetworkBytesSentCounter.With(labels).Add(float64(bytesSent))
	}
}

// handleError logs server errors and updates the error metrics accordingly.
func handleError(err error, errorType string, state *types.ServerState, labels prometheus.Labels) {
	log.Printf(">>> Error (%s): %v", errorType, err)
	errorLabels := copyLabels(labels)
	errorLabels["error_type"] = errorType
	metrics.ServerErrorsCounter.With(errorLabels).Inc()
}

// copyLabels creates and returns a copy of the provided Prometheus labels.
func copyLabels(labels prometheus.Labels) prometheus.Labels {
	newLabels := make(prometheus.Labels)
	for k, v := range labels {
		newLabels[k] = v
	}
	return newLabels
}

// updatePlayerCount updates the player count annotation in the SDK.
func updatePlayerCount(s *sdk.SDK, count int) {
	if err := s.SetAnnotation("players", fmt.Sprintf("%d", count)); err != nil {
		log.Printf(">>> Warning: Failed to update players annotation: %v", err)
	}
}

// logEvent logs an event with contextual information about the server state.
func logEvent(eventType string, message string, state *types.ServerState) {
	sessionType := "unknown"
	if state.CurrentSession != nil {
		sessionType = state.CurrentSession.Type
	}

	log.Printf("[%s] %s | Server: %s | Players: %d | Session: %s",
		eventType,
		message,
		state.ServerName,
		state.Players,
		sessionType)
}

// addPlayer adds a new player to the server's state and increments the player count.
func addPlayer(state *types.ServerState, player types.Player) {
	state.Lock()
	defer state.Unlock()

	state.ConnectedPlayers[player.SteamID] = &player
	state.Players++
}

// removePlayer removes a player from the server's state and decrements the player count.
func removePlayer(state *types.ServerState, steamID string) {
	state.Lock()
	defer state.Unlock()

	delete(state.ConnectedPlayers, steamID)
	if state.Players > 0 {
		state.Players--
	}
}

// gracefulShutdown performs a graceful shutdown of the server by updating the state and notifying the SDK.
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

// handleSteamError handles Steam-related errors and updates the error metrics accordingly.
func handleSteamError(output string, state *types.ServerState, labels prometheus.Labels) {
	if strings.Contains(output, "SteamAPI_Init") || strings.Contains(output, "steamclient.so") {
		log.Printf(">>> Steam initialization warning: %s", output)
		metrics.ServerErrorsCounter.With(prometheus.Labels{
			"server_id":   state.ServerID,
			"server_name": state.ServerName,
			"server_type": state.ServerType,
			"error_type":  "steam_init",
		}).Inc()
	}
}

// handleServerVersion handles server version-related events and updates metrics accordingly.
func handleServerVersion(output string, state *types.ServerState, labels prometheus.Labels) {
	version := extractVersion(output)
	log.Printf(">>> Server version: %s", version)
	// Optionnel : stocker la version dans l'état ou les métriques
}

// handleConfigLoading handles server configuration loading-related events and updates metrics accordingly.
func handleConfigLoading(output string, state *types.ServerState, labels prometheus.Labels) {
	configFile := extractConfigFile(output)
	log.Printf(">>> Loading config: %s", configFile)
	metrics.ServerErrorsCounter.With(labels).Inc()
}

// handlePluginLoading handles server plugin loading-related events and updates metrics accordingly.
func handlePluginLoading(output string, state *types.ServerState, labels prometheus.Labels) {
	plugin := extractPluginName(output)
	log.Printf(">>> Loaded plugin: %s", plugin)
}

// handleAISlotUpdate handles server AI slot update-related events and updates metrics accordingly.
func handleAISlotUpdate(output string, state *types.ServerState, labels prometheus.Labels) {
	// Extraire et mettre à jour les informations sur les slots AI
	slots := extractAISlots(output)
	state.Lock()
	state.ActiveCars = slots // Utiliser la variable slots
	state.Unlock()
}

// handleChecksumUpdate handles server checksum update-related events and updates metrics accordingly.
func handleChecksumUpdate(output string, state *types.ServerState, labels prometheus.Labels) {
	// Gérer les mises à jour de checksum
	asset := extractChecksumAsset(output)
	log.Printf(">>> Updated checksum for: %s", asset)
}

// extractVersion extracts the server version from the output string.
func extractVersion(output string) string {
	// Extraire la version du serveur
	return strings.TrimSpace(strings.Split(output, "AssettoServer")[1])
}

// extractConfigFile extracts the configuration file name from the output string.
func extractConfigFile(output string) string {
	// Extraire le nom du fichier de configuration
	return strings.TrimSpace(strings.Split(output, "Loading")[1])
}

// extractPluginName extracts the plugin name from the output string.
func extractPluginName(output string) string {
	// Extraire le nom du plugin
	return strings.TrimSpace(strings.Split(output, "Loaded plugin")[1])
}

// extractAISlots extracts AI slot information from the output string.
func extractAISlots(output string) map[string]int {
	// Extraire les informations sur les slots AI
	slots := make(map[string]int)
	// Parser la chaîne et remplir la map
	return slots
}

// extractChecksumAsset extracts the asset name from the output string.
func extractChecksumAsset(output string) string {
	// Extraire le nom de l'asset dont le checksum est mis à jour
	return strings.TrimSpace(strings.Split(output, "Added checksum for")[1])
}
