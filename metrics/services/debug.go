// Package metrics contains metric definitions for the server
package metrics

import (
	"time"
)

// Debug related metrics
const (
	DebugEventTotal     = "assetto_debug_events_total"
	DebugWarningTotal   = "assetto_debug_warnings_total"
	DebugErrorTotal     = "assetto_debug_errors_total"
	DebugLatencySeconds = "assetto_debug_latency_seconds"
	DebugMemoryBytes    = "assetto_debug_memory_bytes"
	DebugGoroutines     = "assetto_debug_goroutines_total"
)

// Debug metrics collector
type DebugMetrics struct {
	client *VictoriaMetricsClient
}

// NewDebugMetrics creates a new debug metrics collector
func NewDebugMetrics(client *VictoriaMetricsClient) *DebugMetrics {
	return &DebugMetrics{
		client: client,
	}
}

// RecordEvent records a debug event
func (d *DebugMetrics) RecordEvent(eventType string, labels map[string]string) {
	d.client.SendMetrics(MetricBatch{
		Metrics: []Metric{
			{
				Name:        DebugEventTotal,
				Value:       1,
				Type:        Counter,
				Timestamp:   time.Now(),
				LabelValues: labels,
			},
		},
		Time: time.Now(),
	})
}

// RecordWarning records a debug warning
func (d *DebugMetrics) RecordWarning(warningType string, labels map[string]string) {
	d.client.SendMetrics(MetricBatch{
		Metrics: []Metric{
			{
				Name:        DebugWarningTotal,
				Value:       1,
				Type:        Counter,
				Timestamp:   time.Now(),
				LabelValues: labels,
			},
		},
		Time: time.Now(),
	})
}

// RecordError records a debug error
func (d *DebugMetrics) RecordError(errorType string, labels map[string]string) {
	d.client.SendMetrics(MetricBatch{
		Metrics: []Metric{
			{
				Name:        DebugErrorTotal,
				Value:       1,
				Type:        Counter,
				Timestamp:   time.Now(),
				LabelValues: labels,
			},
		},
		Time: time.Now(),
	})
}

// RecordLatency records operation latency
func (d *DebugMetrics) RecordLatency(operation string, duration time.Duration, labels map[string]string) {
	d.client.SendMetrics(MetricBatch{
		Metrics: []Metric{
			{
				Name:        DebugLatencySeconds,
				Value:       duration.Seconds(),
				Type:        Gauge,
				Timestamp:   time.Now(),
				LabelValues: labels,
			},
		},
		Time: time.Now(),
	})
}

// RecordMemoryUsage records memory usage
func (d *DebugMetrics) RecordMemoryUsage(bytes int64, labels map[string]string) {
	d.client.SendMetrics(MetricBatch{
		Metrics: []Metric{
			{
				Name:        DebugMemoryBytes,
				Value:       float64(bytes),
				Type:        Gauge,
				Timestamp:   time.Now(),
				LabelValues: labels,
			},
		},
		Time: time.Now(),
	})
}

// RecordGoroutines records the number of goroutines
func (d *DebugMetrics) RecordGoroutines(count int, labels map[string]string) {
	d.client.SendMetrics(MetricBatch{
		Metrics: []Metric{
			{
				Name:        DebugGoroutines,
				Value:       float64(count),
				Type:        Gauge,
				Timestamp:   time.Now(),
				LabelValues: labels,
			},
		},
		Time: time.Now(),
	})
}
