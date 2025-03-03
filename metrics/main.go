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

	// Configuration flags
	input := flag.String("i", "./start-server.sh", "Path to server start script")
	args := flag.String("args", "", "Arguments for the server")

	flag.Parse()

	// In main() function, after flag parsing
	serverID := os.Getenv("GAMESERVER_ID") // Use Agones ID if running in Kubernetes
	serverRegion := os.Getenv("GAMESERVER_REGION")
	serverName := os.Getenv("SERVER_NAME")

	if serverID == "" {
		serverID = utils.GenerateServerID() // Fallback to generated ID
	}

	// Initialize server state with ID
	serverState := &types.ServerState{
		ServerID:         serverID,
		ServerRegion:     serverRegion,
		ServerName:       serverName,
		ServerType:       os.Getenv("SERVER_TYPE"),
		LastPing:         time.Now(),
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

	go metricsClient.StartMetricBuffer(ctx)
	go logMonitor.Start(ctx)

	// Start monitoring
	go monitoring.MonitorHealthMetrics(ctx, metricsClient, serverState)
	go monitoring.MonitorSystemResources(ctx, serverState)

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

	// At the end of main()
	utils.LogInfo("Main function completed, container should continue running")
}

// prepareServerCommand creates and configures the exec.Cmd for the Assetto Corsa server.
// It sets up output interception and command arguments.
func prepareServerCommand(ctx context.Context, input *string, args *string, state *types.ServerState, serverReady chan struct{}, vmClient *victoria.MetricsClient, wsServer *websocket.WebSocketServer) *exec.Cmd {
	utils.LogInfo("Preparing server command: %s %s", *input, *args)

	// Check if the input file exists and is executable
	fileInfo, err := os.Stat(*input)
	if os.IsNotExist(err) {
		utils.LogError("Server script not found: %s", *input)
		os.Exit(1)
	}

	// Check permissions
	utils.LogInfo("Script file permissions: %s", fileInfo.Mode().String())

	// Try to read the first few bytes of the script to verify it's accessible
	file, err := os.Open(*input)
	if err != nil {
		utils.LogError("Failed to open script file: %v", err)
	} else {
		defer file.Close()
		buffer := make([]byte, 100)
		n, err := file.Read(buffer)
		if err != nil {
			utils.LogError("Failed to read script file: %v", err)
		} else {
			utils.LogInfo("Script file starts with: %s", string(buffer[:n]))
		}
	}

	argsList := strings.Fields(*args)
	cmd := exec.CommandContext(ctx, *input, argsList...)

	// Set the working directory explicitly
	cmd.Dir = "/app"

	// Log the command details
	utils.LogInfo("Command: %s, Args: %v, Dir: %s", cmd.Path, cmd.Args, cmd.Dir)

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

// setupSignalHandler configures signal handling for graceful shutdown.
func setupSignalHandler(cancel context.CancelFunc, state *types.ServerState) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		sig := <-c
		utils.LogSDK("Received signal %v, initiating shutdown", sig)

		state.Lock()
		state.ShuttingDown = true
		state.Unlock()

		// Only cancel if not in test mode
		if os.Getenv("TEST_MODE") != "true" {
			cancel()
		} else {
			utils.LogInfo("Received signal %v in test mode, ignoring shutdown request", sig)
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
			Addr:         ":9001",
			Handler:      healthMux,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 5 * time.Second,
		}

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
