package handlers

import (
	"fmt"
	"strings"

	"metrics/geoip"
	"metrics/metrics"
	"metrics/types"
	"metrics/utils"
	"metrics/victoria"
)

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
			if logsClient, ok := utils.GetLogsClient(); ok {
				logsClient.LogEvent("INFO", fmt.Sprintf("Player %s connected from %s, %s (%s)", player.Name, player.City, player.Country, player.CountryCode), "player_connection", map[string]string{
					"player_name":  player.Name,
					"city":         player.City,
					"country":      player.Country,
					"country_code": player.CountryCode,
				})
			}
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
			if logsClient, ok := utils.GetLogsClient(); ok {
				logsClient.LogEvent("INFO", fmt.Sprintf("Player %s disconnected from %s, %s (%s)", player.Name, player.City, player.Country, player.CountryCode), "player_disconnection", map[string]string{
					"player_name":  player.Name,
					"city":         player.City,
					"country":      player.Country,
					"country_code": player.CountryCode,
				})
			}
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

// handleSteamAuth records successful Steam authentication events.
func handleSteamAuth(state *types.ServerState, labels map[string]string) {
	utils.LogSDK("Steam authentication successful for player")
	metrics.AuthSuccessCounter.With(labels).Inc()
}

// handleCSPHandshake handles CSP handshake events
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
