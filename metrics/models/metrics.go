package models

import "time"

type MetricType int

const (
	Gauge MetricType = iota
	Counter
	Histogram
)

type Metric struct {
	Name        string
	Value       float64
	Type        MetricType
	LabelValues map[string]string
	Timestamp   time.Time
	Buckets     []float64
}

type MetricBatch struct {
	Metrics []Metric
	Time    time.Time
}

type LogEntry struct {
	Timestamp time.Time
	Level     string
	Message   string
	Labels    map[string]string
}
