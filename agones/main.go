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
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	sdk "agones.dev/agones/sdks/go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Prometheus metrics
var (
	// Server identification labels
	serverLabels = []string{"server_id", "server_name", "server_type"}

	// Server state metrics
	serverStateGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "assetto_server_state",
		Help: "Current state of the server (0: Starting, 1: Ready, 2: Allocated, 3: Reserved, 4: Shutdown)",
	}, serverLabels)

	// Player metrics
	playersGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "assetto_server_players_total",
		Help: "Current number of players on the server",
	}, serverLabels)

	playerConnectCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "assetto_server_player_connects_total",
		Help: "Total number of player connections since server start",
	}, serverLabels)

	playerDisconnectCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "assetto_server_player_disconnects_total",
		Help: "Total number of player disconnections since server start",
	}, serverLabels)

	// Session metrics
	sessionChangeCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "assetto_server_session_changes_total",
		Help: "Total number of session changes",
	}, serverLabels)

	sessionDurationGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "assetto_server_session_duration_seconds",
		Help: "Duration of the current session in seconds",
	}, append(serverLabels, "session_type"))

	sessionStartTimeGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "assetto_server_session_start_timestamp",
		Help: "Start timestamp of the current session",
	}, append(serverLabels, "session_type"))

	// Server health metrics
	lastHealthPingGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "assetto_server_last_health_ping_seconds",
		Help: "Time since last successful health ping in seconds",
	}, serverLabels)

	healthPingFailuresCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "assetto_server_health_ping_failures_total",
		Help: "Total number of failed health pings",
	}, serverLabels)

	// Error metrics
	serverErrorsCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "assetto_server_errors_total",
		Help: "Total number of server errors detected",
	}, append(serverLabels, "error_type"))

	// Authentication metrics
	authSuccessCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "assetto_server_auth_successes_total",
		Help: "Total number of successful Steam authentications",
	}, serverLabels)

	// Resource metrics
	cpuUsageGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "assetto_server_cpu_usage_percent",
		Help: "CPU usage percentage of the server process",
	}, serverLabels)

	memoryUsageGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "assetto_server_memory_usage_bytes",
		Help: "Memory usage in bytes of the server process",
	}, serverLabels)

	// Network metrics
	networkBytesReceivedCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "assetto_server_network_bytes_received_total",
		Help: "Total number of bytes received by the server",
	}, serverLabels)

	networkBytesSentCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "assetto_server_network_bytes_sent_total",
		Help: "Total number of bytes sent by the server",
	}, serverLabels)

	// Track metrics
	trackUsageCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "assetto_server_track_usage_total",
		Help: "Number of times each track has been used",
	}, append(serverLabels, "track_name"))

	// Car metrics
	carUsageCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "assetto_server_car_usage_total",
		Help: "Number of times each car has been used",
	}, append(serverLabels, "car_name"))

	// Detailed session metrics
	sessionTypeGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "assetto_server_session_type",
		Help: "Current session type (0: Practice, 1: Qualifying, 2: Race)",
	}, serverLabels)

	sessionTimeLeftGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "assetto_server_session_time_left_seconds",
		Help: "Time remaining in current session in seconds",
	}, serverLabels)

	// Detailed player metrics
	playerLatencyGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "assetto_server_player_latency_ms",
		Help: "Player latency in milliseconds",
	}, append(serverLabels, "player_name", "steam_id"))

	playerBestLapGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "assetto_server_player_best_lap_ms",
		Help: "Player best lap time in milliseconds",
	}, append(serverLabels, "player_name", "steam_id", "car_model"))

	// Track conditions metrics
	trackGripGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "assetto_server_track_grip_level",
		Help: "Current track grip level percentage",
	}, serverLabels)

	trackTemperatureGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "assetto_server_track_temperature_celsius",
		Help: "Current track temperature in Celsius",
	}, serverLabels)

	airTemperatureGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "assetto_server_air_temperature_celsius",
		Help: "Current air temperature in Celsius",
	}, serverLabels)

	// Server performance metrics
	tickRateGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "assetto_server_tick_rate",
		Help: "Current server tick rate",
	}, serverLabels)

	// Connection quality metrics
	packetLossGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "assetto_server_packet_loss_percent",
		Help: "Current packet loss percentage",
	}, append(serverLabels, "player_name"))

	// Car metrics
	carCountGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "assetto_server_car_count",
		Help: "Number of cars per model currently in use",
	}, append(serverLabels, "car_model"))

	// Event metrics
	collisionCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "assetto_server_collisions_total",
		Help: "Total number of collisions",
	}, append(serverLabels, "severity"))

	penaltyCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "assetto_server_penalties_total",
		Help: "Total number of penalties given",
	}, append(serverLabels, "type"))

	// Chat metrics
	chatMessageCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "assetto_server_chat_messages_total",
		Help: "Total number of chat messages",
	}, append(serverLabels, "type")) // type: all, admin, team, driver

	// Vote metrics
	voteCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "assetto_server_votes_total",
		Help: "Total number of votes initiated",
	}, append(serverLabels, "type", "result"))

	// Ajouter des métriques réseau supplémentaires
	networkLatencyGauge = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "assetto_server_network_latency_ms",
		Help: "Average network latency in milliseconds",
	}, serverLabels)
)

// ServerState represents the current state of the Assetto Corsa server.
// It maintains thread-safe access to server metrics and status information.
type ServerState struct {
	sync.RWMutex
	ready            bool               // Indicates if the server is ready to accept connections
	players          int                // Current number of connected players
	lastPing         time.Time          // Timestamp of the last successful health check
	allocated        bool               // Indicates if the server is currently allocated
	serverID         string             // Unique identifier for the server
	serverName       string             // Name of the server
	serverType       string             // Type of the server (practice, race, etc.)
	sessionType      string             // Current session type
	sessionStart     time.Time          // Start time of current session
	sessionTimeLeft  int                // Time left in current session (seconds)
	currentTrack     string             // Current track name
	currentLayout    string             // Current track layout
	trackTemp        float64            // Track temperature
	airTemp          float64            // Air temperature
	trackGrip        float64            // Track grip level
	connectedPlayers map[string]*Player // Map of connected players
	activeCars       map[string]int     // Map of active car models and their count
	tickRate         float64            // Current server tick rate
	currentSession   *Session           // Current session
}

// Player structure to store per-player metrics
type Player struct {
	Name       string
	SteamID    string
	CarModel   string
	BestLap    int64
	LastLap    int64
	Latency    int
	PacketLoss float64
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
	serverState := newServerState()

	// Create cancellable context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start health checking and metrics monitoring
	log.Println(">>> Starting health checking")
	go doHealth(ctx, s, serverState)
	go monitorMetrics(ctx, s, serverState)

	// Setup initial GameServer configuration
	if err := setupGameServer(s, serverState); err != nil {
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
	manageServerLifecycle(ctx, cmd, serverReady, s, cancel, *reserveDuration, serverState)

	// Initialize Prometheus metrics
	initMetrics()

	// Dans la fonction main, après l'initialisation du serveur
	go monitorSystemResources(ctx, serverState)

	// Utiliser logEvent pour les messages importants
	logEvent("SERVER_START", "Starting Assetto Corsa Server...", serverState)
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
func manageServerLifecycle(ctx context.Context, cmd *exec.Cmd, serverReady chan struct{}, s *sdk.SDK, cancel context.CancelFunc, reserveDuration time.Duration, state *ServerState) {
	// Wait for server readiness
	if err := waitForServerReady(ctx, serverReady, s); err != nil {
		log.Printf(">>> Error waiting for server ready: %v", err)
		gracefulShutdown(s, cancel)
		return
	}

	// Start server reservation
	go handleReservation(ctx, s, state, reserveDuration)

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
		serverStateGauge.With(prometheus.Labels{"server_id": state.serverID, "server_name": state.serverName, "server_type": state.serverType}).Set(0) // Starting state
	case strings.Contains(output, "Lobby registration successful"):
		log.Println(">>> Server is ready")
		state.Lock()
		state.ready = true
		state.Unlock()
		serverStateGauge.With(prometheus.Labels{"server_id": state.serverID, "server_name": state.serverName, "server_type": state.serverType}).Set(1) // Ready state
		serverReady <- struct{}{}
	case strings.Contains(output, "End of session"):
		log.Println(">>> Session ended, initiating server shutdown")
		serverStateGauge.With(prometheus.Labels{"server_id": state.serverID, "server_name": state.serverName, "server_type": state.serverType}).Set(4) // Shutdown state
		gracefulShutdown(s, cancel)
	case strings.Contains(output, "has connected"):
		player := extractPlayerInfo(output)
		addPlayer(state, player)
		playersGauge.With(prometheus.Labels{
			"server_id":   state.serverID,
			"server_name": state.serverName,
			"server_type": state.serverType,
		}).Set(float64(state.players))
		playerConnectCounter.With(prometheus.Labels{
			"server_id":   state.serverID,
			"server_name": state.serverName,
			"server_type": state.serverType,
		}).Inc()
		updatePlayerCount(s, state.players)
	case strings.Contains(output, "has disconnected"):
		steamID := extractSteamID(output)
		removePlayer(state, steamID)
		playersGauge.With(prometheus.Labels{
			"server_id":   state.serverID,
			"server_name": state.serverName,
			"server_type": state.serverType,
		}).Set(float64(state.players))
		playerDisconnectCounter.With(prometheus.Labels{
			"server_id":   state.serverID,
			"server_name": state.serverName,
			"server_type": state.serverType,
		}).Inc()
		updatePlayerCount(s, state.players)
	case strings.Contains(output, "Next session:"):
		logEvent("SESSION_CHANGE", "Session change detected", state)
		sessionType := extractSessionType(output)
		track := extractTrackName(output)
		startNewSession(state, sessionType, track)
		sessionChangeCounter.With(prometheus.Labels{
			"server_id":   state.serverID,
			"server_name": state.serverName,
			"server_type": state.serverType,
		}).Inc()
	case strings.Contains(output, "[ERR]"):
		handleError(fmt.Errorf(output), "server_error", state)
	case strings.Contains(output, "Steam authentication succeeded"):
		log.Println(">>> Steam authentication successful for player")
		authSuccessCounter.With(prometheus.Labels{"server_id": state.serverID, "server_name": state.serverName, "server_type": state.serverType}).Inc()
	case strings.Contains(output, "[S_API FAIL]"):
		handleError(fmt.Errorf(output), "steam_api_error", state)
		// Tentative de réinitialisation
		go func() {
			time.Sleep(5 * time.Second)
			if err := s.Ready(); err != nil {
				log.Printf(">>> Failed to reinitialize Steam connection: %v", err)
			}
		}()
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
				healthPingFailuresCounter.With(prometheus.Labels{"server_id": state.serverID, "server_name": state.serverName, "server_type": state.serverType}).Inc()
				// Attempt to reconnect to SDK if necessary
				if newSDK, err := sdk.NewSDK(); err == nil {
					s = newSDK
				}
			}

			state.RLock()
			lastHealthPingGauge.With(prometheus.Labels{"server_id": state.serverID, "server_name": state.serverName, "server_type": state.serverType}).Set(time.Since(state.lastPing).Seconds())
			state.RUnlock()
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
				updateMetrics(s, state)
				updateDetailedMetrics(s, state)
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
func handleReservation(ctx context.Context, s *sdk.SDK, state *ServerState, duration time.Duration) {
	ticker := time.NewTicker(duration / 2)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.Reserve(duration); err != nil {
				log.Printf(">>> Warning: Failed to reserve server: %v", err)
			} else {
				serverStateGauge.With(prometheus.Labels{"server_id": state.serverID, "server_name": state.serverName, "server_type": state.serverType}).Set(3) // Reserved state
			}
		}
	}
}

// setupGameServer initializes the GameServer configuration.
// It sets up labels and annotations for server identification and monitoring.
func setupGameServer(s *sdk.SDK, state *ServerState) error {
	// Get server details from GameServer object
	gameServer, err := s.GameServer()
	if err != nil {
		return fmt.Errorf("failed to get GameServer: %v", err)
	}

	serverID := gameServer.ObjectMeta.Name
	serverName := gameServer.ObjectMeta.Labels["name"]
	serverType := gameServer.ObjectMeta.Labels["type"]

	// Update ServerState with server identification
	state.Lock()
	state.serverID = serverID
	state.serverName = serverName
	state.serverType = serverType
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

// updateMetrics updates all Prometheus metrics with current values
func updateMetrics(s *sdk.SDK, state *ServerState) {
	state.RLock()
	defer state.RUnlock()

	labels := prometheus.Labels{
		"server_id":   state.serverID,
		"server_name": state.serverName,
		"server_type": state.serverType,
	}

	// Update all metrics with proper labels
	serverStateGauge.With(labels).Set(float64(getServerStateValue(state)))
	playersGauge.With(labels).Set(float64(state.players))
	lastHealthPingGauge.With(labels).Set(time.Since(state.lastPing).Seconds())

	// Update session metrics if a session is active
	if !state.sessionStart.IsZero() {
		sessionLabels := prometheus.Labels{
			"server_id":    state.serverID,
			"server_name":  state.serverName,
			"server_type":  state.serverType,
			"session_type": state.sessionType,
		}
		sessionDurationGauge.With(sessionLabels).Set(time.Since(state.sessionStart).Seconds())
		sessionStartTimeGauge.With(sessionLabels).Set(float64(state.sessionStart.Unix()))
	}

	// Update resource metrics (example implementation)
	if usage, err := getProcessCPUUsage(); err == nil {
		cpuUsageGauge.With(labels).Set(usage)
	}
	if usage, err := getProcessMemoryUsage(); err == nil {
		memoryUsageGauge.With(labels).Set(float64(usage))
	}
}

// getServerStateValue converts server state to numeric value
func getServerStateValue(state *ServerState) int {
	switch {
	case !state.ready:
		return 0 // Starting
	case state.ready && !state.allocated:
		return 1 // Ready
	case state.allocated:
		return 2 // Allocated
	default:
		return 0
	}
}

// getProcessCPUUsage returns the CPU usage percentage of the current process
func getProcessCPUUsage() (float64, error) {
	cmd := exec.Command("ps", "-p", fmt.Sprintf("%d", os.Getpid()), "-o", "%cpu")
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("failed to get CPU usage: %v", err)
	}

	// Parse the output, skipping the header line
	lines := strings.Split(string(output), "\n")
	if len(lines) < 2 {
		return 0, fmt.Errorf("unexpected ps output format")
	}

	cpu := strings.TrimSpace(lines[1])
	cpuUsage, err := strconv.ParseFloat(cpu, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse CPU usage: %v", err)
	}

	return cpuUsage, nil
}

// getProcessMemoryUsage returns the memory usage in bytes of the current process
func getProcessMemoryUsage() (uint64, error) {
	cmd := exec.Command("ps", "-p", fmt.Sprintf("%d", os.Getpid()), "-o", "rss=")
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("failed to get memory usage: %v", err)
	}

	// Convert KB to bytes
	memKB, err := strconv.ParseUint(strings.TrimSpace(string(output)), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse memory usage: %v", err)
	}

	return memKB * 1024, nil
}

// updateDetailedMetrics updates all detailed Prometheus metrics
func updateDetailedMetrics(s *sdk.SDK, state *ServerState) {
	state.RLock()
	defer state.RUnlock()

	labels := prometheus.Labels{
		"server_id":   state.serverID,
		"server_name": state.serverName,
		"server_type": state.serverType,
	}

	// Update session metrics
	sessionTypeValue := 0
	switch state.sessionType {
	case "practice":
		sessionTypeValue = 0
	case "qualifying":
		sessionTypeValue = 1
	case "race":
		sessionTypeValue = 2
	}
	sessionTypeGauge.With(labels).Set(float64(sessionTypeValue))
	sessionTimeLeftGauge.With(labels).Set(float64(state.sessionTimeLeft))

	// Update track condition metrics
	trackGripGauge.With(labels).Set(state.trackGrip)
	trackTemperatureGauge.With(labels).Set(state.trackTemp)
	airTemperatureGauge.With(labels).Set(state.airTemp)
	tickRateGauge.With(labels).Set(state.tickRate)

	// Update per-player metrics
	for _, player := range state.connectedPlayers {
		playerLabels := prometheus.Labels{
			"server_id":   state.serverID,
			"server_name": state.serverName,
			"server_type": state.serverType,
			"player_name": player.Name,
			"steam_id":    player.SteamID,
		}

		// Latency metrics
		playerLatencyGauge.With(playerLabels).Set(float64(player.Latency))

		// Packet loss metrics
		packetLossGauge.With(playerLabels).Set(player.PacketLoss)

		// Best lap metrics
		if player.BestLap > 0 {
			lapLabels := prometheus.Labels{
				"server_id":   state.serverID,
				"server_name": state.serverName,
				"server_type": state.serverType,
				"player_name": player.Name,
				"steam_id":    player.SteamID,
				"car_model":   player.CarModel,
			}
			playerBestLapGauge.With(lapLabels).Set(float64(player.BestLap))
		}
	}

	// Update car count metrics
	for model, count := range state.activeCars {
		carLabels := prometheus.Labels{
			"server_id":   state.serverID,
			"server_name": state.serverName,
			"server_type": state.serverType,
			"car_model":   model,
		}
		carCountGauge.With(carLabels).Set(float64(count))
	}
}

// Initialize maps in ServerState constructor
func newServerState() *ServerState {
	return &ServerState{
		lastPing:         time.Now(),
		connectedPlayers: make(map[string]*Player),
		activeCars:       make(map[string]int),
	}
}

// Ajouter une fonction pour surveiller les ressources système
func monitorSystemResources(ctx context.Context, state *ServerState) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cpu, err := getProcessCPUUsage()
			if err == nil {
				state.Lock()
				cpuUsageGauge.With(prometheus.Labels{
					"server_id":   state.serverID,
					"server_name": state.serverName,
					"server_type": state.serverType,
				}).Set(cpu)
				state.Unlock()
			}

			mem, err := getProcessMemoryUsage()
			if err == nil {
				state.Lock()
				memoryUsageGauge.With(prometheus.Labels{
					"server_id":   state.serverID,
					"server_name": state.serverName,
					"server_type": state.serverType,
				}).Set(float64(mem))
				state.Unlock()
			}
		}
	}
}

// Ajouter une structure et des fonctions pour mieux gérer les sessions
type Session struct {
	Type       string
	StartTime  time.Time
	EndTime    time.Time
	Duration   time.Duration
	Track      string
	Conditions TrackConditions
}

type TrackConditions struct {
	GripLevel   float64
	Temperature float64
	Weather     string
	TimeOfDay   string
}

// Fonction pour démarrer une nouvelle session
func startNewSession(state *ServerState, sessionType, track string) {
	state.Lock()
	defer state.Unlock()

	state.currentSession = &Session{
		Type:      sessionType,
		StartTime: time.Now(),
		Track:     track,
	}
}

// Ajouter une fonction de journalisation structurée
func logEvent(eventType string, message string, state *ServerState) {
	log.Printf("[%s] %s | Server: %s | Players: %d | Session: %s",
		eventType,
		message,
		state.serverName,
		state.players,
		state.currentSession.Type)
}

// Ajouter des fonctions pour mieux gérer les joueurs
func addPlayer(state *ServerState, player Player) {
	state.Lock()
	defer state.Unlock()

	state.connectedPlayers[player.SteamID] = &player
	state.players++
}

func removePlayer(state *ServerState, steamID string) {
	state.Lock()
	defer state.Unlock()

	delete(state.connectedPlayers, steamID)
	if state.players > 0 {
		state.players--
	}
}

// Ajouter une fonction pour gérer les erreurs de manière centralisée
func handleError(err error, errorType string, state *ServerState) {
	log.Printf(">>> Error (%s): %v", errorType, err)
	serverErrorsCounter.With(prometheus.Labels{
		"server_id":   state.serverID,
		"server_name": state.serverName,
		"server_type": state.serverType,
		"error_type":  errorType,
	}).Inc()
}

// Fonctions pour extraire les informations des logs
func extractSessionType(output string) string {
	// Implémentation pour extraire le type de session
	return "practice" // Exemple
}

func extractTrackName(output string) string {
	// Implémentation pour extraire le nom de la piste
	return "monza" // Exemple
}

func extractPlayerInfo(output string) Player {
	// Implémentation pour extraire les informations du joueur
	return Player{
		Name:     "Player1",
		SteamID:  "123456789",
		CarModel: "ks_corvette_c7",
	}
}

func extractSteamID(output string) string {
	// Implémentation pour extraire le SteamID
	return "123456789"
}
