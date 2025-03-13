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

	return &Server{
		State:         state,
		Config:        cfg,
		MetricsClient: metricsClient,
		LogsClient:    logsClient,
		WSServer:      wsServer,
		EventChan:     make(chan string, 100),
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

	// Start event processing
	go events.ProcessServerEvents(ctx, s.EventChan, s.State, s.MetricsClient, s.LogsClient, geoipService)

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
