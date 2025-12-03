# NIST Statistical Test Suite - gRPC Microservice

![CI](https://github.com/AmmannChristian/nist-sp800-22-rev1a/actions/workflows/ci.yml/badge.svg)
![NIST Validation](https://github.com/AmmannChristian/nist-sp800-22-rev1a/actions/workflows/nist-validation.yml/badge.svg)
[![Go Report Card](https://goreportcard.com/badge/github.com/AmmannChristian/NIST-SP-800-22-Rev.1A-Go)](https://goreportcard.com/report/github.com/AmmannChristian/NIST-SP-800-22-Rev.1A-Go)
[![License](https://img.shields.io/github/license/AmmannChristian/nist-sp800-22-rev1a)](LICENSE)
[![codecov](https://codecov.io/gh/AmmannChristian/NIST-SP-800-22-Rev.1A-Go/branch/main/graph/badge.svg)](https://app.codecov.io/gh/AmmannChristian/NIST-SP-800-22-Rev.1A-Go)
[![Go Version](https://img.shields.io/github/go-mod/go-version/AmmannChristian/NIST-SP-800-22-Rev.1A-Go)](go.mod)

A high-performance gRPC service implementing the NIST SP 800-22 statistical test suite for random number generator validation. This service provides a modern API for executing all 15 NIST tests specified in the Special Publication 800-22 revision 1a.

The implementation maintains 95.4% test coverage with comprehensive benchmarking and observability features including request tracking and performance profiling.

## Validation Results

The Pure Go implementation has been validated against the original NIST C reference implementation across multiple datasets:

| Dataset | Status | Tests Passed | P-Value Match |
|---------|--------|--------------|---------------|
| data.pi | Validated | 15/15 | ±0.0001 |
| data.e | Validated | 15/15 | ±0.0001 |
| data.sqrt2 | Validated | 15/15 | ±0.0001 |
| data.bad_rng | Validated | 15/15 | ±0.0001 |

All tests produce numerically identical P-values to the NIST reference implementation within floating-point epsilon tolerance.

## Quick Start

### Using Docker Compose (Recommended)

```bash
# Start the service with monitoring stack
docker-compose up -d

# The service is now available on:
# - gRPC: localhost:9090
# - Metrics: localhost:9091
# - Prometheus: localhost:9092
# - Grafana: localhost:3000
```

### Local Build

```bash
# Install dependencies
make deps

# Build the service
make build

# Run locally
make run
```

The service will start on port 9090 (gRPC) and 9091 (metrics).

## Implementation Guide

### Architecture Overview

```
nist-800-22-test-suite/
├── api/nist/v1/          # Protobuf API definitions
├── cmd/server/           # Service entry point
├── internal/
│   ├── config/          # Configuration management
│   ├── metrics/         # Prometheus metrics
│   ├── middleware/      # Request interceptors (Request-ID, logging)
│   ├── nist/            # Pure Go test implementations
│   └── service/         # gRPC service handlers
├── pkg/pb/              # Generated protobuf code
└── testdata/           # NIST test datasets
```

### Core Components

**NIST Tests Implementation** (`internal/nist/`)

All 15 NIST SP 800-22 tests are implemented in pure Go:
- Frequency (Monobit) Test
- Block Frequency Test
- Cumulative Sums Test
- Runs Test
- Longest Run of Ones Test
- Binary Matrix Rank Test
- Discrete Fourier Transform Test
- Non-Overlapping Template Matching Test
- Overlapping Template Matching Test
- Universal Statistical Test
- Approximate Entropy Test
- Random Excursions Test
- Random Excursions Variant Test
- Serial Test
- Linear Complexity Test

**Service Layer** (`internal/service/`)

gRPC service implementation with:
- Request validation (bit count requirements)
- Parallel test execution
- Metrics collection (Prometheus)
- Request-ID tracking for distributed tracing
- Structured logging with zerolog
- Error handling

**Middleware** (`internal/middleware/`)

gRPC interceptors for observability:
- Request-ID generation (UUID-based)
- Automatic request/response logging with duration tracking
- Metadata injection for client-side tracing

**Configuration** (`internal/config/`)

Environment-based configuration:
- `GRPC_PORT` - gRPC service port (default: 9090)
- `METRICS_PORT` - Prometheus metrics and pprof profiling port (default: 9091)
- `LOG_LEVEL` - Logging verbosity (debug, info, warn, error)
- `AUTH_ENABLED` - Enable JWT validation for gRPC calls (default: false)
- `AUTH_ISSUER` - Expected token issuer (required when auth is enabled)
- `AUTH_AUDIENCE` - Expected token audience (required when auth is enabled)
- `AUTH_JWKS_URL` - Optional custom JWKS endpoint (defaults to issuer well-known URL)

### Extending the Service

To add custom test implementations:

1. Implement test function in `internal/nist/`:
```go
func CustomTest(bits []byte) (pValue float64, passed bool) {
    // Your test logic
    return pValue, pValue >= 0.01
}
```

2. Register in `internal/nist/run_all.go`:
```go
results = append(results, TestResult{
    Name: "custom_test",
    PValue: pValue,
    Passed: passed,
})
```

3. Update protobuf if needed and regenerate: `make proto`

## Testing

### Run Unit Tests

```bash
# Run all tests
make test

# Run with coverage report
make coverage

# Run with race detector
make test-race
```

### Coverage Report

The project maintains comprehensive test coverage with a 90% minimum threshold enforced in CI.

Current coverage: 95.4%

| Package | Coverage |
|---------|----------|
| internal/config | 100.0% |
| internal/metrics | 100.0% |
| internal/middleware | 100.0% |
| internal/nist | 95.3% |
| internal/service | 97.6% |
| cmd/server | 90.6% |

Generate detailed HTML coverage report:
```bash
make cover-html
```

### Scientific Validation

Validation tests compare the Pure Go implementation against the original NIST C reference:

```bash
# Run validation tests
go test -v ./internal/nist/

# Expected output:
# === RUN   TestMatchesSTSReferenceOnSamples
# --- PASS: TestMatchesSTSReferenceOnSamples (2.34s)
```

## Performance

### Benchmarking

The project includes comprehensive benchmarks for all 15 NIST tests. Run benchmarks using:

```bash
# Quick benchmark run
make bench

# Run with 10 iterations for statistical significance
make bench-all

# Capture baseline for regression testing
make bench-baseline

# Compare current performance with baseline
make bench-compare
```

For statistical comparison, install benchstat:
```bash
go install golang.org/x/perf/cmd/benchstat@latest
```

### Performance Metrics

Typical performance on 1,000,000 bits (125KB) measured on AMD Ryzen 7 PRO 7840U:

| Test | Time per Operation | Allocations |
|------|-------------------|-------------|
| Frequency (Monobit) | 29 µs | 0 |
| Block Frequency | 665 µs | 0 |
| Cumulative Sums | 8.7 ms | 1 |
| Runs | 3.5 ms | 0 |
| Longest Run of Ones | 4.3 ms | 2 |
| Binary Matrix Rank | 15 ms | 31,233 |
| Discrete Fourier Transform | 41 ms | 5 |
| Non-Overlapping Template | 655 ms | 1 |
| Overlapping Template | 5.4 ms | 1 |
| Universal Statistical | 3.6 ms | 2 |
| Approximate Entropy | 10 ms | 1 |
| Random Excursions | 8.5 ms | 1 |
| Random Excursions Variant | 8.9 ms | 1 |
| Serial | 23 ms | 1 |
| Linear Complexity | 13 ms | 1 |
| Full Suite (all 15 tests) | 1.42 s | 42,000 |

### Constraints

- Minimum bits: 387,840 (required for Universal Statistical Test)
- Maximum bits: 10,000,000 (performance limit)
- Recommended: 1,000,000 bits for optimal reliability

### Performance Profiling

The service includes pprof endpoints for detailed runtime analysis. Access profiling data at `http://localhost:9091/debug/pprof/`:

```bash
# CPU profiling (30 second sample)
curl http://localhost:9091/debug/pprof/profile?seconds=30 > cpu.prof
go tool pprof -http=:8080 cpu.prof

# Memory heap profiling
curl http://localhost:9091/debug/pprof/heap > heap.prof
go tool pprof -http=:8080 heap.prof

# Goroutine analysis
curl http://localhost:9091/debug/pprof/goroutine > goroutine.prof
go tool pprof -http=:8080 goroutine.prof
```

Available pprof profiles: heap, goroutine, threadcreate, block, mutex, profile (CPU), trace

## Monitoring and Observability

### Prometheus Metrics

Metrics are exposed at `http://localhost:9091/metrics`:

- `nist_tests_total` - Total number of test executions
- `nist_test_duration_seconds` - Test execution duration histogram
- `nist_test_failures_total` - Count of failed tests
- `nist_requests_total` - Total gRPC requests

Access Grafana at `http://localhost:3000` (default credentials: admin/admin) after starting with docker-compose.

### Request Tracking

Every gRPC request is assigned a unique Request-ID (UUID) for distributed tracing:

- Logged in all server logs under the `request_id` field
- Returned to clients via gRPC metadata header `x-request-id`
- Enables end-to-end request correlation across distributed systems

Example log entry:
```json
{
  "level": "debug",
  "request_id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
  "method": "/nist.v1.NISTTestService/RunTests",
  "duration": 1423,
  "message": "gRPC request completed"
}
```

### Structured Logging

The service uses zerolog for high-performance structured logging with zero allocations. Log output includes request IDs, method names, durations, and error details for comprehensive observability.

## Attribution and License

This project is licensed under the MIT License. See the LICENSE file for details.

This implementation is based on the algorithms described in NIST Special Publication 800-22 Revision 1a (April 2010): "A Statistical Test Suite for Random and Pseudorandom Number Generators for Cryptographic Applications". The original NIST specification is a public domain work of the United States Government.

This project reimplements the NIST test suite algorithms in pure Go. All test implementations have been validated to produce numerically identical results to the original NIST C reference implementation.

Test datasets from the original NIST Statistical Test Suite are included in the testdata directory for validation and testing purposes.

Note: While this implementation has been validated against the NIST reference implementation, users are responsible for determining whether it meets their specific requirements. For applications requiring FIPS compliance or other regulatory certifications, appropriate validation and certification procedures must be followed.

Reference: [NIST SP 800-22 Rev. 1a](https://csrc.nist.gov/publications/detail/sp/800-22/rev-1a/final)

## Development

### Prerequisites

- Go 1.25 or later
- Protocol Buffers compiler (`protoc`)
- Docker and Docker Compose (for containerized deployment)
- GCC (only if modifying C reference code)

### Build Targets

```bash
make help          # Show all available targets
make proto         # Generate protobuf code
make build         # Build binary
make build-arm64   # Build for ARM64
make dev           # Run in development mode
make clean         # Remove build artifacts
make fmt           # Format code
make lint          # Run linters
make tools         # Install development tools

# Testing
make test          # Run unit tests
make test-race     # Run tests with race detector
make coverage      # Generate coverage report with 90% threshold check
make cover-html    # Generate HTML coverage report

# Benchmarking
make bench         # Run performance benchmarks
make bench-all     # Run benchmarks with 10 iterations
make bench-baseline # Capture baseline for comparison
make bench-compare # Compare current benchmarks with baseline
```

### CI/CD

The project includes two GitHub Actions workflows:

**Continuous Integration (`ci.yml`):**
- Format checking and code style validation
- Static analysis (staticcheck, go vet)
- Security scanning (gosec, govulncheck)
- Unit tests with coverage gating (90% threshold)
- Race condition detection
- Binary builds (native and ARM64)
- Artifact uploads for test results and binaries

**NIST Validation (`nist-validation.yml`):**
- Builds and runs original NIST C reference suite
- Generates reference P-values from multiple datasets
- Validates Pure Go implementation matches C reference
- Tests 6 datasets (data.pi, data.e, data.sqrt2, data.sqrt3, data.sha1, data.bad_rng)
- Automatic failure if P-values differ from reference

Both workflows run on push and pull requests to main branch.

## Troubleshooting

**Service won't start**
- Check port availability: `lsof -i :9090`
- Verify protobuf generation: `make proto`
- Check logs for configuration errors

**Tests fail validation**
- Ensure dataset has sufficient bits (minimum 387,840)
- Verify data format (raw binary, not text)
- Check for data corruption

**Performance issues**
- Reduce input size (max 10M bits recommended)
- Check system resources (2 CPU cores, 512MB RAM minimum)
- Review metrics at `/metrics` endpoint

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make changes with tests
4. Ensure `make test` and `make lint` pass
5. Submit a pull request

All contributions must maintain validation against the NIST reference implementation.
