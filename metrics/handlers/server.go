// Package handlers manages interactions with the Assetto Corsa server
package handlers

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	metrics "metrics/services"
	"metrics/types"
	"metrics/utils"
	"metrics/victoria"
)

// HandleServerOutput processes server output and updates metrics.
// It handles various server events based on the output string.
func HandleServerOutput(output string, vmClient *victoria.MetricsClient, state *types.ServerState, serverReady chan struct{}, cancel context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	defer func() {
		if r := recover(); r != nil {
			utils.LogError("Recovered from panic in HandleServerOutput: %v", r)
			// Notify metrics of a critical error
			vmClient.SendMetrics(types.MetricBatch{
				Metrics: []types.Metric{
					{
						Name:        "assetto_server_errors_total",
						Value:       1,
						Type:        types.Counter,
						LabelValues: map[string]string{"server_id": state.ServerID, "session_id": state.CurrentSession.ID, "session_type": state.CurrentSession.Type, "error_type": "panic"},
						Timestamp:   time.Now(),
					},
				},
				Time: time.Now(),
			})
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

	// Log all server output
	logServerOutput(output, state)

	// Create base labels for metrics
	baseLabels := map[string]string{
		"server_id":    state.ServerID,
		"server_name":  state.ServerName,
		"server_type":  state.ServerType,
		"session_id":   state.CurrentSession.ID,
		"session_type": state.CurrentSession.Type,
	}

	select {
	case <-ctx.Done():
		utils.LogWarning("Timeout while processing server output")
		return
	default:
		switch {
		case strings.Contains(output, "is attempting to connect"):
			handleAttemptingToConnect(output, state, baseLabels)
		case strings.Contains(output, "supports extra CSP features"):
			handleExtraCSPFeatures(output, state, baseLabels)
		case strings.Contains(output, "Starting Assetto Corsa Server..."):
			handleServerStarting(state, baseLabels)
			vmClient.LogEvent(metrics.LogLevelInfo, "Server starting", metrics.EventServerStart, nil)
		case strings.Contains(output, "Lobby registration successful"):
			handleServerReady(state, baseLabels, serverReady)
		case strings.Contains(output, "End of session"):
			handleSessionEnd(vmClient, state, baseLabels, cancel)
		case strings.Contains(output, "has connected"):
			handlePlayerConnect(state, vmClient, output, baseLabels)
			player := utils.ExtractPlayerInfo(output)
			vmClient.LogEvent(metrics.LogLevelInfo,
				fmt.Sprintf("Player %s connected", player.Name),
				metrics.EventPlayerConnect,
				map[string]string{
					"player_id":   player.SteamID,
					"player_name": player.Name,
					"car_model":   player.CarModel,
				})
		case strings.Contains(output, "has disconnected"):
			handlePlayerDisconnect(state, vmClient, output, baseLabels)
			steamID := utils.ExtractSteamID(output)
			vmClient.LogEvent(metrics.LogLevelInfo,
				fmt.Sprintf("Player disconnected (Steam ID: %s)", steamID),
				metrics.EventPlayerDisconnect,
				map[string]string{"player_id": steamID})
		case strings.Contains(output, "Next session:"):
			handleSessionChange(state, output, baseLabels)
			sessionType := utils.ExtractSessionType(output)
			vmClient.LogEvent(metrics.LogLevelInfo,
				fmt.Sprintf("Session changed to %s", sessionType),
				metrics.EventSessionChange,
				map[string]string{"session_type": sessionType})
		case strings.Contains(output, "[ERR]"):
			handleError(fmt.Errorf(output), "server_error", state, baseLabels)
			vmClient.LogEvent(metrics.LogLevelError,
				output,
				metrics.EventError,
				nil)
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
		case strings.Contains(output, "CSP handshake received"):
			handleCSPHandshake(output, state, baseLabels)
		case strings.Contains(output, "CHAT:"):
			handleChatMessage(output, state, baseLabels)
		case strings.Contains(output, "Received clean exit"):
			handleCleanExit(output, state, baseLabels)
		default:
			utils.LogWarning("Unhandled output: %s", output)
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
func handleServerStarting(state *types.ServerState, labels map[string]string) {
	utils.LogSDK("Server starting up...")
	state.Lock()
	state.Ready = false
	state.ShuttingDown = false
	state.Unlock()
	metrics.ServerStateGauge.With(labels).Set(types.ServerStateStarting)
	metrics.ServerStartCounter.With(labels).Inc()
}

// handleServerReady updates the server state to ready and signals readiness.
func handleServerReady(state *types.ServerState, labels map[string]string, serverReady chan struct{}) {
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
				Value:       float64(types.ServerStateShutdown),
				Type:        types.Gauge,
				LabelValues: labels,
				Timestamp:   time.Now(),
			},
		},
		Time: time.Now(),
	})
}

// handlePlayerConnect processes a player's connection, updates player counts, and increments relevant metrics.
func handlePlayerConnect(state *types.ServerState, vmClient *victoria.MetricsClient, output string, labels map[string]string) {
	// Extract player info using the utility function
	player := utils.ExtractPlayerInfo(output)
	if player.SteamID == "" {
		utils.LogWarning("Invalid player info from output: %s", output)
		return
	}

	addPlayer(state, player)

	// Update basic metrics with base labels
	metrics.PlayersGauge.With(labels).Set(float64(state.Players))
	metrics.PlayerConnectCounter.With(labels).Inc()

	// Create player-specific labels by copying base labels and adding player info
	playerLabels := map[string]string{
		"server_id":   labels["server_id"],
		"server_name": labels["server_name"],
		"server_type": labels["server_type"],
		"player_name": player.Name,     // Use clean player name
		"steam_id":    player.SteamID,  // Use clean Steam ID
		"car_name":    player.CarModel, // Use clean car model
	}

	// Update player-specific metrics with complete set of labels
	metrics.PlayerLatencyGauge.With(playerLabels).Set(float64(player.Latency))
	metrics.CarUsageCounter.With(playerLabels).Inc()

	updatePlayerCount(state, vmClient)
}

// handlePlayerDisconnect processes a player's disconnection and updates relevant metrics.
func handlePlayerDisconnect(state *types.ServerState, vmClient *victoria.MetricsClient, output string, labels map[string]string) {
	steamID := utils.ExtractSteamID(output)
	removePlayer(state, steamID)

	metrics.PlayersGauge.With(labels).Set(float64(state.Players))
	metrics.PlayerDisconnectCounter.With(labels).Inc()
	updatePlayerCount(state, vmClient)

	utils.LogSDK("Player disconnected: %s", steamID)
}

// handleSessionChange manages changes to the game session, such as switching tracks or session types.
func handleSessionChange(state *types.ServerState, output string, labels map[string]string) {
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
		metrics.SessionDurationHistogram.With(labels).Observe(sessionDuration.Seconds())
	}

	metrics.SessionChangeCounter.With(labels).Inc()
	trackLabels := copyLabels(labels)
	trackLabels["track_name"] = track
	metrics.TrackUsageCounter.With(trackLabels).Inc()
}

// handleSteamAuth records successful Steam authentication events.
func handleSteamAuth(state *types.ServerState, labels map[string]string) {
	utils.LogSDK("Steam authentication successful for player")
	metrics.AuthSuccessCounter.With(labels).Inc()
}

// handleNetworkStats updates network-related metrics based on the server output.
func handleNetworkStats(output string, labels map[string]string) {
	if bytesReceived := utils.ExtractBytesReceived(output); bytesReceived > 0 {
		metrics.NetworkBytesReceivedCounter.With(labels).Add(float64(bytesReceived))
	}
	if bytesSent := utils.ExtractBytesSent(output); bytesSent > 0 {
		metrics.NetworkBytesSentCounter.With(labels).Add(float64(bytesSent))
	}

	utils.LogSDK("Network stats update: %s", output)
}

// handleError logs server errors and updates the error metrics accordingly.
func handleError(err error, errorType string, state *types.ServerState, labels map[string]string) {
	utils.LogError("(%s): %v", errorType, err)
	errorLabels := copyLabels(labels)
	errorLabels["error_type"] = errorType
	metrics.ServerErrorsCounter.With(errorLabels).Inc()

	utils.LogError("Server error: %v", err)
}

// copyLabels creates and returns a copy of the provided Prometheus labels.
func copyLabels(labels map[string]string) map[string]string {
	newLabels := make(map[string]string)
	for k, v := range labels {
		newLabels[k] = v
	}
	return newLabels
}

// updatePlayerCount updates the player count metric in VictoriaMetrics
func updatePlayerCount(state *types.ServerState, vmClient *victoria.MetricsClient) {
	vmClient.SendMetrics(types.MetricBatch{
		Metrics: []types.Metric{
			{
				Name:  metrics.ServerPlayersConnected,
				Value: float64(state.Players),
				Type:  types.Gauge,
				LabelValues: map[string]string{
					"server_id":    state.ServerID,
					"session_id":   state.CurrentSession.ID,
					"session_type": state.CurrentSession.Type,
				},
			},
		},
		Time: time.Now(),
	})
}

// logEvent logs an event with contextual information about the server state.
func logEvent(eventType string, message string, state *types.ServerState, additionalLabels ...map[string]string) {
	sessionType := "unknown"
	if state.CurrentSession != nil {
		sessionType = state.CurrentSession.Type
	}

	// Create base labels
	labels := map[string]string{
		"server_id":    state.ServerID,
		"server_name":  state.ServerName,
		"server_type":  state.ServerType,
		"session_type": sessionType,
	}

	// Add additional labels if provided
	if len(additionalLabels) > 0 {
		for k, v := range additionalLabels[0] {
			labels[k] = v
		}
	}

	utils.LogSDK("[%s] %s | Server: %s | Players: %d | Session: %s",
		eventType,
		message,
		state.ServerName,
		state.Players,
		sessionType)

	// Send to VictoriaLogs if available
	if logsClient, ok := utils.GetLogsClient(); ok {
		logsClient.LogEvent("INFO", message, eventType, labels)
	}
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
func gracefulShutdown(cancel context.CancelFunc, state *types.ServerState) {
	state.Lock()
	state.ShuttingDown = true
	state.Unlock()

	time.Sleep(time.Second)
	cancel()

	utils.LogSDK("Server shutdown initiated")
}

// handleSteamError handles Steam-related errors and updates the error metrics accordingly.
func handleSteamError(output string, state *types.ServerState, labels map[string]string) {
	if strings.Contains(output, "SteamAPI_Init") || strings.Contains(output, "steamclient.so") {
		utils.LogWarning("Steam initialization warning: %s", output)
		metrics.ServerErrorsCounter.With(labels).Inc()
	}
}

// handleServerVersion handles server version-related events and updates metrics accordingly.
func handleServerVersion(output string, _ *types.ServerState, _ map[string]string) {
	//version := extractVersion(output)
	//utils.LogSDK("Server version: %s", version)
}

// handleConfigLoading handles server configuration loading-related events and updates metrics accordingly.
func handleConfigLoading(output string, state *types.ServerState, labels map[string]string) {
	//configFile := extractConfigFile(output)
	metrics.ServerErrorsCounter.With(labels).Inc()
}

// handlePluginLoading handles server plugin loading-related events and updates metrics accordingly.
func handlePluginLoading(output string, _ *types.ServerState, _ map[string]string) {
	// Don't log anything
}

// handleAISlotUpdate handles server AI slot update-related events and updates metrics accordingly.
func handleAISlotUpdate(output string, state *types.ServerState, labels map[string]string) {
	// Extract AI slot information
	slots := utils.ExtractAISlots(output)
	state.Lock()
	state.ActiveCars = slots
	state.Unlock()

	// Ensure all required labels are present
	aiLabels := map[string]string{
		"server_id":   labels["server_id"],
		"server_name": labels["server_name"],
		"server_type": labels["server_type"],
		"slot_type":   "ai", // Add missing label
	}

	// Update metrics with complete labels
	metrics.ServerStateGauge.With(aiLabels).Set(float64(len(slots)))
}

// handleChecksumUpdate handles server checksum update-related events and updates metrics accordingly.
func handleChecksumUpdate(output string, _ *types.ServerState, _ map[string]string) {
	// Don't log anything
}

// extractVersion extracts the server version from the output string.
func extractVersion(output string) string {
	// Extract server version
	return strings.TrimSpace(strings.Split(output, "AssettoServer")[1])
}

// extractConfigFile extracts the configuration file name from the output string.
func extractConfigFile(output string) string {
	// Extract configuration file name
	return strings.TrimSpace(strings.Split(output, "Loading")[1])
}

// extractPluginName extracts the plugin name from the output string.
func extractPluginName(output string) string {
	// Extract plugin name
	return strings.TrimSpace(strings.Split(output, "Loaded plugin")[1])
}

// extractChecksumAsset extracts the asset name from the output string.
func extractChecksumAsset(output string) string {
	// Extract the asset name from the output string
	return strings.TrimSpace(strings.Split(output, "Added checksum for")[1])
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

// handleSessionSwitch handles session switch-related events and updates metrics accordingly.
func handleSessionSwitch(output string, state *types.ServerState, labels map[string]string) {
	fromType := state.SessionType // ancien type
	toType := utils.ExtractSessionType(output)
	track := state.CurrentTrack
	config := state.CurrentLayout

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
	state.Unlock()
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

// handleLobbyRegistration handles lobby registration-related events
func handleLobbyRegistration(output string, state *types.ServerState, labels map[string]string) {
	status := "success"
	details := utils.ExtractLobbyDetails(output)

	metrics.LobbyRegistrationCounter.With(labels).Inc()

	// Enregistrer le statut détaillé
	metrics.LobbyRegistrationStatusCounter.With(map[string]string{
		"server_id": labels["server_id"],
		"status":    status,
		"details":   details,
	}).Inc()
}

// handleUpdateLoop handles update loop-related events
func handleUpdateLoop(output string, _ *types.ServerState, labels map[string]string) {
	rate := strings.Split(output, "rate of")[1]
	metrics.ServerUpdateRateGauge.With(labels).Set(parseUpdateRate(rate))
}

// handleLobbySuccess handles lobby success-related events
func handleLobbySuccess(_ string, _ *types.ServerState, labels map[string]string) {
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
func handleCSPVersion(output string, _ *types.ServerState, _ map[string]string) {
	//version := strings.Split(output, "Version")[1]
	//utils.LogSDK("Using minimum required CSP Version %s", strings.TrimSpace(version))
}

// handleAISpline handles AI spline cache events
func handleAISpline(output string, _ *types.ServerState, _ map[string]string) {
	// Don't log anything
}

// handleAILaneDetection handles AI lane detection events
func handleAILaneDetection(output string, _ *types.ServerState, _ map[string]string) {
	// Don't log anything
}

// handleAISplineCache handles AI spline caching events
func handleAISplineCache(output string, _ *types.ServerState, _ map[string]string) {
	// Don't log anything
}

// handleAISplineMapping handles AI spline mapping events
func handleAISplineMapping(output string, _ *types.ServerState, _ map[string]string) {
	// Don't log anything
}

// handleKeysStorage handles key storage events
func handleKeysStorage(output string, _ *types.ServerState, _ map[string]string) {
	utils.LogWarning(output)
}

// handleXMLEncryption handles XML encryption configuration events
func handleXMLEncryption(output string, _ *types.ServerState, _ map[string]string) {
	utils.LogWarning(output)
}

// handleBlacklistLoading handles blacklist loading events
func handleBlacklistLoading(output string, _ *types.ServerState, _ map[string]string) {
	// Don't log anything
}

// handleWhitelistLoading handles whitelist loading events
func handleWhitelistLoading(output string, _ *types.ServerState, _ map[string]string) {
	// Don't log anything
}

// handleAdminsLoading handles admin list loading events
func handleAdminsLoading(output string, _ *types.ServerState, _ map[string]string) {
	// Don't log anything
}

// handleSteamConnection handles Steam connection events
func handleSteamConnection(output string, _ *types.ServerState, _ map[string]string) {
	// Don't log anything
}

func handleAttemptingToConnect(output string, state *types.ServerState, labels map[string]string) {
	playerInfo := utils.ExtractPlayerInfo(output)

	// Incrémenter le compteur de tentatives
	metrics.ConnectionAttemptsCounter.With(map[string]string{
		"server_id": labels["server_id"],
		"status":    "attempt",
	}).Inc()

	// Enregistrer le statut détaillé
	metrics.ConnectionStatusCounter.With(map[string]string{
		"server_id":   labels["server_id"],
		"player_name": playerInfo.Name,
		"steam_id":    playerInfo.SteamID,
		"status":      "attempting",
	}).Inc()
}

func handleExtraCSPFeatures(output string, _ *types.ServerState, _ map[string]string) {
	// Don't log anything
}

func handleCSPHandshake(output string, state *types.ServerState, labels map[string]string) {
	if strings.Contains(output, "Version=") {
		version := utils.ExtractCSPVersion(output)
		playerName := utils.ExtractCSPPlayerName(output)

		// S'assurer que tous les labels requis sont présents
		cspLabels := map[string]string{
			"server_id":   labels["server_id"],
			"server_name": labels["server_name"],
			"server_type": labels["server_type"],
			"player_name": playerName, // Ajouter le label manquant
		}

		metrics.CSPVersionGauge.With(cspLabels).Set(float64(version))
	}
}

func handleChatMessage(output string, state *types.ServerState, labels map[string]string) {
	// Extraire le nom du joueur et le contenu du message
	playerName := utils.ExtractName(output)
	messageContent := utils.ExtractChatMessage(output)
	messageType := determineChatType(messageContent) // admin, global, team, etc.

	// Try to extract the player's Steam ID
	steamID := utils.ExtractSteamID(output)
	if steamID == "" {
		// If we can't extract the Steam ID from the output, try to find it in the state
		state.RLock()
		for id, player := range state.ConnectedPlayers {
			if player.Name == playerName {
				steamID = id
				break
			}
		}
		state.RUnlock()
	}

	// Get session information
	sessionType := "unknown"
	sessionID := ""
	if state.CurrentSession != nil {
		sessionType = state.CurrentSession.Type
		sessionID = state.CurrentSession.ID
	}

	// Incrémenter le compteur général
	metrics.ChatMessagesCounter.With(labels).Inc()

	// Incrémenter le compteur par joueur
	playerLabels := map[string]string{
		"server_id":   labels["server_id"],
		"server_name": labels["server_name"],
		"server_type": labels["server_type"],
		"player_name": playerName,
		"player_id":   steamID,
	}
	metrics.ChatMessagesByPlayerCounter.With(playerLabels).Inc()

	// Incrémenter le compteur par session
	sessionLabels := map[string]string{
		"server_id":    labels["server_id"],
		"server_name":  labels["server_name"],
		"server_type":  labels["server_type"],
		"session_type": sessionType,
		"session_id":   sessionID,
	}
	metrics.ChatMessagesBySessionCounter.With(sessionLabels).Inc()

	// Incrémenter le compteur détaillé
	metrics.ChatMessagesByTypeCounter.With(map[string]string{
		"server_id":    labels["server_id"],
		"player_name":  playerName,
		"message_type": messageType,
		"content":      messageContent,
	}).Inc()

	// Record message length in the histogram
	lengthLabels := map[string]string{
		"server_id":    labels["server_id"],
		"server_name":  labels["server_name"],
		"server_type":  labels["server_type"],
		"player_name":  playerName,
		"message_type": messageType,
	}
	metrics.ChatMessageLengthHistogram.With(lengthLabels).Observe(float64(len(messageContent)))

	// Log the chat message with dedicated labels
	logEvent("chat_message", messageContent, state, map[string]string{
		"player_name":  playerName,
		"message_type": messageType,
		"player_id":    steamID,
		"session_type": sessionType,
		"session_id":   sessionID,
	})
}

func handleCleanExit(output string, _ *types.ServerState, _ map[string]string) {
	steamID := utils.ExtractSteamID(output)
	utils.LogDebug("Clean exit received for player with Steam ID: %s", steamID)
}

// Fonction utilitaire pour déterminer le type de message chat
func determineChatType(message string) string {
	if strings.HasPrefix(message, "/admin") {
		return "admin"
	}
	if strings.HasPrefix(message, "/t ") {
		return "team"
	}
	return "global"
}

// logServerOutput logs all server output with appropriate categorization
func logServerOutput(output string, state *types.ServerState) {
	// Determine the event type based on the output content
	eventType := "server_output"
	level := "INFO"

	// Categorize the output
	if strings.Contains(output, "ERROR") || strings.Contains(output, "Error") || strings.Contains(output, "error") {
		eventType = "server_error"
		level = "ERROR"
	} else if strings.Contains(output, "WARNING") || strings.Contains(output, "Warning") || strings.Contains(output, "warning") {
		eventType = "server_warning"
		level = "WARNING"
	} else if strings.Contains(output, "CHAT") {
		eventType = "chat_message"
	} else if strings.Contains(output, "CONNECTED") {
		eventType = "player_connect"
	} else if strings.Contains(output, "DISCONNECTED") {
		eventType = "player_disconnect"
	} else if strings.Contains(output, "SESSION") {
		eventType = "session_change"
	}

	// Create labels based on the event type
	labels := map[string]string{
		"server_id":    state.ServerID,
		"server_name":  state.ServerName,
		"server_type":  state.ServerType,
		"session_id":   state.CurrentSession.ID,
		"session_type": state.CurrentSession.Type,
		"event_type":   eventType,
	}

	// Add player information if available
	if strings.Contains(output, "CHAT") || strings.Contains(output, "CONNECTED") || strings.Contains(output, "DISCONNECTED") {
		playerName := utils.ExtractName(output)
		if playerName != "" {
			labels["player_name"] = playerName
		}

		steamID := utils.ExtractSteamID(output)
		if steamID != "" {
			labels["player_id"] = steamID
		}
	}

	// Send to VictoriaLogs if available
	if logsClient, ok := utils.GetLogsClient(); ok {
		logsClient.LogEvent(level, output, eventType, labels)
	}

	// Also log to standard output for debugging
	utils.LogInfo("[%s] %s", eventType, output)
}
