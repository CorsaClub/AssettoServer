// Package utils provides utility functions for data extraction and processing.
package utils

import (
	"strconv"
	"strings"

	"agones/types"
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
	return strings.TrimSpace(strings.Split(output, "has connected")[0])
}

// ExtractSteamID extracts the Steam ID from server output.
func ExtractSteamID(output string) string {
	if start := strings.Index(output, "("); start != -1 {
		if end := strings.Index(output[start:], ")"); end != -1 {
			steamInfo := output[start+1 : start+end]
			parts := strings.Split(steamInfo, " - ")
			if len(parts) > 0 {
				return parts[0]
			}
		}
	}
	return ""
}

// ExtractCarModel extracts the car model from server output.
func ExtractCarModel(output string) string {
	if start := strings.LastIndex(output, "("); start != -1 {
		if end := strings.LastIndex(output, ")"); end != -1 && end > start {
			return output[start+1 : end]
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
