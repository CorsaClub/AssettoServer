package config

import (
	"os"
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

type VictoriaLogsConfig struct {
	URL            string        `json:"url"`
	Port           string        `json:"port"`
	Username       string        `json:"username"`
	Password       string        `json:"password"`
	RequestTimeout time.Duration `json:"request_timeout"`
	ConnectTimeout time.Duration `json:"connect_timeout"`
	MaxRetries     int           `json:"max_retries"`
	RetryBackoff   time.Duration `json:"retry_backoff"`
	Timeout        time.Duration `json:"timeout"`
	Compression    bool          `json:"compression"`
}

type MetricsConfig struct {
	BatchSize           int           `json:"batch_size"`
	FlushInterval       time.Duration `json:"flush_interval"`
	BufferSize          int           `json:"buffer_size"`
	RetentionTime       time.Duration `json:"retention_time"`
	Compression         bool          `json:"compression"`
	MaxLabelsPerMetric  int           `json:"max_labels_per_metric"`
	MaxUniqueMetrics    int           `json:"max_unique_metrics"`
	MaxLabelValueLength int           `json:"max_label_value_length"`
	MaxMetricNameLength int           `json:"max_metric_name_length"`
}

// Config represents the configuration for the metrics service.
type Config struct {
	ServerID     string `yaml:"server_id"`
	ServerName   string `yaml:"server_name"`
	ServerRegion string `yaml:"server_region"`
	ServerType   string `yaml:"server_type"`

	Victoria     VictoriaConfig     `json:"victoria"`
	VictoriaLogs VictoriaLogsConfig `json:"victoria_logs"`
	Metrics      MetricsConfig      `json:"metrics"`
	GeoIP        GeoIPConfig        `json:"geoip"`
}

// AuthConfig is for websocket authentication
type AuthConfig struct {
	SteamID string
	UserID  string
}

// Valeurs par défaut pour la configuration
const (
	DefaultVictoriaURL         = "http://localhost:8428"
	DefaultVictoriaPort        = "8428"
	DefaultRequestTimeout      = 10 * time.Second
	DefaultConnectTimeout      = 5 * time.Second
	DefaultMaxRetries          = 3
	DefaultRetryBackoff        = time.Second
	DefaultBatchSize           = 1000
	DefaultFlushInterval       = 15 * time.Second
	DefaultBufferSize          = 10000
	DefaultRetentionTime       = 24 * time.Hour
	DefaultCompression         = true
	DefaultUsername            = ""
	DefaultPassword            = ""
	DefaultVictoriaLogsURL     = "http://localhost:9428"
	DefaultVictoriaLogsPort    = "9428"
	DefaultMaxLabelsPerMetric  = 10
	DefaultMaxUniqueMetrics    = 1000
	DefaultMaxLabelValueLength = 100
	DefaultMaxMetricNameLength = 200
	DefaultTimeout             = 10 * time.Second
	DefaultGeoIPEnabled        = false
	DefaultGeoIPDatabasePath   = "./GeoLite2-City.mmdb"
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
		VictoriaLogs: VictoriaLogsConfig{
			URL:            DefaultVictoriaLogsURL,
			Port:           DefaultVictoriaLogsPort,
			RequestTimeout: DefaultRequestTimeout,
			ConnectTimeout: DefaultConnectTimeout,
			MaxRetries:     DefaultMaxRetries,
			RetryBackoff:   DefaultRetryBackoff,
			Username:       DefaultUsername,
			Password:       DefaultPassword,
			Timeout:        DefaultTimeout,
			Compression:    DefaultCompression,
		},
		Metrics: MetricsConfig{
			BatchSize:           DefaultBatchSize,
			FlushInterval:       DefaultFlushInterval,
			BufferSize:          DefaultBufferSize,
			RetentionTime:       DefaultRetentionTime,
			Compression:         DefaultCompression,
			MaxLabelsPerMetric:  DefaultMaxLabelsPerMetric,
			MaxUniqueMetrics:    DefaultMaxUniqueMetrics,
			MaxLabelValueLength: DefaultMaxLabelValueLength,
			MaxMetricNameLength: DefaultMaxMetricNameLength,
		},
		GeoIP: GeoIPConfig{
			Enabled:      DefaultGeoIPEnabled,
			DatabasePath: DefaultGeoIPDatabasePath,
		},
	}
}

// NewAuthConfig is for websocket authentication
func NewAuthConfig() *AuthConfig {
	return &AuthConfig{
		SteamID: os.Getenv("AUTH_STEAM_ID"),
		UserID:  os.Getenv("AUTH_USER_ID"),
	}
}

// IsValid is for websocket authentication
func (c *AuthConfig) IsValid() bool {
	return c.SteamID != "" && c.UserID != ""
}

// GeoIPConfig represents the configuration for the GeoIP service.
type GeoIPConfig struct {
	Enabled      bool   `json:"enabled"`
	DatabasePath string `json:"database_path"`
}
