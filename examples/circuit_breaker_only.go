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

	fmt.Println("=== Circuit Breaker Only Example ===")
	fmt.Println("This example demonstrates circuit breaker protection.")
	fmt.Println()

	// Create base HTTP client
	baseClient := &HTTPClient{
		client: &http.Client{Timeout: 10 * time.Second},
	}

	// Track state changes
	stateChanges := []string{}

	// Wrap with circuit breaker
	resilientClient := resilience.NewCircuitBreakerWrapper(
		baseClient,
		resilience.WithMaxRequests(3),
		resilience.WithInterval(5*time.Second),
		resilience.WithTimeout(10*time.Second),
		resilience.WithReadyToTrip(func(counts resilience.CircuitBreakerCounts) bool {
			// Trip if we have 3+ requests with 60%+ failure rate
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			shouldTrip := counts.Requests >= 3 && failureRatio >= 0.6
			if shouldTrip {
				fmt.Printf("Circuit breaker tripping! Requests: %d, Failures: %d, Ratio: %.2f\n",
					counts.Requests, counts.TotalFailures, failureRatio)
			}
			return shouldTrip
		}),
		resilience.WithStateChangeHandler(func(name string, from, to resilience.CircuitBreakerState) {
			change := fmt.Sprintf("%s -> %s", from, to)
			stateChanges = append(stateChanges, change)
			fmt.Printf("Circuit breaker state changed: %s\n", change)
		}),
		resilience.WithCircuitBreakerLogger(logger),
	)

	ctx := context.Background()

	// Example 1: Successful requests in closed state
	fmt.Println("Example 1: Making successful requests (circuit should stay closed)...")
	for i := 0; i < 3; i++ {
		req, err := http.NewRequest("GET", "https://httpbin.org/status/200", nil)
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
	}
	fmt.Printf("Circuit state: %s\n", resilientClient.State())
	fmt.Println()

	// Example 2: Failing requests to trip the circuit
	fmt.Println("Example 2: Making failing requests (circuit should trip)...")
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
		time.Sleep(100 * time.Millisecond)
	}
	fmt.Printf("Circuit state: %s\n", resilientClient.State())
	fmt.Println()

	// Example 3: Requests fail fast when circuit is open
	fmt.Println("Example 3: Circuit is open, requests should fail immediately...")
	for i := 0; i < 2; i++ {
		req, err := http.NewRequest("GET", "https://httpbin.org/status/200", nil)
		if err != nil {
			log.Fatal(err)
		}

		start := time.Now()
		resp, err := resilientClient.Execute(ctx, req)
		duration := time.Since(start)

		if err != nil {
			fmt.Printf("Request %d failed fast (%v): %v\n", i+1, duration, err)
		} else {
			fmt.Printf("Request %d unexpectedly succeeded: %d\n", i+1, resp.StatusCode)
			resp.Body.Close()
		}
	}
	fmt.Println()

	// Show circuit breaker health
	health := resilientClient.GetHealth()
	fmt.Println("Circuit Breaker Health:")
	fmt.Printf("  Healthy: %v\n", health.Healthy)
	fmt.Printf("  Status: %s\n", health.Status)
	fmt.Printf("  State: %s\n", health.State)
	fmt.Printf("  Requests: %d\n", health.Requests)
	fmt.Printf("  Total Successes: %d\n", health.TotalSuccesses)
	fmt.Printf("  Total Failures: %d\n", health.TotalFailures)
	fmt.Printf("  Consecutive Failures: %d\n", health.ConsecutiveFailures)

	fmt.Println()
	fmt.Println("State Changes:")
	for _, change := range stateChanges {
		fmt.Printf("  - %s\n", change)
	}

	fmt.Println()
	fmt.Println("Key Points:")
	fmt.Println("- Circuit breaker protects downstream services from overload")
	fmt.Println("- Trips after threshold failures (default: 3 requests, 60% failure rate)")
	fmt.Println("- Open circuit fails requests immediately (fail fast)")
	fmt.Println("- After timeout, circuit enters half-open state to test recovery")
	fmt.Println("- Successful probes in half-open state close the circuit")
}
