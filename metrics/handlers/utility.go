package handlers

import (
	"strings"
	"time"

	"metrics/metrics"
	"metrics/types"
	"metrics/utils"
	"metrics/victoria"
)

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
	// Créer les labels
	labels := map[string]string{
		"server_id":   state.ServerID,
		"server_name": state.ServerName,
		"server_type": state.ServerType,
	}

	// Envoyer la métrique
	vmClient.SendMetrics(types.MetricBatch{
		Metrics: []types.Metric{
			{
				Name:        metrics.ServerPlayersConnected.Name,
				Type:        types.Gauge,
				Value:       float64(state.Players),
				Timestamp:   time.Now(),
				LabelValues: labels,
			},
		},
	})
}

// logEvent logs an event with contextual information about the server state.
func logEvent(eventType string, message string, state *types.ServerState, additionalLabels ...map[string]string) {
	if state == nil {
		utils.LogWarning("Cannot log event: state is nil")
		return
	}

	sessionType := "unknown"
	sessionID := ""
	if state.CurrentSession != nil {
		sessionType = state.CurrentSession.Type
		sessionID = state.CurrentSession.ID
	}

	// Create base labels
	labels := map[string]string{
		"server_id":    state.ServerID,
		"server_name":  state.ServerName,
		"server_type":  state.ServerType,
		"session_type": sessionType,
		"session_id":   sessionID,
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
		logsClient.LogServerEvent("INFO", message, eventType, labels)
	}
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
