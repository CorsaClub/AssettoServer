package server

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"metrics/config"
	"metrics/env"
	"metrics/events"
	"metrics/health"
	"metrics/metrics"
	"metrics/monitoring"
	"metrics/process"
	"metrics/types"
	"metrics/utils"
	"metrics/victoria"
	"metrics/websocket"
)

// Server represents the main server instance
type Server struct {
	State         *types.ServerState
	Config        *config.Config
	MetricsClient *victoria.MetricsClient
	LogsClient    victoria.LogsClient
	WSServer      *websocket.WebSocketServer
	EventChan     chan string
}

// New creates a new server instance
func New(cfg *config.Config, metricsClient *victoria.MetricsClient, logsClient victoria.LogsClient) *Server {
	envVars := env.GetEnv()

	serverID := envVars.GameServerID
	if serverID == "" {
		serverID = utils.GenerateServerID()
		logsClient.LogEvent("INFO", fmt.Sprintf("Generated server ID: %s", serverID), "server_id", nil)
	}

	state := &types.ServerState{
		ServerID:         serverID,
		ServerRegion:     envVars.GameServerRegion,
		ServerName:       envVars.ServerName,
		ServerType:       envVars.ServerType,
		LastPing:         time.Now(),
		StartTime:        time.Now(),
		ConnectedPlayers: make(map[string]*types.Player),
		ActiveCars:       make(map[string]int),
		CurrentSession: &types.Session{
			Type: "initializing",
		},
	}

	wsServer := websocket.NewWebSocketServer(envVars.GetAuthConfig())

	// Test connections
	if err := metrics.TestVictoriaMetricsConnection(metricsClient, state.ServerID, logsClient); err != nil {
		logsClient.LogEvent("WARNING", "VictoriaMetrics connection test failed", "startup", nil)
	}
	if err := metrics.TestVictoriaLogsConnection(logsClient); err != nil {
		logsClient.LogEvent("WARNING", "VictoriaLogs connection test failed", "startup", nil)
	}

	// Create server instance
	server := &Server{
		State:         state,
		Config:        cfg,
		MetricsClient: metricsClient,
		LogsClient:    logsClient,
		WSServer:      wsServer,
		EventChan:     make(chan string, 100),
	}

	// Send initial server state metric
	server.sendInitialMetrics()

	return server
}

// sendInitialMetrics sends the initial metrics when the server starts
func (s *Server) sendInitialMetrics() {
	// Create base labels
	baseLabels := map[string]string{
		"server_id":     s.State.ServerID,
		"server_name":   s.State.ServerName,
		"server_type":   s.State.ServerType,
		"server_region": s.State.ServerRegion,
	}

	// Create session labels
	sessionLabels := map[string]string{
		"server_id":    s.State.ServerID,
		"server_name":  s.State.ServerName,
		"session_id":   s.State.CurrentSession.ID,
		"session_type": s.State.CurrentSession.Type,
	}

	// Create a batch of initial metrics
	batch := types.MetricBatch{
		Metrics: []types.Metric{
			// Server state metric - starting
			{
				Name:        types.ServerStateMetric,
				Value:       float64(metrics.ServerStateStarting),
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: baseLabels,
			},
			// Server starts counter
			{
				Name:        types.ServerStartsTotal,
				Value:       1,
				Type:        types.Counter,
				Timestamp:   time.Now(),
				LabelValues: baseLabels,
			},
			// Initial player count (0)
			{
				Name:        types.ServerPlayers,
				Value:       0,
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: baseLabels,
			},
			// Initial uptime
			{
				Name:        types.ServerUptime,
				Value:       0,
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: baseLabels,
			},
			// Initial health status
			{
				Name:        types.ServerHealth,
				Value:       0, // Not healthy yet
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: baseLabels,
			},
			// Session metrics
			{
				Name:        metrics.SessionDurationGauge.Name,
				Value:       0,
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: sessionLabels,
			},
			{
				Name:        metrics.SessionTimeLeftGauge.Name,
				Value:       0,
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: baseLabels,
			},
			{
				Name:      metrics.SessionRemainingTimeGauge.Name,
				Value:     0,
				Type:      types.Gauge,
				Timestamp: time.Now(),
				LabelValues: map[string]string{
					"server_id":    s.State.ServerID,
					"session_type": s.State.CurrentSession.Type,
				},
			},
			// Track metrics with default values
			{
				Name:        metrics.TrackGrip.Name,
				Value:       0,
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: baseLabels,
			},
			{
				Name:        metrics.TrackTemperature.Name,
				Value:       0,
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: baseLabels,
			},
			{
				Name:        metrics.AirTemperature.Name,
				Value:       0,
				Type:        types.Gauge,
				Timestamp:   time.Now(),
				LabelValues: baseLabels,
			},
		},
		Time: time.Now(),
	}

	// Send the initial metrics
	if err := s.MetricsClient.SendMetrics(batch); err != nil {
		s.LogsClient.LogEvent("ERROR", "Failed to send initial server metrics: "+err.Error(), "startup", nil)
	} else {
		s.LogsClient.LogEvent("INFO", "Initial server metrics sent successfully", "startup", nil)
	}
}

// Start initializes and starts the server
func (s *Server) Start(ctx context.Context, wg *sync.WaitGroup, input, args string) error {
	// Start WebSocket server
	go s.WSServer.Start(ctx)

	// Initialize health check server
	health.InitServer(s.State, s.WSServer)

	// Initialize GeoIP service
	geoipService := metrics.InitGeoIPService(s.Config.GeoIP, s.LogsClient)

	// Initialize system metrics
	monitoring.InitializeSystemMetrics(s.State, s.MetricsClient)
	s.LogsClient.LogEvent("INFO", "System metrics initialized", "startup", nil)

	// Start event processing
	go events.ProcessServerEvents(ctx, s.EventChan, s.State, s.MetricsClient, s.LogsClient, geoipService)

	// Initialize and start system monitoring
	systemMonitor, err := monitoring.NewSystemMonitor(s.State, s.MetricsClient)
	if err != nil {
		s.LogsClient.LogEvent("ERROR", "Failed to initialize system monitor: "+err.Error(), "startup", nil)
	} else {
		s.LogsClient.LogEvent("INFO", "System monitor initialized", "startup", nil)
		wg.Add(1)
		go func() {
			defer wg.Done()
			systemMonitor.Start(ctx)
		}()
	}

	// Initialize and start performance monitoring
	perfMonitor := monitoring.NewPerformanceMonitor(s.State, s.MetricsClient)
	s.LogsClient.LogEvent("INFO", "Performance monitor initialized", "startup", nil)
	wg.Add(1)
	go func() {
		defer wg.Done()
		perfMonitor.Start(ctx)
	}()

	// Start metrics system monitoring
	wg.Add(1)
	go func() {
		defer wg.Done()
		monitoring.MonitorMetricsSystem(ctx, s.MetricsClient)
	}()

	// Start the server process
	cmd := process.StartServer(ctx, input, args, s.State, s.MetricsClient, s.WSServer, s.LogsClient)

	// Monitor process exit
	go process.MonitorExit(cmd, s.LogsClient)

	// Start keep-alive routine
	go s.startKeepAlive(ctx)

	return nil
}

// startKeepAlive starts the keep-alive routine
func (s *Server) startKeepAlive(ctx context.Context) {
	s.LogsClient.LogEvent("INFO", "Starting keep-alive routine", "keepalive", nil)
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.LogsClient.LogEvent("INFO", "Keep-alive routine stopped", "keepalive", nil)
			return
		case <-ticker.C:
			s.LogsClient.LogEvent("DEBUG", "Keep-alive tick", "keepalive", nil)
		}
	}
}

// SetupSignalHandler sets up signal handling for graceful shutdown
func (s *Server) SetupSignalHandler(cancel context.CancelFunc) {
	envVars := env.GetEnv()
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)

	go func() {
		sig := <-c
		s.LogsClient.LogEvent("INFO", "Signal received: "+sig.String(), "signal", nil)

		s.State.Lock()
		s.State.ShuttingDown = true
		s.State.Unlock()

		if !envVars.TestMode {
			cancel()
		}
	}()

	s.LogsClient.LogEvent("INFO", "Signal handler set up", "signal", nil)
}
