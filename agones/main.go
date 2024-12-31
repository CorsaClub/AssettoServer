// Package main provides an Agones game server wrapper for Assetto Corsa Server.
// It handles server lifecycle, health checking, metrics monitoring, and graceful shutdown.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	sdk "agones.dev/agones/sdks/go"
)

// ServerState represents the current state of the Assetto Corsa server.
// It maintains thread-safe access to server metrics and status information.
type ServerState struct {
	sync.RWMutex
	ready     bool      // Indicates if the server is ready to accept connections
	players   int       // Current number of connected players
	lastPing  time.Time // Timestamp of the last successful health check
	allocated bool      // Indicates if the server is currently allocated
}

// interceptor implements an io.Writer that intercepts and forwards written data.
// It's used to capture and process server output while maintaining the original output stream.
type interceptor struct {
	forward   io.Writer      // The destination writer to forward data to
	intercept func(p []byte) // Function called for each write with the data
}

// Write implements io.Writer interface for interceptor.
// It calls the intercept function if defined and forwards the data to the original writer.
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
	serverState := &ServerState{
		lastPing: time.Now(),
	}

	// Create cancellable context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start health checking and metrics monitoring
	log.Println(">>> Starting health checking")
	go doHealth(ctx, s, serverState)
	go monitorMetrics(ctx, s, serverState)

	// Setup initial GameServer configuration
	if err := setupGameServer(s); err != nil {
		log.Fatalf(">>> Failed to setup GameServer: %v", err)
	}

	// Prepare and start the Assetto Corsa server
	cmd, serverReady := prepareServerCommand(ctx, input, args, s, serverState)
	if err := startServer(cmd, serverReady); err != nil {
		log.Fatalf(">>> Error Starting Cmd: %v", err)
	}

	// Handle termination signals
	go handleSignalsWithTimeout(cancel, s, *shutdownTimeout)

	// Wait for server readiness and manage lifecycle
	manageServerLifecycle(ctx, cmd, serverReady, s, cancel, *reserveDuration)
}

// prepareServerCommand creates and configures the exec.Cmd for the Assetto Corsa server.
// It sets up output interception and command arguments.
func prepareServerCommand(ctx context.Context, input *string, args *string, s *sdk.SDK, state *ServerState) (*exec.Cmd, chan struct{}) {
	argsList := strings.Fields(*args)
	cmd := exec.CommandContext(ctx, *input, argsList...)
	cmd.Stderr = &interceptor{forward: os.Stderr}

	serverReady := make(chan struct{}, 1)
	cmd.Stdout = &interceptor{
		forward: os.Stdout,
		intercept: func(p []byte) {
			str := strings.TrimSpace(string(p))
			handleServerOutput(str, s, state, serverReady, nil)
		}}

	return cmd, serverReady
}

// startServer starts the Assetto Corsa server process.
// It returns an error if the server fails to start.
func startServer(cmd *exec.Cmd, serverReady chan struct{}) error {
	log.Printf(">>> Starting server script: %s %v\n", cmd.Path, cmd.Args[1:])
	return cmd.Start()
}

// manageServerLifecycle handles the main server lifecycle including:
// - Waiting for server readiness
// - Managing server reservation
// - Handling server exit
func manageServerLifecycle(ctx context.Context, cmd *exec.Cmd, serverReady chan struct{}, s *sdk.SDK, cancel context.CancelFunc, reserveDuration time.Duration) {
	// Wait for server readiness
	if err := waitForServerReady(ctx, serverReady, s); err != nil {
		log.Printf(">>> Error waiting for server ready: %v", err)
		gracefulShutdown(s, cancel)
		return
	}

	// Start server reservation
	go handleReservation(ctx, s, reserveDuration)

	// Wait for server exit
	if err := cmd.Wait(); err != nil {
		handleServerExit(ctx, err, s, cancel)
	}
}

// handleServerOutput processes the server's output stream and updates server state accordingly.
// It detects various server events and triggers appropriate actions.
func handleServerOutput(output string, s *sdk.SDK, state *ServerState, serverReady chan struct{}, cancel context.CancelFunc) {
	switch {
	case strings.Contains(output, "Starting Assetto Corsa Server..."):
		log.Println(">>> Server starting up...")
	case strings.Contains(output, "Lobby registration successful"):
		log.Println(">>> Server is ready")
		state.Lock()
		state.ready = true
		state.Unlock()
		serverReady <- struct{}{}
	case strings.Contains(output, "timeleft"):
		log.Println(">>> End of session. Shutting down server.")
		gracefulShutdown(s, cancel)
	case strings.Contains(output, "has connected"):
		state.Lock()
		wasEmpty := state.players == 0
		state.players++
		state.Unlock()
		updatePlayerCount(s, state.players)
		// Allocate server when first player connects
		if wasEmpty {
			log.Println(">>> First player connected, allocating server")
			if err := s.Allocate(); err != nil {
				log.Printf(">>> Warning: Failed to allocate server: %v", err)
			} else {
				state.Lock()
				state.allocated = true
				state.Unlock()
			}
		}
		log.Printf(">>> Player connected, total players: %d", state.players)
	case strings.Contains(output, "has disconnected"):
		state.Lock()
		if state.players > 0 {
			state.players--
		}
		wasLastPlayer := state.players == 0 && state.allocated
		state.Unlock()
		updatePlayerCount(s, state.players)
		// If last player disconnected and server was allocated, mark it as ready again
		if wasLastPlayer {
			log.Println(">>> Last player disconnected, marking server as ready")
			if err := s.Ready(); err != nil {
				log.Printf(">>> Warning: Failed to mark server as ready: %v", err)
			} else {
				state.Lock()
				state.allocated = false
				state.Unlock()
			}
		}
		log.Printf(">>> Player disconnected, total players: %d", state.players)
	case strings.Contains(output, "Next session:"):
		log.Println(">>> Session change detected")
	case strings.Contains(output, "[ERR]"):
		log.Printf(">>> Server Error detected: %s", output)
	case strings.Contains(output, "Steam authentication succeeded"):
		log.Println(">>> Steam authentication successful for player")
	}
}

// doHealth performs periodic health checks and updates the server state.
// It attempts to reconnect to the SDK if health checks fail.
func doHealth(ctx context.Context, s *sdk.SDK, state *ServerState) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			state.Lock()
			state.lastPing = time.Now()
			state.Unlock()

			if err := s.Health(); err != nil {
				log.Printf(">>> Warning: Health ping failed: %v", err)
				// Attempt to reconnect to SDK if necessary
				if newSDK, err := sdk.NewSDK(); err == nil {
					s = newSDK
				}
			}
		}
	}
}

// monitorMetrics periodically collects and reports server metrics.
// It updates Agones annotations with current server state.
func monitorMetrics(ctx context.Context, s *sdk.SDK, state *ServerState) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			state.RLock()
			if gameServer, err := s.GameServer(); err != nil {
				log.Printf(">>> Warning: Failed to get GameServer status: %v", err)
			} else {
				log.Printf(">>> GameServer Status: %v, Players: %d, Ready: %v, Last Ping: %v",
					gameServer.Status.State,
					state.players,
					state.ready,
					time.Since(state.lastPing).Seconds())

				updateServerAnnotations(s, state)
			}
			state.RUnlock()
		}
	}
}

// updateServerAnnotations updates the Agones GameServer annotations with current state.
func updateServerAnnotations(s *sdk.SDK, state *ServerState) {
	annotations := map[string]string{
		"players":   fmt.Sprintf("%d", state.players),
		"ready":     fmt.Sprintf("%v", state.ready),
		"allocated": fmt.Sprintf("%v", state.allocated),
	}

	for key, value := range annotations {
		if err := s.SetAnnotation(key, value); err != nil {
			log.Printf(">>> Warning: Failed to set %s annotation: %v", key, err)
		}
	}
}

// handleReservation manages the GameServer reservation lifecycle.
// It periodically extends the reservation to keep the server allocated.
func handleReservation(ctx context.Context, s *sdk.SDK, duration time.Duration) {
	ticker := time.NewTicker(duration / 2)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.Reserve(duration); err != nil {
				log.Printf(">>> Warning: Failed to reserve server: %v", err)
			}
		}
	}
}

// setupGameServer initializes the GameServer configuration.
// It sets up labels and annotations for server identification and monitoring.
func setupGameServer(s *sdk.SDK) error {
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

// waitForServerReady waits for the server to signal readiness.
// It returns an error if the server fails to become ready within the timeout period.
func waitForServerReady(ctx context.Context, serverReady chan struct{}, s *sdk.SDK) error {
	select {
	case <-serverReady:
		log.Println(">>> Server reported ready, marking GameServer as Ready")
		if err := s.Ready(); err != nil {
			return fmt.Errorf("could not send ready message: %v", err)
		}
		return nil
	case <-time.After(2 * time.Minute):
		return fmt.Errorf("timeout waiting for server to be ready")
	case <-ctx.Done():
		return fmt.Errorf("context cancelled while waiting for server")
	}
}

// handleServerExit processes server termination events.
// It determines if the exit was expected and initiates appropriate shutdown procedures.
func handleServerExit(ctx context.Context, err error, s *sdk.SDK, cancel context.CancelFunc) {
	if ctx.Err() == context.Canceled {
		log.Println(">>> Server shutdown completed")
	} else {
		log.Printf(">>> Server exited unexpectedly: %v", err)
		gracefulShutdown(s, cancel)
	}
}

// gracefulShutdown performs a clean shutdown of the server.
// It notifies Agones of the shutdown and cancels the context.
func gracefulShutdown(s *sdk.SDK, cancel context.CancelFunc) {
	if err := s.Shutdown(); err != nil {
		log.Printf(">>> Warning: Could not send shutdown message: %v", err)
	}
	time.Sleep(time.Second)
	cancel()
}

// updatePlayerCount updates the player count metric and annotations.
// It logs the current player count and server state.
func updatePlayerCount(s *sdk.SDK, count int) {
	if gameServer, err := s.GameServer(); err != nil {
		log.Printf(">>> Warning: Failed to get GameServer status: %v", err)
	} else {
		log.Printf(">>> Player count updated: %d, Server State: %v", count, gameServer.Status.State)
		if err := s.SetAnnotation("players", fmt.Sprintf("%d", count)); err != nil {
			log.Printf(">>> Warning: Failed to update players annotation: %v", err)
		}
	}
}

// handleSignalsWithTimeout sets up signal handling for graceful shutdown.
// It waits for termination signals and initiates the shutdown process.
func handleSignalsWithTimeout(cancel context.CancelFunc, s *sdk.SDK, timeout time.Duration) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c
	log.Println(">>> Received termination signal. Starting graceful shutdown.")

	gracefulShutdown(s, cancel)

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	log.Printf(">>> Shutdown sequence completed")
}
