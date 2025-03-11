// Package handlers manages interactions with the Assetto Corsa server
package handlers

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"metrics/geoip"
	metrics "metrics/services"
	"metrics/types"
	"metrics/utils"
	"metrics/victoria"
)

// HandleServerOutput processes server output and updates metrics.
// It handles various server events based on the output string.
func HandleServerOutput(output string, vmClient *victoria.MetricsClient, state *types.ServerState, serverReady chan struct{}, cancel context.CancelFunc, geoipService *geoip.GeoIPService) {
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
			handlePlayerConnect(state, vmClient, output, baseLabels, geoipService)
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
		case strings.Contains(output, "Collision between") && strings.Contains(output, "and"):
			handleCollision(output, state, baseLabels)
		case strings.Contains(output, "LAP"):
			handleLap(output, state, baseLabels)
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
func handlePlayerConnect(state *types.ServerState, vmClient *victoria.MetricsClient, output string, labels map[string]string, geoipService *geoip.GeoIPService) {
	// Extract player info using the utility function
	player := utils.ExtractPlayerInfo(output)
	if player.SteamID == "" {
		utils.LogWarning("Invalid player info from output: %s", output)
		return
	}

	// Extract IP address if available
	player.IP = utils.ExtractIPAddress(output)

	// Enrich player with GeoIP information if available
	if geoipService != nil && player.IP != "" {
		if err := geoipService.EnrichPlayerWithGeoIP(&player); err != nil {
			utils.LogWarning("Failed to enrich player with GeoIP information: %v", err)
		} else {
			utils.LogInfo("Player %s connected from %s, %s (%s)", player.Name, player.City, player.Country, player.CountryCode)
		}
	}

	addPlayer(state, player)

	// Update basic metrics with base labels
	metrics.PlayersGauge.With(labels).Set(float64(state.Players))
	metrics.PlayerConnectCounter.With(labels).Inc()

	// Update GeoIP metrics
	updateGeoIPMetrics(state, labels)

	// Create player-specific labels by copying base labels and adding player info
	playerLabels := map[string]string{
		"server_id":   labels["server_id"],
		"server_name": labels["server_name"],
		"server_type": labels["server_type"],
		"player_name": player.Name,     // Use clean player name
		"steam_id":    player.SteamID,  // Use clean Steam ID
		"car_model":   player.CarModel, // Use clean car model
	}

	// Add GeoIP information to player labels if available
	if player.Country != "" {
		playerLabels["country"] = player.Country
		playerLabels["country_code"] = player.CountryCode
	}
	if player.City != "" {
		playerLabels["city"] = player.City
	}

	// Update player-specific metrics with complete set of labels
	metrics.PlayerLatencyGauge.With(playerLabels).Set(float64(player.Latency))

	// Update car usage metrics
	carLabels := map[string]string{
		"server_id":   labels["server_id"],
		"server_name": labels["server_name"],
		"server_type": labels["server_type"],
		"car_name":    player.CarModel,
	}
	metrics.CarUsageCounter.With(carLabels).Inc()

	// Increment session players counter if in a session
	if state.CurrentSession != nil {
		sessionLabels := map[string]string{
			"server_id":    labels["server_id"],
			"server_name":  labels["server_name"],
			"server_type":  labels["server_type"],
			"session_id":   state.CurrentSession.ID,
			"session_type": state.CurrentSession.Type,
		}
		metrics.SessionPlayersCounter.With(sessionLabels).Inc()
	}

	updatePlayerCount(state, vmClient)

	// Log the player connection event
	logEvent("player_connect", fmt.Sprintf("Player %s connected", player.Name), state, map[string]string{
		"player_name": player.Name,
		"player_id":   player.SteamID,
		"car_model":   player.CarModel,
	})
}

// handlePlayerDisconnect processes a player's disconnection, updates player counts, and decrements relevant metrics.
func handlePlayerDisconnect(state *types.ServerState, vmClient *victoria.MetricsClient, output string, labels map[string]string) {
	steamID := utils.ExtractSteamID(output)
	if steamID == "" {
		utils.LogWarning("Invalid Steam ID from output: %s", output)
		return
	}

	// Get player info before removing from state
	state.RLock()
	player, exists := state.ConnectedPlayers[steamID]
	state.RUnlock()

	if exists && player != nil {
		// Log player disconnection with GeoIP information if available
		if player.Country != "" {
			utils.LogInfo("Player %s disconnected from %s, %s (%s)", player.Name, player.City, player.Country, player.CountryCode)
		}
	}

	removePlayer(state, steamID)

	// Update basic metrics
	metrics.PlayersGauge.With(labels).Set(float64(state.Players))
	metrics.PlayerDisconnectCounter.With(labels).Inc()

	// Update GeoIP metrics
	updateGeoIPMetrics(state, labels)

	// Decrement session players counter if in a session
	if state.CurrentSession != nil {
		sessionLabels := map[string]string{
			"server_id":    labels["server_id"],
			"server_name":  labels["server_name"],
			"server_type":  labels["server_type"],
			"session_id":   state.CurrentSession.ID,
			"session_type": state.CurrentSession.Type,
		}
		// We don't want to go below zero
		if state.Players >= 0 {
			// Decrement the counter by adding -1
			metrics.SessionPlayersCounter.With(sessionLabels).Add(-1)
		}
	}

	updatePlayerCount(state, vmClient)

	// Log the player disconnection event
	logEvent("player_disconnect", fmt.Sprintf("Player disconnected (Steam ID: %s)", steamID), state, map[string]string{
		"player_id": steamID,
	})
}

// updateGeoIPMetrics updates the GeoIP metrics based on the current state.
func updateGeoIPMetrics(state *types.ServerState, baseLabels map[string]string) {
	// Create maps to count players by country and city
	countryCounts := make(map[string]int)
	cityCounts := make(map[string]map[string]int)

	// Count players by country and city
	state.RLock()
	for _, player := range state.ConnectedPlayers {
		if player.Country != "" {
			countryCounts[player.Country+"|"+player.CountryCode]++
			if player.City != "" {
				if cityCounts[player.Country+"|"+player.CountryCode] == nil {
					cityCounts[player.Country+"|"+player.CountryCode] = make(map[string]int)
				}
				cityCounts[player.Country+"|"+player.CountryCode][player.City]++
			}
		}
	}
	state.RUnlock()

	// Update country metrics
	for countryKey, count := range countryCounts {
		parts := strings.Split(countryKey, "|")
		country := parts[0]
		countryCode := parts[1]
		countryLabels := map[string]string{
			"server_id":    baseLabels["server_id"],
			"server_name":  baseLabels["server_name"],
			"server_type":  baseLabels["server_type"],
			"country":      country,
			"country_code": countryCode,
		}
		metrics.PlayerCountryActiveGauge.With(countryLabels).Set(float64(count))

		// Update city metrics for this country
		if cityMap := cityCounts[countryKey]; cityMap != nil {
			for city, cityCount := range cityMap {
				cityLabels := map[string]string{
					"server_id":    baseLabels["server_id"],
					"server_name":  baseLabels["server_name"],
					"server_type":  baseLabels["server_type"],
					"country":      country,
					"country_code": countryCode,
					"city":         city,
				}
				metrics.PlayerCityActiveGauge.With(cityLabels).Set(float64(cityCount))
			}
		}
	}
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

// handleSteamAuth records successful Steam authentication events.
func handleSteamAuth(state *types.ServerState, labels map[string]string) {
	utils.LogSDK("Steam authentication successful for player")
	metrics.AuthSuccessCounter.With(labels).Inc()
}

// handleNetworkStats updates network-related metrics based on the server output.
func handleNetworkStats(output string, labels map[string]string) {
	// Extract network statistics
	bytesReceived := utils.ExtractBytesReceived(output)
	bytesSent := utils.ExtractBytesSent(output)
	packetLoss := utils.ExtractPacketLoss(output)
	latency := utils.ExtractLatency(output)

	// Update bytes received counter
	if bytesReceived > 0 {
		metrics.NetworkBytesReceivedCounter.With(labels).Add(float64(bytesReceived))
	}

	// Update bytes sent counter
	if bytesSent > 0 {
		metrics.NetworkBytesSentCounter.With(labels).Add(float64(bytesSent))
	}

	// Update packet loss gauge
	if packetLoss > 0 {
		metrics.PacketLossGauge.With(labels).Set(packetLoss)
	}

	// Update latency gauge
	if latency > 0 {
		metrics.PlayerLatencyGauge.With(labels).Set(latency)
	}

	// Log network stats
	logEvent("network_stats", fmt.Sprintf("Network stats: Received=%d bytes, Sent=%d bytes, Loss=%.2f%%, Latency=%.2f ms",
		bytesReceived, bytesSent, packetLoss*100, latency), nil, map[string]string{
		"server_id":      labels["server_id"],
		"server_name":    labels["server_name"],
		"server_type":    labels["server_type"],
		"bytes_received": fmt.Sprintf("%d", bytesReceived),
		"bytes_sent":     fmt.Sprintf("%d", bytesSent),
		"packet_loss":    fmt.Sprintf("%.4f", packetLoss),
		"latency_ms":     fmt.Sprintf("%.2f", latency),
	})
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

// handleAttemptingToConnect processes connection attempts and updates relevant metrics.
func handleAttemptingToConnect(output string, state *types.ServerState, labels map[string]string) {
	playerInfo := utils.ExtractPlayerInfo(output)

	// Increment connection attempts counter
	metrics.ConnectionAttemptsCounter.With(map[string]string{
		"server_id":   labels["server_id"],
		"server_name": labels["server_name"],
		"server_type": labels["server_type"],
		"status":      "attempting",
	}).Inc()

	// Increment connection status counter with player details
	metrics.ConnectionStatusCounter.With(map[string]string{
		"server_id":   labels["server_id"],
		"server_name": labels["server_name"],
		"server_type": labels["server_type"],
		"player_name": playerInfo.Name,
		"steam_id":    playerInfo.SteamID,
		"status":      "attempting",
	}).Inc()

	// Log the connection attempt
	logEvent("connection_attempt", fmt.Sprintf("Player %s is attempting to connect", playerInfo.Name), state, map[string]string{
		"player_name": playerInfo.Name,
		"player_id":   playerInfo.SteamID,
		"status":      "attempting",
	})
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

	// Si le message est vide, ne rien faire
	if messageContent == "" {
		utils.LogWarning("Empty chat message detected: %s", output)
		return
	}

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
	// Note: Nous ne stockons pas le contenu complet du message pour éviter les problèmes de cardinalité
	detailedLabels := map[string]string{
		"server_id":    labels["server_id"],
		"player_name":  playerName,
		"message_type": messageType,
		// Stocker seulement les premiers mots du message ou une version hachée pour les métriques
		"content_hash": utils.HashString(messageContent)[:8],
	}
	metrics.ChatMessagesByTypeCounter.With(detailedLabels).Inc()

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
	// Pour les logs, nous pouvons stocker le message complet car ils sont moins sensibles à la cardinalité
	chatLabels := map[string]string{
		"player_name":  playerName,
		"message_type": messageType,
		"player_id":    steamID,
		"session_type": sessionType,
		"session_id":   sessionID,
		"event_type":   "chat_message", // Ajouter un label spécifique pour les messages de chat
	}

	// Créer un message formaté pour les logs
	formattedMessage := fmt.Sprintf("[CHAT] %s: %s", playerName, messageContent)

	// Envoyer le log avec le message formaté
	logEvent("chat_message", formattedMessage, state, chatLabels)
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
		// Les messages de chat sont traités séparément par handleChatMessage
		// Nous ne faisons qu'un log basique ici pour éviter les doublons
		utils.LogInfo("[CHAT] Raw message: %s", output)
		return
	} else if strings.Contains(output, "CONNECTED") {
		eventType = "player_connect"
	} else if strings.Contains(output, "DISCONNECTED") {
		eventType = "player_disconnect"
	} else if strings.Contains(output, "SESSION") {
		eventType = "session_change"
	} else if strings.Contains(output, "LAP") {
		eventType = "lap_completed"
	} else if strings.Contains(output, "Collision") {
		eventType = "collision"
		level = "WARNING"
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
	if strings.Contains(output, "CONNECTED") || strings.Contains(output, "DISCONNECTED") {
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
