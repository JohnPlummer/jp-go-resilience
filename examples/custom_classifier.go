package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strings"
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

// CustomErrorClassifier demonstrates custom error classification logic.
// This classifier treats specific errors differently from the default behavior.
type CustomErrorClassifier struct {
	baseClassifier *resilience.HTTPStatusClassifier
}

// IsRetryable implements resilience.ErrorClassifier.
// Custom logic: Also retry on network errors containing "connection refused"
func (c *CustomErrorClassifier) IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	// Use base classifier first
	if c.baseClassifier.IsRetryable(err) {
		return true
	}

	// Add custom logic: Retry on connection refused errors
	if strings.Contains(err.Error(), "connection refused") {
		fmt.Println("Custom classifier: Retrying connection refused error")
		return true
	}

	// Add custom logic: Retry on DNS errors
	if strings.Contains(err.Error(), "no such host") {
		fmt.Println("Custom classifier: Retrying DNS resolution error")
		return true
	}

	return false
}

// ShouldTripCircuit implements resilience.CircuitBreakerErrorClassifier.
// Custom logic: Don't trip on 401/403, these are likely client errors
func (c *CustomErrorClassifier) ShouldTripCircuit(err error) bool {
	if err == nil {
		return false
	}

	// Check if it's a 401 or 403 - don't trip on auth errors
	var httpErr resilience.HTTPError
	if errors.As(err, &httpErr) {
		statusCode := httpErr.StatusCode()
		if statusCode == 401 || statusCode == 403 {
			fmt.Printf("Custom classifier: Not tripping circuit on %d (auth error)\n", statusCode)
			return false
		}
	}

	// For other errors, use base classifier
	return c.baseClassifier.ShouldTripCircuit(err)
}

// StrictErrorClassifier is even more conservative about what to retry.
type StrictErrorClassifier struct{}

// IsRetryable only retries on 503 and 429.
func (c *StrictErrorClassifier) IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	var httpErr resilience.HTTPError
	if errors.As(err, &httpErr) {
		statusCode := httpErr.StatusCode()
		// Only retry on service unavailable and rate limit
		if statusCode == 503 || statusCode == 429 {
			fmt.Printf("Strict classifier: Retrying %d\n", statusCode)
			return true
		}
		fmt.Printf("Strict classifier: Not retrying %d\n", statusCode)
		return false
	}

	// Don't retry unknown errors
	return false
}

// ShouldTripCircuit trips on any 5xx error.
func (c *StrictErrorClassifier) ShouldTripCircuit(err error) bool {
	if err == nil {
		return false
	}

	var httpErr resilience.HTTPError
	if errors.As(err, &httpErr) {
		statusCode := httpErr.StatusCode()
		if statusCode >= 500 {
			fmt.Printf("Strict classifier: Tripping circuit on %d\n", statusCode)
			return true
		}
	}

	return false
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	fmt.Println("=== Custom Error Classifier Example ===")
	fmt.Println("This example demonstrates custom error classification.")
	fmt.Println()

	// Create base HTTP client
	baseClient := &HTTPClient{
		client: &http.Client{Timeout: 10 * time.Second},
	}

	ctx := context.Background()

	// Example 1: Custom classifier that extends default behavior
	fmt.Println("Example 1: Custom classifier (extends default)...")
	customClassifier := &CustomErrorClassifier{
		baseClassifier: resilience.NewHTTPStatusClassifier(),
	}

	resilientClient1 := resilience.NewRetryWrapper(
		baseClient,
		resilience.WithMaxAttempts(3),
		resilience.WithConstantBackoff(200*time.Millisecond),
		resilience.WithErrorClassifier(customClassifier),
		resilience.WithRetryLogger(logger),
	)

	// Test with 500 error (should retry via base classifier)
	req1, err := http.NewRequest("GET", "https://httpbin.org/status/500", nil)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Testing 500 error (should retry via default behavior)...")
	resp1, err := resilientClient1.Execute(ctx, req1)
	if err != nil {
		fmt.Printf("Failed after retries: %v\n", err)
	} else {
		fmt.Printf("Success: %d\n", resp1.StatusCode)
		resp1.Body.Close()
	}
	fmt.Println()

	// Example 2: Strict classifier that only retries specific errors
	fmt.Println("Example 2: Strict classifier (very conservative)...")
	strictClassifier := &StrictErrorClassifier{}

	resilientClient2 := resilience.NewRetryWrapper(
		baseClient,
		resilience.WithMaxAttempts(3),
		resilience.WithConstantBackoff(200*time.Millisecond),
		resilience.WithErrorClassifier(strictClassifier),
		resilience.WithRetryLogger(logger),
	)

	// Test with 500 error (should NOT retry with strict classifier)
	req2, err := http.NewRequest("GET", "https://httpbin.org/status/500", nil)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Testing 500 error with strict classifier (should NOT retry)...")
	resp2, err := resilientClient2.Execute(ctx, req2)
	if err != nil {
		fmt.Printf("Failed (no retries): %v\n", err)
	} else {
		fmt.Printf("Success: %d\n", resp2.StatusCode)
		resp2.Body.Close()
	}
	fmt.Println()

	// Test with 503 error (should retry with strict classifier)
	req3, err := http.NewRequest("GET", "https://httpbin.org/status/503", nil)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Testing 503 error with strict classifier (SHOULD retry)...")
	resp3, err := resilientClient2.Execute(ctx, req3)
	if err != nil {
		fmt.Printf("Failed after retries: %v\n", err)
	} else {
		fmt.Printf("Success: %d\n", resp3.StatusCode)
		resp3.Body.Close()
	}
	fmt.Println()

	// Example 3: Custom classifier with circuit breaker
	fmt.Println("Example 3: Custom classifier with circuit breaker...")
	cbClient := resilience.NewCircuitBreakerWrapper(
		baseClient,
		resilience.WithMaxRequests(2),
		resilience.WithTimeout(5*time.Second),
		resilience.WithCircuitBreakerErrorClassifier(customClassifier),
		resilience.WithCircuitBreakerLogger(logger),
	)

	// Test with 401 (custom classifier won't trip circuit)
	req4, err := http.NewRequest("GET", "https://httpbin.org/status/401", nil)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Testing 401 error (custom classifier won't trip circuit)...")
	resp4, err := cbClient.Execute(ctx, req4)
	if err != nil {
		fmt.Printf("Failed: %v\n", err)
	} else {
		fmt.Printf("Success: %d\n", resp4.StatusCode)
		resp4.Body.Close()
	}
	fmt.Printf("Circuit state: %s\n", cbClient.State())
	fmt.Println()

	fmt.Println("Use Cases for Custom Classifiers:")
	fmt.Println("  1. Domain-specific error handling (e.g., custom error codes)")
	fmt.Println("  2. Different retry logic for internal vs external services")
	fmt.Println("  3. Fine-grained control over circuit breaker trip conditions")
	fmt.Println("  4. Integration with custom error types")
	fmt.Println("  5. Business logic requirements (e.g., never retry payment failures)")
	fmt.Println()
	fmt.Println("Key Points:")
	fmt.Println("- Implement ErrorClassifier for retry logic")
	fmt.Println("- Implement CircuitBreakerErrorClassifier for circuit breaker logic")
	fmt.Println("- Can extend default behavior or create completely custom logic")
	fmt.Println("- Same classifier can implement both interfaces")
}
