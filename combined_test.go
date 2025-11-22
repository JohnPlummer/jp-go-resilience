package resilience_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sony/gobreaker/v2"

	resilience "github.com/JohnPlummer/jp-go-resilience"
)

var _ = Describe("CombineRetryAndCircuitBreaker", func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc
		client *mockClient
		logger *slog.Logger
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
		client = &mockClient{}
		logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelError,
		}))
	})

	AfterEach(func() {
		cancel()
	})

	Describe("Layering", func() {
		It("wraps circuit breaker with retry (CB inner, retry outer)", func() {
			combined := resilience.CombineRetryAndCircuitBreaker(
				client,
				resilience.DefaultRetryConfig(),
				resilience.DefaultCircuitBreakerConfig(),
				logger,
			)
			Expect(combined).NotTo(BeNil())

			// Combined should be a RetryWrapper
			_, ok := combined.(*resilience.RetryWrapper[string, string])
			Expect(ok).To(BeTrue(), "Combined wrapper should be a RetryWrapper (outer layer)")
		})

		It("returns correct type", func() {
			combined := resilience.CombineRetryAndCircuitBreaker(
				client,
				resilience.DefaultRetryConfig(),
				resilience.DefaultCircuitBreakerConfig(),
				logger,
			)

			// Should implement ResilientClient (compile-time interface check)
			var _ resilience.ResilientClient[string, string] = combined //nolint:staticcheck // intentional interface verification
		})
	})

	Describe("Transient error handling", func() {
		It("retries on transient errors", func() {
			attempts := atomic.Int32{}
			client.executeFunc = func(ctx context.Context, req string) (string, error) {
				count := attempts.Add(1)
				if count < 3 {
					// First two attempts fail with retryable error
					return "", resilience.NewStatusCodeError(503, errors.New("service unavailable"))
				}
				return "success", nil
			}

			retryConfig := resilience.DefaultRetryConfig()
			retryConfig.MaxAttempts = 5
			retryConfig.InitialDelay = 10 * time.Millisecond
			retryConfig.MaxDelay = 50 * time.Millisecond

			combined := resilience.CombineRetryAndCircuitBreaker(
				client,
				retryConfig,
				resilience.DefaultCircuitBreakerConfig(),
				logger,
			)

			resp, err := combined.Execute(ctx, "test")
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).To(Equal("success"))
			Expect(attempts.Load()).To(Equal(int32(3)))
		})
	})

	Describe("Circuit breaker protection", func() {
		It("trips circuit after threshold failures", func() {
			// Circuit breaker config that trips quickly
			cbConfig := &resilience.CircuitBreakerConfig{
				MaxRequests: 1,
				Interval:    100 * time.Millisecond,
				Timeout:     200 * time.Millisecond,
				ReadyToTrip: func(counts resilience.CircuitBreakerCounts) bool {
					// Trip after 3 failures
					return counts.TotalFailures >= 3
				},
				Logger: logger,
			}

			retryConfig := &resilience.RetryConfig{
				MaxAttempts:  1, // No retries, just let circuit breaker handle it
				InitialDelay: 10 * time.Millisecond,
				Logger:       logger,
			}

			client.executeFunc = func(ctx context.Context, req string) (string, error) {
				// Always fail with error that should trip circuit
				return "", resilience.NewStatusCodeError(500, errors.New("server error"))
			}

			combined := resilience.CombineRetryAndCircuitBreaker(
				client,
				retryConfig,
				cbConfig,
				logger,
			)

			// Make 3 requests to trip the circuit
			for i := 0; i < 3; i++ {
				_, err := combined.Execute(ctx, fmt.Sprintf("request-%d", i))
				Expect(err).To(HaveOccurred())
			}

			// Next request should fail immediately with circuit open error
			_, err := combined.Execute(ctx, "should-fail-fast")
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, gobreaker.ErrOpenState)).To(BeTrue())
		})
	})

	Describe("Retry respects open circuit", func() {
		It("does not retry when circuit is open", func() {
			attempts := atomic.Int32{}

			// Circuit breaker that trips immediately
			cbConfig := &resilience.CircuitBreakerConfig{
				MaxRequests: 1,
				Interval:    100 * time.Millisecond,
				Timeout:     500 * time.Millisecond,
				ReadyToTrip: func(counts resilience.CircuitBreakerCounts) bool {
					return counts.TotalFailures >= 2
				},
				Logger: logger,
			}

			retryConfig := &resilience.RetryConfig{
				MaxAttempts:  3,
				InitialDelay: 10 * time.Millisecond,
				MaxDelay:     50 * time.Millisecond,
				Logger:       logger,
			}

			client.executeFunc = func(ctx context.Context, req string) (string, error) {
				attempts.Add(1)
				return "", resilience.NewStatusCodeError(500, errors.New("server error"))
			}

			combined := resilience.CombineRetryAndCircuitBreaker(
				client,
				retryConfig,
				cbConfig,
				logger,
			)

			// First request - will fail and contribute to circuit breaker
			_, err := combined.Execute(ctx, "request-1")
			Expect(err).To(HaveOccurred())

			// Second request - will fail and trip the circuit
			_, err = combined.Execute(ctx, "request-2")
			Expect(err).To(HaveOccurred())

			// Reset attempts counter
			attempts.Store(0)

			// Third request - circuit is open, should fail immediately without retries
			_, err = combined.Execute(ctx, "request-3")
			Expect(err).To(HaveOccurred())
			Expect(errors.Is(err, gobreaker.ErrOpenState)).To(BeTrue())

			// Circuit is open, so no actual client call should have been made
			// The retry wrapper will attempt once, see circuit is open, and not retry
			Expect(attempts.Load()).To(Equal(int32(0)))
		})
	})

	Describe("Half-open state", func() {
		It("allows probes in half-open state", func() {
			attempts := atomic.Int32{}
			successAfter := atomic.Int32{}
			successAfter.Store(5) // Succeed after 5 attempts

			// Circuit breaker config
			cbConfig := &resilience.CircuitBreakerConfig{
				MaxRequests: 2, // Allow 2 probes in half-open
				Interval:    50 * time.Millisecond,
				Timeout:     100 * time.Millisecond, // Short timeout to reach half-open quickly
				ReadyToTrip: func(counts resilience.CircuitBreakerCounts) bool {
					return counts.TotalFailures >= 2
				},
				Logger: logger,
			}

			retryConfig := &resilience.RetryConfig{
				MaxAttempts:  1, // No retries
				InitialDelay: 10 * time.Millisecond,
				Logger:       logger,
			}

			client.executeFunc = func(ctx context.Context, req string) (string, error) {
				count := attempts.Add(1)
				if count <= successAfter.Load() {
					return "", resilience.NewStatusCodeError(500, errors.New("server error"))
				}
				return "success", nil
			}

			combined := resilience.CombineRetryAndCircuitBreaker(
				client,
				retryConfig,
				cbConfig,
				logger,
			)

			// Trip the circuit with 2 failures
			_, _ = combined.Execute(ctx, "fail-1")
			_, _ = combined.Execute(ctx, "fail-2")

			// Wait for circuit to move to half-open
			time.Sleep(150 * time.Millisecond)

			// Now service is healthy, probe should succeed
			successAfter.Store(2) // Already made 2 failed attempts
			resp, err := combined.Execute(ctx, "probe")
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).To(Equal("success"))
		})
	})

	Describe("Configuration propagation", func() {
		It("uses provided retry config", func() {
			retryConfig := &resilience.RetryConfig{
				MaxAttempts:  10,
				InitialDelay: 5 * time.Millisecond,
				MaxDelay:     100 * time.Millisecond,
				Logger:       logger,
			}

			combined := resilience.CombineRetryAndCircuitBreaker(
				client,
				retryConfig,
				resilience.DefaultCircuitBreakerConfig(),
				logger,
			)

			Expect(combined).NotTo(BeNil())
		})

		It("uses provided circuit breaker config", func() {
			cbConfig := &resilience.CircuitBreakerConfig{
				MaxRequests: 10,
				Interval:    5 * time.Second,
				Timeout:     60 * time.Second,
				Logger:      logger,
			}

			combined := resilience.CombineRetryAndCircuitBreaker(
				client,
				resilience.DefaultRetryConfig(),
				cbConfig,
				logger,
			)

			Expect(combined).NotTo(BeNil())
		})

		It("sets logger on both configs", func() {
			customLogger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

			combined := resilience.CombineRetryAndCircuitBreaker(
				client,
				resilience.DefaultRetryConfig(),
				resilience.DefaultCircuitBreakerConfig(),
				customLogger,
			)

			Expect(combined).NotTo(BeNil())
		})
	})
})

// Example_combineRetryAndCircuitBreaker demonstrates using both retry and circuit breaker together.
func Example_combineRetryAndCircuitBreaker() {
	// Create a mock client
	client := &mockClient{
		executeFunc: func(ctx context.Context, req string) (string, error) {
			return "success", nil
		},
	}

	// Combine retry and circuit breaker with default configs
	combined := resilience.CombineRetryAndCircuitBreaker(
		client,
		resilience.DefaultRetryConfig(),
		resilience.DefaultCircuitBreakerConfig(),
		slog.Default(),
	)

	// Execute request with both retry and circuit breaker protection
	ctx := context.Background()
	resp, err := combined.Execute(ctx, "test request")
	if err != nil {
		fmt.Printf("Request failed: %v\n", err)
		return
	}

	fmt.Printf("Response: %s\n", resp)
	// Output: Response: success
}

// Example_customConfiguration demonstrates custom retry and circuit breaker configuration.
func Example_customConfiguration() {
	client := &mockClient{
		executeFunc: func(ctx context.Context, req string) (string, error) {
			return "success", nil
		},
	}

	// Custom retry configuration
	retryConfig := &resilience.RetryConfig{
		MaxAttempts:  5,
		Strategy:     resilience.RetryStrategyExponential,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     5 * time.Second,
	}

	// Custom circuit breaker configuration
	cbConfig := &resilience.CircuitBreakerConfig{
		MaxRequests: 5,
		Interval:    10 * time.Second,
		Timeout:     60 * time.Second,
		ReadyToTrip: func(counts resilience.CircuitBreakerCounts) bool {
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return counts.Requests >= 10 && failureRatio >= 0.5
		},
	}

	combined := resilience.CombineRetryAndCircuitBreaker(
		client,
		retryConfig,
		cbConfig,
		slog.Default(),
	)

	ctx := context.Background()
	resp, err := combined.Execute(ctx, "test")
	if err != nil {
		fmt.Printf("Failed: %v\n", err)
		return
	}

	fmt.Printf("Success: %s\n", resp)
	// Output: Success: success
}
