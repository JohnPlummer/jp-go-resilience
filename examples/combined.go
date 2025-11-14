package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"time"

	resilience "github.com/JohnPlummer/jp-go-resilience"
)

// HTTPClient wraps the standard http.Client to implement the ResilientClient interface.
type HTTPClient struct {
	client *http.Client
}

// Execute implements resilience.ResilientClient[*http.Request, *http.Response].
func (c *HTTPClient) Execute(ctx context.Context, req *http.Request) (*http.Response, error) {
	return c.client.Do(req.WithContext(ctx))
}

func main() {
	// Setup structured logging
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	fmt.Println("=== Combined Retry and Circuit Breaker Example ===")
	fmt.Println("This example demonstrates using both patterns together.")
	fmt.Println()

	// Create base HTTP client
	baseClient := &HTTPClient{
		client: &http.Client{Timeout: 10 * time.Second},
	}

	// Configure retry
	retryConfig := &resilience.RetryConfig{
		MaxAttempts:  3,
		Strategy:     resilience.RetryStrategyExponential,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     2 * time.Second,
		Logger:       logger,
	}

	// Configure circuit breaker
	cbConfig := &resilience.CircuitBreakerConfig{
		MaxRequests: 3,
		Interval:    5 * time.Second,
		Timeout:     10 * time.Second,
		ReadyToTrip: func(counts resilience.CircuitBreakerCounts) bool {
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			shouldTrip := counts.Requests >= 3 && failureRatio >= 0.6
			if shouldTrip {
				fmt.Printf("Circuit tripping after %d requests with %.0f%% failure rate\n",
					counts.Requests, failureRatio*100)
			}
			return shouldTrip
		},
		OnStateChange: func(name string, from, to resilience.CircuitBreakerState) {
			fmt.Printf("Circuit state: %s -> %s\n", from, to)
		},
		Logger: logger,
	}

	// Combine retry and circuit breaker
	resilientClient := resilience.CombineRetryAndCircuitBreaker(
		baseClient,
		retryConfig,
		cbConfig,
		logger,
	)

	ctx := context.Background()

	// Example 1: Successful request (no retry needed, circuit stays closed)
	fmt.Println("Example 1: Successful request...")
	req1, err := http.NewRequest("GET", "https://httpbin.org/status/200", nil)
	if err != nil {
		log.Fatal(err)
	}

	resp1, err := resilientClient.Execute(ctx, req1)
	if err != nil {
		fmt.Printf("Failed: %v\n", err)
	} else {
		fmt.Printf("Success! Status: %d\n", resp1.StatusCode)
		resp1.Body.Close()
	}
	fmt.Println()

	// Example 2: Transient failures (retries succeed)
	fmt.Println("Example 2: Simulating transient failures...")
	fmt.Println("Note: httpbin 503 errors will be retried, but all attempts will fail")
	req2, err := http.NewRequest("GET", "https://httpbin.org/status/503", nil)
	if err != nil {
		log.Fatal(err)
	}

	resp2, err := resilientClient.Execute(ctx, req2)
	if err != nil {
		fmt.Printf("Failed after retries (expected): %v\n", err)
	} else {
		fmt.Printf("Success: %d\n", resp2.StatusCode)
		resp2.Body.Close()
	}
	fmt.Println()

	// Example 3: Multiple failures trip the circuit
	fmt.Println("Example 3: Making multiple failing requests to trip circuit...")
	for i := 0; i < 3; i++ {
		req, err := http.NewRequest("GET", "https://httpbin.org/status/500", nil)
		if err != nil {
			log.Fatal(err)
		}

		resp, err := resilientClient.Execute(ctx, req)
		if err != nil {
			fmt.Printf("Request %d failed: %v\n", i+1, err)
		} else {
			fmt.Printf("Request %d succeeded: %d\n", i+1, resp.StatusCode)
			resp.Body.Close()
		}
		time.Sleep(200 * time.Millisecond)
	}
	fmt.Println()

	// Example 4: Circuit is open, requests fail fast
	fmt.Println("Example 4: Circuit is open, requests should fail immediately...")
	req4, err := http.NewRequest("GET", "https://httpbin.org/status/200", nil)
	if err != nil {
		log.Fatal(err)
	}

	start := time.Now()
	resp4, err := resilientClient.Execute(ctx, req4)
	duration := time.Since(start)

	if err != nil {
		fmt.Printf("Failed fast (%v): %v\n", duration, err)
	} else {
		fmt.Printf("Unexpected success: %d\n", resp4.StatusCode)
		resp4.Body.Close()
	}
	fmt.Println()

	fmt.Println("Architecture:")
	fmt.Println("  Request -> Retry Wrapper -> Circuit Breaker -> Base Client -> Service")
	fmt.Println()
	fmt.Println("Layering benefits:")
	fmt.Println("  1. Circuit breaker (inner) protects downstream service")
	fmt.Println("  2. Retry (outer) handles transient failures")
	fmt.Println("  3. Retry respects circuit state (won't retry when open)")
	fmt.Println("  4. Failed retries count toward circuit breaker trip logic")
	fmt.Println()
	fmt.Println("Key Points:")
	fmt.Println("- Combined pattern provides comprehensive resilience")
	fmt.Println("- Use this for production HTTP/gRPC clients")
	fmt.Println("- Circuit breaker prevents cascade failures")
	fmt.Println("- Retry handles intermittent network issues")
}
