package process

import (
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"metrics/env"
	"metrics/handlers"
	"metrics/metrics"
	"metrics/types"
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
	if i.forward != nil {
		return i.forward.Write(p)
	}
	return len(p), nil
}

// StartServer prepares and starts the server process
func StartServer(ctx context.Context, input, args string, state *types.ServerState,
	metricsClient *victoria.MetricsClient, wsServer *websocket.WebSocketServer,
	logsClient victoria.LogsClient) *exec.Cmd {

	// Parse arguments
	argsList := strings.Fields(args)
	cmd := exec.CommandContext(ctx, input, argsList...)

	// Set working directory
	cmd.Dir = "/app"

	// Set environment variables
	cmd.Env = os.Environ()

	// Configure output interception
	cmd.Stderr = &interceptor{
		forward: os.Stderr,
		intercept: func(p []byte) {
			str := strings.TrimSpace(string(p))
			logsClient.LogServerEvent("ERROR", str, "server_error", map[string]string{
				"server_id":  state.ServerID,
				"session_id": state.CurrentSession.ID,
			})
		},
	}

	// Setup stdout interceptor
	serverReady := make(chan struct{})
	cmd.Stdout = &interceptor{
		forward: os.Stdout,
		intercept: func(p []byte) {
			str := strings.TrimSpace(string(p))

			// Traiter les messages spéciaux (chat, etc.)
			if strings.Contains(str, "CHAT") {
				handleChatMessage(str, state, logsClient)
			}

			// Déterminer le niveau de log pour VictoriaLogs
			logLevel := "INFO"
			eventType := "server_output"
			if strings.Contains(str, "ERROR") {
				logLevel = "ERROR"
				eventType = "error"
			} else if strings.Contains(str, "Warning") || strings.Contains(str, "WARNING") {
				logLevel = "WARNING"
				eventType = "warning"
			}

			// Envoyer à VictoriaLogs
			logsClient.LogServerEvent(logLevel, str, eventType, map[string]string{
				"server_id":  state.ServerID,
				"session_id": state.CurrentSession.ID,
			})

			// Create log entry for WebSocket
			logEntry := types.LogEntry{
				Timestamp: state.LastPing,
				Level:     logLevel,
				Message:   str,
				ServerID:  state.ServerID,
				SessionID: state.CurrentSession.ID,
			}

			// Send to WebSocket
			wsServer.BroadcastLog(logEntry)

			// Process the output
			handlers.HandleServerOutput(str, metricsClient, state, serverReady, nil, nil)
		},
	}

	// Start the process
	if err := cmd.Start(); err != nil {
		logsClient.LogEvent("ERROR", "Failed to start server process: "+err.Error(), "server_start", nil)
	} else {
		logsClient.LogEvent("INFO", "Server process started", "server_start", map[string]string{
			"pid": string(cmd.Process.Pid),
		})
	}

	// Wait for server ready signal in a goroutine
	go func() {
		select {
		case <-serverReady:
			// Server is ready, update state
			state.Lock()
			state.Ready = true
			state.Unlock()

			// Update server state metric to "ready"
			baseLabels := map[string]string{
				"server_id":     state.ServerID,
				"server_name":   state.ServerName,
				"server_type":   state.ServerType,
				"server_region": state.ServerRegion,
			}

			// Send server ready metric
			readyMetric := types.MetricBatch{
				Metrics: []types.Metric{
					{
						Name:        types.ServerStateMetric,
						Value:       float64(metrics.ServerStateReady),
						Type:        types.Gauge,
						Timestamp:   time.Now(),
						LabelValues: baseLabels,
					},
					{
						Name:        types.ServerHealth,
						Value:       1, // Now healthy
						Type:        types.Gauge,
						Timestamp:   time.Now(),
						LabelValues: baseLabels,
					},
				},
				Time: time.Now(),
			}

			if err := metricsClient.SendMetrics(readyMetric); err != nil {
				logsClient.LogEvent("ERROR", "Failed to send server ready metric: "+err.Error(), "server_ready", nil)
			} else {
				logsClient.LogEvent("INFO", "Server ready metric sent successfully", "server_ready", nil)
			}
		case <-ctx.Done():
			// Context cancelled, do nothing
		}
	}()

	return cmd
}

// MonitorExit monitors the exit of the server process
func MonitorExit(cmd *exec.Cmd, logsClient victoria.LogsClient) {
	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			logsClient.LogEvent("ERROR", "Server process exited with error", "process_exit", map[string]string{
				"exit_code": string(exitErr.ExitCode()),
				"error":     err.Error(),
			})
		} else {
			logsClient.LogEvent("ERROR", "Server process exited with error: "+err.Error(), "process_exit", nil)
		}

		envVars := env.GetEnv()

		if !envVars.TestMode {
			// Signal termination
			p, err := os.FindProcess(os.Getpid())
			if err == nil {
				p.Signal(syscall.SIGTERM)
			}
		}
	} else {
		logsClient.LogEvent("INFO", "Server process exited normally", "process_exit", nil)
	}
}

// handleChatMessage processes chat messages
func handleChatMessage(str string, state *types.ServerState, logsClient victoria.LogsClient) {
	// Format typique: [CHAT] PlayerName: message
	parts := strings.SplitN(str, "]", 2)
	if len(parts) == 2 {
		chatParts := strings.SplitN(strings.TrimSpace(parts[1]), ":", 2)
		if len(chatParts) == 2 {
			playerName := strings.TrimSpace(chatParts[0])
			message := strings.TrimSpace(chatParts[1])

			logsClient.LogChatMessage(playerName, message, map[string]string{
				"server_id":  state.ServerID,
				"session_id": state.CurrentSession.ID,
			})
		}
	}
}
