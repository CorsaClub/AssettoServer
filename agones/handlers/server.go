// Package handlers manages interactions with the Assetto Corsa server
package handlers

import (
	"context"
	"fmt"
	"strconv"
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	defer func() {
		if r := recover(); r != nil {
			utils.LogError("Recovered from panic in HandleServerOutput: %v", r)
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
		utils.LogWarning("Large output received (%d bytes)", len(output))
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

	select {
	case <-ctx.Done():
		utils.LogWarning("Timeout while processing server output")
		return
	default:
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
		case strings.Contains(output, "Server invite link:"):
			handleServerInvite(output, state, baseLabels)
		case strings.Contains(output, "Switching session to id"):
			handleSessionSwitch(output, state, baseLabels)
		case strings.Contains(output, "Starting TCP server"):
			handleTCPServer(output, state, baseLabels)
		case strings.Contains(output, "Starting UDP server"):
			handleUDPServer(output, state, baseLabels)
		case strings.Contains(output, "Remaining time of session"):
			handleSessionTime(output, state, baseLabels)
		case strings.Contains(output, "Registering server to lobby"):
			handleLobbyRegistration(output, state, baseLabels)
		case strings.Contains(output, "Starting update loop"):
			handleUpdateLoop(output, state, baseLabels)
		case strings.Contains(output, "Lobby registration successful"):
			handleLobbySuccess(output, state, baseLabels)
		case strings.Contains(output, "Loading extra_cfg.yml"):
			handleConfigLoading(output, state, baseLabels)
		case strings.Contains(output, "Using minimum required CSP Version"):
			handleCSPVersion(output, state, baseLabels)
		case strings.Contains(output, "Cached AI spline"):
			handleAISpline(output, state, baseLabels)
		case strings.Contains(output, "Adjacent lane detection"):
			handleAILaneDetection(output, state, baseLabels)
		case strings.Contains(output, "Writing cached AI spline"):
			handleAISplineCache(output, state, baseLabels)
		case strings.Contains(output, "Mapping cached AI spline"):
			handleAISplineMapping(output, state, baseLabels)
		case strings.Contains(output, "Storing keys in a directory"):
			handleKeysStorage(output, state, baseLabels)
		case strings.Contains(output, "No XML encryptor configured"):
			handleXMLEncryption(output, state, baseLabels)
		case strings.Contains(output, "Loaded blacklist.txt"):
			handleBlacklistLoading(output, state, baseLabels)
		case strings.Contains(output, "Loaded whitelist.txt"):
			handleWhitelistLoading(output, state, baseLabels)
		case strings.Contains(output, "Loaded admins.txt"):
			handleAdminsLoading(output, state, baseLabels)
		case strings.Contains(output, "Connected to Steam Servers"):
			handleSteamConnection(output, state, baseLabels)
		default:
			utils.LogSDK("Unhandled server output: %s", output)
		}
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
	utils.LogSDK("Server starting up...")
	state.Lock()
	state.Ready = false
	state.ShuttingDown = false
	state.Unlock()
	metrics.ServerStateGauge.With(labels).Set(types.ServerStateStarting)
	metrics.ServerStartCounter.With(labels).Inc()
}

// handleServerReady updates the server state to ready and signals readiness.
func handleServerReady(state *types.ServerState, labels prometheus.Labels, serverReady chan struct{}) {
	state.Lock()
	if state.Ready {
		state.Unlock()
		return
	}
	state.Ready = true
	state.Unlock()

	utils.LogSDK("Server is ready")
	metrics.ServerStateGauge.With(labels).Set(types.ServerStateReady)

	select {
	case serverReady <- struct{}{}:
	default:
		utils.LogWarning("Server ready signal dropped - channel full")
	}
}

// handleSessionEnd handles the end of a game session by initiating a graceful shutdown.
func handleSessionEnd(s *sdk.SDK, state *types.ServerState, labels prometheus.Labels, cancel context.CancelFunc) {
	state.Lock()
	if state.ShuttingDown {
		state.Unlock()
		return
	}
	state.ShuttingDown = true
	state.Unlock()

	utils.LogSDK("Session ended, initiating server shutdown")
	metrics.ServerStateGauge.With(labels).Set(types.ServerStateShutdown)
	metrics.SessionEndCounter.With(labels).Inc()
	gracefulShutdown(s, cancel, state)
}

// handlePlayerConnect processes a player's connection, updates player counts, and increments relevant metrics.
func handlePlayerConnect(s *sdk.SDK, state *types.ServerState, output string, labels prometheus.Labels) {
	player := utils.ExtractPlayerInfo(output)
	if player.SteamID == "" {
		utils.LogWarning("Invalid player info from output: %s", output)
		return
	}

	addPlayer(state, player)

	metrics.PlayersGauge.With(labels).Set(float64(state.Players))
	metrics.PlayerConnectCounter.With(labels).Inc()
	metrics.PlayerLatencyGauge.With(prometheus.Labels{
		"player_id": player.SteamID,
		"server_id": labels["server_id"],
	}).Set(float64(player.Latency))
	updatePlayerCount(s, state.Players)

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

	utils.LogSDK("Player disconnected: %s", steamID)
}

// handleSessionChange manages changes to the game session, such as switching tracks or session types.
func handleSessionChange(state *types.ServerState, output string, labels prometheus.Labels) {
	logEvent("SESSION_CHANGE", "Session change detected", state)
	sessionType := utils.ExtractSessionType(output)
	track := utils.ExtractTrackName(output)

	if sessionType == "" || track == "" {
		utils.LogWarning("Invalid session info from output: %s", output)
		return
	}

	state.Lock()
	oldSession := state.CurrentSession
	state.Unlock()

	StartNewSession(state, sessionType, track)

	if oldSession != nil {
		sessionDuration := time.Since(oldSession.StartTime)
		metrics.SessionDurationHistogram.With(prometheus.Labels{
			"session_type": oldSession.Type,
			"track":        oldSession.Track,
		}).Observe(sessionDuration.Seconds())
	}

	metrics.SessionChangeCounter.With(labels).Inc()
	trackLabels := copyLabels(labels)
	trackLabels["track_name"] = track
	metrics.TrackUsageCounter.With(trackLabels).Inc()
}

// handleSteamAuth records successful Steam authentication events.
func handleSteamAuth(_ *types.ServerState, labels prometheus.Labels) {
	utils.LogSDK("Steam authentication successful for player")
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

	utils.LogSDK("Network stats update: %s", output)
}

// handleError logs server errors and updates the error metrics accordingly.
func handleError(err error, errorType string, _ *types.ServerState, labels prometheus.Labels) {
	utils.LogError("(%s): %v", errorType, err)
	errorLabels := copyLabels(labels)
	errorLabels["error_type"] = errorType
	metrics.ServerErrorsCounter.With(errorLabels).Inc()

	utils.LogError("Server error: %v", err)
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
		utils.LogWarning("Failed to update players annotation: %v", err)
	}
}

// logEvent logs an event with contextual information about the server state.
func logEvent(eventType string, message string, state *types.ServerState) {
	sessionType := "unknown"
	if state.CurrentSession != nil {
		sessionType = state.CurrentSession.Type
	}

	utils.LogSDK("[%s] %s | Server: %s | Players: %d | Session: %s",
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
		utils.LogWarning("Could not send shutdown message: %v", err)
	}
	time.Sleep(time.Second)
	cancel()

	utils.LogSDK("Server shutdown initiated")
}

// handleSteamError handles Steam-related errors and updates the error metrics accordingly.
func handleSteamError(output string, state *types.ServerState, _ prometheus.Labels) {
	if strings.Contains(output, "SteamAPI_Init") || strings.Contains(output, "steamclient.so") {
		utils.LogWarning("Steam initialization warning: %s", output)
		metrics.ServerErrorsCounter.With(prometheus.Labels{
			"server_id":   state.ServerID,
			"server_name": state.ServerName,
			"server_type": state.ServerType,
			"error_type":  "steam_init",
		}).Inc()
	}
}

// handleServerVersion handles server version-related events and updates metrics accordingly.
func handleServerVersion(output string, _ *types.ServerState, _ prometheus.Labels) {
	version := extractVersion(output)
	utils.LogSDK("Server version: %s", version)
}

// handleConfigLoading handles server configuration loading-related events and updates metrics accordingly.
func handleConfigLoading(output string, state *types.ServerState, _ prometheus.Labels) {
	configFile := extractConfigFile(output)
	utils.LogSDK("Loading config: %s", configFile)
	metrics.ServerErrorsCounter.With(prometheus.Labels{
		"server_id":   state.ServerID,
		"server_name": state.ServerName,
		"server_type": state.ServerType,
		"error_type":  "config_loading",
	}).Inc()
}

// handlePluginLoading handles server plugin loading-related events and updates metrics accordingly.
func handlePluginLoading(output string, _ *types.ServerState, _ prometheus.Labels) {
	plugin := extractPluginName(output)
	utils.LogSDK("Loaded plugin: %s", plugin)
}

// handleAISlotUpdate handles server AI slot update-related events and updates metrics accordingly.
func handleAISlotUpdate(output string, state *types.ServerState, _ prometheus.Labels) {
	// Extraire et mettre à jour les informations sur les slots AI
	slots := extractAISlots(output)
	state.Lock()
	state.ActiveCars = slots // Utiliser la variable slots
	state.Unlock()
}

// handleChecksumUpdate handles server checksum update-related events and updates metrics accordingly.
func handleChecksumUpdate(output string, _ *types.ServerState, _ prometheus.Labels) {
	// Gérer les mises à jour de checksum
	asset := extractChecksumAsset(output)
	utils.LogSDK("Updated checksum for: %s", asset)
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

// handleServerInvite handles server invite-related events
func handleServerInvite(output string, _ *types.ServerState, _ prometheus.Labels) {
	url := strings.Split(output, "Server invite link:")[1]
	utils.LogSDK("Server invite URL available: %s", strings.TrimSpace(url))
}

// handleSessionSwitch handles session switch-related events and updates metrics accordingly.
func handleSessionSwitch(output string, state *types.ServerState, _ prometheus.Labels) {
	sessionID := extractSessionID(output)
	utils.LogSDK("Switching to session ID: %s", sessionID)
	state.Lock()
	if state.CurrentSession != nil {
		state.CurrentSession.ID = sessionID
	}
	state.Unlock()
}

// handleTCPServer handles TCP server-related events
func handleTCPServer(output string, _ *types.ServerState, _ prometheus.Labels) {
	port := strings.Split(output, "port")[1]
	utils.LogSDK("Starting TCP server on port%s", port)
	metrics.ServerPortsGauge.With(prometheus.Labels{
		"port_type": "tcp",
		"port":      strings.TrimSpace(port),
	}).Set(1)
}

// handleUDPServer handles UDP server-related events
func handleUDPServer(output string, _ *types.ServerState, _ prometheus.Labels) {
	port := strings.Split(output, "port")[1]
	utils.LogSDK("Starting UDP server on port%s", port)
	metrics.ServerPortsGauge.With(prometheus.Labels{
		"port_type": "udp",
		"port":      strings.TrimSpace(port),
	}).Set(1)
}

// handleSessionTime handles session time-related events and updates metrics accordingly.
func handleSessionTime(output string, state *types.ServerState, _ prometheus.Labels) {
	duration := strings.Split(output, "session :")[1]
	utils.LogSDK("Remaining time of session :%s", duration)
	state.Lock()
	if state.CurrentSession != nil {
		state.CurrentSession.RemainingTime = strings.TrimSpace(duration)
	}
	state.Unlock()
}

// handleLobbyRegistration handles lobby registration-related events
func handleLobbyRegistration(_ string, _ *types.ServerState, _ prometheus.Labels) {
	utils.LogSDK("Registering server to lobby...")
}

// handleUpdateLoop handles update loop-related events
func handleUpdateLoop(output string, _ *types.ServerState, labels prometheus.Labels) {
	rate := strings.Split(output, "rate of")[1]
	utils.LogSDK("Starting update loop with an update rate of%s", rate)
	metrics.ServerUpdateRateGauge.With(labels).Set(parseUpdateRate(rate))
}

// handleLobbySuccess handles lobby success-related events
func handleLobbySuccess(_ string, _ *types.ServerState, labels prometheus.Labels) {
	utils.LogSDK("Lobby registration successful")
	metrics.LobbyRegistrationCounter.With(labels).Inc()
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

// handleCSPVersion handles CSP version information
func handleCSPVersion(output string, _ *types.ServerState, _ prometheus.Labels) {
	version := strings.Split(output, "Version")[1]
	utils.LogSDK("Using minimum required CSP Version %s", strings.TrimSpace(version))
}

// handleAISpline handles AI spline cache events
func handleAISpline(output string, _ *types.ServerState, _ prometheus.Labels) {
	utils.LogSDK(output)
}

// handleAILaneDetection handles AI lane detection events
func handleAILaneDetection(output string, _ *types.ServerState, _ prometheus.Labels) {
	utils.LogSDK(output)
}

// handleAISplineCache handles AI spline caching events
func handleAISplineCache(output string, _ *types.ServerState, _ prometheus.Labels) {
	utils.LogSDK(output)
}

// handleAISplineMapping handles AI spline mapping events
func handleAISplineMapping(output string, _ *types.ServerState, _ prometheus.Labels) {
	utils.LogSDK(output)
}

// handleKeysStorage handles key storage events
func handleKeysStorage(output string, _ *types.ServerState, _ prometheus.Labels) {
	utils.LogWarning(output)
}

// handleXMLEncryption handles XML encryption configuration events
func handleXMLEncryption(output string, _ *types.ServerState, _ prometheus.Labels) {
	utils.LogWarning(output)
}

// handleBlacklistLoading handles blacklist loading events
func handleBlacklistLoading(output string, _ *types.ServerState, _ prometheus.Labels) {
	entries := strings.Split(output, "with")[1]
	utils.LogSDK("Loaded blacklist.txt%s", entries)
}

// handleWhitelistLoading handles whitelist loading events
func handleWhitelistLoading(output string, _ *types.ServerState, _ prometheus.Labels) {
	entries := strings.Split(output, "with")[1]
	utils.LogSDK("Loaded whitelist.txt%s", entries)
}

// handleAdminsLoading handles admin list loading events
func handleAdminsLoading(output string, _ *types.ServerState, _ prometheus.Labels) {
	entries := strings.Split(output, "with")[1]
	utils.LogSDK("Loaded admins.txt%s", entries)
}

// handleSteamConnection handles Steam connection events
func handleSteamConnection(output string, _ *types.ServerState, _ prometheus.Labels) {
	utils.LogSDK(output)
}
