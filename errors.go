package resilience

import (
	"context"
	"errors"

	pkgerrors "github.com/JohnPlummer/jp-go-errors"
)

// ErrorClassifier determines whether an error should trigger a retry.
// Implement this interface to customize retry behavior for your specific error types.
type ErrorClassifier interface {
	// IsRetryable returns true if the error represents a transient failure
	// that should be retried.
	IsRetryable(err error) bool
}

// CircuitBreakerErrorClassifier determines whether an error should trip the circuit breaker.
// Implement this interface to customize circuit breaker behavior for your specific error types.
type CircuitBreakerErrorClassifier interface {
	// ShouldTripCircuit returns true if the error represents a failure serious enough
	// to open the circuit breaker and stop requests temporarily.
	ShouldTripCircuit(err error) bool
}

// HTTPStatusClassifier provides HTTP status code-based error classification.
// It classifies errors based on HTTP status codes, treating certain codes as retryable
// and others as circuit breaker trip conditions.
type HTTPStatusClassifier struct {
	// RetryableStatuses lists HTTP status codes that should trigger retries.
	// Defaults to 429, 500, 502, 503, 504 if nil.
	RetryableStatuses []int

	// CircuitTripStatuses lists HTTP status codes that should trip the circuit breaker.
	// Defaults to 401, 403, 500, 502, 503, 504 if nil.
	CircuitTripStatuses []int
}

// HTTPError represents an error with an associated HTTP status code.
// Many HTTP client libraries provide errors that implement this interface.
type HTTPError interface {
	error
	StatusCode() int
}

// NewHTTPStatusClassifier creates a new HTTPStatusClassifier with default status code mappings.
// Retryable: 429 (rate limit), 500, 502, 503, 504 (server errors)
// Circuit trip: 401, 403 (auth errors), 500, 502, 503, 504 (server errors)
func NewHTTPStatusClassifier() *HTTPStatusClassifier {
	return &HTTPStatusClassifier{
		RetryableStatuses:   []int{429, 500, 502, 503, 504},
		CircuitTripStatuses: []int{401, 403, 500, 502, 503, 504},
	}
}

// IsRetryable implements ErrorClassifier for HTTP status codes.
// It checks if the error has an HTTP status code that indicates a retryable condition.
func (c *HTTPStatusClassifier) IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	// Context errors are NOT retryable - if the parent context is exceeded or canceled,
	// retrying with the same context will fail immediately.
	// Check these FIRST before other timeout checks, as context.DeadlineExceeded
	// may be considered a timeout by other error checkers.
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return false
	}

	// Check for jp-go-errors sentinel errors
	if errors.Is(err, pkgerrors.ErrRateLimited) {
		return true
	}
	if pkgerrors.IsTimeout(err) {
		return true
	}

	// Check for HTTP status code
	statusCode := extractStatusCode(err)
	if statusCode == 0 {
		// Unknown errors might be retryable (network issues, etc.)
		return true
	}

	return containsStatus(c.getRetryableStatuses(), statusCode)
}

// ShouldTripCircuit implements CircuitBreakerErrorClassifier for HTTP status codes.
// It checks if the error has an HTTP status code that indicates the circuit should trip.
func (c *HTTPStatusClassifier) ShouldTripCircuit(err error) bool {
	if err == nil {
		return false
	}

	// Rate limits and timeouts should NOT trip the circuit - these are transient
	if errors.Is(err, pkgerrors.ErrRateLimited) {
		return false
	}
	if pkgerrors.IsTimeout(err) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return false
	}

	// Check for HTTP status code
	statusCode := extractStatusCode(err)
	if statusCode == 0 {
		// Unknown errors should trip the circuit to be safe
		return true
	}

	return containsStatus(c.getCircuitTripStatuses(), statusCode)
}

// getRetryableStatuses returns the configured retryable statuses or defaults.
func (c *HTTPStatusClassifier) getRetryableStatuses() []int {
	if c.RetryableStatuses != nil {
		return c.RetryableStatuses
	}
	return []int{429, 500, 502, 503, 504}
}

// getCircuitTripStatuses returns the configured circuit trip statuses or defaults.
func (c *HTTPStatusClassifier) getCircuitTripStatuses() []int {
	if c.CircuitTripStatuses != nil {
		return c.CircuitTripStatuses
	}
	return []int{401, 403, 500, 502, 503, 504}
}

// extractStatusCode attempts to extract an HTTP status code from various error types.
func extractStatusCode(err error) int {
	// Check if error implements HTTPError interface
	var httpErr HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.StatusCode()
	}

	// Check if error is from jp-go-errors package (implements HTTPError interface)
	type httpStatusProvider interface {
		StatusCode() int
	}
	var statusProvider httpStatusProvider
	if errors.As(err, &statusProvider) {
		return statusProvider.StatusCode()
	}

	return 0
}

// containsStatus checks if a status code is in the list.
func containsStatus(statuses []int, status int) bool {
	for _, s := range statuses {
		if s == status {
			return true
		}
	}
	return false
}

// DefaultErrorClassifier provides reasonable defaults for most use cases.
// It treats 5xx errors, 429 (rate limit), network errors, and timeouts as retryable.
// It trips the circuit on authentication errors and persistent server errors.
func DefaultErrorClassifier() ErrorClassifier {
	return NewHTTPStatusClassifier()
}

// DefaultCircuitBreakerErrorClassifier provides reasonable defaults for circuit breaker tripping.
// It trips on authentication errors (401, 403) and server errors (5xx),
// but not on rate limits or timeouts which are transient.
func DefaultCircuitBreakerErrorClassifier() CircuitBreakerErrorClassifier {
	return NewHTTPStatusClassifier()
}

// StatusCodeError wraps an error with an HTTP status code.
// Use this when you need to add status code information to an existing error.
type StatusCodeError struct {
	Err  error
	Code int
}

// Error implements the error interface.
func (e *StatusCodeError) Error() string {
	return e.Err.Error()
}

// Unwrap implements error unwrapping for errors.Is and errors.As.
func (e *StatusCodeError) Unwrap() error {
	return e.Err
}

// StatusCode returns the HTTP status code.
// This implements the HTTPError interface.
func (e *StatusCodeError) StatusCode() int {
	return e.Code
}

// NewStatusCodeError creates a new StatusCodeError.
// This is useful when wrapping errors from systems that don't provide status codes.
//
// Example:
//
//	err := doRequest()
//	if err != nil {
//	    return resilience.NewStatusCodeError(http.StatusServiceUnavailable, err)
//	}
func NewStatusCodeError(statusCode int, err error) error {
	return &StatusCodeError{
		Code: statusCode,
		Err:  err,
	}
}
