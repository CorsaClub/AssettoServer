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

// waitForTerminationSignal attend un signal de terminaison et annule le contexte
func waitForTerminationSignal(cancel context.CancelFunc) {
	// Créer un canal pour les signaux
	sigs := make(chan os.Signal, 1)

	// Enregistrer les signaux à intercepter
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	// Attendre un signal
	sig := <-sigs
	utils.LogInfo("Received signal: %v", sig)

	// Annuler le contexte pour déclencher l'arrêt gracieux
	cancel()
}

// main is the entry point of the application.
// It initializes the Agones SDK, starts the Assetto Corsa server,
// and manages the server's lifecycle including health checks and metrics.
func main() {
	// At the beginning of main()
	os.Setenv("DEBUG_LOGS", "true")
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

	// Create server configuration
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
		GeoIP: struct {
			Enabled      bool   `json:"enabled"`
			DatabasePath string `json:"database_path"`
		}{
			Enabled:      true,
			DatabasePath: "./GeoLite2-City.mmdb",
		},
	}

	// Initialize VictoriaMetrics client
	metricsClient := initVictoriaMetrics()

	// Initialize VictoriaLogs client
	logsClient := initVictoriaLogs()

	// Set the logs client in the utils package for global access
	utils.SetLogsClient(logsClient)

	// Initialize GeoIP service if enabled
	var geoipService *geoip.GeoIPService
	if serverConfig.GeoIP.Enabled {
		utils.LogInfo("Initializing GeoIP service with database: %s", serverConfig.GeoIP.DatabasePath)
		var err error
		geoipConfig := &config.GeoIPConfig{
			Enabled:      serverConfig.GeoIP.Enabled,
			DatabasePath: serverConfig.GeoIP.DatabasePath,
		}
		geoipService, err = geoip.InitGeoIPService(geoipConfig)
		if err != nil {
			utils.LogWarning("Failed to initialize GeoIP service: %v", err)
		} else {
			utils.LogInfo("GeoIP service initialized successfully")
		}
	} else {
		utils.LogInfo("GeoIP service is disabled")
	}

	// Test connections
	testVictoriaMetricsConnection(metricsClient, serverState.ServerID)
	testVictoriaLogsConnection(logsClient)

	// Ajouter un canal pour les événements
	eventChan := make(chan string, 100)

	// Ajouter cette fonction pour capturer les événements du serveur
	captureServerEvents := func(line string) {
		// Filtrer les lignes pertinentes
		if strings.Contains(line, "Collision between") ||
			strings.Contains(line, "LAP") ||
			strings.Contains(line, "Network stats") ||
			strings.Contains(line, "CONNECTED") ||
			strings.Contains(line, "DISCONNECTED") ||
			strings.Contains(line, "SESSION") ||
			strings.Contains(line, "Lobby registration") ||
			strings.Contains(line, "ERROR") ||
			strings.Contains(line, "Warning") {

			// Envoyer l'événement au canal pour traitement
			eventChan <- line

			// Déterminer le type d'événement pour le log
			eventType := "server_output"
			level := "INFO"

			if strings.Contains(line, "Collision between") {
				eventType = "collision"
				level = "WARNING"
			} else if strings.Contains(line, "LAP") {
				eventType = "lap_completed"
			} else if strings.Contains(line, "CONNECTED") {
				eventType = "player_connect"
			} else if strings.Contains(line, "DISCONNECTED") {
				eventType = "player_disconnect"
			} else if strings.Contains(line, "SESSION") {
				eventType = "session_change"
			} else if strings.Contains(line, "Lobby registration") {
				eventType = "lobby_registration"
			} else if strings.Contains(line, "ERROR") {
				eventType = "server_error"
				level = "ERROR"
			} else if strings.Contains(line, "Warning") {
				eventType = "server_warning"
				level = "WARNING"
			}

			// Envoyer l'événement à VictoriaLogs
			logsClient.LogEvent(level, line, eventType, map[string]string{
				"server_id":  serverState.ServerID,
				"source":     "server_log",
				"event_type": eventType,
			})
		} else if strings.Contains(line, "CHAT:") {
			// Traitement spécial pour les messages de chat
			eventType := "chat_message"
			level := "INFO"

			// Extraire le message de chat
			chatParts := strings.SplitN(line, "CHAT:", 2)
			if len(chatParts) > 1 {
				// Extraire le nom du joueur si possible
				playerName := utils.ExtractName(line)

				// Extraire le contenu du message
				chatMessage := utils.ExtractChatMessage(line)

				// Créer un message formaté
				formattedMessage := chatMessage
				if playerName != "" {
					formattedMessage = fmt.Sprintf("%s: %s", playerName, chatMessage)
				}

				// Envoyer le message au canal pour traitement
				eventChan <- line

				// Envoyer directement à VictoriaLogs avec des labels spécifiques
				logsClient.LogEvent(level, formattedMessage, eventType, map[string]string{
					"server_id":   serverState.ServerID,
					"source":      "chat",
					"event_type":  eventType,
					"player_name": playerName,
				})
			}
		}
	}

	// Démarrer le monitoring avec les deux clients
	logMonitor := monitoring.NewLogMonitor(
		metricsClient,
		logsClient,
		serverConfig.Logging.Directory,
		monitoring.WithPatterns(serverConfig.Logging.Patterns),
		monitoring.WithMaxFileSize(serverConfig.Logging.MaxFileSize),
		monitoring.WithLineCallback(captureServerEvents),
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

	// Initialiser et démarrer le moniteur système
	systemMonitor, err := monitoring.NewSystemMonitor(serverState, metricsClient)
	if err != nil {
		utils.LogError("Erreur lors de l'initialisation du moniteur système: %v", err)
	} else {
		wg.Add(1)
		go func() {
			defer wg.Done()
			systemMonitor.Start(ctx)
		}()

		// Surveiller la latence réseau vers des hôtes spécifiques
		targetHosts := []string{
			"steam.corsa.club:27015",
			"api.corsa.club:443",
			"metrics.corsa.club:8428",
		}

		for _, host := range targetHosts {
			wg.Add(1)
			go func(targetHost string) {
				defer wg.Done()
				systemMonitor.MonitorNetworkLatency(ctx, targetHost)
			}(host)
		}
	}

	// Start monitoring (utiliser le nouveau SystemMonitor au lieu de MonitorSystemResources)
	go monitoring.MonitorHealthMetrics(ctx, metricsClient, serverState)
	// Remplacer l'ancienne fonction par notre nouveau moniteur système
	// go monitoring.MonitorSystemResources(ctx, serverState)
	go monitoring.MonitorDetailedMetrics(ctx, metricsClient, serverState)
	go monitoring.MonitorSessionMetrics(ctx, metricsClient, serverState)

	// Démarrer le monitoring des performances internes
	go metrics.StartPerformanceMonitoring(ctx, metricsClient)

	// Démarrer le monitoring des événements
	go monitoring.MonitorServerEvents(ctx, metricsClient, logsClient, serverState, eventChan)

	// Initialiser la configuration d'authentification
	authConfig := config.NewAuthConfig()
	if !authConfig.IsValid() {
		utils.LogWarning("WebSocket authentication not configured (AUTH_STEAM_ID and AUTH_USER_ID required)")
	}

	// Initialiser le serveur WebSocket avec l'authentification
	wsServer := websocket.NewWebSocketServer(authConfig)
	go wsServer.Start(ctx)

	// Create a channel to signal when the server is ready
	serverReady := make(chan struct{}, 1)

	// Prepare and start the server
	var scriptPath string
	if os.Getenv("TEST_MODE") == "true" {
		scriptPath = "/app/test-script.sh"
		utils.LogInfo("Running in test mode with script: %s", scriptPath)
	} else {
		scriptPath = *input
	}
	cmd := prepareServerCommand(ctx, &scriptPath, args, serverState, serverReady, metricsClient, wsServer, logsClient, geoipService, cancel)
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
		}
	}()

	// Wait for termination signals
	waitForTerminationSignal(cancel)

	// Wait for all goroutines to finish
	wg.Wait()
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

	// Afficher les informations de configuration pour le débogage
	utils.LogInfo("Configuration VictoriaLogs:")
	utils.LogInfo("  URL: %s", cfg.VictoriaLogs.URL)
	utils.LogInfo("  Username: %s", cfg.VictoriaLogs.Username != "")
	utils.LogInfo("  Password: %s", cfg.VictoriaLogs.Password != "")

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

// Fonction pour tester la connexion à VictoriaLogs
func testVictoriaLogsConnection(client victoria.LogsClient) {
	utils.LogInfo("Test de connexion à VictoriaLogs")

	testLog := []types.Log{
		{
			Timestamp: time.Now(),
			Level:     "INFO",
			Message:   "Test log message from Assetto Corsa server wrapper",
			Source:    "acserver",
			Labels: map[string]string{
				"test":      "true",
				"server_id": utils.GenerateServerID(),
				"timestamp": time.Now().Format(time.RFC3339),
			},
		},
	}

	utils.LogInfo("Envoi d'un log de test à VictoriaLogs")
	if err := client.SendLogs(testLog); err != nil {
		utils.LogError("Échec de l'envoi du log de test à VictoriaLogs: %v", err)
	} else {
		utils.LogInfo("Log de test envoyé avec succès à VictoriaLogs")
	}
}
