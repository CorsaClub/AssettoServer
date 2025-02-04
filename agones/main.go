// Package main provides an Agones game server wrapper for Assetto Corsa Server.
// It handles server lifecycle, health checking, metrics monitoring, and graceful shutdown.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	sdk "agones.dev/agones/sdks/go"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"agones/handlers"
	"agones/monitoring"
	"agones/types"
)

// interceptor implémente un io.Writer qui intercepte et transmet les données écrites
type interceptor struct {
	forward   io.Writer
	intercept func(p []byte)
}

func (i *interceptor) Write(p []byte) (n int, err error) {
	if i.intercept != nil {
		i.intercept(p)
	}
	return i.forward.Write(p)
}

// main is the entry point of the application.
// It initializes the Agones SDK, starts the Assetto Corsa server,
// and manages the server's lifecycle including health checks and metrics.
func main() {
	// Command line flags
	input := flag.String("i", "./start-server.sh", "Path to server start script")
	args := flag.String("args", "", "Arguments for the server")
	shutdownTimeout := flag.Duration("shutdown-timeout", 8*time.Second, "Shutdown timeout")
	reserveDuration := flag.Duration("reserve-duration", 10*time.Minute, "Duration for server reservation")
	flag.Parse()

	// Configure logging
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.LUTC | log.Lshortfile)

	// Initialize Agones SDK
	log.Println(">>> Connecting to Agones with the SDK")
	s, err := sdk.NewSDK()
	if err != nil {
		log.Fatalf(">>> Could not connect to sdk: %v", err)
	}

	// Initialize server state
	serverState := &types.ServerState{
		LastPing:         time.Now(),
		ConnectedPlayers: make(map[string]*types.Player),
		ActiveCars:       make(map[string]int),
		CurrentSession: &types.Session{
			Type: "initializing",
		},
	}

	// Create cancellable context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start health checking and metrics monitoring
	log.Println(">>> Starting health checking")
	go monitoring.DoHealth(ctx, s, serverState, cancel)
	go monitoring.MonitorMetrics(ctx, s, serverState)
	go monitoring.MonitorSystemResources(ctx, serverState)

	// Setup initial GameServer configuration
	if err := setupGameServer(s, serverState); err != nil {
		log.Fatalf(">>> Failed to setup GameServer: %v", err)
	}

	// Prepare and start the Assetto Corsa server
	serverReady := make(chan struct{}, 1)
	cmd := prepareServerCommand(ctx, input, args, s, serverState, serverReady)
	if err := cmd.Start(); err != nil {
		log.Fatalf(">>> Error Starting Cmd: %v", err)
	}

	// Handle termination signals
	setupSignalHandler(cancel, s, serverState, *shutdownTimeout)

	// Wait for server readiness and manage lifecycle
	waitForServerEnd(ctx, serverReady, s, *reserveDuration)

	// Initialize Prometheus metrics
	initMetrics()

	// Utiliser logEvent pour les messages importants
	logEvent("SERVER_START", "Starting Assetto Corsa Server...", serverState)

	// Create a separate mux for health checks
	healthMux := http.NewServeMux()

	// Add HTTP health endpoint
	healthMux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		serverState.RLock()
		defer serverState.RUnlock()

		conditions := []struct {
			check bool
			msg   string
		}{
			{serverState.Ready, "Server not ready"},
			{time.Since(serverState.LastPing) < 5*time.Second, "Health check timeout"},
			{!serverState.ShuttingDown, "Server is shutting down"},
		}

		for _, condition := range conditions {
			if !condition.check {
				log.Printf(">>> Health check failed: %s", condition.msg)
				w.WriteHeader(http.StatusServiceUnavailable)
				w.Write([]byte(condition.msg))
				return
			}
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Start HTTP server for health checks on a separate port
	go func() {
		server := &http.Server{
			Addr:         ":9001",
			Handler:      healthMux,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 5 * time.Second,
		}

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf(">>> HTTP health server error: %v", err)
		}
	}()
}

// prepareServerCommand creates and configures the exec.Cmd for the Assetto Corsa server.
// It sets up output interception and command arguments.
func prepareServerCommand(ctx context.Context, input *string, args *string, s *sdk.SDK, state *types.ServerState, serverReady chan struct{}) *exec.Cmd {
	argsList := strings.Fields(*args)
	cmd := exec.CommandContext(ctx, *input, argsList...)
	cmd.Stderr = &interceptor{forward: os.Stderr}

	cmd.Stdout = &interceptor{
		forward: os.Stdout,
		intercept: func(p []byte) {
			str := strings.TrimSpace(string(p))
			handlers.HandleServerOutput(str, s, state, serverReady, nil)
		},
	}

	return cmd
}

// waitForServerEnd waits for the server to signal readiness.
// It returns an error if the server fails to become ready within the timeout period.
func waitForServerEnd(ctx context.Context, serverReady chan struct{}, s *sdk.SDK, reserveDuration time.Duration) {
	select {
	case <-serverReady:
		log.Println(">>> Server reported ready, marking GameServer as Ready")
		if err := s.Ready(); err != nil {
			log.Fatalf(">>> Error marking GameServer as Ready: %v", err)
		}
		return
	case <-time.After(reserveDuration):
		log.Printf(">>> Reservation duration (%v) expired", reserveDuration)
		if err := s.Shutdown(); err != nil {
			log.Printf(">>> Failed to initiate shutdown after reservation: %v", err)
		}
		return
	case <-ctx.Done():
		log.Println(">>> Server shutdown completed")
		return
	}
}

// setupSignalHandler configures signal handling for graceful shutdown.
func setupSignalHandler(cancel context.CancelFunc, s *sdk.SDK, state *types.ServerState, timeout time.Duration) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		sig := <-sigChan
		log.Printf(">>> Received signal %v, initiating shutdown", sig)

		state.Lock()
		state.ShuttingDown = true
		state.Unlock()

		// Notify Agones of shutdown
		if err := s.Shutdown(); err != nil {
			log.Printf(">>> Failed to notify Agones of shutdown: %v", err)
		}

		time.Sleep(timeout)
		cancel()
	}()
}

// initMetrics initializes and exposes Prometheus metrics
func initMetrics() {
	// Expose metrics on /metrics
	http.Handle("/metrics", promhttp.Handler())
	go func() {
		if err := http.ListenAndServe(":9090", nil); err != nil {
			log.Printf(">>> Warning: Metrics server failed: %v", err)
		}
	}()
}

// logEvent logs important events
func logEvent(eventType string, message string, state *types.ServerState) {
	sessionType := "unknown"
	if state.CurrentSession != nil {
		sessionType = state.CurrentSession.Type
	}

	log.Printf("[%s] %s | Server: %s | Players: %d | Session: %s",
		eventType,
		message,
		state.ServerName,
		state.Players,
		sessionType)
}

// setupGameServer initializes the GameServer configuration
func setupGameServer(s *sdk.SDK, state *types.ServerState) error {
	gameServer, err := s.GameServer()
	if err != nil {
		return fmt.Errorf("failed to get GameServer: %v", err)
	}

	serverID := gameServer.ObjectMeta.Name
	serverName := gameServer.ObjectMeta.Labels["name"]
	serverType := gameServer.ObjectMeta.Labels["type"]

	state.Lock()
	state.ServerID = serverID
	state.ServerName = serverName
	state.ServerType = serverType
	state.Unlock()

	labels := map[string]string{
		"game":    "assetto-corsa",
		"version": "1.0",
		"type":    "racing",
		"region":  "weu",
	}

	for key, value := range labels {
		if err := s.SetLabel(key, value); err != nil {
			return fmt.Errorf("failed to set %s label: %v", key, err)
		}
	}

	annotations := map[string]string{
		"players":      "0",
		"ready":        "false",
		"session_type": "practice",
		"last_restart": time.Now().Format(time.RFC3339),
	}

	for key, value := range annotations {
		if err := s.SetAnnotation(key, value); err != nil {
			return fmt.Errorf("failed to set %s annotation: %v", key, err)
		}
	}

	return nil
}
