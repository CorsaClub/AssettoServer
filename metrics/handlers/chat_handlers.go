package handlers

import (
	"strings"

	"metrics/metrics"
	"metrics/types"
	"metrics/utils"
)

// handleChatMessage processes chat messages and updates relevant metrics.
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
		"server_id":    labels["server_id"],
		"server_name":  labels["server_name"],
		"server_type":  labels["server_type"],
	}

	// Envoyer directement à VictoriaLogs comme message de chat
	if logsClient, ok := utils.GetLogsClient(); ok {
		logsClient.LogChatMessage(playerName, messageContent, chatLabels)
	}
}

// determineChatType determines the type of chat message based on its content.
func determineChatType(message string) string {
	if strings.HasPrefix(message, "/admin") {
		return "admin"
	}
	if strings.HasPrefix(message, "/t ") {
		return "team"
	}
	return "global"
}
