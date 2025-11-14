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

	fmt.Println("=== Retry Only Example ===")
	fmt.Println("This example demonstrates retry logic with exponential backoff.")
	fmt.Println()

	// Create base HTTP client
	baseClient := &HTTPClient{
		client: &http.Client{Timeout: 10 * time.Second},
	}

	// Wrap with retry logic - exponential backoff with jitter
	resilientClient := resilience.NewRetryWrapper(
		baseClient,
		resilience.WithMaxAttempts(3),
		resilience.WithExponentialBackoff(time.Second, 30*time.Second),
		resilience.WithRetryLogger(logger),
	)

	// Example 1: Successful request
	fmt.Println("Example 1: Making successful request...")
	ctx := context.Background()
	req, err := http.NewRequest("GET", "https://httpbin.org/status/200", nil)
	if err != nil {
		log.Fatal(err)
	}

	resp, err := resilientClient.Execute(ctx, req)
	if err != nil {
		fmt.Printf("Request failed: %v\n", err)
	} else {
		fmt.Printf("Success! Status: %d\n", resp.StatusCode)
		resp.Body.Close()
	}
	fmt.Println()

	// Example 2: Request that needs retry (500 error)
	fmt.Println("Example 2: Making request that returns 500 (will retry)...")
	req2, err := http.NewRequest("GET", "https://httpbin.org/status/500", nil)
	if err != nil {
		log.Fatal(err)
	}

	resp2, err := resilientClient.Execute(ctx, req2)
	if err != nil {
		fmt.Printf("Request failed after retries (expected): %v\n", err)
	} else {
		fmt.Printf("Unexpected success: %d\n", resp2.StatusCode)
		resp2.Body.Close()
	}
	fmt.Println()

	// Example 3: Request with timeout (demonstrates context support)
	fmt.Println("Example 3: Making request with short timeout...")
	ctxTimeout, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	req3, err := http.NewRequest("GET", "https://httpbin.org/delay/2", nil)
	if err != nil {
		log.Fatal(err)
	}

	resp3, err := resilientClient.Execute(ctxTimeout, req3)
	if err != nil {
		fmt.Printf("Request timed out (expected): %v\n", err)
	} else {
		fmt.Printf("Unexpected success: %d\n", resp3.StatusCode)
		resp3.Body.Close()
	}
	fmt.Println()

	// Show retry statistics
	stats := resilientClient.GetRetryStats()
	fmt.Println("Retry Statistics:")
	fmt.Printf("  Total Attempts: %d\n", stats.TotalAttempts)
	fmt.Printf("  Total Retries: %d\n", stats.TotalRetries)
	fmt.Printf("  Total Successes: %d\n", stats.TotalSuccesses)
	fmt.Printf("  Total Failures: %d\n", stats.TotalFailures)

	fmt.Println()
	fmt.Println("Key Points:")
	fmt.Println("- Retry wrapper automatically retries on 5xx errors and network failures")
	fmt.Println("- Exponential backoff with jitter prevents thundering herd")
	fmt.Println("- Context cancellation/timeout stops retries immediately")
	fmt.Println("- Statistics help monitor retry behavior")
}
