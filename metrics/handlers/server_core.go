// Package handlers manages interactions with the Assetto Corsa server
package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"metrics/geoip"
	"metrics/metrics"
	"metrics/types"
	"metrics/utils"
	"metrics/victoria"
)

// handleServerOutput processes server output and updates metrics.
// It handles various server events based on the output string.
func handleServerOutput(output string, vmClient *victoria.MetricsClient, state *types.ServerState, serverReady chan struct{}, cancel context.CancelFunc, geoipService *geoip.GeoIPService) {
	ctx, ctxCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer ctxCancel()

	defer func() {
		if r := recover(); r != nil {
			utils.LogError("Recovered from panic in HandleServerOutput: %v", r)
			// Notify metrics of a critical error
			vmClient.SendMetrics(types.MetricBatch{
				Metrics: []types.Metric{
					{
						Name:        types.ServerErrorsTotal,
						Value:       1,
						Type:        types.Counter,
						Timestamp:   time.Now(),
						LabelValues: map[string]string{"error_type": "panic", "component": "server_output_handler"},
					},
				},
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

	// Create base labels for all metrics
	baseLabels := map[string]string{
		"server_id":   state.ServerID,
		"server_name": state.ServerName,
		"server_type": state.ServerType,
	}

	// Extract track information if present
	if strings.Contains(output, "track:") || strings.Contains(output, "Track:") {
		trackInfo := utils.ExtractTrackInfo(output)
		if trackInfo.Name != "" {
			state.Lock()
			if state.CurrentSession != nil {
				state.CurrentSession.Track = trackInfo.Name
				state.CurrentSession.Layout = trackInfo.Layout
			}
			state.Unlock()

			// Update track metrics
			trackLabels := copyLabels(baseLabels)
			trackLabels["track"] = trackInfo.Name
			trackLabels["layout"] = trackInfo.Layout
			metrics.TrackUsageCounter.With(trackLabels).Inc()
		}
	}

	// Extract grip and temperature information
	if strings.Contains(output, "grip:") || strings.Contains(output, "temperature") {
		gripInfo := utils.ExtractGripInfo(output)
		if gripInfo.Grip > 0 {
			state.Lock()
			state.TrackGrip = gripInfo.Grip
			state.Unlock()

			metrics.TrackGrip.With(baseLabels).Set(gripInfo.Grip)
		}

		if gripInfo.TrackTemp > 0 {
			state.Lock()
			state.TrackTemp = gripInfo.TrackTemp
			state.Unlock()

			metrics.TrackTemperature.With(baseLabels).Set(gripInfo.TrackTemp)
		}

		if gripInfo.AirTemp > 0 {
			state.Lock()
			state.AirTemp = gripInfo.AirTemp
			state.Unlock()

			metrics.AirTemperature.With(baseLabels).Set(gripInfo.AirTemp)
		}
	}

	// Extract CSP version information
	if strings.Contains(output, "CSP") && strings.Contains(output, "version") {
		cspInfo := utils.ExtractCSPInfo(output)
		if cspInfo.Version != "" {
			playerID := utils.ExtractSteamID(output)
			playerName := utils.ExtractName(output)

			if playerID != "" && playerName != "" {
				playerLabels := copyLabels(baseLabels)
				playerLabels["player_id"] = playerID
				playerLabels["player_name"] = playerName

				// Convert version string to float for metric
				versionValue := utils.ParseVersionToFloat(cspInfo.Version)
				metrics.CSPVersionGauge.With(playerLabels).Set(versionValue)
			}
		}
	}

	// Extract collision information
	if strings.Contains(output, "Collision") || strings.Contains(output, "collision") {
		collisionInfo := utils.ExtractCollisionInfo(output)
		if collisionInfo.Car1 != "" || collisionInfo.Car2 != "" {
			collisionLabels := copyLabels(baseLabels)

			if collisionInfo.Car1 != "" {
				collisionLabels["car1"] = collisionInfo.Car1
			}

			if collisionInfo.Car2 != "" {
				collisionLabels["car2"] = collisionInfo.Car2
			}

			if collisionInfo.Speed > 0 {
				collisionLabels["speed"] = fmt.Sprintf("%.1f", collisionInfo.Speed)
			}

			if collisionInfo.Force > 0 {
				collisionLabels["force"] = fmt.Sprintf("%.1f", collisionInfo.Force)
			}

			metrics.CollisionCounter.With(collisionLabels).Inc()
		}
	}

	// Extract lap time information
	if strings.Contains(output, "LAP") || strings.Contains(output, "lap time") {
		lapInfo := utils.ExtractLapInfo(output)
		if lapInfo.PlayerID != "" && lapInfo.LapTime > 0 {
			lapLabels := copyLabels(baseLabels)
			lapLabels["player_id"] = lapInfo.PlayerID
			lapLabels["player_name"] = lapInfo.PlayerName
			lapLabels["car_model"] = lapInfo.CarModel

			if state.CurrentSession != nil {
				lapLabels["track"] = state.CurrentSession.Track
			}

			// Convert lap time to seconds for the metric
			lapTimeSeconds := float64(lapInfo.LapTime) / 1000.0
			metrics.LapTimeHistogram.With(lapLabels).Observe(lapTimeSeconds)

			// If it's a best lap, update the best lap metric
			if lapInfo.IsBest {
				metrics.PlayerBestLapGauge.With(lapLabels).Set(lapTimeSeconds)
			}
		}
	}

	select {
	case <-ctx.Done():
		utils.LogWarning("Timeout while processing server output")
		return
	default:
		switch {
		case strings.Contains(output, "Server started"):
			handleServerStarting(state, baseLabels)
			vmClient.LogEvent(metrics.LogLevelInfo,
				"Server starting",
				metrics.EventServerStart,
				nil)
		case strings.Contains(output, "Server ready"):
			handleServerReady(state, baseLabels, serverReady)
			vmClient.LogEvent(metrics.LogLevelInfo,
				"Server ready",
				metrics.EventServerStart,
				nil)
		case strings.Contains(output, "Client connected"):
			handlePlayerConnect(state, vmClient, output, baseLabels, geoipService)
			steamID := utils.ExtractSteamID(output)
			vmClient.LogEvent(metrics.LogLevelInfo,
				fmt.Sprintf("Player connected (Steam ID: %s)", steamID),
				metrics.EventPlayerConnect,
				map[string]string{"player_id": steamID})
		case strings.Contains(output, "Client disconnected"):
			handlePlayerDisconnect(state, vmClient, output, baseLabels)
			steamID := utils.ExtractSteamID(output)
			vmClient.LogEvent(metrics.LogLevelInfo,
				fmt.Sprintf("Player disconnected (Steam ID: %s)", steamID),
				metrics.EventPlayerDisconnect,
				map[string]string{"player_id": steamID})
		case strings.Contains(output, "attempting to connect") || strings.Contains(output, "connection attempt"):
			handleAttemptingToConnect(output, state, baseLabels)
			playerInfo := utils.ExtractPlayerInfo(output)
			vmClient.LogEvent(metrics.LogLevelInfo,
				fmt.Sprintf("Player %s is attempting to connect", playerInfo.Name),
				metrics.EventConnectionAttempt,
				map[string]string{"player_name": playerInfo.Name, "player_id": playerInfo.SteamID})
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
		case strings.Contains(output, "Session ended") || strings.Contains(output, "End of session"):
			handleSessionEnd(vmClient, state, baseLabels, cancel)
			vmClient.LogEvent(metrics.LogLevelInfo,
				"Session ended",
				metrics.EventSessionEnd,
				nil)
		default:
			utils.LogWarning("Unhandled output: %s", output)
		}
	}
}

// handleServerStarting updates the server state to starting.
func handleServerStarting(state *types.ServerState, labels map[string]string) {
	utils.LogSDK("Server starting")
	state.Lock()
	state.Ready = false
	state.ShuttingDown = false
	state.Unlock()

	// Update metrics
	metrics.ServerStateGauge.With(labels).Set(float64(metrics.ServerStateStarting))
	metrics.ServerStartCounter.With(labels).Inc()
}

// handleServerReady processes the server ready event
func handleServerReady(state *types.ServerState, baseLabels map[string]string, serverReady chan struct{}) {
	// Update server state
	state.Lock()
	state.Ready = true

	// Initialize session if not already done
	if state.CurrentSession.ID == "" {
		state.CurrentSession.ID = utils.GenerateSessionID()
		state.CurrentSession.Type = "practice" // Default session type
		state.CurrentSession.StartTime = time.Now()
	}

	// Update session start time if not set
	if state.CurrentSession.StartTime.IsZero() {
		state.CurrentSession.StartTime = time.Now()
	}

	state.Unlock()

	// Signal that the server is ready
	select {
	case serverReady <- struct{}{}:
		// Signal sent
	default:
		// Channel already closed or full
	}

	// Update metrics
	metrics.ServerStateGauge.With(baseLabels).Set(float64(metrics.ServerStateReady))
	metrics.ServerHealth.With(baseLabels).Set(1) // Server is healthy

	// Update session metrics
	sessionLabels := map[string]string{
		"server_id":    state.ServerID,
		"server_name":  state.ServerName,
		"session_id":   state.CurrentSession.ID,
		"session_type": state.CurrentSession.Type,
	}

	metrics.SessionDurationGauge.With(sessionLabels).Set(0) // Initial duration

	// Log server ready event
	utils.LogInfo("Server ready")
}

// handleError logs server errors and updates the error metrics accordingly.
func handleError(err error, errorType string, state *types.ServerState, labels map[string]string) {
	utils.LogError("(%s): %v", errorType, err)
	errorLabels := copyLabels(labels)
	errorLabels["error_type"] = errorType
	metrics.ServerErrorsCounter.With(errorLabels).Inc()

	utils.LogError("Server error: %v", err)
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

// handleSteamError handles Steam-related errors and updates the error metrics accordingly.
func handleSteamError(output string, state *types.ServerState, labels map[string]string) {
	if strings.Contains(output, "SteamAPI_Init") || strings.Contains(output, "steamclient.so") {
		utils.LogWarning("Steam initialization warning: %s", output)
		metrics.ServerErrorsCounter.With(labels).Inc()
	}
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

// handleCleanExit handles clean exit events
func handleCleanExit(output string, _ *types.ServerState, _ map[string]string) {
	steamID := utils.ExtractSteamID(output)
	utils.LogDebug("Clean exit received for player with Steam ID: %s", steamID)
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

// handleCSPVersion handles CSP version information
func handleCSPVersion(output string, _ *types.ServerState, _ map[string]string) {
	//version := strings.Split(output, "Version")[1]
	//utils.LogSDK("Using minimum required CSP Version %s", strings.TrimSpace(version))
}

// handleExtraCSPFeatures handles extra CSP features
func handleExtraCSPFeatures(output string, _ *types.ServerState, _ map[string]string) {
	// Don't log anything
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
		// Nous ne faisons aucun log ici pour éviter les doublons
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
		logsClient.LogServerEvent(level, output, eventType, labels)
	}
}
