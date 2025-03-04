// Package main provides an Agones game server wrapper for Assetto Corsa Server.
// It handles server lifecycle, health checking, metrics monitoring, and graceful shutdown.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
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
	"metrics/handlers"
	"metrics/monitoring"
	metrics "metrics/services"
	"metrics/types"
	"metrics/utils"
	"metrics/victoria"
	"metrics/websocket"
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
	// At the beginning of main()
	utils.LogInfo("Starting wrapper with TEST_MODE=%s", os.Getenv("TEST_MODE"))

	// Create a WaitGroup to track goroutines
	var wg sync.WaitGroup

	// Configuration flags
	input := flag.String("i", "./start-server.sh", "Path to server start script")
	args := flag.String("args", "", "Arguments for the server")

	flag.Parse()

	// In main() function, after flag parsing
	serverID := os.Getenv("GAMESERVER_ID")
	if serverID == "" {
		serverID = utils.GenerateServerID()
		utils.LogInfo("Generated server ID: %s", serverID)
	} else {
		utils.LogInfo("Using environment server ID: %s", serverID)
	}

	serverRegion := os.Getenv("GAMESERVER_REGION")
	serverName := os.Getenv("SERVER_NAME")

	// Initialize server state with ID
	serverState := &types.ServerState{
		ServerID:         serverID,
		ServerRegion:     serverRegion,
		ServerName:       serverName,
		ServerType:       os.Getenv("SERVER_TYPE"),
		LastPing:         time.Now(),
		StartTime:        time.Now(),
		ConnectedPlayers: make(map[string]*types.Player),
		ActiveCars:       make(map[string]int),
		CurrentSession: &types.Session{
			Type: "initializing",
		},
	}

	// Charger la configuration
	serverConfig := &types.Config{
		VictoriaMetrics: struct {
			Endpoint    string        `json:"endpoint"`
			MaxRetries  int           `json:"max_retries"`
			BatchSize   int           `json:"batch_size"`
			BatchPeriod time.Duration `json:"batch_period"`
			Timeout     time.Duration `json:"timeout"`
		}{
			Endpoint:    *flag.String("victoria-endpoint", "http://localhost:8428", "VictoriaMetrics endpoint"),
			MaxRetries:  3,
			BatchSize:   100,
			BatchPeriod: 5 * time.Second,
			Timeout:     10 * time.Second,
		},
		Logging: struct {
			Directory   string   `json:"directory"`
			Patterns    []string `json:"patterns"`
			MaxFileSize int64    `json:"max_file_size"`
			MaxFiles    int      `json:"max_files"`
		}{
			Directory:   "/var/log/acserver",
			Patterns:    []string{"error.log", "server.log", "access.log"},
			MaxFileSize: 100 * 1024 * 1024, // 100MB
			MaxFiles:    10,
		},
	}

	// Initialiser les deux clients
	metricsClient := initVictoriaMetrics()
	logsClient := initVictoriaLogs()

	// Démarrer le monitoring avec les deux clients
	logMonitor := monitoring.NewLogMonitor(
		metricsClient,
		logsClient,
		serverConfig.Logging.Directory,
		monitoring.WithPatterns(serverConfig.Logging.Patterns),
		monitoring.WithMaxFileSize(serverConfig.Logging.MaxFileSize),
	)

	// Démarrer les goroutines avec gestion appropriée
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// For each goroutine, add to the WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		metricsClient.StartMetricBuffer(ctx)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		logMonitor.Start(ctx)
	}()

	// Start monitoring
	go monitoring.MonitorHealthMetrics(ctx, metricsClient, serverState)
	go monitoring.MonitorSystemResources(ctx, serverState)
	go monitoring.MonitorDetailedMetrics(ctx, metricsClient, serverState)

	// Démarrer le monitoring des performances internes
	go metrics.StartPerformanceMonitoring(ctx, metricsClient)

	// Initialiser la configuration d'authentification
	authConfig := config.NewAuthConfig()
	if !authConfig.IsValid() {
		utils.LogWarning("WebSocket authentication not configured (AUTH_STEAM_ID and AUTH_USER_ID required)")
	}

	// Initialiser le serveur WebSocket avec l'authentification
	wsServer := websocket.NewWebSocketServer(authConfig)
	go wsServer.Start(ctx)

	// Prepare and start the server
	serverReady := make(chan struct{}, 1)
	var scriptPath string
	if os.Getenv("TEST_MODE") == "true" {
		scriptPath = "/app/test-script.sh"
		utils.LogInfo("Running in test mode with script: %s", scriptPath)
	} else {
		scriptPath = *input
	}
	cmd := prepareServerCommand(ctx, &scriptPath, args, serverState, serverReady, metricsClient, wsServer)
	if err := cmd.Start(); err != nil {
		utils.LogError("Error Starting Cmd: %v", err)
		os.Exit(1)
	}

	// Start monitoring the process
	monitorProcessExit(cmd)

	// Add this code to wait for the command to finish with detailed error reporting
	go func() {
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
				cancel()
			} else {
				utils.LogInfo("Test script completed, but keeping container alive")
				// In test mode, start a simple keep-alive routine
				startKeepAliveRoutine(ctx)
			}
		} else {
			utils.LogInfo("Server process exited normally")
			// Only initiate graceful shutdown if this is not a test mode
			if os.Getenv("TEST_MODE") != "true" {
				cancel()
			} else {
				utils.LogInfo("Test script completed, but keeping container alive")
				// In test mode, start a simple keep-alive routine
				startKeepAliveRoutine(ctx)
			}
		}
	}()

	// Handle termination signals
	setupSignalHandler(cancel, serverState)

	// Initialize HTTP server for health checks
	initHealthServer(serverState, wsServer)

	// After initializing the metrics client
	utils.LogInfo("Testing VictoriaMetrics connection")
	testVictoriaMetricsConnection(metricsClient, serverID)

	// Dans la fonction main, après l'initialisation du client
	utils.LogInfo("Testing direct metric send to VictoriaMetrics")
	testMetric := types.MetricBatch{
		Metrics: []types.Metric{
			{
				Name:      "assetto_server_test_direct",
				Value:     float64(time.Now().Unix()),
				Type:      types.Gauge,
				Timestamp: time.Now(),
				LabelValues: map[string]string{
					"server_id": serverState.ServerID,
					"test":      "direct_send",
					"timestamp": time.Now().Format(time.RFC3339),
				},
			},
		},
		Time: time.Now(),
	}

	// Envoi direct sans passer par le buffer
	if err := metricsClient.SendMetricsImmediate(testMetric); err != nil {
		utils.LogError("Direct metric send failed: %v", err)
	} else {
		utils.LogInfo("Direct metric send successful")
	}

	// At the end of main()
	utils.LogInfo("Main function completed, container should continue running")

	// Block forever to keep the application running
	utils.LogInfo("Blocking main goroutine to keep container alive")
	blockForever := make(chan struct{})

	// Add a goroutine to periodically log the application state
	go func() {
		stateTicker := time.NewTicker(60 * time.Second)
		defer stateTicker.Stop()

		for {
			select {
			case <-ctx.Done():
				utils.LogInfo("Context cancelled, stopping state logging")
				return
			case <-stateTicker.C:
				// Log detailed application state
				serverState.RLock()
				utils.LogInfo("=== APPLICATION STATE ===")
				utils.LogInfo("Server ID: %s, Name: %s, Region: %s",
					serverState.ServerID, serverState.ServerName, serverState.ServerRegion)
				utils.LogInfo("Ready: %v, Players: %d, ShuttingDown: %v",
					serverState.Ready, serverState.Players, serverState.ShuttingDown)
				utils.LogInfo("Session Type: %s, Session ID: %s",
					serverState.CurrentSession.Type, serverState.CurrentSession.ID)
				utils.LogInfo("Last Ping: %v (%v ago)",
					serverState.LastPing, time.Since(serverState.LastPing))
				utils.LogInfo("Connected Players: %d", len(serverState.ConnectedPlayers))
				utils.LogInfo("Active Goroutines: %d", runtime.NumGoroutine())

				var memStats runtime.MemStats
				runtime.ReadMemStats(&memStats)
				utils.LogInfo("Memory Usage: Alloc=%v MB, Sys=%v MB",
					memStats.Alloc/1024/1024, memStats.Sys/1024/1024)

				utils.LogInfo("=== END STATE ===")
				serverState.RUnlock()
			}
		}
	}()

	// Add a goroutine to monitor for potential exit conditions
	go func() {
		utils.LogInfo("Starting exit condition monitor")
		for {
			time.Sleep(5 * time.Second)

			// Check if main context is done
			select {
			case <-ctx.Done():
				utils.LogInfo("Main context cancelled - this could lead to application exit")
				utils.LogInfo("Context error: %v", ctx.Err())
				break
			default:
				// Context still active
			}

			// Check server state
			serverState.RLock()
			if serverState.ShuttingDown {
				utils.LogInfo("Server is in shutting down state - this could lead to application exit")
			}
			serverState.RUnlock()
		}
	}()

	<-blockForever // This will block forever
}

// prepareServerCommand creates and configures the exec.Cmd for the Assetto Corsa server.
// It sets up output interception and command arguments.
func prepareServerCommand(ctx context.Context, input *string, args *string, state *types.ServerState, serverReady chan struct{}, vmClient *victoria.MetricsClient, wsServer *websocket.WebSocketServer) *exec.Cmd {
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

			// Process log normally
			handlers.HandleServerOutput(str, vmClient, state, serverReady, nil)
		},
	}

	return cmd
}

// waitForServerEnd waits for the server to signal readiness.
// It returns an error if the server fails to become ready within the timeout period.
func waitForServerEnd(ctx context.Context, serverReady chan struct{}, vmClient *victoria.MetricsClient, reserveDuration time.Duration) {
	select {
	case <-serverReady:
		utils.LogSDK("Server reported ready")
		vmClient.SendMetrics(types.MetricBatch{
			Metrics: []types.Metric{
				{
					Name:        "server_ready",
					Value:       1,
					Type:        types.Gauge,
					Timestamp:   time.Now(),
					LabelValues: map[string]string{},
				},
			},
			Time: time.Now(),
		})
	case <-ctx.Done():
		utils.LogSDK("Context cancelled, initiating graceful shutdown")
		return
	}

	// Add graceful shutdown handling
	<-ctx.Done()
	utils.LogSDK("Server shutdown initiated")
	vmClient.SendMetrics(types.MetricBatch{
		Metrics: []types.Metric{
			{
				Name:        "server_shutdown",
				Value:       1,
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: map[string]string{},
			},
		},
		Time: time.Now(),
	})
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

	// Configuration Victoria Metrics URL et credentials
	if url := os.Getenv("VICTORIA_METRICS_URL"); url != "" {
		if port := os.Getenv("VICTORIA_METRICS_PORT"); port != "" {
			cfg.Victoria.URL = fmt.Sprintf("http://%s:%s", url, port)
		} else {
			cfg.Victoria.URL = fmt.Sprintf("http://%s:%s", url, config.DefaultVictoriaPort)
		}
	}

	// Configuration Victoria Logs URL et credentials
	if url := os.Getenv("VICTORIA_LOGS_URL"); url != "" {
		if port := os.Getenv("VICTORIA_LOGS_PORT"); port != "" {
			cfg.VictoriaLogs.URL = fmt.Sprintf("http://%s:%s", url, port)
		} else {
			cfg.VictoriaLogs.URL = fmt.Sprintf("http://%s:%s", url, config.DefaultVictoriaLogsPort)
		}
	}

	// Configuration des credentials pour VictoriaMetrics
	if user := os.Getenv("VICTORIA_METRICS_USERNAME"); user != "" {
		cfg.Victoria.Username = user
	}
	if pass := os.Getenv("VICTORIA_METRICS_PASSWORD"); pass != "" {
		cfg.Victoria.Password = pass
	}

	// Configuration des credentials pour VictoriaLogs
	if user := os.Getenv("VICTORIA_LOGS_USERNAME"); user != "" {
		cfg.VictoriaLogs.Username = user
	}
	if pass := os.Getenv("VICTORIA_LOGS_PASSWORD"); pass != "" {
		cfg.VictoriaLogs.Password = pass
	}

	if size := os.Getenv("METRICS_BATCH_SIZE"); size != "" {
		if val, err := strconv.Atoi(size); err == nil {
			cfg.Metrics.BatchSize = val
		}
	}

	if interval := os.Getenv("METRICS_FLUSH_INTERVAL"); interval != "" {
		if duration, err := time.ParseDuration(interval); err == nil {
			cfg.Metrics.FlushInterval = duration
		}
	}

	if size := os.Getenv("METRICS_BUFFER_SIZE"); size != "" {
		if val, err := strconv.Atoi(size); err == nil {
			cfg.Metrics.BufferSize = val
		}
	}

	if retention := os.Getenv("METRICS_RETENTION_TIME"); retention != "" {
		if duration, err := time.ParseDuration(retention); err == nil {
			cfg.Metrics.RetentionTime = duration
		}
	}

	if compression := os.Getenv("METRICS_COMPRESSION"); compression != "" {
		if val, err := strconv.ParseBool(compression); err == nil {
			cfg.Metrics.Compression = val
		}
	}

	utils.LogInfo("Initializing VictoriaMetrics client with URL: %s", cfg.Victoria.URL)
	utils.LogInfo("VictoriaMetrics Username: %s", cfg.Victoria.Username)
	utils.LogInfo("VictoriaMetrics Password: %s", strings.Repeat("*", len(cfg.Victoria.Password)))
	utils.LogInfo("VictoriaMetrics configuration:")
	utils.LogInfo("  URL: %s", cfg.Victoria.URL)
	utils.LogInfo("  Username: %s", cfg.Victoria.Username)
	utils.LogInfo("  BatchSize: %d", cfg.Metrics.BatchSize)
	utils.LogInfo("  FlushInterval: %v", cfg.Metrics.FlushInterval)
	utils.LogInfo("  BufferSize: %d", cfg.Metrics.BufferSize)
	utils.LogInfo("  Compression: %v", cfg.Metrics.Compression)

	return victoria.NewClient(cfg)
}

func initVictoriaLogs() victoria.LogsClient {
	cfg := config.NewDefaultConfig()

	// Configuration Victoria Logs URL et credentials
	if url := os.Getenv("VICTORIA_LOGS_URL"); url != "" {
		if port := os.Getenv("VICTORIA_LOGS_PORT"); port != "" {
			cfg.VictoriaLogs.URL = fmt.Sprintf("http://%s:%s", url, port)
		} else {
			cfg.VictoriaLogs.URL = fmt.Sprintf("http://%s:%s", url, config.DefaultVictoriaLogsPort)
		}
	}

	// Configuration des credentials pour VictoriaLogs
	if user := os.Getenv("VICTORIA_LOGS_USERNAME"); user != "" {
		cfg.VictoriaLogs.Username = user
	}
	if pass := os.Getenv("VICTORIA_LOGS_PASSWORD"); pass != "" {
		cfg.VictoriaLogs.Password = pass
	}

	return victoria.NewLogsClient(&cfg.VictoriaLogs)
}

// Add this function to keep the container alive in test mode
func startKeepAliveRoutine(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	utils.LogInfo("Starting keep-alive routine")

	for {
		select {
		case <-ctx.Done():
			utils.LogInfo("Context cancelled, stopping keep-alive routine")
			return
		case <-ticker.C:
			utils.LogInfo("Keep-alive tick - container is still running")
		}
	}
}

// Add this function to monitor for process exit
func monitorProcessExit(cmd *exec.Cmd) {
	utils.LogInfo("Starting process exit monitor for PID %d", cmd.Process.Pid)

	// Start a goroutine to periodically check if the process is still running
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for {
			<-ticker.C

			// Check if process is still running
			process, err := os.FindProcess(cmd.Process.Pid)
			if err != nil {
				utils.LogWarning("Error finding process %d: %v", cmd.Process.Pid, err)
				continue
			}

			// On Unix, FindProcess always succeeds, so we need to send signal 0
			// to check if the process exists
			err = process.Signal(syscall.Signal(0))
			if err != nil {
				utils.LogWarning("Process %d no longer exists: %v", cmd.Process.Pid, err)
				return
			}

			utils.LogInfo("Process %d is still running", cmd.Process.Pid)
		}
	}()
}

// Add this function to test VictoriaMetrics connectivity
func testVictoriaMetricsConnection(client *victoria.MetricsClient, serverID string) {
	// Create a simple test metric
	testMetric := types.MetricBatch{
		Metrics: []types.Metric{
			{
				Name:      "assetto_server_test_connection",
				Value:     1,
				Type:      types.Counter,
				Timestamp: time.Now(),
				LabelValues: map[string]string{
					"server_id": serverID,
					"test":      "true",
					"timestamp": time.Now().Format(time.RFC3339),
				},
			},
		},
		Time: time.Now(),
	}

	// Send the test metric
	utils.LogInfo("Sending test connection metric to VictoriaMetrics")
	if err := client.SendMetrics(testMetric); err != nil {
		utils.LogError("Failed to send test metric: %v", err)
	} else {
		utils.LogInfo("Successfully sent test metric to VictoriaMetrics")
	}
}
