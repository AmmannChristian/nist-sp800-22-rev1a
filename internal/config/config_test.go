package config

import (
	"testing"
)

func TestLoadWithEnvOverrides(t *testing.T) {
	t.Setenv("GRPC_PORT", "5000")
	t.Setenv("METRICS_PORT", "6000")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("AUTH_ENABLED", "true")
	t.Setenv("AUTH_ISSUER", "https://issuer.example.com")
	t.Setenv("AUTH_AUDIENCE", "nist-api")
	t.Setenv("AUTH_JWKS_URL", "https://issuer.example.com/jwks.json")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.GRPCPort != 5000 || cfg.MetricsPort != 6000 {
		t.Fatalf("unexpected ports: %+v", cfg)
	}
	if cfg.LogLevel != "debug" {
		t.Fatalf("unexpected log level: %s", cfg.LogLevel)
	}
	if !cfg.AuthEnabled {
		t.Fatalf("expected AuthEnabled to be true")
	}
	if cfg.AuthIssuer != "https://issuer.example.com" {
		t.Fatalf("unexpected issuer: %s", cfg.AuthIssuer)
	}
	if cfg.AuthAudience != "nist-api" {
		t.Fatalf("unexpected audience: %s", cfg.AuthAudience)
	}
	if cfg.AuthJWKSURL != "https://issuer.example.com/jwks.json" {
		t.Fatalf("unexpected JWKS URL: %s", cfg.AuthJWKSURL)
	}
}

func TestValidateFailures(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
	}{
		{"bad grpc port", Config{GRPCPort: 0, MetricsPort: 9000, LogLevel: "info"}},
		{"bad metrics port", Config{GRPCPort: 9000, MetricsPort: 70000, LogLevel: "info"}},
		{"bad log level", Config{GRPCPort: 9000, MetricsPort: 9001, LogLevel: "verbose"}},
		{"auth enabled missing issuer", Config{GRPCPort: 9000, MetricsPort: 9001, LogLevel: "info", AuthEnabled: true, AuthAudience: "api"}},
		{"auth enabled missing audience", Config{GRPCPort: 9000, MetricsPort: 9001, LogLevel: "info", AuthEnabled: true, AuthIssuer: "https://issuer.example.com"}},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.cfg.Validate(); err == nil {
				t.Fatalf("expected error for %s", tt.name)
			}
		})
	}

	// getEnvInt falls back on parse error
	t.Setenv("SOME_INT", "notanint")
	if v := getEnvInt("SOME_INT", 42); v != 42 {
		t.Fatalf("expected default on parse error, got %d", v)
	}
}

func TestLoadDefaults(t *testing.T) {
	// Clear any environment variables
	for _, key := range []string{"GRPC_PORT", "METRICS_PORT", "LOG_LEVEL", "AUTH_ENABLED", "AUTH_ISSUER", "AUTH_AUDIENCE", "AUTH_JWKS_URL"} {
		t.Setenv(key, "")
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	// Check default values
	if cfg.GRPCPort != 9090 {
		t.Errorf("expected default GRPCPort=9090, got %d", cfg.GRPCPort)
	}
	if cfg.MetricsPort != 9091 {
		t.Errorf("expected default MetricsPort=9091, got %d", cfg.MetricsPort)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("expected default LogLevel=info, got %s", cfg.LogLevel)
	}
	if cfg.AuthEnabled {
		t.Errorf("expected AuthEnabled to be false by default")
	}
	if cfg.AuthIssuer != "" || cfg.AuthAudience != "" || cfg.AuthJWKSURL != "" {
		t.Errorf("expected auth config defaults to be empty, got %+v", cfg)
	}
}

func TestLoadInvalidConfig(t *testing.T) {
	t.Setenv("GRPC_PORT", "0")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid port")
	}
}
