// Package utils provides utility functions for data extraction and processing.
package utils

import (
	"context"
	"sync"

	"golang.org/x/time/rate"
)

// RateLimitType defines the type of rate limiting operation
type RateLimitType string

const (
	// RateLimitTypeHealthCheck is for health check operations
	RateLimitTypeHealthCheck RateLimitType = "health"
	// RateLimitTypeMetrics is for metrics operations
	RateLimitTypeMetrics RateLimitType = "metrics"
	// RateLimitTypeCommand is for command operations
	RateLimitTypeCommand RateLimitType = "command"
	// RateLimitTypeAPI is for API operations
	RateLimitTypeAPI RateLimitType = "api"
	// RateLimitTypeWebSocket is for WebSocket operations
	RateLimitTypeWebSocket RateLimitType = "websocket"
)

// RateLimiterConfig holds configuration for a rate limiter
type RateLimiterConfig struct {
	// Rate is the number of events per second
	Rate float64
	// Burst is the maximum number of events allowed at once
	Burst int
}

// DefaultRateLimiterConfigs provides default configurations for different rate limiter types
var DefaultRateLimiterConfigs = map[RateLimitType]RateLimiterConfig{
	RateLimitTypeHealthCheck: {Rate: 2, Burst: 2},   // 2 health checks/sec with burst of 2
	RateLimitTypeMetrics:     {Rate: 0.2, Burst: 5}, // 1 metric update/5sec with burst of 5
	RateLimitTypeCommand:     {Rate: 10, Burst: 20}, // 10 commands/sec with burst of 20
	RateLimitTypeAPI:         {Rate: 5, Burst: 10},  // 5 API calls/sec with burst of 10
	RateLimitTypeWebSocket:   {Rate: 20, Burst: 50}, // 20 WebSocket messages/sec with burst of 50
}

// RateLimiter manages rate limits for different operations.
type RateLimiter struct {
	mu       sync.RWMutex
	limiters map[RateLimitType]*rate.Limiter
	configs  map[RateLimitType]RateLimiterConfig
}

// RateLimiterOption is a function that configures a RateLimiter
type RateLimiterOption func(*RateLimiter)

// WithConfig sets a specific configuration for a rate limiter type
func WithConfig(limitType RateLimitType, config RateLimiterConfig) RateLimiterOption {
	return func(r *RateLimiter) {
		r.configs[limitType] = config
		r.limiters[limitType] = createLimiter(config)
	}
}

// createLimiter creates a new rate limiter from a configuration
func createLimiter(config RateLimiterConfig) *rate.Limiter {
	return rate.NewLimiter(rate.Limit(config.Rate), config.Burst)
}

// NewRateLimiter creates a new RateLimiter with predefined limits.
func NewRateLimiter(options ...RateLimiterOption) *RateLimiter {
	r := &RateLimiter{
		limiters: make(map[RateLimitType]*rate.Limiter),
		configs:  make(map[RateLimitType]RateLimiterConfig),
	}

	// Set default configurations
	for limitType, config := range DefaultRateLimiterConfigs {
		r.configs[limitType] = config
		r.limiters[limitType] = createLimiter(config)
	}

	// Apply options
	for _, option := range options {
		option(r)
	}

	return r
}

// Wait blocks until an operation is allowed under the rate limit.
func (r *RateLimiter) Wait(ctx context.Context, limitType RateLimitType) error {
	r.mu.RLock()
	limiter, exists := r.limiters[limitType]
	r.mu.RUnlock()

	if !exists {
		// If no limiter exists for this type, create one with default config
		r.mu.Lock()
		config, configExists := DefaultRateLimiterConfigs[limitType]
		if !configExists {
			// If no default config exists, use a permissive default
			config = RateLimiterConfig{Rate: 100, Burst: 100}
		}
		limiter = createLimiter(config)
		r.limiters[limitType] = limiter
		r.configs[limitType] = config
		r.mu.Unlock()
	}

	return limiter.Wait(ctx)
}

// Allow checks if an operation is allowed under the current rate limit without blocking.
// Returns true if allowed, false otherwise.
func (r *RateLimiter) Allow(limitType RateLimitType) bool {
	r.mu.RLock()
	limiter, exists := r.limiters[limitType]
	r.mu.RUnlock()

	if !exists {
		// If no limiter exists for this type, create one with default config
		r.mu.Lock()
		config, configExists := DefaultRateLimiterConfigs[limitType]
		if !configExists {
			// If no default config exists, use a permissive default
			config = RateLimiterConfig{Rate: 100, Burst: 100}
		}
		limiter = createLimiter(config)
		r.limiters[limitType] = limiter
		r.configs[limitType] = config
		r.mu.Unlock()
	}

	return limiter.Allow()
}

// UpdateConfig updates the configuration for a specific rate limiter type
func (r *RateLimiter) UpdateConfig(limitType RateLimitType, config RateLimiterConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.configs[limitType] = config
	r.limiters[limitType] = createLimiter(config)
}

// GetConfig returns the current configuration for a specific rate limiter type
func (r *RateLimiter) GetConfig(limitType RateLimitType) (RateLimiterConfig, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	config, exists := r.configs[limitType]
	return config, exists
}

// ResetAll resets all rate limiters to their initial state
func (r *RateLimiter) ResetAll() {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Recreate all limiters with their current configs
	for limitType, config := range r.configs {
		r.limiters[limitType] = createLimiter(config)
	}
}

// For backward compatibility
func (r *RateLimiter) WaitHealthCheck(ctx context.Context) error {
	return r.Wait(ctx, RateLimitTypeHealthCheck)
}

func (r *RateLimiter) WaitMetrics(ctx context.Context) error {
	return r.Wait(ctx, RateLimitTypeMetrics)
}

func (r *RateLimiter) WaitCommand(ctx context.Context) error {
	return r.Wait(ctx, RateLimitTypeCommand)
}
