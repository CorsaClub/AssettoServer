// Package utils provides utility functions for data extraction and processing.
package utils

import (
	"fmt"
	"sync"
	"time"
)

// CircuitState represents the state of a circuit breaker
type CircuitState int

const (
	// CircuitClosed indicates the circuit is closed and operations are allowed
	CircuitClosed CircuitState = iota
	// CircuitHalfOpen indicates the circuit is testing if operations can be allowed
	CircuitHalfOpen
	// CircuitOpen indicates the circuit is open and operations are not allowed
	CircuitOpen
)

// CircuitBreaker implements a circuit breaker pattern with three states:
// - Closed: Operations are allowed
// - Half-Open: A limited number of operations are allowed to test if the system has recovered
// - Open: Operations are not allowed
type CircuitBreaker struct {
	sync.RWMutex
	name              string        // Name of the circuit breaker for identification
	failures          int           // Number of consecutive failures
	maxFailures       int           // Maximum allowed failures before opening the circuit
	timeout           time.Duration // Duration to wait before attempting to reset the circuit
	lastFailure       time.Time     // Timestamp of the last failure
	state             CircuitState  // Current state of the circuit breaker
	halfOpenMaxCalls  int           // Maximum number of calls allowed in half-open state
	halfOpenCallCount int           // Current number of calls in half-open state
	successThreshold  int           // Number of consecutive successes needed to close the circuit
	successCount      int           // Current number of consecutive successes
}

// CircuitBreakerOption is a function that configures a CircuitBreaker
type CircuitBreakerOption func(*CircuitBreaker)

// WithName sets the name of the circuit breaker
func WithName(name string) CircuitBreakerOption {
	return func(cb *CircuitBreaker) {
		cb.name = name
	}
}

// WithHalfOpenMaxCalls sets the maximum number of calls allowed in half-open state
func WithHalfOpenMaxCalls(maxCalls int) CircuitBreakerOption {
	return func(cb *CircuitBreaker) {
		cb.halfOpenMaxCalls = maxCalls
	}
}

// WithSuccessThreshold sets the number of consecutive successes needed to close the circuit
func WithSuccessThreshold(threshold int) CircuitBreakerOption {
	return func(cb *CircuitBreaker) {
		cb.successThreshold = threshold
	}
}

// NewCircuitBreaker creates a new CircuitBreaker with specified maximum failures and timeout.
func NewCircuitBreaker(maxFailures int, timeout time.Duration, options ...CircuitBreakerOption) *CircuitBreaker {
	cb := &CircuitBreaker{
		maxFailures:      maxFailures,
		timeout:          timeout,
		state:            CircuitClosed,
		halfOpenMaxCalls: 1,
		successThreshold: 1,
	}

	// Apply options
	for _, option := range options {
		option(cb)
	}

	return cb
}

// RecordFailure increments the failure count and opens the circuit if the maximum failures are reached.
// Returns true if the circuit is open after recording the failure.
func (cb *CircuitBreaker) RecordFailure() bool {
	cb.Lock()
	defer cb.Unlock()

	cb.failures++
	cb.successCount = 0
	cb.lastFailure = time.Now()

	if cb.state == CircuitHalfOpen || cb.failures >= cb.maxFailures {
		cb.state = CircuitOpen
		LogWarning("Circuit breaker %s opened due to %d consecutive failures", cb.name, cb.failures)
	}

	return cb.state == CircuitOpen
}

// RecordSuccess records a successful operation and potentially closes the circuit if enough successes occur.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.Lock()
	defer cb.Unlock()

	cb.failures = 0
	cb.successCount++

	if cb.state == CircuitHalfOpen && cb.successCount >= cb.successThreshold {
		cb.state = CircuitClosed
		cb.halfOpenCallCount = 0
		cb.successCount = 0
		if logsClient, ok := GetLogsClient(); ok {
			logsClient.LogEvent("INFO", fmt.Sprintf("Circuit breaker %s closed after %d consecutive successes", cb.name, cb.successThreshold), "circuit_breaker", map[string]string{
				"name":  cb.name,
				"state": "closed",
			})
		}
	}
}

// AllowRequest checks if a request should be allowed based on the current circuit state.
// Returns true if the request is allowed, false otherwise.
func (cb *CircuitBreaker) AllowRequest() bool {
	cb.Lock()
	defer cb.Unlock()

	switch cb.state {
	case CircuitClosed:
		return true
	case CircuitOpen:
		// Auto-reset to half-open after timeout
		if time.Since(cb.lastFailure) > cb.timeout {
			cb.state = CircuitHalfOpen
			cb.halfOpenCallCount = 0
			cb.successCount = 0
			if logsClient, ok := GetLogsClient(); ok {
				logsClient.LogEvent("INFO", fmt.Sprintf("Circuit breaker %s half-opened after timeout of %v", cb.name, cb.timeout), "circuit_breaker", map[string]string{
					"name":  cb.name,
					"state": "half-open",
				})
			}
			return true
		}
		return false
	case CircuitHalfOpen:
		// Allow a limited number of requests in half-open state
		if cb.halfOpenCallCount < cb.halfOpenMaxCalls {
			cb.halfOpenCallCount++
			return true
		}
		return false
	default:
		return false
	}
}

// State returns the current state of the circuit breaker.
func (cb *CircuitBreaker) State() CircuitState {
	cb.RLock()
	defer cb.RUnlock()
	return cb.state
}

// Reset clears the failure count and closes the circuit.
func (cb *CircuitBreaker) Reset() {
	cb.Lock()
	defer cb.Unlock()
	cb.failures = 0
	cb.successCount = 0
	cb.halfOpenCallCount = 0
	cb.state = CircuitClosed
	if logsClient, ok := GetLogsClient(); ok {
		logsClient.LogEvent("INFO", fmt.Sprintf("Circuit breaker %s manually reset", cb.name), "circuit_breaker", map[string]string{
			"name":  cb.name,
			"state": "closed",
		})
	}
}

// GetStats returns statistics about the circuit breaker.
func (cb *CircuitBreaker) GetStats() map[string]interface{} {
	cb.RLock()
	defer cb.RUnlock()

	var stateStr string
	switch cb.state {
	case CircuitClosed:
		stateStr = "closed"
	case CircuitHalfOpen:
		stateStr = "half-open"
	case CircuitOpen:
		stateStr = "open"
	}

	return map[string]interface{}{
		"name":              cb.name,
		"state":             stateStr,
		"failures":          cb.failures,
		"max_failures":      cb.maxFailures,
		"success_count":     cb.successCount,
		"success_threshold": cb.successThreshold,
		"last_failure":      cb.lastFailure,
		"timeout":           cb.timeout.String(),
	}
}
