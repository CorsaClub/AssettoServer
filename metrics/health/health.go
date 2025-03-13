package health

import (
	"net/http"
	"time"

	"metrics/env"
	"metrics/types"
	"metrics/utils"
	"metrics/websocket"
)

// InitServer initializes and starts the health check server
func InitServer(state *types.ServerState, wsServer *websocket.WebSocketServer) {
	logsClient, _ := utils.GetLogsClient()
	envVars := env.GetEnv()

	// Create a separate mux for health checks
	healthMux := http.NewServeMux()

	// Add HTTP health endpoint
	healthMux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		// In test mode, always return healthy
		if envVars.TestMode {
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

	// Start HTTP server for health checks
	go func() {
		server := &http.Server{
			Addr:         ":9600",
			Handler:      healthMux,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 5 * time.Second,
		}

		logsClient.LogEvent("INFO", "Starting health check server on port 9600", "health_server", nil)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logsClient.LogEvent("ERROR", "HTTP health server error: "+err.Error(), "health_server", nil)
		}
	}()
}
