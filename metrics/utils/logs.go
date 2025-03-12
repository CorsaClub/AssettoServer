package utils

import (
	"sync"
)

// LogsClient interface defines the methods required for a logs client
type LogsClient interface {
	// LogEvent envoie un log du wrapper
	LogEvent(level, message string, eventType string, labels map[string]string) error
	// LogServerEvent envoie un log du serveur Assetto Corsa
	LogServerEvent(level, message string, eventType string, labels map[string]string) error
	// LogChatMessage envoie un message de chat
	LogChatMessage(playerName, message string, labels map[string]string) error
}

var (
	logsClient     LogsClient
	logsClientLock sync.RWMutex
)

// SetLogsClient sets the global logs client
func SetLogsClient(client LogsClient) {
	logsClientLock.Lock()
	defer logsClientLock.Unlock()
	logsClient = client
}

// GetLogsClient returns the global logs client if available
func GetLogsClient() (LogsClient, bool) {
	logsClientLock.RLock()
	defer logsClientLock.RUnlock()
	if logsClient == nil {
		return nil, false
	}
	return logsClient, true
}
