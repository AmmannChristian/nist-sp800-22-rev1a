// Package service implements the gRPC Sp80022TestService (NIST SP 800-22 Rev 1a)
package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	pb "github.com/AmmannChristian/nist-sp800-22-rev1a/pkg/pb"

	"github.com/AmmannChristian/nist-sp800-22-rev1a/internal/metrics"
	"github.com/AmmannChristian/nist-sp800-22-rev1a/internal/nist"

	"gonum.org/v1/gonum/mathext"
)

// runAllTests is a variable to allow mocking in tests
var runAllTests = nist.RunAllTests

const (
	// Version of the service (2.0.0 for breaking API change)
	Version = "2.0.0"

	// Alpha significance level from NIST (p-value threshold)
	Alpha = 0.01
)

// Server implements the Sp80022TestService
type Server struct {
	pb.UnimplementedSp80022TestServiceServer
}

// NewServer creates a new Sp80022TestService server
func NewServer() *Server {
	return &Server{}
}

// RunTestSuite implements the RunTestSuite RPC
func (s *Server) RunTestSuite(ctx context.Context, req *pb.Sp80022TestRequest) (*pb.Sp80022TestResponse, error) {
	startTime := time.Now()

	// Generate unique request ID for log correlation
	requestID := uuid.New().String()

	log.Info().
		Str("request_id", requestID).
		Int("bitstream_bytes", len(req.Bitstream)).
		Msg("RunTestSuite request received")

	// Validate request
	if err := s.validateRequest(req); err != nil {
		log.Error().
			Str("request_id", requestID).
			Err(err).
			Msg("Request validation failed")
		metrics.RequestsTotal.WithLabelValues("RunTestSuite", "error").Inc()
		return nil, err
	}

	metrics.RequestsTotal.WithLabelValues("RunTestSuite", "success").Inc()

	// Run NIST tests in pure Go
	testStart := time.Now()
	results, err := runAllTests(req.Bitstream)
	if err != nil {
		log.Error().
			Str("request_id", requestID).
			Err(err).
			Msg("NIST test execution failed")
		return nil, fmt.Errorf("test execution failed: %w", err)
	}

	// Record overall duration
	duration := time.Since(testStart)
	metrics.OverallDuration.Observe(duration.Seconds())

	sampleBits := int32(len(req.Bitstream) * 8) //nolint:gosec // safe: MaxBits < 2^31

	// Build response
	response := &pb.Sp80022TestResponse{
		Timestamp:       time.Now().Format(time.RFC3339),
		SampleSizeBits:  sampleBits,
		Results:         make([]*pb.Sp80022TestResult, len(results)),
		ExecutionTimeMs: time.Since(startTime).Milliseconds(),
	}

	// Convert results and compute overall metrics
	passedCount := 0
	testsRun := 0
	pValues := make([]float64, 0, len(results))

	for i, result := range results {
		// Skip tests that weren't implemented (p_value < 0)
		if result.PValue < 0.0 {
			// Mark as skipped
			pbResult := &pb.Sp80022TestResult{
				Name:   result.Name,
				PValue: result.PValue,
				Passed: false,
			}

			if result.Warning != "" {
				pbResult.Warning = &result.Warning
			}

			response.Results[i] = pbResult
			continue
		}

		// This is a real test result
		testsRun++

		// Record metrics for this test
		status := "fail"
		if result.Passed {
			status = "pass"
			passedCount++
		}
		metrics.TestsTotal.WithLabelValues(result.Name, status).Inc()
		metrics.PValue.WithLabelValues(result.Name).Set(result.PValue)

		// Convert to protobuf message
		pbResult := &pb.Sp80022TestResult{
			Name:   result.Name,
			PValue: result.PValue,
			Passed: result.Passed,
		}

		if result.Proportion > 0 {
			pbResult.Proportion = &result.Proportion
		}

		if result.Warning != "" {
			pbResult.Warning = &result.Warning
		}

		response.Results[i] = pbResult

		// Only add real p-values to uniformity check
		pValues = append(pValues, result.PValue)
	}

	// Calculate overall pass rate ONLY for implemented tests
	if testsRun > 0 {
		response.OverallPassRate = float64(passedCount) / float64(testsRun)
		metrics.LastOverallPassRate.Set(response.OverallPassRate)
	} else {
		response.OverallPassRate = 0.0
	}

	// Transparency fields
	response.TestsRun = int32(testsRun)                    //nolint:gosec // testsRun <= len(results) <= 15
	response.TestsSkipped = int32(len(results) - testsRun) //nolint:gosec // bounded by len(results)
	response.TestsTotal = int32(len(results))              //nolint:gosec // Always 15 and fits int32
	response.NistCompliant = (testsRun == len(results))

	// Calculate p-value uniformity ONLY for real tests
	if len(pValues) >= 5 { // Need at least 5 tests for meaningful chi²
		response.PValueUniformityChi2 = calculatePValueUniformity(pValues)
	} else {
		response.PValueUniformityChi2 = -1.0 // Not enough data
	}

	log.Info().
		Str("request_id", requestID).
		Float64("overall_pass_rate", response.OverallPassRate).
		Float64("p_value_uniformity", response.PValueUniformityChi2).
		Int64("execution_time_ms", response.ExecutionTimeMs).
		Msg("Tests completed successfully")

	return response, nil
}

// validateRequest validates the test request
func (s *Server) validateRequest(req *pb.Sp80022TestRequest) error {
	if len(req.Bitstream) == 0 {
		return fmt.Errorf("bitstream cannot be empty")
	}

	numBits := len(req.Bitstream) * 8

	// Check minimum bits (Universal Test requires 387,840)
	if numBits < nist.MinBits {
		return fmt.Errorf("insufficient bits: got %d, need at least %d (%d bytes)",
			numBits, nist.MinBits, nist.MinBits/8)
	}

	// Check maximum bits (prevent excessive memory use)
	if numBits > nist.MaxBits {
		return fmt.Errorf("too many bits: got %d, maximum %d (%d bytes)",
			numBits, nist.MaxBits, nist.MaxBits/8)
	}

	return nil
}

// calculatePValueUniformity performs a chi-squared test on p-value distribution
// NIST expects p-values to be uniformly distributed in [0, 1]
func calculatePValueUniformity(pValues []float64) float64 {
	if len(pValues) == 0 {
		return 0.0
	}

	// Use 10 bins as per NIST specification
	const numBins = 10
	bins := make([]int, numBins)

	// Distribute p-values into bins
	for _, pval := range pValues {
		if pval < 0 || pval > 1 {
			continue // Skip invalid p-values
		}

		binIndex := int(pval * float64(numBins))
		if binIndex == numBins {
			binIndex = numBins - 1 // Handle p-value = 1.0
		}
		bins[binIndex]++
	}

	// Calculate chi-squared statistic
	expectedCount := float64(len(pValues)) / float64(numBins)
	chi2 := 0.0

	for _, observed := range bins {
		diff := float64(observed) - expectedCount
		chi2 += (diff * diff) / expectedCount
	}

	// Convert chi2 to p-value using incomplete gamma function
	// For now, return the raw chi2 statistic
	df := float64(numBins - 1)
	pValue := mathext.GammaIncRegComp(df/2.0, chi2/2.0)

	return pValue
}
