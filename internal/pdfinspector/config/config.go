package config

import (
	"time"

	"github.com/spf13/viper"
)

// Config defines the runtime configuration for PDF Inspector and Firecrawl integration.
type Config struct {
	FirecrawlBaseURL string        `mapstructure:"FIRECRAWL_BASE_URL"`
	HTTPTimeout      time.Duration `mapstructure:"HTTP_TIMEOUT"`
	MaxRetries       int           `mapstructure:"MAX_RETRIES"`
	MaxDocumentSize  int64         `mapstructure:"MAX_DOCUMENT_SIZE_BYTES"`
	MaxPageCount     int           `mapstructure:"MAX_PAGE_COUNT"`
}

// LoadFromEnv loads configuration using Viper from environment variables and optional .env files.
func LoadFromEnv() *Config {
	v := viper.New()
	v.AutomaticEnv()

	// Defaults
	v.SetDefault("FIRECRAWL_BASE_URL", "http://localhost:3002")
	v.SetDefault("HTTP_TIMEOUT", 30*time.Second)
	v.SetDefault("MAX_RETRIES", 3)
	v.SetDefault("MAX_DOCUMENT_SIZE_BYTES", int64(100*1024*1024)) // 100 MB default
	v.SetDefault("MAX_PAGE_COUNT", 1000)

	cfg := &Config{
		FirecrawlBaseURL: v.GetString("FIRECRAWL_BASE_URL"),
		HTTPTimeout:      v.GetDuration("HTTP_TIMEOUT"),
		MaxRetries:       v.GetInt("MAX_RETRIES"),
		MaxDocumentSize:  v.GetInt64("MAX_DOCUMENT_SIZE_BYTES"),
		MaxPageCount:     v.GetInt("MAX_PAGE_COUNT"),
	}

	return cfg
}
