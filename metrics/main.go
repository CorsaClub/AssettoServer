// Package main provides an Agones game server wrapper for Assetto Corsa Server.
// It handles server lifecycle, health checking, metrics monitoring, and graceful shutdown.
package main

import (
	"context"
	"flag"
	"os"
	"sync"

	"metrics/config"
	"metrics/logging"
	"metrics/server"
	"metrics/utils"
	"metrics/victoria"
)

func main() {
	// Enable debug logs if needed
	os.Setenv("DEBUG_LOGS", "true")

	// Create configuration
	cfg := config.NewDefaultConfig()

	// Initialize logging
	logManager, err := logging.NewLogManager(&cfg.VictoriaLogs)
	if err != nil {
		panic("Failed to initialize logging: " + err.Error())
	}
	utils.SetLogsClient(logManager.Client())

	logManager.LogEvent("INFO", "Starting wrapper", "startup", map[string]string{
		"test_mode": os.Getenv("TEST_MODE"),
	})

	// Create a WaitGroup to track goroutines
	var wg sync.WaitGroup

	// Parse configuration flags
	input := flag.String("i", "./start-server.sh", "Path to server start script")
	args := flag.String("args", "", "Arguments for the server")
	flag.Parse()

	// Initialize VictoriaMetrics client
	metricsClient := victoria.NewClient(cfg)

	// Create a context with cancellation for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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
