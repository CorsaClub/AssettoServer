// Package metrics contains Prometheus metric definitions for the server
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Debugging metrics
var (
	DebugEventCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "assetto_server_debug_events_total",
			Help: "Total number of debug events by type",
		},
		append(ServerLabels, "event_type"),
	)

	DebugTimingHistogram = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "assetto_server_debug_timing_seconds",
			Help:    "Timing of various operations for debugging",
			Buckets: prometheus.ExponentialBuckets(0.001, 2, 10),
		},
		append(ServerLabels, "operation"),
	)

	GoroutineGauge = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "assetto_server_goroutines",
			Help: "Number of goroutines by type",
		},
		append(ServerLabels, "type"),
	)
)
