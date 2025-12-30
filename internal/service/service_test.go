package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/AmmannChristian/nist-sp800-22-rev1a/internal/nist"
	pb "github.com/AmmannChristian/nist-sp800-22-rev1a/pkg/pb"
)

func TestValidateRequest(t *testing.T) {
	s := NewServer()

	tooSmall := &pb.Sp80022TestRequest{Bitstream: make([]byte, 10)}
	if err := s.validateRequest(tooSmall); err == nil {
		t.Fatalf("expected error for insufficient bits")
	}

	justRight := &pb.Sp80022TestRequest{Bitstream: make([]byte, nist.MinBits/8)}
	if err := s.validateRequest(justRight); err != nil {
		t.Fatalf("unexpected error for valid size: %v", err)
	}
}

func TestCalculatePValueUniformity(t *testing.T) {
	values := []float64{0.1, 0.3, 0.5, 0.7, 0.9}
	chi2 := calculatePValueUniformity(values)
	if chi2 <= 0 {
		t.Fatalf("expected positive chi2, got %f", chi2)
	}

	if got := calculatePValueUniformity(nil); got != 0 {
		t.Fatalf("expected zero for empty input, got %f", got)
	}
}

func TestRunTestSuiteSuccessAndFailure(t *testing.T) {
	s := NewServer()

	// Too small should error
	if _, err := s.RunTestSuite(context.Background(), &pb.Sp80022TestRequest{Bitstream: make([]byte, 10)}); err == nil {
		t.Fatalf("expected error on insufficient bits")
	}

	// Valid size should succeed
	bits := make([]byte, nist.MinBits/8)
	start := time.Now()
	resp, err := s.RunTestSuite(context.Background(), &pb.Sp80022TestRequest{Bitstream: bits})
	if err != nil {
		t.Fatalf("RunTestSuite failed: %v", err)
	}
	if len(resp.Results) != 15 {
		t.Fatalf("expected 15 test results, got %d", len(resp.Results))
	}
	if resp.SampleSizeBits != int32(nist.MinBits) {
		t.Fatalf("unexpected sample size bits: %d", resp.SampleSizeBits)
	}
	if resp.ExecutionTimeMs <= 0 || time.Since(start) < 0 {
		t.Fatalf("invalid execution time ms: %d", resp.ExecutionTimeMs)
	}
}

func TestValidateRequestEdgeCases(t *testing.T) {
	s := NewServer()

	// Empty bitstream
	empty := &pb.Sp80022TestRequest{Bitstream: []byte{}}
	if err := s.validateRequest(empty); err == nil {
		t.Error("expected error for empty bitstream")
	}

	// Max bits exceeded
	// We can't easily allocate 10MB+ in a test without being slow/memory heavy,
	// but we can mock or just trust the logic.
	// Actually, nist.MaxBits is 10,000,000 bits = 1.25MB. That's fine to allocate.
	huge := &pb.Sp80022TestRequest{Bitstream: make([]byte, nist.MaxBits/8+1)}
	if err := s.validateRequest(huge); err == nil {
		t.Error("expected error for exceeding max bits")
	}
}

func TestCalculatePValueUniformityEdgeCases(t *testing.T) {
	// Test boundary conditions
	pValues := []float64{
		-0.1, // Should be ignored
		1.1,  // Should be ignored
		1.0,  // Should go to last bin
		0.0,  // Should go to first bin
		0.5,
	}

	// We have 3 valid values: 1.0, 0.0, 0.5
	// Bins: [1, 0, 0, 0, 0, 1, 0, 0, 0, 1] (roughly)
	// Expected count per bin: 3/10 = 0.3
	// Chi2 should be calculated.

	chi2 := calculatePValueUniformity(pValues)
	if chi2 <= 0 {
		t.Errorf("expected positive chi2, got %f", chi2)
	}
}

func TestRunTestSuiteCoverage(t *testing.T) {
	s := NewServer()

	// 1. Random data (should pass most tests) -> Covers Proportion > 0
	// We need MinBits
	n := nist.MinBits / 8
	randomBits := make([]byte, n)
	// Simple pseudo-random generation
	state := uint64(12345)
	for i := range randomBits {
		state = state*6364136223846793005 + 1442695040888963407
		randomBits[i] = byte(state >> 56)
	}

	_, err := s.RunTestSuite(context.Background(), &pb.Sp80022TestRequest{Bitstream: randomBits})
	if err != nil {
		t.Fatalf("RunTestSuite with random data failed: %v", err)
	}

	// 2. All zeros (should fail and warn) -> Covers Warning != ""
	zeros := make([]byte, n)
	_, err = s.RunTestSuite(context.Background(), &pb.Sp80022TestRequest{Bitstream: zeros})
	if err != nil {
		t.Fatalf("RunTestSuite with zeros failed: %v", err)
	}
}

func TestRunTestSuiteMocked(t *testing.T) {
	orig := runAllTests
	defer func() { runAllTests = orig }()

	s := NewServer()

	runAllTests = func(bitstream []byte) ([]nist.TestResult, error) {
		return nil, fmt.Errorf("mock error")
	}
	validBits := make([]byte, nist.MinBits/8)
	_, err := s.RunTestSuite(context.Background(), &pb.Sp80022TestRequest{Bitstream: validBits})
	if err == nil {
		t.Error("expected error from mocked RunAllTests")
	}

	runAllTests = func(bitstream []byte) ([]nist.TestResult, error) {
		return []nist.TestResult{
			{Name: "SkippedTest", PValue: -1.0, Passed: false},
			{Name: "ValidTest", PValue: 0.5, Passed: true, Proportion: 1.0},
		}, nil
	}
	resp, err := s.RunTestSuite(context.Background(), &pb.Sp80022TestRequest{Bitstream: validBits})
	if err != nil {
		t.Fatalf("RunTestSuite failed: %v", err)
	}
	if len(resp.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(resp.Results))
	}
	if resp.Results[0].PValue >= 0 {
		t.Error("expected negative p-value for skipped test")
	}

	if resp.PValueUniformityChi2 != -1.0 {
		t.Errorf("expected -1.0 for uniformity chi2, got %f", resp.PValueUniformityChi2)
	}
}
