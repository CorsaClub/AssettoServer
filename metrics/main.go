// Package main provides an Agones game server wrapper for Assetto Corsa Server.
// It handles server lifecycle, health checking, metrics monitoring, and graceful shutdown.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"metrics/config"
	"metrics/geoip"
	"metrics/handlers"
	"metrics/types"
	"metrics/utils"
	"metrics/victoria"
	"metrics/websocket"
)

// interceptor implements an io.Writer that intercepts and forwards written data
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

// waitForTerminationSignal waits for a termination signal and cancels the context
func waitForTerminationSignal(cancel context.CancelFunc) {
	// Create a channel for signals
	sigs := make(chan os.Signal, 1)

	// Register signals to intercept
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	// Wait for a signal
	sig := <-sigs
	utils.LogInfo("Received signal: %v", sig)

	// Cancel the context to trigger graceful shutdown
	cancel()
}

// main is the entry point of the application.
// It initializes the Agones SDK, starts the Assetto Corsa server,
// and manages the server's lifecycle including health checks and metrics.
func main() {
	// Enable debug logs if needed
	os.Setenv("DEBUG_LOGS", "true")
	utils.LogInfo("Starting wrapper with TEST_MODE=%s", os.Getenv("TEST_MODE"))

	// Create a WaitGroup to track goroutines
	var wg sync.WaitGroup

	// Parse configuration flags
	input := flag.String("i", "./start-server.sh", "Path to server start script")
	args := flag.String("args", "", "Arguments for the server")
	flag.Parse()

	// Get or generate server ID
	serverID := os.Getenv("GAMESERVER_ID")
	if serverID == "" {
		serverID = utils.GenerateServerID()
		utils.LogInfo("Generated server ID: %s", serverID)
	} else {
		utils.LogInfo("Using environment server ID: %s", serverID)
	}

	// Get server metadata from environment
	serverRegion := os.Getenv("GAMESERVER_REGION")
	serverName := os.Getenv("SERVER_NAME")
	serverType := os.Getenv("SERVER_TYPE")

	// Initialize server state with ID
	serverState := &types.ServerState{
		ServerID:         serverID,
		ServerRegion:     serverRegion,
		ServerName:       serverName,
		ServerType:       serverType,
		LastPing:         time.Now(),
		StartTime:        time.Now(),
		ConnectedPlayers: make(map[string]*types.Player),
		ActiveCars:       make(map[string]int),
		CurrentSession: &types.Session{
			Type: "initializing",
		},
	}

	// Create server configuration
	cfg := config.NewDefaultConfig()
	cfg.ServerID = serverID
	cfg.ServerName = serverName
	cfg.ServerRegion = serverRegion
	cfg.ServerType = serverType

	// Initialize VictoriaMetrics client
	metricsClient := initVictoriaMetrics()

	// Initialize VictoriaLogs client
	logsClient := initVictoriaLogs()

	// Set the logs client in the utils package for global access
	utils.SetLogsClient(logsClient)

	// Initialize GeoIP service if enabled
	geoipService := initGeoIPService(cfg.GeoIP)

	// Test connections to metrics and logs services
	testVictoriaMetricsConnection(metricsClient, serverState.ServerID)
	testVictoriaLogsConnection(logsClient)

	// Create a channel for server events
	eventChan := make(chan string, 100)

	// Create a context with cancellation for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize WebSocket server
	authConfig := config.NewAuthConfig()
	wsServer := websocket.NewWebSocketServer(authConfig)
	go wsServer.Start(ctx)

	// Initialize health check server
	initHealthServer(serverState, wsServer)

	// Start the keep-alive routine
	go startKeepAliveRoutine(ctx)

	// Start the server ready channel
	serverReady := make(chan struct{})

	// Prepare and start the server command
	cmd := prepareServerCommand(ctx, input, args, serverState, serverReady, metricsClient, wsServer, logsClient, geoipService, cancel)

	// Setup signal handler for graceful shutdown
	setupSignalHandler(cancel, serverState)

	// Start event processing goroutine
	go processServerEvents(ctx, eventChan, serverState, metricsClient, logsClient, geoipService)

	// Wait for server to be ready
	<-serverReady
	utils.LogInfo("Server is ready to accept connections")

	// Monitor process exit
	go monitorProcessExit(cmd)

	// Wait for context cancellation
	<-ctx.Done()
	utils.LogInfo("Context cancelled, shutting down...")

	// Wait for all goroutines to finish
	wg.Wait()
	utils.LogInfo("All goroutines finished, exiting")
}

// processServerEvents handles server events from the event channel
func processServerEvents(ctx context.Context, eventChan <-chan string, state *types.ServerState,
	metricsClient *victoria.MetricsClient, logsClient victoria.LogsClient, geoipService *geoip.GeoIPService) {

	for {
		select {
		case <-ctx.Done():
			return
		case event := <-eventChan:
			// Determine event type and log level
			eventType := "server_output"
			level := "INFO"

			// Process different event types
			if strings.Contains(event, "Collision between") {
				eventType = "collision"
				level = "WARNING"
				// Process collision event
				processCollisionEvent(event, state, metricsClient)
			} else if strings.Contains(event, "LAP") {
				eventType = "lap"
				// Process lap event
				processLapEvent(event, state, metricsClient)
			} else if strings.Contains(event, "Network stats") {
				eventType = "network_stats"
				// Process network stats
				processNetworkStats(event, state, metricsClient)
			} else if strings.Contains(event, "CONNECTED") {
				eventType = "player_connected"
				// Process player connection
				processPlayerConnection(event, state, metricsClient, geoipService)
			} else if strings.Contains(event, "DISCONNECTED") {
				eventType = "player_disconnected"
				// Process player disconnection
				processPlayerDisconnection(event, state, metricsClient)
			} else if strings.Contains(event, "SESSION") {
				eventType = "session_change"
				// Process session change
				processSessionChange(event, state, metricsClient)
			} else if strings.Contains(event, "ERROR") {
				eventType = "error"
				level = "ERROR"
				// Process error
				processErrorEvent(event, state, metricsClient)
			} else if strings.Contains(event, "Warning") {
				eventType = "warning"
				level = "WARNING"
				// Process warning
				processWarningEvent(event, state, metricsClient)
			}

			// Log the event
			labels := map[string]string{
				"server_id":   state.ServerID,
				"server_name": state.ServerName,
				"event_type":  eventType,
			}
			logsClient.LogEvent(level, event, eventType, labels)
		}
	}
}

// initGeoIPService initializes the GeoIP service if enabled
func initGeoIPService(geoipConfig config.GeoIPConfig) *geoip.GeoIPService {
	if !geoipConfig.Enabled {
		utils.LogInfo("GeoIP service is disabled")
		return nil
	}

	utils.LogInfo("Initializing GeoIP service with database: %s", geoipConfig.DatabasePath)
	geoipService, err := geoip.InitGeoIPService(&geoipConfig)
	if err != nil {
		utils.LogWarning("Failed to initialize GeoIP service: %v", err)
		return nil
	}

	utils.LogInfo("GeoIP service initialized successfully")
	return geoipService
}

// processCollisionEvent processes a collision event
func processCollisionEvent(event string, state *types.ServerState, metricsClient *victoria.MetricsClient) {
	// Implementation details
}

// processLapEvent processes a lap event
func processLapEvent(event string, state *types.ServerState, metricsClient *victoria.MetricsClient) {
	// Implementation details
}

// processNetworkStats processes network statistics
func processNetworkStats(event string, state *types.ServerState, metricsClient *victoria.MetricsClient) {
	// Implementation details
}

// processPlayerConnection processes a player connection event
func processPlayerConnection(event string, state *types.ServerState,
	metricsClient *victoria.MetricsClient, geoipService *geoip.GeoIPService) {
	// Implementation details
}

// processPlayerDisconnection processes a player disconnection event
func processPlayerDisconnection(event string, state *types.ServerState, metricsClient *victoria.MetricsClient) {
	// Implementation details
}

// processSessionChange processes a session change event
func processSessionChange(event string, state *types.ServerState, metricsClient *victoria.MetricsClient) {
	// Implementation details
}

// processErrorEvent processes an error event
func processErrorEvent(event string, state *types.ServerState, metricsClient *victoria.MetricsClient) {
	// Implementation details
}

// processWarningEvent processes a warning event
func processWarningEvent(event string, state *types.ServerState, metricsClient *victoria.MetricsClient) {
	// Implementation details
}

// prepareServerCommand creates and configures the exec.Cmd for the Assetto Corsa server.
// It sets up output interception and command arguments.
func prepareServerCommand(ctx context.Context, input *string, args *string, state *types.ServerState, serverReady chan struct{}, metricsClient *victoria.MetricsClient, wsServer *websocket.WebSocketServer, logsClient victoria.LogsClient, geoipService *geoip.GeoIPService, cancel context.CancelFunc) *exec.Cmd {
	utils.LogInfo("Preparing server command: %s", *input)

	// Check if the script file exists and is executable
	fileInfo, err := os.Stat(*input)
	if err != nil {
		utils.LogError("Error checking script file: %v", err)
		if os.IsNotExist(err) {
			utils.LogError("Script file does not exist: %s", *input)
			// List files in directory to help diagnose
			dir := filepath.Dir(*input)
			files, listErr := os.ReadDir(dir)
			if listErr != nil {
				utils.LogError("Error listing directory %s: %v", dir, listErr)
			} else {
				utils.LogInfo("Files in %s:", dir)
				for _, file := range files {
					fileInfo, err := file.Info()
					if err != nil {
						utils.LogInfo("  %s (dir: %v, size: unknown - %v)",
							file.Name(), file.IsDir(), err)
					} else {
						utils.LogInfo("  %s (dir: %v, size: %d)",
							file.Name(), file.IsDir(), fileInfo.Size())
					}
				}
			}
		}
	} else {
		// Log file permissions
		mode := fileInfo.Mode()
		utils.LogInfo("Script file permissions: %s", mode.String())

		// Check if file is executable
		if mode&0111 == 0 {
			utils.LogWarning("Script file is not executable, attempting to make it executable")
			if chmodErr := os.Chmod(*input, 0755); chmodErr != nil {
				utils.LogError("Failed to make script executable: %v", chmodErr)
			} else {
				utils.LogInfo("Successfully made script executable")
			}
		}

		// Read first few lines of the script for debugging
		file, err := os.Open(*input)
		if err != nil {
			utils.LogError("Failed to open script file: %v", err)
		} else {
			defer file.Close()
			scanner := bufio.NewScanner(file)
			lineCount := 0
			utils.LogInfo("Script file contents (first 5 lines):")
			for scanner.Scan() && lineCount < 5 {
				utils.LogInfo("  %s", scanner.Text())
				lineCount++
			}
			if err := scanner.Err(); err != nil {
				utils.LogError("Error reading script file: %v", err)
			}
		}
	}

	argsList := strings.Fields(*args)
	cmd := exec.CommandContext(ctx, *input, argsList...)

	// Set the working directory explicitly
	cmd.Dir = "/app"

	// Log the command details
	utils.LogInfo("Command details:")
	utils.LogInfo("  Path: %s", cmd.Path)
	utils.LogInfo("  Args: %v", cmd.Args)
	utils.LogInfo("  Dir: %s", cmd.Dir)
	utils.LogInfo("  Env vars: %d", len(cmd.Env))

	// Set environment variables explicitly
	cmd.Env = os.Environ()

	cmd.Stderr = &interceptor{
		forward: os.Stderr,
		intercept: func(p []byte) {
			str := strings.TrimSpace(string(p))
			utils.LogError("Server stderr: %s", str)

			// Envoyer directement à VictoriaLogs
			logsClient.LogEvent("ERROR", str, "server_error", map[string]string{
				"server_id":  state.ServerID,
				"session_id": state.CurrentSession.ID,
			})
		},
	}

	cmd.Stdout = &interceptor{
		forward: os.Stdout,
		intercept: func(p []byte) {
			str := strings.TrimSpace(string(p))

			// Log all server output for debugging
			utils.LogInfo("Server stdout: %s", str)

			// Create log entry
			logEntry := types.LogEntry{
				Timestamp: time.Now(),
				Level:     "INFO",
				Message:   str,
				ServerID:  state.ServerID,
				SessionID: state.CurrentSession.ID,
			}

			// Send to WebSocket
			wsServer.BroadcastLog(logEntry)

			// Process the output through the handler
			handlers.HandleServerOutput(str, metricsClient, state, serverReady, cancel, geoipService)
		},
	}

	return cmd
}

// Enhanced signal handler with more detailed logging
func setupSignalHandler(cancel context.CancelFunc, state *types.ServerState) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)

	go func() {
		sig := <-c
		utils.LogInfo("=== SIGNAL RECEIVED ===")
		utils.LogInfo("Received signal: %v", sig)
		utils.LogInfo("Current goroutines: %d", runtime.NumGoroutine())

		// Log server state
		state.RLock()
		utils.LogInfo("Server state at signal: Ready=%v, Players=%d, ShuttingDown=%v",
			state.Ready, state.Players, state.ShuttingDown)
		state.RUnlock()

		// Set shutting down flag
		state.Lock()
		state.ShuttingDown = true
		state.Unlock()

		utils.LogInfo("Set ShuttingDown flag to true")

		// Only cancel if not in test mode
		if os.Getenv("TEST_MODE") != "true" {
			utils.LogInfo("Calling cancel() to terminate context")
			cancel()
		} else {
			utils.LogInfo("In TEST_MODE, ignoring shutdown request")
		}

		utils.LogInfo("=== END SIGNAL HANDLING ===")
	}()

	utils.LogInfo("Signal handler set up for SIGTERM, SIGINT, SIGHUP")
}

// initHealthServer initializes and exposes health check endpoints
func initHealthServer(state *types.ServerState, wsServer *websocket.WebSocketServer) {
	// Create a separate mux for health checks
	healthMux := http.NewServeMux()

	// Add HTTP health endpoint
	healthMux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		// In test mode, always return healthy
		if os.Getenv("TEST_MODE") == "true" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK (Test Mode)"))
			return
		}

		state.RLock()
		defer state.RUnlock()

		conditions := []struct {
			check bool
			msg   string
		}{
			{state.Ready, "Server not ready"},
			{time.Since(state.LastPing) < 5*time.Second, "Health check timeout"},
			{!state.ShuttingDown, "Server is shutting down"},
		}

		for _, condition := range conditions {
			if !condition.check {
				utils.LogWarning("Health check failed: %s", condition.msg)
				w.WriteHeader(http.StatusServiceUnavailable)
				w.Write([]byte(condition.msg))
				return
			}
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Add WebSocket handler
	healthMux.HandleFunc("/ws", wsServer.HandleWebSocket)

	// Start HTTP server for health checks on a separate port
	go func() {
		server := &http.Server{
			Addr:         ":9600",
			Handler:      healthMux,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 5 * time.Second,
		}

		utils.LogInfo("Starting health check server on port 9600")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			utils.LogError("HTTP health server error: %v", err)
		}
	}()
}

func initVictoriaMetrics() *victoria.MetricsClient {
	cfg := config.NewDefaultConfig()

	// Configure VictoriaMetrics URL and credentials
	if url := os.Getenv("VICTORIA_METRICS_URL"); url != "" {
		if port := os.Getenv("VICTORIA_METRICS_PORT"); port != "" {
			cfg.Victoria.URL = fmt.Sprintf("http://%s:%s", url, port)
		} else {
			cfg.Victoria.URL = fmt.Sprintf("http://%s:%s", url, config.DefaultVictoriaPort)
		}
	}

	// Configure credentials for VictoriaMetrics
	if user := os.Getenv("VICTORIA_METRICS_USERNAME"); user != "" {
		cfg.Victoria.Username = user
	}
	if pass := os.Getenv("VICTORIA_METRICS_PASSWORD"); pass != "" {
		cfg.Victoria.Password = pass
	}

	// Configure request timeout
	if timeout := os.Getenv("VICTORIA_METRICS_REQUEST_TIMEOUT"); timeout != "" {
		if duration, err := time.ParseDuration(timeout); err == nil {
			cfg.Victoria.RequestTimeout = duration
		}
	}

	// Configure connection timeout
	if timeout := os.Getenv("VICTORIA_METRICS_CONNECT_TIMEOUT"); timeout != "" {
		if duration, err := time.ParseDuration(timeout); err == nil {
			cfg.Victoria.ConnectTimeout = duration
		}
	}

	// Configure retry settings
	if retries := os.Getenv("VICTORIA_METRICS_MAX_RETRIES"); retries != "" {
		if val, err := strconv.Atoi(retries); err == nil {
			cfg.Victoria.MaxRetries = val
		}
	}

	// Configure retry backoff
	if backoff := os.Getenv("VICTORIA_METRICS_RETRY_BACKOFF"); backoff != "" {
		if duration, err := time.ParseDuration(backoff); err == nil {
			cfg.Victoria.RetryBackoff = duration
		}
	}

	// Configure metrics batch size
	if batchSize := os.Getenv("METRICS_BATCH_SIZE"); batchSize != "" {
		if val, err := strconv.Atoi(batchSize); err == nil {
			cfg.Metrics.BatchSize = val
		}
	}

	// Configure metrics flush interval
	if flushInterval := os.Getenv("METRICS_FLUSH_INTERVAL"); flushInterval != "" {
		if duration, err := time.ParseDuration(flushInterval); err == nil {
			cfg.Metrics.FlushInterval = duration
		}
	}

	// Configure metrics buffer size
	if bufferSize := os.Getenv("METRICS_BUFFER_SIZE"); bufferSize != "" {
		if val, err := strconv.Atoi(bufferSize); err == nil {
			cfg.Metrics.BufferSize = val
		}
	}

	// Configure metrics retention time
	if retention := os.Getenv("METRICS_RETENTION_TIME"); retention != "" {
		if duration, err := time.ParseDuration(retention); err == nil {
			cfg.Metrics.RetentionTime = duration
		}
	}

	// Configure metrics compression
	if compression := os.Getenv("METRICS_COMPRESSION"); compression != "" {
		if val, err := strconv.ParseBool(compression); err == nil {
			cfg.Metrics.Compression = val
		}
	}

	// Display configuration information for debugging
	utils.LogInfo("VictoriaMetrics Configuration:")
	utils.LogInfo("  URL: %s", cfg.Victoria.URL)
	utils.LogInfo("  Username: %v", cfg.Victoria.Username != "")
	utils.LogInfo("  Password: %v", cfg.Victoria.Password != "")
	utils.LogInfo("  Request Timeout: %v", cfg.Victoria.RequestTimeout)
	utils.LogInfo("  Connect Timeout: %v", cfg.Victoria.ConnectTimeout)
	utils.LogInfo("  Max Retries: %d", cfg.Victoria.MaxRetries)
	utils.LogInfo("  Retry Backoff: %v", cfg.Victoria.RetryBackoff)
	utils.LogInfo("  Batch Size: %d", cfg.Metrics.BatchSize)
	utils.LogInfo("  Flush Interval: %v", cfg.Metrics.FlushInterval)
	utils.LogInfo("  Buffer Size: %d", cfg.Metrics.BufferSize)
	utils.LogInfo("  Retention Time: %v", cfg.Metrics.RetentionTime)
	utils.LogInfo("  Compression: %v", cfg.Metrics.Compression)

	return victoria.NewClient(cfg)
}

func initVictoriaLogs() victoria.LogsClient {
	cfg := config.NewDefaultConfig()

	// Configure VictoriaLogs URL and credentials
	if url := os.Getenv("VICTORIA_LOGS_URL"); url != "" {
		if port := os.Getenv("VICTORIA_LOGS_PORT"); port != "" {
			cfg.VictoriaLogs.URL = fmt.Sprintf("http://%s:%s", url, port)
		} else {
			cfg.VictoriaLogs.URL = fmt.Sprintf("http://%s:%s", url, config.DefaultVictoriaLogsPort)
		}
	}

	// Configure credentials for VictoriaLogs
	if user := os.Getenv("VICTORIA_LOGS_USERNAME"); user != "" {
		cfg.VictoriaLogs.Username = user
	}
	if pass := os.Getenv("VICTORIA_LOGS_PASSWORD"); pass != "" {
		cfg.VictoriaLogs.Password = pass
	}

	// Display configuration information for debugging
	utils.LogInfo("VictoriaLogs Configuration:")
	utils.LogInfo("  URL: %s", cfg.VictoriaLogs.URL)
	utils.LogInfo("  Username: %s", cfg.VictoriaLogs.Username != "")
	utils.LogInfo("  Password: %s", cfg.VictoriaLogs.Password != "")

	// Configure timeout settings
	if timeout := os.Getenv("VICTORIA_LOGS_TIMEOUT"); timeout != "" {
		if duration, err := time.ParseDuration(timeout); err == nil {
			cfg.VictoriaLogs.Timeout = duration
			utils.LogInfo("  Timeout: %s", duration)
		}
	}

	// Configure compression
	if compression := os.Getenv("VICTORIA_LOGS_COMPRESSION"); compression != "" {
		if val, err := strconv.ParseBool(compression); err == nil {
			cfg.VictoriaLogs.Compression = val
			utils.LogInfo("  Compression: %t", val)
		}
	}

	return victoria.NewLogsClient(&cfg.VictoriaLogs)
}

// startKeepAliveRoutine starts a routine that keeps the process alive
func startKeepAliveRoutine(ctx context.Context) {
	utils.LogInfo("Starting keep-alive routine")
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			utils.LogInfo("Keep-alive routine stopped")
			return
		case <-ticker.C:
			utils.LogDebug("Keep-alive tick")
		}
	}
}

// monitorProcessExit monitors the exit of the server process
func monitorProcessExit(cmd *exec.Cmd) {
	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			utils.LogError("Server process exited with code %d: %v", exitErr.ExitCode(), err)
			if exitErr.Stderr != nil {
				utils.LogError("Server stderr: %s", string(exitErr.Stderr))
			}
		} else {
			utils.LogError("Server process exited with error: %v", err)
		}

		// Only initiate graceful shutdown if this is not a test mode
		if os.Getenv("TEST_MODE") != "true" {
			utils.LogInfo("Initiating graceful shutdown due to server process exit")
			// Signal termination to the main process
			p, err := os.FindProcess(os.Getpid())
			if err == nil {
				p.Signal(syscall.SIGTERM)
			}
		} else {
			utils.LogInfo("Test script completed, but keeping container alive")
		}
	} else {
		utils.LogInfo("Server process exited normally")

		// Signal termination to the main process if not in test mode
		if os.Getenv("TEST_MODE") != "true" {
			utils.LogInfo("Initiating graceful shutdown due to server process exit")
			p, err := os.FindProcess(os.Getpid())
			if err == nil {
				p.Signal(syscall.SIGTERM)
			}
		} else {
			utils.LogInfo("Test script completed, but keeping container alive")
		}
	}
}

// testVictoriaMetricsConnection tests the connection to VictoriaMetrics
func testVictoriaMetricsConnection(client *victoria.MetricsClient, serverID string) {
	// Create a test metric
	testMetric := types.MetricBatch{
		Metrics: []types.Metric{
			{
				Name:      "assetto_server_test",
				Value:     1.0,
				Timestamp: time.Now(),
				LabelValues: map[string]string{
					"server_id": serverID,
					"test":      "true",
				},
			},
		},
	}

	// Send the test metric
	err := client.SendMetricsImmediate(testMetric)
	if err != nil {
		utils.LogWarning("Failed to connect to VictoriaMetrics: %v", err)
		utils.LogInfo("Metrics will be buffered and retried later")
	} else {
		utils.LogInfo("Successfully connected to VictoriaMetrics")
	}
}

// testVictoriaLogsConnection tests the connection to VictoriaLogs
func testVictoriaLogsConnection(client victoria.LogsClient) {
	// Create a test log
	err := client.LogEvent("info", "Test connection to VictoriaLogs", "test", map[string]string{
		"test": "true",
	})

	if err != nil {
		utils.LogWarning("Failed to connect to VictoriaLogs: %v", err)
		utils.LogInfo("Logs will be buffered and retried later")
	} else {
		utils.LogInfo("Successfully connected to VictoriaLogs")
	}
}
