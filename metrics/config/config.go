package config

import (
	"time"
)

type VictoriaConfig struct {
	URL            string        `json:"url"`
	Port           string        `json:"port"`
	Username       string        `json:"username"`
	Password       string        `json:"password"`
	RequestTimeout time.Duration `json:"request_timeout"`
	ConnectTimeout time.Duration `json:"connect_timeout"`
	MaxRetries     int           `json:"max_retries"`
	RetryBackoff   time.Duration `json:"retry_backoff"`
	Compression    bool          `json:"compression"`
}

type MetricsConfig struct {
	BatchSize     int           `json:"batch_size"`
	FlushInterval time.Duration `json:"flush_interval"`
	BufferSize    int           `json:"buffer_size"`
	RetentionTime time.Duration `json:"retention_time"`
}

type Config struct {
	Victoria VictoriaConfig `json:"victoria"`
	Metrics  MetricsConfig  `json:"metrics"`
}

// Valeurs par défaut pour la configuration
const (
	DefaultVictoriaURL    = "http://localhost:8428"
	DefaultVictoriaPort   = "8428"
	DefaultRequestTimeout = 10 * time.Second
	DefaultConnectTimeout = 5 * time.Second
	DefaultMaxRetries     = 3
	DefaultRetryBackoff   = time.Second
	DefaultBatchSize      = 1000
	DefaultFlushInterval  = 15 * time.Second
	DefaultBufferSize     = 10000
	DefaultRetentionTime  = 24 * time.Hour
	DefaultCompression    = true
	DefaultUsername       = ""
	DefaultPassword       = ""
)

// NewDefaultConfig returns a Config with sensible defaults
func NewDefaultConfig() *Config {
	return &Config{
		Victoria: VictoriaConfig{
			URL:            DefaultVictoriaURL,
			RequestTimeout: DefaultRequestTimeout,
			ConnectTimeout: DefaultConnectTimeout,
			MaxRetries:     DefaultMaxRetries,
			RetryBackoff:   DefaultRetryBackoff,
			Compression:    DefaultCompression,
			Username:       DefaultUsername,
			Password:       DefaultPassword,
		},
		Metrics: MetricsConfig{
			BatchSize:     DefaultBatchSize,
			FlushInterval: DefaultFlushInterval,
			BufferSize:    DefaultBufferSize,
			RetentionTime: DefaultRetentionTime,
		},
	}
}
