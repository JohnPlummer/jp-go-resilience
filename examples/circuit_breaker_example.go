package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"time"

	resilience "github.com/JohnPlummer/jp-go-resilience"
)

// HTTPClient simulates an HTTP client that can fail.
type HTTPClient struct {
	failureCount int
	maxFailures  int
}

// Execute simulates making an HTTP request.
func (c *HTTPClient) Execute(ctx context.Context, req string) (string, error) {
	// Simulate failures
	if c.failureCount < c.maxFailures {
		c.failureCount++
		return "", resilience.NewStatusCodeError(500, errors.New("server error"))
	}

	// After maxFailures, start succeeding
	return fmt.Sprintf("Response for: %s", req), nil
}

func main() {
	logger := slog.Default()

	// Create the underlying client
	httpClient := &HTTPClient{
		maxFailures: 3,
	}

	// Wrap with circuit breaker
	// The circuit will open after 60% failures with at least 3 requests
	wrapper := resilience.NewCircuitBreakerWrapper(
		httpClient,
		resilience.WithMaxRequests(2),
		resilience.WithTimeout(5*time.Second),
		resilience.WithInterval(10*time.Second),
		resilience.WithCircuitBreakerLogger(logger),
		resilience.WithStateChangeHandler(func(name string, from, to resilience.CircuitBreakerState) {
			log.Printf("Circuit breaker state changed: %s -> %s\n", from, to)
		}),
	)

	ctx := context.Background()

	// Demonstrate circuit breaker behavior
	fmt.Println("=== Circuit Breaker Example ===\n")

	// Make some requests
	for i := 1; i <= 10; i++ {
		fmt.Printf("Request %d: ", i)

		resp, err := wrapper.Execute(ctx, fmt.Sprintf("request-%d", i))
		if err != nil {
			fmt.Printf("Error: %v\n", err)
		} else {
			fmt.Printf("Success: %s\n", resp)
		}

		// Check circuit state and health
		state := wrapper.State()
		health := wrapper.GetHealth()
		counts := wrapper.Counts()

		fmt.Printf("  State: %s, Healthy: %t, Requests: %d, Failures: %d\n",
			state, health.Healthy, counts.Requests, counts.TotalFailures)

		// If circuit is open, wait for timeout to transition to half-open
		if state == resilience.StateOpen && i < 10 {
			fmt.Println("  Circuit is open, waiting for timeout...")
			time.Sleep(6 * time.Second)
		} else {
			time.Sleep(500 * time.Millisecond)
		}
	}

	// Final health check
	fmt.Println("\n=== Final Health Status ===")
	health := wrapper.GetHealth()
	fmt.Printf("Healthy: %t\n", health.Healthy)
	fmt.Printf("Status: %s\n", health.Status)
	fmt.Printf("State: %s\n", health.State)
	fmt.Printf("Total Requests: %d\n", health.Requests)
	fmt.Printf("Total Successes: %d\n", health.TotalSuccesses)
	fmt.Printf("Total Failures: %d\n", health.TotalFailures)
	fmt.Printf("Consecutive Successes: %d\n", health.ConsecutiveSuccesses)
	fmt.Printf("Consecutive Failures: %d\n", health.ConsecutiveFailures)
}
