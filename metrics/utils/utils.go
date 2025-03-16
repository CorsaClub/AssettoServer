// Package utils provides utility functions for the application.
package utils

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"time"
)

// Global logger
var logger *log.Logger

// Initialize the logger
func init() {
	logger = log.New(os.Stdout, "[METRICS] ", log.LstdFlags)
}

// GenerateServerID generates a unique server ID
func GenerateServerID() string {
	// Generate a random ID
	b := make([]byte, 4)
	_, err := rand.Read(b)
	if err != nil {
		// Fallback to timestamp if random generation fails
		return fmt.Sprintf("srv-%d", time.Now().UnixNano())
	}

	return fmt.Sprintf("srv-%s", hex.EncodeToString(b))
}

// GenerateSessionID generates a unique session ID
func GenerateSessionID() string {
	// Generate a random ID
	b := make([]byte, 4)
	_, err := rand.Read(b)
	if err != nil {
		// Fallback to timestamp if random generation fails
		return fmt.Sprintf("sess-%d", time.Now().UnixNano())
	}

	return fmt.Sprintf("sess-%s", hex.EncodeToString(b))
}
