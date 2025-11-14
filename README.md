# jp-go-resilience

Generic retry and circuit breaker patterns for building resilient Go clients using generics.

## Overview

`jp-go-resilience` provides a generic, reusable implementation of common resilience patterns for Go applications. Using Go 1.18+ generics, it works with any request/response types, making it suitable for HTTP clients, gRPC clients, database clients, or any other operation that needs fault tolerance.

## Features

- **Generic interfaces**: Works with any request/response types using Go generics
- **Retry patterns**: Configurable retry with exponential, constant, or fibonacci backoff
- **Circuit breaker**: Protects downstream services from cascading failures
- **Combined wrapper**: Easy composition of retry and circuit breaker
- **Error classification**: Flexible error classification for retry and circuit breaker decisions
- **HTTP support**: Built-in HTTP status code classification
- **Integration**: Works seamlessly with `jp-go-errors` package
- **Functional options**: Clean, extensible configuration API
- **Context-aware**: Full support for Go context cancellation and timeouts

## Installation

```bash
go get github.com/JohnPlummer/jp-go-resilience
```

## Quick Start

```go
package main

import (
    "context"
    "log/slog"
    "net/http"
    "time"

    "github.com/JohnPlummer/jp-go-resilience"
)

// HTTPClient wraps http.Client to implement ResilientClient interface
type HTTPClient struct {
    client *http.Client
}

func (c *HTTPClient) Execute(ctx context.Context, req *http.Request) (*http.Response, error) {
    return c.client.Do(req.WithContext(ctx))
}

func main() {
    // Create base client
    baseClient := &HTTPClient{
        client: &http.Client{Timeout: 10 * time.Second},
    }

    // Wrap with both retry and circuit breaker
    resilientClient := resilience.CombineRetryAndCircuitBreaker(
        baseClient,
        resilience.DefaultRetryConfig(),
        resilience.DefaultCircuitBreakerConfig(),
        slog.Default(),
    )

    // Use the client
    req, _ := http.NewRequest("GET", "https://api.example.com/data", nil)
    resp, err := resilientClient.Execute(context.Background(), req)
    if err != nil {
        panic(err)
    }
    defer resp.Body.Close()
}
```

## Usage Patterns

### Retry Only

Use retry when you need to handle transient failures without circuit breaker protection.

```go
resilientClient := resilience.NewRetryWrapper(
    baseClient,
    resilience.WithMaxAttempts(3),
    resilience.WithExponentialBackoff(time.Second, 30*time.Second),
)
```

**When to use:**

- External API calls with occasional transient failures
- Network requests that may timeout or fail intermittently
- Operations where the service is generally stable

**See:** [examples/retry_only.go](./examples/retry_only.go)

### Circuit Breaker Only

Use circuit breaker when you want to fail fast and protect downstream services without retries.

```go
resilientClient := resilience.NewCircuitBreakerWrapper(
    baseClient,
    resilience.WithMaxRequests(5),
    resilience.WithTimeout(60*time.Second),
    resilience.WithReadyToTrip(func(counts resilience.CircuitBreakerCounts) bool {
        failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
        return counts.Requests >= 5 && failureRatio >= 0.5
    }),
)
```

**When to use:**

- Protecting a failing downstream service from overload
- Preventing cascading failures in microservices
- Operations where retrying would make things worse

**See:** [examples/circuit_breaker_only.go](./examples/circuit_breaker_only.go)

### Combined (Recommended)

Use both retry and circuit breaker for comprehensive resilience. The circuit breaker protects the downstream service (inner layer), while retry handles transient failures (outer layer).

```go
resilientClient := resilience.CombineRetryAndCircuitBreaker(
    baseClient,
    resilience.DefaultRetryConfig(),
    resilience.DefaultCircuitBreakerConfig(),
    slog.Default(),
)
```

**When to use:**

- Production HTTP/gRPC clients
- Database connection pools
- Any critical external dependency

**Layering:** Circuit breaker wraps the base client (inner), retry wraps the circuit breaker (outer). This ensures:

- Circuit breaker state is accurately maintained
- Retries respect circuit breaker state (won't retry when circuit is open)
- Failed retries contribute to circuit breaker trip logic

**See:** [examples/combined.go](./examples/combined.go)

### Custom Error Classification

Implement custom error classification to control which errors trigger retries or trip the circuit breaker.

```go
type MyClassifier struct{}

func (c *MyClassifier) IsRetryable(err error) bool {
    // Custom retry logic
    return errors.Is(err, MyTransientError)
}

func (c *MyClassifier) ShouldTripCircuit(err error) bool {
    // Custom circuit breaker logic
    return errors.Is(err, MySevereError)
}

resilientClient := resilience.NewRetryWrapper(
    baseClient,
    resilience.WithErrorClassifier(&MyClassifier{}),
)
```

**See:** [examples/custom_classifier.go](./examples/custom_classifier.go)

## Migration Guide

### From OpenAI Client Code

**Before:**

```go
// Old OpenAI-specific retry code
client := &http.Client{Timeout: 30 * time.Second}

for attempt := 0; attempt < 3; attempt++ {
    resp, err := client.Do(req)
    if err != nil {
        if attempt < 2 {
            time.Sleep(time.Second * time.Duration(1<<attempt))
            continue
        }
        return nil, err
    }
    return resp, nil
}
```

**After:**

```go
// Generic resilience wrapper
type HTTPClient struct {
    client *http.Client
}

func (c *HTTPClient) Execute(ctx context.Context, req *http.Request) (*http.Response, error) {
    return c.client.Do(req.WithContext(ctx))
}

baseClient := &HTTPClient{
    client: &http.Client{Timeout: 30 * time.Second},
}

resilientClient := resilience.CombineRetryAndCircuitBreaker(
    baseClient,
    resilience.DefaultRetryConfig(),
    resilience.DefaultCircuitBreakerConfig(),
    slog.Default(),
)

resp, err := resilientClient.Execute(ctx, req)
```

**Benefits:**

- No manual retry loops
- Automatic exponential backoff with jitter
- Circuit breaker protection
- Standardized error classification
- Context support
- Comprehensive logging

**See:** [examples/openai_migration.go](./examples/openai_migration.go)

## Configuration

### Retry Options

```go
resilience.NewRetryWrapper(
    client,
    resilience.WithMaxAttempts(5),                                    // Total attempts (default: 3)
    resilience.WithExponentialBackoff(time.Second, 30*time.Second),   // Exponential with cap
    resilience.WithConstantBackoff(2*time.Second),                    // Constant delay
    resilience.WithFibonacciBackoff(time.Second, 30*time.Second),     // Fibonacci sequence
    resilience.WithErrorClassifier(customClassifier),                 // Custom error logic
    resilience.WithRetryLogger(logger),                               // Custom logger
)
```

**Backoff Strategies:**

- **Exponential** (default): Delays double each retry (~1s, ~2s, ~4s, ~8s)
- **Constant**: Same delay between retries (~2s, ~2s, ~2s)
- **Fibonacci**: Delays follow fibonacci sequence (~1s, ~1s, ~2s, ~3s, ~5s)

All strategies include jitter to prevent thundering herd.

### Circuit Breaker Options

```go
resilience.NewCircuitBreakerWrapper(
    client,
    resilience.WithMaxRequests(5),                  // Requests in half-open state (default: 3)
    resilience.WithInterval(10*time.Second),        // Count reset interval (default: 10s)
    resilience.WithTimeout(60*time.Second),         // Open state timeout (default: 30s)
    resilience.WithReadyToTrip(tripFunc),           // Custom trip logic
    resilience.WithCircuitBreakerErrorClassifier(customClassifier),
    resilience.WithStateChangeHandler(stateFunc),   // State change callback
    resilience.WithCircuitBreakerLogger(logger),    // Custom logger
)
```

**Default trip logic:**

- Requires at least 3 requests
- Trips when failure rate >= 60%

**Circuit States:**

- **Closed**: Normal operation, requests flow through
- **Open**: Too many failures, requests fail immediately
- **Half-Open**: Testing if service recovered, limited requests allowed

### Error Classification

#### Built-in HTTP Classification

The `HTTPStatusClassifier` provides sensible defaults:

**Retryable errors:**

- 429 (Rate Limited)
- 500, 502, 503, 504 (Server Errors)
- Network errors
- Timeouts (via jp-go-errors)

**Circuit breaker trip conditions:**

- 401, 403 (Authentication/Authorization)
- 500, 502, 503, 504 (Server Errors)
- Unknown errors

**Non-retryable:**

- Context cancellation or deadline exceeded
- 4xx errors (except 429)

#### Custom Classification

```go
type MyClassifier struct{}

func (c *MyClassifier) IsRetryable(err error) bool {
    // Return true for errors that should trigger retry
    return true
}

func (c *MyClassifier) ShouldTripCircuit(err error) bool {
    // Return true for errors that should trip circuit breaker
    return false
}

wrapper := resilience.NewRetryWrapper(
    client,
    resilience.WithErrorClassifier(&MyClassifier{}),
)
```

#### Integration with jp-go-errors

Works seamlessly with jp-go-errors sentinel errors:

```go
import pkgerrors "github.com/JohnPlummer/jp-go-errors"

// Automatically handled:
// - pkgerrors.ErrRateLimited -> retryable, doesn't trip circuit
// - pkgerrors.ErrTimeout -> retryable, doesn't trip circuit
// - pkgerrors.ErrUnauthorized -> not retryable, trips circuit
```

## Testing

### Unit Testing Your Client

```go
type mockClient struct {
    executeFunc func(ctx context.Context, req Request) (Response, error)
}

func (m *mockClient) Execute(ctx context.Context, req Request) (Response, error) {
    return m.executeFunc(ctx, req)
}

func TestMyService(t *testing.T) {
    mock := &mockClient{
        executeFunc: func(ctx context.Context, req Request) (Response, error) {
            return Response{Data: "test"}, nil
        },
    }

    resilientClient := resilience.NewRetryWrapper(mock)
    // Test your service with the resilient client
}
```

### Test Configuration

Use shorter timeouts and fewer attempts for tests:

```go
testRetryConfig := &resilience.RetryConfig{
    MaxAttempts:  2,
    InitialDelay: 10 * time.Millisecond,
    MaxDelay:     50 * time.Millisecond,
}

testCBConfig := &resilience.CircuitBreakerConfig{
    MaxRequests: 2,
    Interval:    100 * time.Millisecond,
    Timeout:     200 * time.Millisecond,
}

testClient := resilience.CombineRetryAndCircuitBreaker(
    baseClient,
    testRetryConfig,
    testCBConfig,
    testLogger,
)
```

### Simulating Failures

```go
type failingClient struct {
    failCount int
    calls     int
}

func (c *failingClient) Execute(ctx context.Context, req Request) (Response, error) {
    c.calls++
    if c.calls <= c.failCount {
        return Response{}, errors.New("simulated failure")
    }
    return Response{Data: "success"}, nil
}

// Test retry behavior
client := &failingClient{failCount: 2}
wrapper := resilience.NewRetryWrapper(client, resilience.WithMaxAttempts(3))
resp, err := wrapper.Execute(ctx, req)
// Should succeed on third attempt
```

## Examples

Complete working examples are available in the [examples](./examples) directory:

- [retry_only.go](./examples/retry_only.go) - Retry pattern with exponential backoff
- [circuit_breaker_only.go](./examples/circuit_breaker_only.go) - Circuit breaker protection
- [combined.go](./examples/combined.go) - Both retry and circuit breaker
- [custom_classifier.go](./examples/custom_classifier.go) - Custom error classification
- [openai_migration.go](./examples/openai_migration.go) - Migrating from manual retry code

Run examples:

```bash
cd examples
go run retry_only.go
go run circuit_breaker_only.go
go run combined.go
go run custom_classifier.go
go run openai_migration.go
```

## Architecture

The package uses the decorator pattern with generics:

1. **Base client**: Implements `ResilientClient[Req, Resp]`
2. **Retry wrapper**: Adds retry logic, wraps base client
3. **Circuit breaker wrapper**: Adds circuit breaker, wraps base client
4. **Combined wrapper**: Circuit breaker (inner) + Retry (outer)
5. **Error classifiers**: Determine retry and circuit breaker behavior

This layered approach allows flexible composition of resilience patterns.

```
Request -> Retry -> Circuit Breaker -> Base Client -> External Service
           ^        ^
           |        |
           |        Protects service, fails fast when unhealthy
           |
           Handles transient failures, respects circuit state
```

## License

MIT License - see [LICENSE](./LICENSE) file for details.

## Contributing

Contributions welcome! Please ensure:

1. Tests pass: `ginkgo run ./...`
2. Linting passes: `golangci-lint run`
3. Documentation is updated
4. Examples demonstrate new features

## Related Packages

- [jp-go-errors](https://github.com/JohnPlummer/jp-go-errors): Standardized error handling
- [sethvargo/go-retry](https://github.com/sethvargo/go-retry): Retry implementation
- [sony/gobreaker](https://github.com/sony/gobreaker): Circuit breaker implementation
