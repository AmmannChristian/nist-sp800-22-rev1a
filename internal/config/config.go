package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config holds all service configuration
type Config struct {
	// gRPC server configuration
	GRPCPort int

	// Metrics server configuration
	MetricsPort int

	// Logging configuration
	LogLevel string

	// Authentication configuration
	AuthEnabled  bool
	AuthIssuer   string
	AuthAudience string
	AuthJWKSURL  string
}

// Load reads configuration from environment variables
func Load() (*Config, error) {
	cfg := &Config{
		GRPCPort:     getEnvInt("GRPC_PORT", 9090),
		MetricsPort:  getEnvInt("METRICS_PORT", 9091),
		LogLevel:     getEnvString("LOG_LEVEL", "info"),
		AuthEnabled:  getEnvBool("AUTH_ENABLED", false),
		AuthIssuer:   getEnvString("AUTH_ISSUER", ""),
		AuthAudience: getEnvString("AUTH_AUDIENCE", ""),
		AuthJWKSURL:  getEnvString("AUTH_JWKS_URL", ""),
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.GRPCPort < 1 || c.GRPCPort > 65535 {
		return fmt.Errorf("invalid GRPC_PORT: %d (must be 1-65535)", c.GRPCPort)
	}

	if c.MetricsPort < 1 || c.MetricsPort > 65535 {
		return fmt.Errorf("invalid METRICS_PORT: %d (must be 1-65535)", c.MetricsPort)
	}

	validLogLevels := map[string]bool{
		"debug": true,
		"info":  true,
		"warn":  true,
		"error": true,
	}

	if !validLogLevels[c.LogLevel] {
		return fmt.Errorf("invalid LOG_LEVEL: %s (must be debug/info/warn/error)", c.LogLevel)
	}

	if c.AuthEnabled {
		if c.AuthIssuer == "" {
			return fmt.Errorf("invalid AUTH_ISSUER: required when AUTH_ENABLED=true")
		}
		if c.AuthAudience == "" {
			return fmt.Errorf("invalid AUTH_AUDIENCE: required when AUTH_ENABLED=true")
		}
	}

	return nil
}

// getEnvString reads a string from environment variable or returns default
func getEnvString(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvInt reads an integer from environment variable or returns default
func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

// getEnvBool reads a boolean from environment variable or returns default
func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolVal, err := strconv.ParseBool(value); err == nil {
			return boolVal
		}
	}
	return defaultValue
}
