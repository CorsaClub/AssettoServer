// Création d'un nouveau client VictoriaLogs avec support pour le format JSON Stream

package victoria

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"metrics/config"
	"metrics/types"
	"metrics/utils"
)

// LogsClient est le client pour envoyer des logs à VictoriaLogs
type LogsClient struct {
	URL         string
	Username    string
	Password    string
	client      *http.Client
	config      config.VictoriaLogsConfig
	Compression bool
	Timeout     time.Duration
}

// NewLogsClient crée un nouveau client VictoriaLogs
func NewLogsClient(cfg *config.VictoriaLogsConfig) LogsClient {
	client := &http.Client{
		Timeout: cfg.Timeout,
	}

	return LogsClient{
		URL:         cfg.URL,
		Username:    cfg.Username,
		Password:    cfg.Password,
		client:      client,
		config:      *cfg,
		Compression: cfg.Compression,
		Timeout:     cfg.Timeout,
	}
}

// SendLogs envoie des logs à VictoriaLogs
func (c LogsClient) SendLogs(logs []types.Log) error {
	if len(logs) == 0 {
		return nil
	}

	// Préparer les données au format ndjson (une ligne JSON par log)
	var buffer bytes.Buffer
	for _, log := range logs {
		// Récupérer le server_id des labels ou utiliser une valeur par défaut
		serverID := "unknown"
		if id, ok := log.Labels["server_id"]; ok {
			serverID = id
		}

		// Convertir le log en format compatible avec VictoriaLogs
		// Structure conforme à l'exemple fourni avec server_id dans le stream
		logEntry := map[string]interface{}{
			"date": log.Timestamp.Format(time.RFC3339Nano),
			"log": map[string]interface{}{
				"level":   log.Level,
				"message": log.Message,
			},
			"stream": fmt.Sprintf("%s-%s", log.Source, serverID),
		}

		// Ajouter les labels comme champs supplémentaires
		for k, v := range log.Labels {
			// Éviter d'écraser les champs existants
			if k != "log" && k != "date" && k != "stream" {
				logEntry[k] = v
			}
		}

		// Encoder en JSON et ajouter au buffer
		if err := json.NewEncoder(&buffer).Encode(logEntry); err != nil {
			utils.LogError("Erreur lors de la sérialisation du log: %v", err)
			continue
		}
	}

	// Compresser les données si nécessaire
	var body io.Reader = &buffer
	if c.Compression {
		var compressedBuf bytes.Buffer
		gz := gzip.NewWriter(&compressedBuf)
		if _, err := io.Copy(gz, &buffer); err != nil {
			return fmt.Errorf("erreur de compression: %w", err)
		}
		if err := gz.Close(); err != nil {
			return fmt.Errorf("erreur de fermeture du compresseur: %w", err)
		}
		body = &compressedBuf
	}

	// Créer la requête avec les paramètres appropriés
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout)
	defer cancel()

	// Construire l'URL avec les paramètres requis conformes à l'exemple
	url := fmt.Sprintf("%s/insert/jsonline?_msg_field=log.message&_time_field=date&_stream_fields=stream", c.URL)

	req, err := http.NewRequestWithContext(ctx, "POST", url, body)
	if err != nil {
		utils.LogError("Erreur lors de la création de la requête: %v", err)
		return fmt.Errorf("erreur de création de requête: %w", err)
	}

	// Ajouter l'authentification si nécessaire
	if c.Username != "" && c.Password != "" {
		req.SetBasicAuth(c.Username, c.Password)
	}

	// Définir les en-têtes
	req.Header.Set("Content-Type", "application/stream+json")
	if c.Compression {
		req.Header.Set("Content-Encoding", "gzip")
	}

	// Ajouter un log de débogage pour voir les données envoyées
	if os.Getenv("DEBUG_LOGS") == "true" {
		sample := buffer.String()
		if len(sample) > 500 {
			sample = sample[:500] + "..." // Tronquer pour éviter des logs trop longs
		}
		utils.LogInfo("Échantillon de données JSONL envoyées: %s", sample)
	}

	// Envoyer la requête
	resp, err := c.client.Do(req)
	if err != nil {
		utils.LogError("Erreur lors de l'envoi des logs: %v", err)
		return fmt.Errorf("erreur d'envoi: %w", err)
	}
	defer resp.Body.Close()

	// Vérifier la réponse
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		utils.LogError("Échec de l'envoi des logs (code %d): %s",
			resp.StatusCode, string(respBody))
		return fmt.Errorf("code de statut inattendu: %d - %s", resp.StatusCode, string(respBody))
	}

	utils.LogInfo("Logs envoyés avec succès à VictoriaLogs (%d entrées)", len(logs))
	return nil
}

// LogEvent envoie un événement de log à VictoriaLogs
func (c LogsClient) LogEvent(level string, message string, eventType string, labels map[string]string) error {
	// S'assurer que server_id est présent
	serverID := "unknown"
	if id, ok := labels["server_id"]; ok {
		serverID = id
	}

	log := types.Log{
		Timestamp: time.Now(),
		Level:     level,
		Message:   message,
		Source:    "acserver",
		Labels:    labels,
	}

	// Ajouter server_id s'il n'est pas déjà présent
	if _, ok := log.Labels["server_id"]; !ok {
		log.Labels["server_id"] = serverID
	}

	return c.SendLogs([]types.Log{log})
}
