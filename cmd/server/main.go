// Package main is the entry point for the NIST service
package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/AmmannChristian/go-authx/grpcserver"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"github.com/AmmannChristian/nist-sp800-22-rev1a/internal/config"
	"github.com/AmmannChristian/nist-sp800-22-rev1a/internal/middleware"
	"github.com/AmmannChristian/nist-sp800-22-rev1a/internal/service"
	pb "github.com/AmmannChristian/nist-sp800-22-rev1a/pkg/pb"
)

func main() {
	if err := run(context.Background()); err != nil {
		log.Fatal().Err(err).Msg("Application failed")
	}
}

func run(ctx context.Context) error {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Setup logging
	setupLogging(cfg.LogLevel)

	log.Info().
		Int("grpc_port", cfg.GRPCPort).
		Int("metrics_port", cfg.MetricsPort).
		Str("log_level", cfg.LogLevel).
		Bool("auth_enabled", cfg.AuthEnabled).
		Msg("Starting NIST Statistical Test Service")

	// Start Prometheus metrics server
	metricsLn, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.MetricsPort))
	if err != nil {
		return fmt.Errorf("failed to create metrics listener: %w", err)
	}
	metricsSrv := startMetricsServer(metricsLn)
	defer metricsSrv.Close()

	// Create gRPC listener
	grpcLn, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.GRPCPort))
	if err != nil {
		return fmt.Errorf("failed to create gRPC listener: %w", err)
	}

	unaryInterceptors, err := buildUnaryInterceptors(cfg)
	if err != nil {
		return fmt.Errorf("failed to configure gRPC server: %w", err)
	}

	grpcServer, err := runGRPCServer(cfg, unaryInterceptors)
	if err != nil {
		return fmt.Errorf("failed to create gRPC server: %w", err)
	}

	// Handle graceful shutdown
	// We merge the provided context with signal handling
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		select {
		case <-sigChan:
			log.Info().Msg("Shutting down gracefully...")
		case <-ctx.Done():
			// Context cancelled (e.g. by test)
		}

		// Graceful stop
		grpcServer.GracefulStop()
		cancel()
	}()

	log.Info().
		Int("port", cfg.GRPCPort).
		Msg("gRPC server listening")

	// Start serving (blocking)
	if err := grpcServer.Serve(grpcLn); err != nil {
		return fmt.Errorf("failed to serve: %w", err)
	}

	return nil
}

// setupLogging configures the zerolog logger
func setupLogging(level string) {
	// Pretty logging for development
	log.Logger = log.Output(zerolog.ConsoleWriter{
		Out:        os.Stderr,
		TimeFormat: time.RFC3339,
	})

	// Set log level
	switch level {
	case "debug":
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case "info":
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	case "warn":
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	case "error":
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	default:
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}
}

// startMetricsServer starts the Prometheus metrics HTTP server
func startMetricsServer(ln net.Listener) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]string{
			"status":  "healthy",
			"version": service.Version,
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			log.Error().Err(err).Msg("failed to write health response")
			http.Error(w, "failed to encode health response", http.StatusInternalServerError)
		}
	})

	log.Info().
		Str("addr", ln.Addr().String()).
		Msg("Metrics server listening (with pprof at /debug/pprof/)")

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal().Err(err).Msg("Metrics server failed")
		}
	}()

	return srv
}

// runGRPCServer creates and configures the gRPC server
func runGRPCServer(cfg *config.Config, unaryInterceptors []grpc.UnaryServerInterceptor) (*grpc.Server, error) {
	serverOpts, err := buildGRPCServerOptions(cfg, unaryInterceptors)
	if err != nil {
		return nil, err
	}

	grpcServer := grpc.NewServer(serverOpts...)

	// Register NIST SP 800-22 service
	nistServer := service.NewServer()
	pb.RegisterSp80022TestServiceServer(grpcServer, nistServer)

	// Register health check service
	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)

	// Register reflection for grpcurl
	reflection.Register(grpcServer)

	return grpcServer, nil
}

func buildUnaryInterceptors(cfg *config.Config) ([]grpc.UnaryServerInterceptor, error) {
	interceptors := []grpc.UnaryServerInterceptor{
		middleware.UnaryRequestIDInterceptor(),
		loggingInterceptor,
	}

	if !cfg.AuthEnabled {
		return interceptors, nil
	}

	validatorBuilder := grpcserver.NewValidatorBuilder(cfg.AuthIssuer, cfg.AuthAudience)
	if cfg.AuthJWKSURL != "" {
		validatorBuilder = validatorBuilder.WithJWKSURL(cfg.AuthJWKSURL)
	}

	validator, err := validatorBuilder.Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build auth validator: %w", err)
	}

	log.Info().
		Str("issuer", cfg.AuthIssuer).
		Str("audience", cfg.AuthAudience).
		Str("jwks_url", cfg.AuthJWKSURL).
		Msg("gRPC authentication enabled")

	authInterceptor := grpcserver.UnaryServerInterceptor(
		validator,
		grpcserver.WithExemptMethods(
			"/grpc.health.v1.Health/Check",
			"/grpc.health.v1.Health/Watch",
		),
	)

	return append(interceptors, authInterceptor), nil
}

func buildGRPCServerOptions(cfg *config.Config, unaryInterceptors []grpc.UnaryServerInterceptor) ([]grpc.ServerOption, error) {
	opts := []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(unaryInterceptors...),
	}

	if !cfg.TLSEnabled {
		return opts, nil
	}

	clientAuth, err := cfg.TLSClientAuthType()
	if err != nil {
		return nil, fmt.Errorf("invalid TLS client auth setting: %w", err)
	}

	minVersion, err := cfg.TLSMinVersionValue()
	if err != nil {
		return nil, fmt.Errorf("invalid TLS min version: %w", err)
	}

	tlsConfig := &grpcserver.TLSConfig{
		CertFile:   cfg.TLSCertFile,
		KeyFile:    cfg.TLSKeyFile,
		CAFile:     cfg.TLSCAFile,
		ClientAuth: clientAuth,
		MinVersion: minVersion,
	}

	tlsOpt, err := grpcserver.ServerOption(tlsConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to configure TLS: %w", err)
	}

	log.Info().
		Bool("tls_enabled", true).
		Str("cert_file", cfg.TLSCertFile).
		Str("key_file", cfg.TLSKeyFile).
		Str("ca_file", cfg.TLSCAFile).
		Str("client_auth", cfg.TLSClientAuth).
		Str("min_version", tlsVersionString(minVersion)).
		Msg("gRPC TLS enabled")

	return append(opts, tlsOpt), nil
}

func tlsVersionString(version uint16) string {
	switch version {
	case tls.VersionTLS13:
		return "1.3"
	case tls.VersionTLS12:
		return "1.2"
	default:
		return fmt.Sprintf("0x%x", version)
	}
}

// loggingInterceptor logs all gRPC requests with request ID
func loggingInterceptor(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (interface{}, error) {
	start := time.Now()

	// Get request ID from context
	requestID := middleware.GetRequestID(ctx)

	// Call the handler
	resp, err := handler(ctx, req)

	// Log the request
	duration := time.Since(start)

	if err != nil {
		log.Error().
			Err(err).
			Str("request_id", requestID).
			Str("method", info.FullMethod).
			Dur("duration", duration).
			Msg("gRPC request failed")
	} else {
		log.Debug().
			Str("request_id", requestID).
			Str("method", info.FullMethod).
			Dur("duration", duration).
			Msg("gRPC request completed")
	}

	return resp, err
}
