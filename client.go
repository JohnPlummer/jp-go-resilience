// Package resilience provides generic retry and circuit breaker patterns for building resilient clients.
// It supports any request/response type using Go generics and integrates with jp-go-errors for
// standardized error handling.
package resilience

import (
	"context"
)

// ResilientClient defines a generic interface for executing requests with retry and circuit breaker support.
// Type parameters Req and Resp can be any types, making this suitable for HTTP clients, gRPC clients,
// database clients, or any other operation that needs resilience patterns.
//
// Example:
//
//	type HTTPClient struct {
//	    client *http.Client
//	}
//
//	func (c *HTTPClient) Execute(ctx context.Context, req *http.Request) (*http.Response, error) {
//	    return c.client.Do(req.WithContext(ctx))
//	}
//
//	// Wrap with retry
//	resilientClient := resilience.NewRetryWrapper(
//	    httpClient,
//	    resilience.WithMaxAttempts(3),
//	    resilience.WithExponentialBackoff(time.Second, 30*time.Second),
//	)
type ResilientClient[Req, Resp any] interface {
	// Execute performs a request and returns a response or error.
	// The context should be used to control timeouts and cancellation.
	Execute(ctx context.Context, req Req) (Resp, error)
}
