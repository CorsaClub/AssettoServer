// Création d'un nouveau client VictoriaLogs avec support pour le format JSON Stream

package victoria

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"metrics/config"
	"metrics/types"
	"metrics/utils"
)

// LogsClient interface pour l'envoi de logs
type LogsClient interface {
	SendLogs(logs []types.Log) error
}

// VictoriaLogsClient implémente l'interface LogsClient pour VictoriaLogs
type VictoriaLogsClient struct {
	config     *config.VictoriaLogsConfig
	httpClient *http.Client
}

// NewLogsClient crée un nouveau client VictoriaLogs
func NewLogsClient(config *config.VictoriaLogsConfig) LogsClient {
	return &VictoriaLogsClient{
		config: config,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// SendLogs envoie des logs à VictoriaLogs en utilisant le format JSON Stream
func (c *VictoriaLogsClient) SendLogs(logs []types.Log) error {
	if len(logs) == 0 {
		return nil
	}

	// Construire l'URL avec les paramètres requis
	url := fmt.Sprintf("%s/insert/jsonline?_time_field=timestamp&_msg_field=message&_stream_fields=source", c.config.URL)

	// Préparer les données au format JSON Stream (ndjson)
	var buffer bytes.Buffer
	for _, log := range logs {
		// Convertir le log en format compatible avec VictoriaLogs
		logEntry := map[string]interface{}{
			"timestamp": log.Timestamp.Format(time.RFC3339Nano),
			"message":   log.Message,
			"level":     log.Level,
			"source":    log.Source,
		}

		// Ajouter les labels comme champs supplémentaires
		for k, v := range log.Labels {
			logEntry[k] = v
		}

		// Encoder en JSON et ajouter au buffer
		if err := json.NewEncoder(&buffer).Encode(logEntry); err != nil {
			utils.LogError("Failed to encode log entry: %v", err)
			continue
		}
	}

	// Créer la requête
	req, err := http.NewRequest("POST", url, &buffer)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Définir les headers
	req.Header.Set("Content-Type", "application/stream+json")

	// Ajouter les credentials si nécessaire
	if c.config.Username != "" && c.config.Password != "" {
		req.SetBasicAuth(c.config.Username, c.config.Password)
	}

	// Envoyer la requête
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send logs: %w", err)
	}
	defer resp.Body.Close()

	// Vérifier la réponse
	if resp.StatusCode >= 400 {
		return fmt.Errorf("failed to send logs, status code: %d", resp.StatusCode)
	}

	utils.LogInfo("Successfully sent %d logs to VictoriaLogs", len(logs))
	return nil
}
