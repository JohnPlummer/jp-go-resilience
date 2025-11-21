package resilience

import (
	"log/slog"
	"time"
)

// RetryStrategy defines the backoff strategy for retry operations.
type RetryStrategy string

const (
	// RetryStrategyExponential uses exponential backoff with jitter.
	RetryStrategyExponential RetryStrategy = "exponential"

	// RetryStrategyConstant uses a constant delay between retries with jitter.
	RetryStrategyConstant RetryStrategy = "constant"

	// RetryStrategyFibonacci uses fibonacci backoff with jitter.
	RetryStrategyFibonacci RetryStrategy = "fibonacci"
)

// RetryConfig holds retry configuration options.
type RetryConfig struct {
	// ErrorClassifier determines which errors should trigger retries.
	// Default: HTTPStatusClassifier with standard retryable codes
	ErrorClassifier ErrorClassifier

	// Logger for retry operations.
	// Default: slog.Default()
	Logger *slog.Logger

	// Strategy defines the backoff strategy.
	// Default: RetryStrategyExponential
	Strategy RetryStrategy

	// InitialDelay is the delay before the first retry.
	// Default: 1 second
	InitialDelay time.Duration

	// MaxDelay is the maximum delay between retries (for exponential/fibonacci).
	// Default: 30 seconds
	MaxDelay time.Duration

	// Multiplier is the backoff multiplier for exponential strategy.
	// For exponential backoff, delay = initialDelay * (multiplier ^ attempt).
	// Default: 2.0 (doubling)
	// Common values: 1.5 (moderate growth), 2.0 (doubling), 3.0 (aggressive growth)
	Multiplier float64

	// MaxAttempts is the maximum number of attempts (including the initial request).
	// Default: 3
	MaxAttempts int
}

// RetryOption is a functional option for configuring retry behavior.
type RetryOption func(*RetryConfig)

// WithMaxAttempts sets the maximum number of retry attempts.
// The total number of calls will be MaxAttempts (including the initial attempt).
//
// Example:
//
//	resilience.WithMaxAttempts(5) // Try up to 5 times total
func WithMaxAttempts(attempts int) RetryOption {
	return func(c *RetryConfig) {
		c.MaxAttempts = attempts
	}
}

// WithExponentialBackoff configures exponential backoff with jitter.
// Each retry delay is multiplied by the configured multiplier (default 2.0) up to maxDelay.
//
// Example:
//
//	resilience.WithExponentialBackoff(time.Second, 30*time.Second)
//	// With default multiplier 2.0: ~1s, ~2s, ~4s, ~8s, ~16s, 30s (capped)
func WithExponentialBackoff(initialDelay, maxDelay time.Duration) RetryOption {
	return func(c *RetryConfig) {
		c.Strategy = RetryStrategyExponential
		c.InitialDelay = initialDelay
		c.MaxDelay = maxDelay
	}
}

// WithMultiplier sets the backoff multiplier for exponential strategy.
// Only applies when using RetryStrategyExponential.
//
// Example:
//
//	resilience.WithMultiplier(1.5) // 50% growth per retry
//	// With InitialDelay=1s: ~1s, ~1.5s, ~2.25s, ~3.375s, ...
func WithMultiplier(multiplier float64) RetryOption {
	return func(c *RetryConfig) {
		c.Multiplier = multiplier
	}
}

// WithConstantBackoff configures constant delay between retries with jitter.
// All retry delays will be approximately the same.
//
// Example:
//
//	resilience.WithConstantBackoff(2 * time.Second)
//	// Delays: ~2s, ~2s, ~2s, ~2s
func WithConstantBackoff(delay time.Duration) RetryOption {
	return func(c *RetryConfig) {
		c.Strategy = RetryStrategyConstant
		c.InitialDelay = delay
		c.MaxDelay = delay
	}
}

// WithFibonacciBackoff configures fibonacci backoff with jitter.
// Delays follow the fibonacci sequence up to maxDelay.
//
// Example:
//
//	resilience.WithFibonacciBackoff(time.Second, 30*time.Second)
//	// Delays: ~1s, ~1s, ~2s, ~3s, ~5s, ~8s, ~13s, ~21s, 30s (capped)
func WithFibonacciBackoff(initialDelay, maxDelay time.Duration) RetryOption {
	return func(c *RetryConfig) {
		c.Strategy = RetryStrategyFibonacci
		c.InitialDelay = initialDelay
		c.MaxDelay = maxDelay
	}
}

// WithErrorClassifier sets a custom error classifier for retry decisions.
//
// Example:
//
//	classifier := &MyCustomClassifier{}
//	resilience.WithErrorClassifier(classifier)
func WithErrorClassifier(classifier ErrorClassifier) RetryOption {
	return func(c *RetryConfig) {
		c.ErrorClassifier = classifier
	}
}

// WithRetryLogger sets a custom logger for retry operations.
//
// Example:
//
//	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
//	resilience.WithRetryLogger(logger)
func WithRetryLogger(logger *slog.Logger) RetryOption {
	return func(c *RetryConfig) {
		c.Logger = logger
	}
}

// DefaultRetryConfig returns retry configuration with sensible defaults.
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxAttempts:     3,
		Strategy:        RetryStrategyExponential,
		InitialDelay:    time.Second,
		MaxDelay:        30 * time.Second,
		Multiplier:      2.0,
		ErrorClassifier: DefaultErrorClassifier(),
		Logger:          slog.Default(),
	}
}

// CircuitBreakerConfig holds circuit breaker configuration options.
type CircuitBreakerConfig struct {
	// ReadyToTrip is called with a copy of counts whenever a request fails in the closed state.
	// If ReadyToTrip returns true, the circuit breaker will be placed into the open state.
	// Default: trips after 3 requests with 60% failure rate
	ReadyToTrip func(counts CircuitBreakerCounts) bool

	// ErrorClassifier determines which errors should trip the circuit breaker.
	// Default: HTTPStatusClassifier with standard trip codes
	ErrorClassifier CircuitBreakerErrorClassifier

	// OnStateChange is called whenever the circuit breaker changes state.
	OnStateChange func(name string, from, to CircuitBreakerState)

	// Logger for circuit breaker operations.
	// Default: slog.Default()
	Logger *slog.Logger

	// Interval is the cyclic period of the closed state for the circuit breaker
	// to clear the internal counts. If 0, never clears.
	// Default: 10 seconds
	Interval time.Duration

	// Timeout is the period of the open state, after which the state becomes half-open.
	// Default: 30 seconds
	Timeout time.Duration

	// MaxRequests is the maximum number of requests allowed to pass through
	// when the circuit breaker is in the half-open state.
	// Default: 3
	MaxRequests uint32
}

// CircuitBreakerOption is a functional option for configuring circuit breaker behavior.
type CircuitBreakerOption func(*CircuitBreakerConfig)

// CircuitBreakerCounts holds the internal counts of the circuit breaker.
type CircuitBreakerCounts struct {
	Requests             uint32
	TotalSuccesses       uint32
	TotalFailures        uint32
	ConsecutiveSuccesses uint32
	ConsecutiveFailures  uint32
}

// CircuitBreakerState represents the state of the circuit breaker.
type CircuitBreakerState int

const (
	// StateClosed means the circuit is closed and requests flow normally.
	StateClosed CircuitBreakerState = iota

	// StateHalfOpen means the circuit is testing if the service has recovered.
	StateHalfOpen

	// StateOpen means the circuit is open and requests are rejected immediately.
	StateOpen
)

// String returns the string representation of the circuit breaker state.
func (s CircuitBreakerState) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateHalfOpen:
		return "half-open"
	case StateOpen:
		return "open"
	default:
		return "unknown"
	}
}

// WithMaxRequests sets the maximum number of requests in half-open state.
//
// Example:
//
//	resilience.WithMaxRequests(5)
func WithMaxRequests(maxRequests uint32) CircuitBreakerOption {
	return func(c *CircuitBreakerConfig) {
		c.MaxRequests = maxRequests
	}
}

// WithInterval sets the interval for clearing counts in closed state.
//
// Example:
//
//	resilience.WithInterval(10 * time.Second)
func WithInterval(interval time.Duration) CircuitBreakerOption {
	return func(c *CircuitBreakerConfig) {
		c.Interval = interval
	}
}

// WithTimeout sets the timeout for staying in open state.
//
// Example:
//
//	resilience.WithTimeout(60 * time.Second)
func WithTimeout(timeout time.Duration) CircuitBreakerOption {
	return func(c *CircuitBreakerConfig) {
		c.Timeout = timeout
	}
}

// WithReadyToTrip sets a custom function to determine when to trip the circuit.
//
// Example:
//
//	resilience.WithReadyToTrip(func(counts resilience.CircuitBreakerCounts) bool {
//	    failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
//	    return counts.Requests >= 5 && failureRatio >= 0.5
//	})
func WithReadyToTrip(fn func(counts CircuitBreakerCounts) bool) CircuitBreakerOption {
	return func(c *CircuitBreakerConfig) {
		c.ReadyToTrip = fn
	}
}

// WithCircuitBreakerErrorClassifier sets a custom error classifier for circuit breaker decisions.
//
// Example:
//
//	classifier := &MyCustomClassifier{}
//	resilience.WithCircuitBreakerErrorClassifier(classifier)
func WithCircuitBreakerErrorClassifier(classifier CircuitBreakerErrorClassifier) CircuitBreakerOption {
	return func(c *CircuitBreakerConfig) {
		c.ErrorClassifier = classifier
	}
}

// WithStateChangeHandler sets a callback for circuit breaker state changes.
//
// Example:
//
//	resilience.WithStateChangeHandler(func(name string, from, to resilience.CircuitBreakerState) {
//	    log.Printf("Circuit %s changed from %s to %s", name, from, to)
//	})
func WithStateChangeHandler(fn func(name string, from, to CircuitBreakerState)) CircuitBreakerOption {
	return func(c *CircuitBreakerConfig) {
		c.OnStateChange = fn
	}
}

// WithCircuitBreakerLogger sets a custom logger for circuit breaker operations.
//
// Example:
//
//	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
//	resilience.WithCircuitBreakerLogger(logger)
func WithCircuitBreakerLogger(logger *slog.Logger) CircuitBreakerOption {
	return func(c *CircuitBreakerConfig) {
		c.Logger = logger
	}
}

// DefaultCircuitBreakerConfig returns circuit breaker configuration with sensible defaults.
func DefaultCircuitBreakerConfig() *CircuitBreakerConfig {
	return &CircuitBreakerConfig{
		MaxRequests: 3,
		Interval:    10 * time.Second,
		Timeout:     30 * time.Second,
		ReadyToTrip: func(counts CircuitBreakerCounts) bool {
			failureRatio := float64(counts.TotalFailures) / float64(counts.Requests)
			return counts.Requests >= 3 && failureRatio >= 0.6
		},
		ErrorClassifier: DefaultCircuitBreakerErrorClassifier(),
		Logger:          slog.Default(),
	}
}
