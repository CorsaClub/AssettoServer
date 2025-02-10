// Package utils provides utility functions for data extraction and processing.
package utils

import (
	"sync"
	"time"
)

// CircuitBreaker implements a simple circuit breaker pattern.
type CircuitBreaker struct {
	sync.RWMutex
	failures    int           // Number of consecutive failures
	maxFailures int           // Maximum allowed failures before opening the circuit
	timeout     time.Duration // Duration to wait before attempting to reset the circuit
	lastFailure time.Time     // Timestamp of the last failure
	isOpen      bool          // Indicates if the circuit is currently open
}

// NewCircuitBreaker creates a new CircuitBreaker with specified maximum failures and timeout.
func NewCircuitBreaker(maxFailures int, timeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		maxFailures: maxFailures,
		timeout:     timeout,
	}
}

// RecordFailure increments the failure count and opens the circuit if the maximum failures are reached.
// Returns true if the circuit is open after recording the failure.
func (cb *CircuitBreaker) RecordFailure() bool {
	cb.Lock()
	defer cb.Unlock()

	cb.failures++
	cb.lastFailure = time.Now()

	if cb.failures >= cb.maxFailures {
		cb.isOpen = true
	}

	return cb.isOpen
}

// IsOpen checks if the circuit is open.
// If the circuit is open and the timeout has expired since the last failure, it resets the circuit.
func (cb *CircuitBreaker) IsOpen() bool {
	cb.RLock()
	defer cb.RUnlock()

	if !cb.isOpen {
		return false
	}

	// Auto-reset after timeout
	if time.Since(cb.lastFailure) > cb.timeout {
		cb.Lock()
		cb.isOpen = false
		cb.failures = 0
		cb.Unlock()
		return false
	}
	return true
}

// Reset clears the failure count and closes the circuit.
func (cb *CircuitBreaker) Reset() {
	cb.Lock()
	defer cb.Unlock()
	cb.failures = 0
	cb.isOpen = false
}
