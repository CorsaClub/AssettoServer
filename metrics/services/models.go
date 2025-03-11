// Package metrics contains helper functions for creating metrics
package metrics

import (
	"metrics/types"
)

// Common labels for all metrics
var ServerLabels = []string{"server_id", "server_name", "server_region"}

// CreateServerMetric creates a new server metric with common labels
func CreateServerMetric(name string, help string, metricType types.MetricType) types.Metric {
	return types.NewMetric(name, help, metricType, ServerLabels)
}

// CreatePlayerMetric creates a new player metric with common labels plus player-specific labels
func CreatePlayerMetric(name string, help string, metricType types.MetricType) types.Metric {
	labels := append(ServerLabels, "player_id", "player_name")
	return types.NewMetric(name, help, metricType, labels)
}

// CreateSessionMetric creates a new session metric with common labels plus session-specific labels
func CreateSessionMetric(name string, help string, metricType types.MetricType) types.Metric {
	labels := append(ServerLabels, "session_id", "session_type")
	return types.NewMetric(name, help, metricType, labels)
}

// CreateTrackMetric creates a new track metric with common labels plus track-specific labels
func CreateTrackMetric(name string, help string, metricType types.MetricType) types.Metric {
	labels := append(ServerLabels, "track", "layout")
	return types.NewMetric(name, help, metricType, labels)
}

// CreateCarMetric creates a new car metric with common labels plus car-specific labels
func CreateCarMetric(name string, help string, metricType types.MetricType) types.Metric {
	labels := append(ServerLabels, "car_model")
	return types.NewMetric(name, help, metricType, labels)
}

// CreateNetworkMetric creates a new network metric with common labels
func CreateNetworkMetric(name string, help string, metricType types.MetricType) types.Metric {
	return types.NewMetric(name, help, metricType, ServerLabels)
}

// CreateSystemMetric creates a new system metric with common labels
func CreateSystemMetric(name string, help string, metricType types.MetricType) types.Metric {
	return types.NewMetric(name, help, metricType, ServerLabels)
}

// CreateMetricsSystemMetric creates a new metrics system metric with common labels
func CreateMetricsSystemMetric(name string, help string, metricType types.MetricType) types.Metric {
	return types.NewMetric(name, help, metricType, ServerLabels)
}

// CreateCollisionMetric creates a new collision metric with common labels plus collision-specific labels
func CreateCollisionMetric(name string, help string, metricType types.MetricType) types.Metric {
	labels := append(ServerLabels, "car1", "car2", "speed", "force")
	return types.NewMetric(name, help, metricType, labels)
}

// CreateChatMetric creates a new chat metric with common labels plus chat-specific labels
func CreateChatMetric(name string, help string, metricType types.MetricType) types.Metric {
	labels := append(ServerLabels, "player_id", "player_name", "message_type")
	return types.NewMetric(name, help, metricType, labels)
}

// WithLabels adds labels to a metric and returns a new copy
func WithLabels(m types.Metric, labels map[string]string) types.Metric {
	return m.With(labels)
}

// SetValue sets the value of a gauge metric
func SetValue(m types.Metric, value float64) types.Metric {
	return m.Set(value)
}

// IncrementCounter increments a counter metric by 1
func IncrementCounter(m types.Metric) types.Metric {
	return m.Inc()
}

// AddToCounter adds a value to a counter metric
func AddToCounter(m types.Metric, value float64) types.Metric {
	return m.Add(value)
}

// ObserveHistogram adds an observation to a histogram metric
func ObserveHistogram(m types.Metric, value float64) types.Metric {
	return m.Observe(value)
}
