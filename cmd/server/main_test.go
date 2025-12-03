package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"

	"github.com/AmmannChristian/nist-sp800-22-rev1a/internal/config"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
)

func TestSetupLogging(t *testing.T) {
	origLevel := zerolog.GlobalLevel()
	defer zerolog.SetGlobalLevel(origLevel)

	tests := []struct {
		level    string
		expected zerolog.Level
	}{
		{"debug", zerolog.DebugLevel},
		{"info", zerolog.InfoLevel},
		{"warn", zerolog.WarnLevel},
		{"error", zerolog.ErrorLevel},
		{"unknown", zerolog.InfoLevel},
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			setupLogging(tt.level)
			if zerolog.GlobalLevel() != tt.expected {
				t.Errorf("expected level %v, got %v", tt.expected, zerolog.GlobalLevel())
			}
		})
	}
}

func TestLoggingInterceptor(t *testing.T) {
	// Setup
	origLevel := zerolog.GlobalLevel()
	defer zerolog.SetGlobalLevel(origLevel)
	setupLogging("debug")
	ctx := context.Background()
	req := "test request"
	info := &grpc.UnaryServerInfo{
		FullMethod: "/test.Service/Method",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return "test response", nil
	}

	// Execute
	resp, err := loggingInterceptor(ctx, req, info, handler)
	// Verify
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if resp != "test response" {
		t.Errorf("expected response 'test response', got %v", resp)
	}
}

func TestStartMetricsServer(t *testing.T) {
	ln := mustListen(t)
	// No defer ln.Close() here, server will close it

	// Run in goroutine
	srv := startMetricsServer(ln)
	defer srv.Close()

	// Poll health endpoint instead of sleeping blindly
	deadline := time.Now().Add(2 * time.Second)
	for {
		resp, err := http.Get(fmt.Sprintf("http://%s/health", ln.Addr().String()))
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			if err != nil {
				t.Fatalf("failed to get health: %v", err)
			}
			t.Fatalf("health endpoint returned %d", resp.StatusCode)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func TestRunGRPCServer(t *testing.T) {
	ln := mustListen(t)
	defer ln.Close()

	interceptors, err := buildUnaryInterceptors(&config.Config{})
	if err != nil {
		t.Fatalf("failed to build interceptors: %v", err)
	}

	srv := runGRPCServer(ln, interceptors)
	defer srv.Stop()

	go func() {
		if err := srv.Serve(ln); err != nil && err != grpc.ErrServerStopped {
			// t.Errorf here might be racy if test ends, but it logs failure
			log.Printf("grpc serve error: %v", err)
		}
	}()

	// Give it a moment
	time.Sleep(100 * time.Millisecond)

	// We could dial it to verify, but just running it covers the setup logic
}

func TestRun(t *testing.T) {
	// Find free ports
	l1 := mustListen(t)
	grpcPort := l1.Addr().(*net.TCPAddr).Port
	l1.Close()

	l2 := mustListen(t)
	metricsPort := l2.Addr().(*net.TCPAddr).Port
	l2.Close()

	// Set ports
	os.Setenv("GRPC_PORT", fmt.Sprintf("%d", grpcPort))
	os.Setenv("METRICS_PORT", fmt.Sprintf("%d", metricsPort))
	defer os.Unsetenv("GRPC_PORT")
	defer os.Unsetenv("METRICS_PORT")

	// Create cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Run in goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- run(ctx)
	}()

	// Give it a moment to start
	time.Sleep(200 * time.Millisecond)

	// Cancel context to trigger graceful shutdown
	cancel()

	// Wait for run to return
	select {
	case err := <-errChan:
		if err != nil {
			t.Errorf("run() returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("run() timed out waiting for shutdown")
	}
}

func TestRunConfigError(t *testing.T) {
	os.Setenv("GRPC_PORT", "-1")
	defer os.Unsetenv("GRPC_PORT")

	if err := run(context.Background()); err == nil {
		t.Error("expected error for invalid config, got nil")
	}
}

func TestRunPortInUse(t *testing.T) {
	// Listen on a port
	l := mustListen(t)
	defer l.Close()
	port := l.Addr().(*net.TCPAddr).Port

	os.Setenv("GRPC_PORT", fmt.Sprintf("%d", port))
	defer os.Unsetenv("GRPC_PORT")

	// run should fail to bind gRPC port
	if err := run(context.Background()); err == nil {
		t.Error("expected error for port in use, got nil")
	}
}

func TestLoggingInterceptorError(t *testing.T) {
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return nil, fmt.Errorf("handler error")
	}

	_, err := loggingInterceptor(context.Background(), "req", &grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}, handler)
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestRunSignal(t *testing.T) {
	// Ensure signal handlers are reset after the test to avoid leaking Notify state.
	t.Cleanup(func() {
		signal.Reset(syscall.SIGTERM, os.Interrupt)
	})

	// Find free ports
	l1 := mustListen(t)
	grpcPort := l1.Addr().(*net.TCPAddr).Port
	l1.Close()

	l2 := mustListen(t)
	metricsPort := l2.Addr().(*net.TCPAddr).Port
	l2.Close()

	// Set ports
	os.Setenv("GRPC_PORT", fmt.Sprintf("%d", grpcPort))
	os.Setenv("METRICS_PORT", fmt.Sprintf("%d", metricsPort))
	defer os.Unsetenv("GRPC_PORT")
	defer os.Unsetenv("METRICS_PORT")

	// Run in goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- run(context.Background())
	}()

	// Give it a moment to start
	time.Sleep(200 * time.Millisecond)

	// Send SIGTERM to self
	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("failed to find process: %v", err)
	}
	if err := p.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("failed to send signal: %v", err)
	}

	// Wait for run to return
	select {
	case err := <-errChan:
		if err != nil {
			t.Errorf("run() returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("run() timed out waiting for shutdown")
	}
}

func mustListen(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Skipf("cannot listen on tcp :0 in test environment: %v", err)
	}
	return ln
}
