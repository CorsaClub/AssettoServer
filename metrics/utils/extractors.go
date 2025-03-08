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
func ExtractSessionTime(output string) int {
	// Extraire le temps restant en secondes
	return 0 // TODO: Implémenter l'extraction
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
