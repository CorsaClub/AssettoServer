// Package types contains error type definitions.
package types

import "fmt"

// ServerError represents a server-related error.
type ServerError struct {
	Code    string // Error code identifying the type of error
	Message string // Descriptive message about the error
	Cause   error  // Underlying cause of the error, if any
}

// Error implements the error interface for ServerError.
// It returns a formatted error message including the code, message, and cause if present.
func (e *ServerError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Common server errors.
var (
	// ErrServerNotReady indicates that the server is not ready to accept connections.
	ErrServerNotReady = &ServerError{
		Code:    "SERVER_NOT_READY",
		Message: "Server is not ready",
	}

	// ErrHealthCheckFailed indicates that a health check has failed.
	ErrHealthCheckFailed = &ServerError{
		Code:    "HEALTH_CHECK_FAILED",
		Message: "Health check failed",
	}

	// ErrInvalidSession indicates that there is an invalid session configuration.
	ErrInvalidSession = &ServerError{
		Code:    "INVALID_SESSION",
		Message: "Invalid session configuration",
	}

	// ErrPlayerLimit indicates that the player limit has been reached.
	ErrPlayerLimit = &ServerError{
		Code:    "PLAYER_LIMIT",
		Message: "Player limit reached",
	}
)
