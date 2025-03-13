package utils

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"
	"time"
)

// LogLevel represents the severity of a log message
type LogLevel int

const (
	// LogLevelDebug is for detailed debugging information
	LogLevelDebug LogLevel = iota
	// LogLevelInfo is for general operational information
	LogLevelInfo
	// LogLevelWarning is for warning conditions
	LogLevelWarning
	// LogLevelError is for error conditions
	LogLevelError
	// LogLevelFatal is for fatal conditions that require immediate shutdown
	LogLevelFatal
)

var (
	// CurrentLogLevel controls the minimum level of logs that will be output
	CurrentLogLevel = LogLevelInfo
	// EnableDebugLogs controls whether debug logs are enabled
	EnableDebugLogs = false
	// DisableConsoleOutput contrôle si les logs sont affichés dans la console
	DisableConsoleOutput = true
	// ShowServerLogsInConsole contrôle si les logs d'AssettoServer sont affichés dans la console
	ShowServerLogsInConsole bool
)

func init() {
	// Disable default logger timestamp as we add our own
	log.SetFlags(0)

	// Check if debug logs are enabled via environment variable
	if os.Getenv("DEBUG_LOGS") == "true" {
		EnableDebugLogs = true
		CurrentLogLevel = LogLevelDebug
	}

	// Check if console output is enabled via environment variable
	if os.Getenv("ENABLE_CONSOLE_LOGS") == "true" {
		DisableConsoleOutput = false
	}

	// Par défaut, on affiche les logs du serveur
	ShowServerLogsInConsole = true

	// Désactiver les logs du serveur si la variable d'environnement est définie
	if os.Getenv("DISABLE_SERVER_LOGS") == "true" {
		ShowServerLogsInConsole = false
	}
}

// Log formats for different log levels
const (
	LogFormatSDK = "[%s SDK] %s"
	LogFormatINF = "[%s INF] %s"
	LogFormatDBG = "[%s DBG] %s"
	LogFormatWRN = "[%s WRN] %s"
	LogFormatERR = "[%s ERR] %s"
	LogFormatFTL = "[%s FTL] %s"
	LogFormatSRV = "[%s SRV] %s" // Format pour les logs du serveur
)

// LogServerOutput logs server output with appropriate formatting
func LogServerOutput(output string) {
	// Les logs du serveur sont toujours affichés sauf si explicitement désactivés
	if ShowServerLogsInConsole {
		timestamp := time.Now().Format("15:04:05")

		// Déterminer le niveau de log pour le formatage
		var format string
		switch {
		case strings.Contains(strings.ToUpper(output), "ERROR"):
			format = LogFormatERR
		case strings.Contains(strings.ToUpper(output), "WARN"):
			format = LogFormatWRN
		case strings.Contains(strings.ToUpper(output), "DEBUG"):
			format = LogFormatDBG
		case strings.Contains(strings.ToUpper(output), "INFO"):
			format = LogFormatINF
		default:
			format = LogFormatSRV
		}

		// Toujours afficher le log, même si c'est une sortie non gérée
		log.Printf(format, timestamp, output)
	}

	// Envoyer également à VictoriaLogs
	sendToLogsClient("info", output, "server_output", map[string]string{
		"source": "assetto_server",
	})
}

// LogSDK logs a message at the SDK level
func LogSDK(format string, v ...interface{}) {
	timestamp := time.Now().Format("15:04:05")
	message := fmt.Sprintf(format, v...)

	// Afficher dans la console uniquement si activé
	if !DisableConsoleOutput {
		log.Printf(LogFormatSDK, timestamp, message)
	}

	// Always send to logs client if available
	sendToLogsClient("info", message, "sdk", nil)
}

// LogInfo logs a message to the console and the logs client at the INFO level
func LogInfo(format string, v ...interface{}) {
	if CurrentLogLevel > LogLevelInfo {
		return
	}

	timestamp := time.Now().Format("15:04:05")
	message := fmt.Sprintf(format, v...)

	// Afficher dans la console uniquement si activé
	if !DisableConsoleOutput {
		log.Printf(LogFormatINF, timestamp, message)
	}

	// Always send to logs client if available
	sendToLogsClient("info", message, "info", nil)
}

// LogDebug logs a message to the console and the logs client at the DEBUG level
func LogDebug(format string, v ...interface{}) {
	if !EnableDebugLogs || CurrentLogLevel > LogLevelDebug {
		return
	}

	timestamp := time.Now().Format("15:04:05")
	message := fmt.Sprintf(format, v...)

	// Afficher dans la console uniquement si activé
	if !DisableConsoleOutput {
		log.Printf(LogFormatDBG, timestamp, message)
	}

	// Always send to logs client if available
	sendToLogsClient("debug", message, "debug", nil)
}

// LogWarning logs a message to the console and the logs client at the WARNING level
func LogWarning(format string, v ...interface{}) {
	if CurrentLogLevel > LogLevelWarning {
		return
	}

	timestamp := time.Now().Format("15:04:05")
	message := fmt.Sprintf(format, v...)

	// Afficher dans la console uniquement si activé
	if !DisableConsoleOutput {
		log.Printf(LogFormatWRN, timestamp, message)
	}

	// Always send to logs client if available
	sendToLogsClient("warning", message, "warning", nil)
}

// LogError logs a message to the console and the logs client at the ERROR level
func LogError(format string, v ...interface{}) {
	if CurrentLogLevel > LogLevelError {
		return
	}

	timestamp := time.Now().Format("15:04:05")
	message := fmt.Sprintf(format, v...)

	// Afficher dans la console uniquement si activé
	if !DisableConsoleOutput {
		log.Printf(LogFormatERR, timestamp, message)
	}

	// Get caller information for better error context
	_, file, line, ok := runtime.Caller(1)
	var callerInfo map[string]string
	if ok {
		callerInfo = map[string]string{
			"file": file,
			"line": fmt.Sprintf("%d", line),
		}
	}

	// Always send to logs client if available
	sendToLogsClient("error", message, "error", callerInfo)
}

// LogFatal logs a message at the FATAL level and exits the program
func LogFatal(format string, v ...interface{}) {
	timestamp := time.Now().Format("15:04:05")
	message := fmt.Sprintf(format, v...)

	// Les logs fatals sont toujours affichés dans la console
	log.Printf(LogFormatFTL, timestamp, message)

	// Get caller information for better error context
	_, file, line, ok := runtime.Caller(1)
	var callerInfo map[string]string
	if ok {
		callerInfo = map[string]string{
			"file": file,
			"line": fmt.Sprintf("%d", line),
		}
	}

	// Always send to logs client if available
	sendToLogsClient("fatal", message, "fatal", callerInfo)

	// Exit the program with error code
	os.Exit(1)
}

// sendToLogsClient sends a log message to the logs client if available
func sendToLogsClient(level, message, eventType string, extraLabels map[string]string) {
	client, ok := GetLogsClient()
	if !ok {
		return
	}

	labels := make(map[string]string)
	for k, v := range extraLabels {
		labels[k] = v
	}

	// Add source information
	labels["source"] = "wrapper"

	// Ignore errors from the logs client to prevent cascading failures
	_ = client.LogEvent(level, message, eventType, labels)
}
