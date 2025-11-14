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

// oldStyleRetry demonstrates the old manual retry approach (BEFORE).
func oldStyleRetry(client *http.Client, req *http.Request) (*http.Response, error) {
	fmt.Println("=== OLD APPROACH: Manual Retry Loop ===")

	maxAttempts := 3
	for attempt := 0; attempt < maxAttempts; attempt++ {
		fmt.Printf("Attempt %d/%d...\n", attempt+1, maxAttempts)

		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			if attempt < maxAttempts-1 {
				// Exponential backoff: 1s, 2s, 4s
				backoff := time.Second * time.Duration(1<<attempt)
				fmt.Printf("Waiting %v before retry...\n", backoff)
				time.Sleep(backoff)
				continue
			}
			return nil, err
		}

		// Check status code
		if resp.StatusCode >= 500 {
			fmt.Printf("Server error %d\n", resp.StatusCode)
			resp.Body.Close()
			if attempt < maxAttempts-1 {
				backoff := time.Second * time.Duration(1<<attempt)
				fmt.Printf("Waiting %v before retry...\n", backoff)
				time.Sleep(backoff)
				continue
			}
			return nil, fmt.Errorf("server error: %d", resp.StatusCode)
		}

		return resp, nil
	}

	return nil, fmt.Errorf("max retries exceeded")
}

// newStyleRetry demonstrates the new resilience wrapper approach (AFTER).
func newStyleRetry(ctx context.Context, resilientClient resilience.ResilientClient[*http.Request, *http.Response], req *http.Request) (*http.Response, error) {
	fmt.Println("=== NEW APPROACH: Resilience Wrapper ===")
	return resilientClient.Execute(ctx, req)
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	fmt.Println("=== OpenAI Migration Example ===")
	fmt.Println("Migrating from manual retry code to jp-go-resilience")
	fmt.Println()

	// Create HTTP request
	req, err := http.NewRequest("GET", "https://httpbin.org/status/200", nil)
	if err != nil {
		log.Fatal(err)
	}

	// BEFORE: Manual retry with exponential backoff
	fmt.Println("BEFORE: Manual retry implementation")
	fmt.Println("Problems:")
	fmt.Println("  - Manual retry loop in every API call")
	fmt.Println("  - No circuit breaker protection")
	fmt.Println("  - No jitter (thundering herd risk)")
	fmt.Println("  - No context cancellation support")
	fmt.Println("  - Hardcoded retry logic")
	fmt.Println("  - No statistics or observability")
	fmt.Println()

	oldClient := &http.Client{Timeout: 30 * time.Second}
	resp1, err := oldStyleRetry(oldClient, req)
	if err != nil {
		fmt.Printf("Old style failed: %v\n", err)
	} else {
		fmt.Printf("Old style succeeded: %d\n", resp1.StatusCode)
		resp1.Body.Close()
	}
	fmt.Println()

	// AFTER: Using jp-go-resilience
	fmt.Println("AFTER: Using jp-go-resilience")
	fmt.Println("Benefits:")
	fmt.Println("  - Declarative configuration")
	fmt.Println("  - Circuit breaker protection included")
	fmt.Println("  - Automatic jitter to prevent thundering herd")
	fmt.Println("  - Full context support")
	fmt.Println("  - Reusable across all API clients")
	fmt.Println("  - Built-in statistics and logging")
	fmt.Println("  - Standardized error classification")
	fmt.Println()

	// Create base client
	baseClient := &HTTPClient{
		client: &http.Client{Timeout: 30 * time.Second},
	}

	// Wrap with resilience (retry + circuit breaker)
	resilientClient := resilience.CombineRetryAndCircuitBreaker(
		baseClient,
		resilience.DefaultRetryConfig(),
		resilience.DefaultCircuitBreakerConfig(),
		logger,
	)

	ctx := context.Background()
	req2, err := http.NewRequest("GET", "https://httpbin.org/status/200", nil)
	if err != nil {
		log.Fatal(err)
	}

	resp2, err := newStyleRetry(ctx, resilientClient, req2)
	if err != nil {
		fmt.Printf("New style failed: %v\n", err)
	} else {
		fmt.Printf("New style succeeded: %d\n", resp2.StatusCode)
		resp2.Body.Close()
	}
	fmt.Println()

	// Show detailed comparison
	fmt.Println("=== Detailed Comparison ===")
	fmt.Println()

	fmt.Println("OLD CODE:")
	fmt.Println("```go")
	fmt.Println("client := &http.Client{Timeout: 30 * time.Second}")
	fmt.Println("for attempt := 0; attempt < 3; attempt++ {")
	fmt.Println("    resp, err := client.Do(req)")
	fmt.Println("    if err != nil {")
	fmt.Println("        if attempt < 2 {")
	fmt.Println("            time.Sleep(time.Second * time.Duration(1<<attempt))")
	fmt.Println("            continue")
	fmt.Println("        }")
	fmt.Println("        return nil, err")
	fmt.Println("    }")
	fmt.Println("    if resp.StatusCode >= 500 {")
	fmt.Println("        // ... more retry logic ...")
	fmt.Println("    }")
	fmt.Println("    return resp, nil")
	fmt.Println("}")
	fmt.Println("```")
	fmt.Println()

	fmt.Println("NEW CODE:")
	fmt.Println("```go")
	fmt.Println("type HTTPClient struct {")
	fmt.Println("    client *http.Client")
	fmt.Println("}")
	fmt.Println()
	fmt.Println("func (c *HTTPClient) Execute(ctx context.Context, req *http.Request) (*http.Response, error) {")
	fmt.Println("    return c.client.Do(req.WithContext(ctx))")
	fmt.Println("}")
	fmt.Println()
	fmt.Println("baseClient := &HTTPClient{client: &http.Client{Timeout: 30 * time.Second}}")
	fmt.Println("resilientClient := resilience.CombineRetryAndCircuitBreaker(")
	fmt.Println("    baseClient,")
	fmt.Println("    resilience.DefaultRetryConfig(),")
	fmt.Println("    resilience.DefaultCircuitBreakerConfig(),")
	fmt.Println("    slog.Default(),")
	fmt.Println(")")
	fmt.Println()
	fmt.Println("resp, err := resilientClient.Execute(ctx, req)")
	fmt.Println("```")
	fmt.Println()

	fmt.Println("=== Migration Steps ===")
	fmt.Println()
	fmt.Println("1. Create wrapper struct that implements ResilientClient:")
	fmt.Println("   - Add Execute(ctx, req) method")
	fmt.Println("   - Wrap your existing client")
	fmt.Println()
	fmt.Println("2. Replace manual retry loops with resilience wrapper:")
	fmt.Println("   - Use CombineRetryAndCircuitBreaker for full protection")
	fmt.Println("   - Or use NewRetryWrapper for retry-only")
	fmt.Println()
	fmt.Println("3. Configure retry and circuit breaker settings:")
	fmt.Println("   - Start with defaults")
	fmt.Println("   - Tune based on your service's characteristics")
	fmt.Println()
	fmt.Println("4. Remove old retry code:")
	fmt.Println("   - Delete manual retry loops")
	fmt.Println("   - Remove hardcoded backoff logic")
	fmt.Println("   - Clean up error handling")
	fmt.Println()
	fmt.Println("5. Test and monitor:")
	fmt.Println("   - Verify retry behavior in logs")
	fmt.Println("   - Check circuit breaker state during failures")
	fmt.Println("   - Monitor retry statistics")
	fmt.Println()

	fmt.Println("=== Key Improvements ===")
	fmt.Println()
	fmt.Println("Reliability:")
	fmt.Println("  - Circuit breaker prevents cascade failures")
	fmt.Println("  - Jitter prevents thundering herd")
	fmt.Println("  - Proper context cancellation")
	fmt.Println()
	fmt.Println("Maintainability:")
	fmt.Println("  - Centralized retry logic")
	fmt.Println("  - Reusable across all clients")
	fmt.Println("  - Easy to test and configure")
	fmt.Println()
	fmt.Println("Observability:")
	fmt.Println("  - Structured logging")
	fmt.Println("  - Retry statistics")
	fmt.Println("  - Circuit breaker health status")
	fmt.Println("  - State change callbacks")
}
