package utils

import (
	"fmt"
	"log"
	"os"
	"runtime"
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
)

func init() {
	// Disable default logger timestamp as we add our own
	log.SetFlags(0)

	// Check if debug logs are enabled via environment variable
	if os.Getenv("DEBUG_LOGS") == "true" {
		EnableDebugLogs = true
		CurrentLogLevel = LogLevelDebug
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
)

// LogSDK logs a message at the SDK level
func LogSDK(format string, v ...interface{}) {
	timestamp := time.Now().Format("15:04:05")
	message := fmt.Sprintf(format, v...)
	log.Printf(LogFormatSDK, timestamp, message)

	// Also send to logs client if available
	sendToLogsClient("info", message, "sdk", nil)
}

// LogInfo logs a message at the INFO level
func LogInfo(format string, v ...interface{}) {
	if CurrentLogLevel > LogLevelInfo {
		return
	}

	timestamp := time.Now().Format("15:04:05")
	message := fmt.Sprintf(format, v...)
	log.Printf(LogFormatINF, timestamp, message)

	// Also send to logs client if available
	sendToLogsClient("info", message, "info", nil)
}

// LogDebug logs a message at the DEBUG level
func LogDebug(format string, v ...interface{}) {
	if !EnableDebugLogs || CurrentLogLevel > LogLevelDebug {
		return
	}

	timestamp := time.Now().Format("15:04:05")
	message := fmt.Sprintf(format, v...)
	log.Printf(LogFormatDBG, timestamp, message)

	// Also send to logs client if available
	sendToLogsClient("debug", message, "debug", nil)
}

// LogWarning logs a message at the WARNING level
func LogWarning(format string, v ...interface{}) {
	if CurrentLogLevel > LogLevelWarning {
		return
	}

	timestamp := time.Now().Format("15:04:05")
	message := fmt.Sprintf(format, v...)
	log.Printf(LogFormatWRN, timestamp, message)

	// Also send to logs client if available
	sendToLogsClient("warning", message, "warning", nil)
}

// LogError logs a message at the ERROR level
func LogError(format string, v ...interface{}) {
	if CurrentLogLevel > LogLevelError {
		return
	}

	timestamp := time.Now().Format("15:04:05")
	message := fmt.Sprintf(format, v...)
	log.Printf(LogFormatERR, timestamp, message)

	// Get caller information for better error context
	_, file, line, ok := runtime.Caller(1)
	var callerInfo map[string]string
	if ok {
		callerInfo = map[string]string{
			"file": file,
			"line": fmt.Sprintf("%d", line),
		}
	}

	// Also send to logs client if available
	sendToLogsClient("error", message, "error", callerInfo)
}

// LogFatal logs a message at the FATAL level and exits the program
func LogFatal(format string, v ...interface{}) {
	timestamp := time.Now().Format("15:04:05")
	message := fmt.Sprintf(format, v...)
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

	// Also send to logs client if available
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
	if extraLabels != nil {
		for k, v := range extraLabels {
			labels[k] = v
		}
	}

	// Add source information
	labels["source"] = "wrapper"

	// Ignore errors from the logs client to prevent cascading failures
	_ = client.LogEvent(level, message, eventType, labels)
}
