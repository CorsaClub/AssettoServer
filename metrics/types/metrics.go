package types

import "time"

type MetricType string

const (
	Counter   MetricType = "counter"
	Gauge     MetricType = "gauge"
	Histogram MetricType = "histogram"
)

type Metric struct {
	Name        string
	Value       float64
	Type        MetricType
	Timestamp   time.Time
	LabelValues map[string]string
	Buckets     []float64
}

type MetricBatch struct {
	Metrics []Metric
	Time    time.Time
}
