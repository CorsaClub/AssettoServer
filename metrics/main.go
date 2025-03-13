// Package main provides an Agones game server wrapper for Assetto Corsa Server.
// It handles server lifecycle, health checking, metrics monitoring, and graceful shutdown.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"metrics/config"
	"metrics/env"
	"metrics/logging"
	"metrics/server"
	"metrics/types"
	"metrics/utils"
	"metrics/victoria"
)

func main() {
	// Enable debug logs if needed
	os.Setenv("DEBUG_LOGS", "true")
	os.Setenv("DEBUG_METRICS", "true")

	// Initialiser les variables d'environnement avec le nouveau package env
	envVars := env.GetEnv()

	// Afficher les variables d'environnement
	envVars.LogEnvironmentVariables()

	// Create configuration
	cfg := config.NewDefaultConfig()

	// Test basic HTTP connectivity to VictoriaMetrics and VictoriaLogs
	testBasicConnectivity(cfg.Victoria.URL, cfg.VictoriaLogs.URL)

	// Initialize logging
	logManager, err := logging.NewLogManager(&cfg.VictoriaLogs)
	if err != nil {
		panic("Failed to initialize logging: " + err.Error())
	}
	utils.SetLogsClient(logManager.Client())

	logManager.LogEvent("INFO", "Starting wrapper", "startup", map[string]string{
		"test_mode": fmt.Sprintf("%v", envVars.TestMode),
	})

	// Create a WaitGroup to track goroutines
	var wg sync.WaitGroup

	// Parse configuration flags
	input := flag.String("i", "./start-server.sh", "Path to server start script")
	args := flag.String("args", "", "Arguments for the server")
	flag.Parse()

	// Initialize VictoriaMetrics client
	metricsClient := victoria.NewClient(cfg)

	// Test connection to VictoriaMetrics
	fmt.Println("=== Testing Functional Connectivity ===")
	fmt.Println("Note: Ces tests vérifient la fonctionnalité complète des services en envoyant des données de test.")

	// Test metrics connection
	if err := metricsClient.TestConnection(); err != nil {
		logManager.LogEvent("ERROR", "Échec du test de connexion à VictoriaMetrics: "+err.Error(), "startup", nil)
		fmt.Printf("ERREUR: Le test de connexion à VictoriaMetrics a échoué: %v\n", err)
		fmt.Println("Les métriques seront mises en mémoire tampon et réessayées plus tard.")
		logManager.LogEvent("WARNING", "Les métriques seront mises en mémoire tampon et réessayées plus tard", "startup", nil)
	} else {
		logManager.LogEvent("INFO", "Test de connexion réussi pour VictoriaMetrics", "startup", nil)
		fmt.Println("SUCCÈS: Le test de connexion a réussi pour VictoriaMetrics.")
		fmt.Println("Les métriques seront envoyées normalement.")
	}

	// Test logs connection
	if err := logManager.Client().TestConnection(); err != nil {
		logManager.LogEvent("ERROR", "Échec du test de connexion à VictoriaLogs: "+err.Error(), "startup", nil)
		fmt.Printf("ERREUR: Le test de connexion à VictoriaLogs a échoué: %v\n", err)
		fmt.Println("Les logs seront mis en mémoire tampon et réessayés plus tard.")
		logManager.LogEvent("WARNING", "Les logs seront mis en mémoire tampon et réessayés plus tard", "startup", nil)
	} else {
		logManager.LogEvent("INFO", "Test de connexion réussi pour VictoriaLogs", "startup", nil)
		fmt.Println("SUCCÈS: Le test de connexion a réussi pour VictoriaLogs.")
		fmt.Println("Les logs seront envoyés normalement.")
	}

	fmt.Println("=====================================")

	// Envoyer des métriques de test supplémentaires
	fmt.Println("=== Envoi de métriques et logs de test supplémentaires ===")

	// Envoyer une métrique de test
	testMetric := types.MetricBatch{
		Metrics: []types.Metric{
			{
				Name:      "assetto_server_test_metric",
				Value:     42.0,
				Type:      types.Gauge,
				Timestamp: time.Now(),
				LabelValues: map[string]string{
					"test":   "true",
					"source": "main",
				},
			},
		},
		Time: time.Now(),
	}

	if err := metricsClient.SendMetricsImmediate(testMetric); err != nil {
		fmt.Printf("ERREUR: Impossible d'envoyer la métrique de test: %v\n", err)
	} else {
		fmt.Println("SUCCÈS: Métrique de test envoyée avec succès")
	}

	// Envoyer un log de test
	logManager.LogEvent("INFO", "Ceci est un log de test depuis main.go", "test", map[string]string{
		"test":      "true",
		"source":    "main",
		"timestamp": time.Now().Format(time.RFC3339),
	})
	fmt.Println("SUCCÈS: Log de test envoyé")

	fmt.Println("=====================================")

	// Create a context with cancellation for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Démarrer le buffer de métriques en arrière-plan
	go metricsClient.StartMetricBuffer(ctx)
	fmt.Println("Buffer de métriques démarré en arrière-plan")

	// Create and start the server
	srv := server.New(cfg, metricsClient, logManager.Client())
	srv.SetupSignalHandler(cancel)

	if err := srv.Start(ctx, &wg, *input, *args); err != nil {
		logManager.LogEvent("ERROR", "Failed to start server: "+err.Error(), "startup", nil)
		os.Exit(1)
	}

	// Wait for context cancellation
	<-ctx.Done()
	logManager.LogEvent("INFO", "Context cancelled, shutting down...", "shutdown", nil)

	// Wait for all goroutines to finish
	wg.Wait()
	logManager.LogEvent("INFO", "All goroutines finished, exiting", "shutdown", nil)
}

// testBasicConnectivity performs a simple HTTP GET request to check if the services are accessible
func testBasicConnectivity(metricsURL, logsURL string) {
	fmt.Println("=== Testing Basic Connectivity ===")
	fmt.Println("Note: Ces tests vérifient uniquement l'accessibilité des services, pas leur fonctionnalité complète.")

	// Test VictoriaMetrics base URL
	resp, err := http.Get(metricsURL)
	if err != nil {
		fmt.Printf("ERREUR: Impossible de se connecter à VictoriaMetrics à %s: %v\n", metricsURL, err)
	} else {
		fmt.Printf("SUCCÈS: VictoriaMetrics est accessible à %s (statut: %s)\n", metricsURL, resp.Status)
		resp.Body.Close()
	}

	// Test VictoriaMetrics metrics endpoint
	metricsEndpoint := fmt.Sprintf("%s/api/v1/import/prometheus", metricsURL)
	req, err := http.NewRequest("HEAD", metricsEndpoint, nil)
	if err != nil {
		fmt.Printf("ERREUR: Impossible de créer une requête pour l'endpoint des métriques VictoriaMetrics: %v\n", err)
	} else {
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			fmt.Printf("ERREUR: Impossible de se connecter à l'endpoint des métriques VictoriaMetrics à %s: %v\n", metricsEndpoint, err)
		} else {
			fmt.Printf("SUCCÈS: L'endpoint des métriques VictoriaMetrics est accessible à %s (statut: %s)\n", metricsEndpoint, resp.Status)
			fmt.Println("        Les métriques seront envoyées à cet endpoint au format Prometheus exposition.")
			resp.Body.Close()
		}
	}

	// Test VictoriaLogs base URL
	resp, err = http.Get(logsURL)
	if err != nil {
		fmt.Printf("ERREUR: Impossible de se connecter à VictoriaLogs à %s: %v\n", logsURL, err)
	} else {
		fmt.Printf("SUCCÈS: VictoriaLogs est accessible à %s (statut: %s)\n", logsURL, resp.Status)
		resp.Body.Close()
	}

	// Test VictoriaLogs logs endpoint
	logsEndpoint := fmt.Sprintf("%s/insert/jsonline", logsURL)
	req, err = http.NewRequest("HEAD", logsEndpoint, nil)
	if err != nil {
		fmt.Printf("ERREUR: Impossible de créer une requête pour l'endpoint des logs VictoriaLogs: %v\n", err)
	} else {
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			fmt.Printf("ERREUR: Impossible de se connecter à l'endpoint des logs VictoriaLogs à %s: %v\n", logsEndpoint, err)
		} else {
			fmt.Printf("SUCCÈS: L'endpoint des logs VictoriaLogs est accessible à %s (statut: %s)\n", logsEndpoint, resp.Status)
			fmt.Println("        Les logs et événements seront envoyés à cet endpoint au format JSON Stream.")
			resp.Body.Close()
		}
	}

	fmt.Println("================================")
}
