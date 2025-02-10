package utils

import (
	"fmt"
	"log"
	"time"
)

func init() {
	// Disable default logger timestamp
	log.SetFlags(0)
}

const (
	LogFormatSDK = "[%s SDK] %s"
	LogFormatINF = "[%s INF] %s"
	LogFormatDBG = "[%s DBG] %s"
	LogFormatWRN = "[%s WRN] %s"
	LogFormatERR = "[%s ERR] %s"
)

func LogSDK(format string, v ...interface{}) {
	timestamp := time.Now().Format("15:04:05")
	message := fmt.Sprintf(format, v...)
	log.Printf(LogFormatSDK, timestamp, message)
}

func LogInfo(format string, v ...interface{}) {
	timestamp := time.Now().Format("15:04:05")
	message := fmt.Sprintf(format, v...)
	log.Printf(LogFormatINF, timestamp, message)
}

func LogDebug(format string, v ...interface{}) {
	timestamp := time.Now().Format("15:04:05")
	message := fmt.Sprintf(format, v...)
	log.Printf(LogFormatDBG, timestamp, message)
}

func LogWarning(format string, v ...interface{}) {
	timestamp := time.Now().Format("15:04:05")
	message := fmt.Sprintf(format, v...)
	log.Printf(LogFormatWRN, timestamp, message)
}

func LogError(format string, v ...interface{}) {
	timestamp := time.Now().Format("15:04:05")
	message := fmt.Sprintf(format, v...)
	log.Printf(LogFormatERR, timestamp, message)
}
