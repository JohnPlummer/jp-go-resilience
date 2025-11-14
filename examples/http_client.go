package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"time"

	resilience "github.com/JohnPlummer/jp-go-resilience"
)

// HTTPClient wraps the standard http.Client to implement the ResilientClient interface.
// This demonstrates how to adapt any existing client to work with jp-go-resilience.
type HTTPClient struct {
	client *http.Client
}

// Execute implements resilience.ResilientClient[*http.Request, *http.Response].
// It executes the HTTP request using the underlying http.Client.
func (c *HTTPClient) Execute(ctx context.Context, req *http.Request) (*http.Response, error) {
	return c.client.Do(req.WithContext(ctx))
}

// NewHTTPClient creates a new HTTPClient with the specified timeout.
func NewHTTPClient(timeout time.Duration) *HTTPClient {
	return &HTTPClient{
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

func main() {
	// Setup structured logging
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	// Example 1: Basic HTTP client with retry
	fmt.Println("=== Example 1: Basic Retry ===")
	exampleBasicRetry(logger)

	fmt.Println("\n=== Example 2: Circuit Breaker ===")
	exampleCircuitBreaker(logger)

	fmt.Println("\n=== Example 3: Combined Retry + Circuit Breaker ===")
	exampleCombined(logger)

	fmt.Println("\n=== Example 4: Custom Error Classification ===")
	exampleCustomClassifier(logger)
}

// exampleBasicRetry demonstrates basic retry functionality with exponential backoff.
func exampleBasicRetry(logger *slog.Logger) {
	// Create base HTTP client
	baseClient := NewHTTPClient(10 * time.Second)

	// Wrap with retry logic - exponential backoff with jitter
	resilientClient := resilience.NewRetryWrapper(
		baseClient,
		resilience.WithMaxAttempts(3),
		resilience.WithExponentialBackoff(time.Second, 30*time.Second),
		resilience.WithRetryLogger(logger),
	)

	// Make request - this will retry on transient failures
	ctx := context.Background()
	req, err := http.NewRequest("GET", "https://httpbin.org/status/200", nil)
	if err != nil {
		log.Fatal(err)
	}

	resp, err := resilientClient.Execute(ctx, req)
	if err != nil {
		fmt.Printf("Request failed after retries: %v\n", err)
		return
	}
	defer resp.Body.Close()

	fmt.Printf("Success! Status: %d\n", resp.StatusCode)
}

// exampleCircuitBreaker demonstrates circuit breaker functionality.
func exampleCircuitBreaker(logger *slog.Logger) {
	// Create base HTTP client
	baseClient := NewHTTPClient(10 * time.Second)

	// Wrap with circuit breaker
	resilientClient := resilience.NewCircuitBreakerWrapper(
		baseClient,
		resilience.WithMaxRequests(5),
		resilience.WithInterval(10*time.Second),
		resilience.WithTimeout(60*time.Second),
		resilience.WithReadyToTrip(func(counts resilience.CircuitBreakerCounts) bool {
			// Trip if we have 5+ requests with 50%+ failure rate
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return counts.Requests >= 5 && failureRatio >= 0.5
		}),
		resilience.WithStateChangeHandler(func(name string, from, to resilience.CircuitBreakerState) {
			fmt.Printf("Circuit breaker state changed: %s -> %s\n", from, to)
		}),
		resilience.WithCircuitBreakerLogger(logger),
	)

	// Make successful request
	ctx := context.Background()
	req, err := http.NewRequest("GET", "https://httpbin.org/status/200", nil)
	if err != nil {
		log.Fatal(err)
	}

	resp, err := resilientClient.Execute(ctx, req)
	if err != nil {
		fmt.Printf("Request failed: %v\n", err)
		return
	}
	defer resp.Body.Close()

	fmt.Printf("Success! Status: %d\n", resp.StatusCode)
	fmt.Printf("Circuit state: %s\n", resilientClient.State())
}

// exampleCombined demonstrates combining retry and circuit breaker.
// Circuit breaker is applied first (inner layer) to protect the service,
// then retry logic (outer layer) handles transient failures.
func exampleCombined(logger *slog.Logger) {
	// Create base HTTP client
	baseClient := NewHTTPClient(10 * time.Second)

	// Combine retry and circuit breaker
	resilientClient := resilience.CombineRetryAndCircuitBreaker(
		baseClient,
		resilience.DefaultRetryConfig(),
		resilience.DefaultCircuitBreakerConfig(),
		logger,
	)

	// Make request with both retry and circuit breaker protection
	ctx := context.Background()
	req, err := http.NewRequest("GET", "https://httpbin.org/status/200", nil)
	if err != nil {
		log.Fatal(err)
	}

	resp, err := resilientClient.Execute(ctx, req)
	if err != nil {
		fmt.Printf("Request failed: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("Success! Status: %d, Body: %s\n", resp.StatusCode, body)
}

// CustomErrorClassifier demonstrates custom error classification.
// This classifier considers any 4xx error as non-retryable.
type CustomErrorClassifier struct {
	baseClassifier *resilience.HTTPStatusClassifier
}

// IsRetryable implements resilience.ErrorClassifier.
func (c *CustomErrorClassifier) IsRetryable(err error) bool {
	// Use base classifier first
	if !c.baseClassifier.IsRetryable(err) {
		return false
	}

	// Add custom logic here if needed
	return true
}

// ShouldTripCircuit implements resilience.CircuitBreakerErrorClassifier.
func (c *CustomErrorClassifier) ShouldTripCircuit(err error) bool {
	// Use base classifier
	return c.baseClassifier.ShouldTripCircuit(err)
}

// exampleCustomClassifier demonstrates using a custom error classifier.
func exampleCustomClassifier(logger *slog.Logger) {
	// Create base HTTP client
	baseClient := NewHTTPClient(10 * time.Second)

	// Create custom classifier
	customClassifier := &CustomErrorClassifier{
		baseClassifier: resilience.NewHTTPStatusClassifier(),
	}

	// Wrap with retry using custom classifier
	resilientClient := resilience.NewRetryWrapper(
		baseClient,
		resilience.WithMaxAttempts(3),
		resilience.WithExponentialBackoff(time.Second, 10*time.Second),
		resilience.WithErrorClassifier(customClassifier),
		resilience.WithRetryLogger(logger),
	)

	// Make request
	ctx := context.Background()
	req, err := http.NewRequest("GET", "https://httpbin.org/delay/1", nil)
	if err != nil {
		log.Fatal(err)
	}

	resp, err := resilientClient.Execute(ctx, req)
	if err != nil {
		fmt.Printf("Request failed: %v\n", err)
		return
	}
	defer resp.Body.Close()

	fmt.Printf("Success with custom classifier! Status: %d\n", resp.StatusCode)
}
