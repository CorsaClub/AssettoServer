// Package utils provides utility functions for data extraction and processing.
package utils

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"metrics/types"
)

// ExtractPlayerInfo extracts player information from server output.
func ExtractPlayerInfo(output string) types.Player {
	name := ExtractName(output)
	steamID := ExtractSteamID(output)
	carModel := ExtractCarModel(output)

	return types.Player{
		Name:     name,
		SteamID:  steamID,
		CarModel: carModel,
	}
}

// ExtractName extracts the player's name from server output.
func ExtractName(output string) string {
	// Remove timestamp if present
	if idx := strings.Index(output, "]"); idx != -1 {
		output = strings.TrimSpace(output[idx+1:])
	}

	// Extract name before "has connected"
	name := strings.Split(output, "has connected")[0]
	// Remove Steam ID and car info if present
	if idx := strings.Index(name, "("); idx != -1 {
		name = name[:idx]
	}
	return strings.TrimSpace(name)
}

// ExtractSteamID extracts the Steam ID from server output.
func ExtractSteamID(output string) string {
	if start := strings.Index(output, "("); start != -1 {
		if end := strings.Index(output[start:], ","); end != -1 {
			steamID := output[start+1 : start+end]
			return strings.TrimSpace(steamID)
		}
	}
	return ""
}

// ExtractCarModel extracts the car model from server output.
func ExtractCarModel(output string) string {
	if start := strings.LastIndex(output, "("); start != -1 {
		if end := strings.LastIndex(output, ")"); end != -1 && end > start {
			carModel := output[start+1 : end]
			// Remove any additional info after the car model
			if idx := strings.Index(carModel, ","); idx != -1 {
				carModel = carModel[:idx]
			}
			return strings.TrimSpace(carModel)
		}
	}
	return ""
}

// ExtractSessionType extracts the session type from server output.
func ExtractSessionType(output string) string {
	if strings.Contains(output, "PRACTICE") {
		return "practice"
	}
	if strings.Contains(output, "QUALIFY") {
		return "qualifying"
	}
	if strings.Contains(output, "RACE") {
		return "race"
	}
	return "unknown"
}

// ExtractTrackName extracts the track name from server output.
func ExtractTrackName(output string) string {
	if strings.Contains(output, "TRACK:") {
		parts := strings.Split(output, "TRACK:")
		if len(parts) > 1 {
			return strings.TrimSpace(parts[1])
		}
	}
	return ""
}

// ExtractBytesReceived extracts the number of bytes received from server output.
func ExtractBytesReceived(output string) int64 {
	return extractBytes(output, "Received:")
}

// ExtractBytesSent extracts the number of bytes sent from server output.
func ExtractBytesSent(output string) int64 {
	return extractBytes(output, "Sent:")
}

// ExtractPacketLoss extracts the packet loss percentage from server output.
func ExtractPacketLoss(output string) float64 {
	if strings.Contains(output, "packet loss") {
		re := regexp.MustCompile(`(\d+\.?\d*)%\s+packet loss`)
		matches := re.FindStringSubmatch(output)
		if len(matches) > 1 {
			loss, err := strconv.ParseFloat(matches[1], 64)
			if err == nil {
				return loss / 100.0 // Convert percentage to ratio
			}
		}
	}
	return 0
}

// ExtractLatency extracts the network latency in milliseconds from server output.
func ExtractLatency(output string) float64 {
	if strings.Contains(output, "latency") || strings.Contains(output, "ping") {
		re := regexp.MustCompile(`(\d+\.?\d*)\s*ms`)
		matches := re.FindStringSubmatch(output)
		if len(matches) > 1 {
			latency, err := strconv.ParseFloat(matches[1], 64)
			if err == nil {
				return latency
			}
		}
	}
	return 0
}

// extractBytes is a utility function to extract byte values based on a prefix.
func extractBytes(output, prefix string) int64 {
	if strings.Contains(output, prefix) {
		parts := strings.Split(output, prefix)
		if len(parts) > 1 {
			bytesStr := strings.Split(parts[1], "bytes")[0]
			bytes, err := strconv.ParseInt(strings.TrimSpace(bytesStr), 10, 64)
			if err == nil {
				return bytes
			}
		}
	}
	return 0
}

// ExtractCSPVersion extracts the CSP version from server output.
func ExtractCSPVersion(output string) int {
	if strings.Contains(output, "Version=") {
		parts := strings.Split(output, "Version=")
		if len(parts) > 1 {
			versionStr := strings.Split(parts[1], " ")[0]
			version, err := strconv.Atoi(versionStr)
			if err == nil {
				return version
			}
		}
	}
	return 0
}

// ExtractCSPPlayerName extracts the player name from CSP handshake output.
func ExtractCSPPlayerName(output string) string {
	// We expect the output to be in the format: "CSP handshake received from PlayerName (0):"
	if start := strings.Index(output, "from"); start != -1 {
		output = output[start+5:] // Skip "from "
		if end := strings.Index(output, "("); end != -1 {
			return strings.TrimSpace(output[:end])
		}
	}
	return "unknown"
}

// ExtractAISlots extracts AI slot information from the output string.
func ExtractAISlots(output string) map[string]int {
	slots := make(map[string]int)

	// Extract the number of AI slots
	if strings.Contains(output, "No. AI Slots:") {
		parts := strings.Split(output, "No. AI Slots:")
		if len(parts) > 1 {
			numStr := strings.Split(parts[1], "-")[0]
			if num, err := strconv.Atoi(strings.TrimSpace(numStr)); err == nil {
				slots["total"] = num
			}
		}
	}

	return slots
}

// ExtractChatMessage extracts the chat message from server output.
func ExtractChatMessage(output string) string {
	// Extraire le message après "CHAT:"
	if idx := strings.Index(output, "CHAT:"); idx != -1 {
		// Get the raw message
		rawMessage := strings.TrimSpace(output[idx+5:])

		// Remove any server-specific prefixes or formatting
		// For example, remove timestamps, server tags, etc.
		// This is a basic implementation, you might need to adjust based on your server's output format

		// Remove player name if it appears at the beginning of the message
		if nameEnd := strings.Index(rawMessage, ":"); nameEnd != -1 {
			rawMessage = strings.TrimSpace(rawMessage[nameEnd+1:])
		}

		// Clean up any control characters or excessive whitespace
		rawMessage = strings.TrimSpace(rawMessage)

		// Limit message length to prevent excessive data
		const maxLength = 500
		if len(rawMessage) > maxLength {
			rawMessage = rawMessage[:maxLength] + "..."
		}

		return rawMessage
	}
	return ""
}

// ExtractSessionTime extracts the session time from server output.
func ExtractSessionTime(output string) float64 {
	// Case 1: Starting session with duration in minutes
	if strings.Contains(output, "Starting session with duration:") {
		parts := strings.Split(output, "duration:")
		if len(parts) > 1 {
			minutesStr := strings.TrimSpace(strings.Split(parts[1], "minutes")[0])
			minutes, err := strconv.ParseFloat(minutesStr, 64)
			if err == nil {
				return minutes * 60 // Convert minutes to seconds
			}
		}
	}

	// Case 2: Minutes remaining
	if strings.Contains(output, "minutes remaining") {
		parts := strings.Split(output, "]")
		if len(parts) > 1 {
			minutesStr := strings.TrimSpace(strings.Split(parts[1], "minutes")[0])
			minutes, err := strconv.ParseFloat(minutesStr, 64)
			if err == nil {
				return minutes * 60 // Convert minutes to seconds
			}
		}
	}

	// Case 3: End of session
	if strings.Contains(output, "End of session") {
		return 0 // No time remaining
	}

	return -1 // Invalid or unrecognized format
}

// ExtractLobbyDetails extracts the lobby details from server output.
func ExtractLobbyDetails(output string) string {
	// Extraire les détails du lobby
	return "" // TODO: Implémenter l'extraction
}

// ExtractInviteURL extracts the invite URL from server output.
func ExtractInviteURL(output string) string {
	// Extraire l'URL d'invitation
	return "" // TODO: Implémenter l'extraction
}

// HashString creates a hash of a string.
func HashString(s string) string {
	// Créer un hash de la chaîne
	h := sha256.New()
	h.Write([]byte(s))
	return fmt.Sprintf("%x", h.Sum(nil))[:8]
}

// ExtractIPAddress extracts the IP address from server output.
func ExtractIPAddress(output string) string {
	// Recherche d'une adresse IPv4
	ipv4Regex := regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
	matches := ipv4Regex.FindStringSubmatch(output)
	if len(matches) > 0 {
		return matches[0]
	}

	// Recherche d'une adresse IPv6
	ipv6Regex := regexp.MustCompile(`\b(?:[0-9a-fA-F]{1,4}:){7}[0-9a-fA-F]{1,4}\b`)
	matches = ipv6Regex.FindStringSubmatch(output)
	if len(matches) > 0 {
		return matches[0]
	}

	return ""
}

// TrackInfo contient les informations sur une piste
type TrackInfo struct {
	Name   string
	Layout string
}

// ExtractTrackInfo extrait les informations de piste à partir d'une chaîne de sortie
func ExtractTrackInfo(output string) TrackInfo {
	// Exemple: "Track: monza, Layout: gp"
	trackInfo := TrackInfo{}

	trackRegex := regexp.MustCompile(`[Tt]rack:?\s*(\w+)`)
	layoutRegex := regexp.MustCompile(`[Ll]ayout:?\s*(\w+)`)

	trackMatches := trackRegex.FindStringSubmatch(output)
	if len(trackMatches) > 1 {
		trackInfo.Name = strings.TrimSpace(trackMatches[1])
	}

	layoutMatches := layoutRegex.FindStringSubmatch(output)
	if len(layoutMatches) > 1 {
		trackInfo.Layout = strings.TrimSpace(layoutMatches[1])
	}

	return trackInfo
}

// GripInfo contient les informations sur l'adhérence et la température
type GripInfo struct {
	Grip      float64
	TrackTemp float64
	AirTemp   float64
}

// ExtractGripInfo extrait les informations d'adhérence et de température
func ExtractGripInfo(output string) GripInfo {
	gripInfo := GripInfo{}

	// Exemple: "Grip: 0.95, Track temp: 25.5, Air temp: 22.3"
	gripRegex := regexp.MustCompile(`[Gg]rip:?\s*([\d.]+)`)
	trackTempRegex := regexp.MustCompile(`[Tt]rack\s*[Tt]emp(?:erature)?:?\s*([\d.]+)`)
	airTempRegex := regexp.MustCompile(`[Aa]ir\s*[Tt]emp(?:erature)?:?\s*([\d.]+)`)

	gripMatches := gripRegex.FindStringSubmatch(output)
	if len(gripMatches) > 1 {
		grip, err := strconv.ParseFloat(gripMatches[1], 64)
		if err == nil {
			gripInfo.Grip = grip
		}
	}

	trackTempMatches := trackTempRegex.FindStringSubmatch(output)
	if len(trackTempMatches) > 1 {
		trackTemp, err := strconv.ParseFloat(trackTempMatches[1], 64)
		if err == nil {
			gripInfo.TrackTemp = trackTemp
		}
	}

	airTempMatches := airTempRegex.FindStringSubmatch(output)
	if len(airTempMatches) > 1 {
		airTemp, err := strconv.ParseFloat(airTempMatches[1], 64)
		if err == nil {
			gripInfo.AirTemp = airTemp
		}
	}

	return gripInfo
}

// CSPInfo contient les informations sur CSP (Custom Shaders Patch)
type CSPInfo struct {
	Version  string
	Features []string
}

// ExtractCSPInfo extrait les informations CSP
func ExtractCSPInfo(output string) CSPInfo {
	cspInfo := CSPInfo{}

	// Exemple: "CSP version: 0.1.79"
	versionRegex := regexp.MustCompile(`CSP\s*[Vv]ersion:?\s*([\d.]+)`)
	featuresRegex := regexp.MustCompile(`CSP\s*[Ff]eatures:?\s*(.+)`)

	versionMatches := versionRegex.FindStringSubmatch(output)
	if len(versionMatches) > 1 {
		cspInfo.Version = strings.TrimSpace(versionMatches[1])
	}

	featuresMatches := featuresRegex.FindStringSubmatch(output)
	if len(featuresMatches) > 1 {
		features := strings.Split(featuresMatches[1], ",")
		for i, feature := range features {
			features[i] = strings.TrimSpace(feature)
		}
		cspInfo.Features = features
	}

	return cspInfo
}

// ParseVersionToFloat convertit une chaîne de version en nombre à virgule flottante
func ParseVersionToFloat(version string) float64 {
	// Exemple: "0.1.79" -> 0.179
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return 0
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0
	}

	minor := ""
	for i := 1; i < len(parts); i++ {
		minor += parts[i]
	}

	minorVal, err := strconv.Atoi(minor)
	if err != nil {
		return 0
	}

	// Calculer la valeur décimale
	minorFloat := float64(minorVal)
	for minorFloat >= 1 {
		minorFloat /= 10
	}

	return float64(major) + minorFloat
}

// CollisionInfo contient les informations sur une collision
type CollisionInfo struct {
	Car1  string
	Car2  string
	Speed float64
	Force float64
}

// ExtractCollisionInfo extrait les informations de collision
func ExtractCollisionInfo(output string) CollisionInfo {
	collisionInfo := CollisionInfo{}

	// Exemple: "Collision between Car1 and Car2 at speed 120.5 with force 85.3"
	car1Regex := regexp.MustCompile(`[Cc]ollision\s*(?:between)?\s*(\w+)`)
	car2Regex := regexp.MustCompile(`and\s*(\w+)`)
	speedRegex := regexp.MustCompile(`speed\s*([\d.]+)`)
	forceRegex := regexp.MustCompile(`force\s*([\d.]+)`)

	car1Matches := car1Regex.FindStringSubmatch(output)
	if len(car1Matches) > 1 {
		collisionInfo.Car1 = strings.TrimSpace(car1Matches[1])
	}

	car2Matches := car2Regex.FindStringSubmatch(output)
	if len(car2Matches) > 1 {
		collisionInfo.Car2 = strings.TrimSpace(car2Matches[1])
	}

	speedMatches := speedRegex.FindStringSubmatch(output)
	if len(speedMatches) > 1 {
		speed, err := strconv.ParseFloat(speedMatches[1], 64)
		if err == nil {
			collisionInfo.Speed = speed
		}
	}

	forceMatches := forceRegex.FindStringSubmatch(output)
	if len(forceMatches) > 1 {
		force, err := strconv.ParseFloat(forceMatches[1], 64)
		if err == nil {
			collisionInfo.Force = force
		}
	}

	return collisionInfo
}

// LapInfo contient les informations sur un tour
type LapInfo struct {
	PlayerID   string
	PlayerName string
	CarModel   string
	LapTime    int64 // en millisecondes
	IsBest     bool
}

// ExtractLapInfo extrait les informations de tour
func ExtractLapInfo(output string) LapInfo {
	lapInfo := LapInfo{}

	// Exemple: "LAP: Player (SteamID) completed lap in 1:45.678 (best: yes) with car ferrari_f40"
	playerRegex := regexp.MustCompile(`LAP:?\s*(\w+)`)
	steamIDRegex := regexp.MustCompile(`\((\d+)\)`)
	timeRegex := regexp.MustCompile(`(?:in|time)\s*(\d+):(\d+)\.(\d+)`)
	bestRegex := regexp.MustCompile(`best:?\s*(yes|no)`)
	carRegex := regexp.MustCompile(`(?:with|car)\s*(\w+)`)

	playerMatches := playerRegex.FindStringSubmatch(output)
	if len(playerMatches) > 1 {
		lapInfo.PlayerName = strings.TrimSpace(playerMatches[1])
	}

	steamIDMatches := steamIDRegex.FindStringSubmatch(output)
	if len(steamIDMatches) > 1 {
		lapInfo.PlayerID = strings.TrimSpace(steamIDMatches[1])
	}

	timeMatches := timeRegex.FindStringSubmatch(output)
	if len(timeMatches) > 3 {
		minutes, _ := strconv.ParseInt(timeMatches[1], 10, 64)
		seconds, _ := strconv.ParseInt(timeMatches[2], 10, 64)
		millis, _ := strconv.ParseInt(timeMatches[3], 10, 64)

		// Convertir en millisecondes
		lapInfo.LapTime = minutes*60*1000 + seconds*1000 + millis
	}

	bestMatches := bestRegex.FindStringSubmatch(output)
	if len(bestMatches) > 1 {
		lapInfo.IsBest = strings.ToLower(bestMatches[1]) == "yes"
	}

	carMatches := carRegex.FindStringSubmatch(output)
	if len(carMatches) > 1 {
		lapInfo.CarModel = strings.TrimSpace(carMatches[1])
	}

	return lapInfo
}
