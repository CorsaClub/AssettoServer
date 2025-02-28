// Package utils provides utility functions for data extraction and processing.
package utils

import (
	"context"
	"time"

	"golang.org/x/time/rate"
)

// RateLimiter manages rate limits for different operations.
type RateLimiter struct {
	healthChecks *rate.Limiter // Rate limiter for health checks
	metrics      *rate.Limiter // Rate limiter for metrics updates
	commands     *rate.Limiter // Rate limiter for command processing
}

// NewRateLimiter creates a new RateLimiter with predefined limits.
func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		healthChecks: rate.NewLimiter(rate.Every(time.Second), 2),           // 2 health checks/sec
		metrics:      rate.NewLimiter(rate.Every(5*time.Second), 1),         // 1 metric update/5sec
		commands:     rate.NewLimiter(rate.Every(100*time.Millisecond), 10), // 10 commands/100ms
	}
}

// WaitHealthCheck blocks until a health check is allowed under the rate limit.
func (r *RateLimiter) WaitHealthCheck(ctx context.Context) error {
	return r.healthChecks.Wait(ctx)
}

// WaitMetrics blocks until a metrics update is allowed under the rate limit.
func (r *RateLimiter) WaitMetrics(ctx context.Context) error {
	return r.metrics.Wait(ctx)
}

// WaitCommand blocks until a command processing is allowed under the rate limit.
func (r *RateLimiter) WaitCommand(ctx context.Context) error {
	return r.commands.Wait(ctx)
}

// Allow checks if an operation is allowed under the current rate limit without blocking.
// Returns true if allowed, false otherwise.
func (r *RateLimiter) Allow(op string) bool {
	switch op {
	case "health":
		return r.healthChecks.Allow()
	case "metrics":
		return r.metrics.Allow()
	case "command":
		return r.commands.Allow()
	default:
		return true
	}
}
