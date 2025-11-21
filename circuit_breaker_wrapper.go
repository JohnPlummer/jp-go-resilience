package resilience

import (
	"context"
	"errors"
	"log/slog"

	jperrors "github.com/JohnPlummer/jp-go-errors"
	"github.com/sony/gobreaker/v2"
)

// CircuitBreakerWrapper wraps a ResilientClient with circuit breaker functionality.
// It tracks failures and opens the circuit when too many failures occur,
// preventing requests from reaching a failing downstream service.
type CircuitBreakerWrapper[Req, Resp any] struct {
	client     ResilientClient[Req, Resp]
	cb         *gobreaker.CircuitBreaker[Resp]
	logger     *slog.Logger
	classifier CircuitBreakerErrorClassifier
}

// NewCircuitBreakerWrapper creates a new circuit breaker wrapper around a ResilientClient.
// It applies the provided options to configure circuit breaker behavior.
//
// Example:
//
//	wrapper := resilience.NewCircuitBreakerWrapper(
//	    client,
//	    resilience.WithMaxRequests(5),
//	    resilience.WithTimeout(60*time.Second),
//	)
func NewCircuitBreakerWrapper[Req, Resp any](
	client ResilientClient[Req, Resp],
	opts ...CircuitBreakerOption,
) *CircuitBreakerWrapper[Req, Resp] {
	config := DefaultCircuitBreakerConfig()
	for _, opt := range opts {
		opt(config)
	}

	if config.Logger == nil {
		config.Logger = slog.Default()
	}

	if config.ErrorClassifier == nil {
		config.ErrorClassifier = DefaultCircuitBreakerErrorClassifier()
	}

	classifier := config.ErrorClassifier

	settings := gobreaker.Settings{
		Name:        "resilient-client",
		MaxRequests: config.MaxRequests,
		Interval:    config.Interval,
		Timeout:     config.Timeout,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			internalCounts := CircuitBreakerCounts{
				Requests:             counts.Requests,
				TotalSuccesses:       counts.TotalSuccesses,
				TotalFailures:        counts.TotalFailures,
				ConsecutiveSuccesses: counts.ConsecutiveSuccesses,
				ConsecutiveFailures:  counts.ConsecutiveFailures,
			}
			return config.ReadyToTrip(internalCounts)
		},
		OnStateChange: func(name string, from, to gobreaker.State) {
			config.Logger.Warn("circuit breaker state changed",
				"name", name,
				"from", from.String(),
				"to", to.String())

			if config.OnStateChange != nil {
				fromState := convertGobreakerState(from)
				toState := convertGobreakerState(to)
				config.OnStateChange(name, fromState, toState)
			}
		},
		IsSuccessful: func(err error) bool {
			if err == nil {
				return true
			}

			// Don't count errors that shouldn't trip the circuit as failures
			return !classifier.ShouldTripCircuit(err)
		},
	}

	cb := gobreaker.NewCircuitBreaker[Resp](settings)

	return &CircuitBreakerWrapper[Req, Resp]{
		client:     client,
		cb:         cb,
		logger:     config.Logger,
		classifier: classifier,
	}
}

// Execute executes the request through the circuit breaker.
// If the circuit is open, requests are rejected immediately without calling the underlying client.
// Circuit breaker errors are wrapped with jperrors types for consistent error handling:
//   - gobreaker.ErrOpenState becomes jperrors.ErrCircuitOpen
//   - gobreaker.ErrTooManyRequests becomes jperrors.ErrCircuitTooManyRequests
func (w *CircuitBreakerWrapper[Req, Resp]) Execute(ctx context.Context, req Req) (Resp, error) {
	var zero Resp

	resp, err := w.cb.Execute(func() (Resp, error) {
		return w.client.Execute(ctx, req)
	})
	if err != nil {
		// Log and wrap circuit breaker errors with jperrors types
		switch {
		case errors.Is(err, gobreaker.ErrOpenState):
			counts := w.cb.Counts()
			w.logger.Warn("circuit breaker is open, request rejected",
				"error", err,
				"state", w.cb.State(),
				"counts", counts)
			return zero, jperrors.NewCircuitBreakerError(
				"request rejected",
				"execute",
				"open",
				jperrors.WithCause(err),
				jperrors.WithCounts(jperrors.CircuitCounts{
					Requests:             counts.Requests,
					TotalSuccesses:       counts.TotalSuccesses,
					TotalFailures:        counts.TotalFailures,
					ConsecutiveSuccesses: counts.ConsecutiveSuccesses,
					ConsecutiveFailures:  counts.ConsecutiveFailures,
				}),
			)
		case errors.Is(err, gobreaker.ErrTooManyRequests):
			counts := w.cb.Counts()
			w.logger.Debug("circuit breaker in half-open state, too many requests",
				"error", err)
			return zero, jperrors.NewCircuitBreakerError(
				"too many requests in half-open state",
				"execute",
				"half-open",
				jperrors.WithCause(err),
				jperrors.WithCounts(jperrors.CircuitCounts{
					Requests:             counts.Requests,
					TotalSuccesses:       counts.TotalSuccesses,
					TotalFailures:        counts.TotalFailures,
					ConsecutiveSuccesses: counts.ConsecutiveSuccesses,
					ConsecutiveFailures:  counts.ConsecutiveFailures,
				}),
			)
		default:
			w.logger.Debug("request failed through circuit breaker",
				"error", err,
				"should_trip", w.classifier.ShouldTripCircuit(err))
		}
		return zero, err
	}

	return resp, nil
}

// State returns the current state of the circuit breaker.
func (w *CircuitBreakerWrapper[Req, Resp]) State() CircuitBreakerState {
	return convertGobreakerState(w.cb.State())
}

// Counts returns the current counts of the circuit breaker.
func (w *CircuitBreakerWrapper[Req, Resp]) Counts() CircuitBreakerCounts {
	counts := w.cb.Counts()
	return CircuitBreakerCounts{
		Requests:             counts.Requests,
		TotalSuccesses:       counts.TotalSuccesses,
		TotalFailures:        counts.TotalFailures,
		ConsecutiveSuccesses: counts.ConsecutiveSuccesses,
		ConsecutiveFailures:  counts.ConsecutiveFailures,
	}
}

// GetHealth returns the health status of the circuit breaker.
func (w *CircuitBreakerWrapper[Req, Resp]) GetHealth() HealthStatus {
	state := w.State()
	counts := w.Counts()

	var healthy bool
	var status string

	switch state {
	case StateClosed:
		healthy = true
		status = "closed"
	case StateHalfOpen:
		healthy = true // Degraded but operational
		status = "half-open"
	case StateOpen:
		healthy = false
		status = "open"
	default:
		status = "unknown"
	}

	return HealthStatus{
		Healthy:              healthy,
		Status:               status,
		State:                state.String(),
		Requests:             counts.Requests,
		TotalSuccesses:       counts.TotalSuccesses,
		TotalFailures:        counts.TotalFailures,
		ConsecutiveFailures:  counts.ConsecutiveFailures,
		ConsecutiveSuccesses: counts.ConsecutiveSuccesses,
	}
}

// convertGobreakerState converts gobreaker.State to our CircuitBreakerState.
func convertGobreakerState(state gobreaker.State) CircuitBreakerState {
	switch state {
	case gobreaker.StateClosed:
		return StateClosed
	case gobreaker.StateHalfOpen:
		return StateHalfOpen
	case gobreaker.StateOpen:
		return StateOpen
	default:
		return StateClosed
	}
}

// CombineRetryAndCircuitBreaker creates a wrapper with both retry and circuit breaker functionality.
// The circuit breaker is applied first (inner layer) to protect the underlying service,
// then retry logic is applied (outer layer) to handle transient failures.
// This layering ensures circuit breaker state is accurately maintained while providing resilience.
func CombineRetryAndCircuitBreaker[Req, Resp any](
	client ResilientClient[Req, Resp],
	retryConfig *RetryConfig,
	cbConfig *CircuitBreakerConfig,
	logger *slog.Logger,
) ResilientClient[Req, Resp] {
	// Set logger on configs if provided
	if logger != nil {
		if retryConfig != nil {
			retryConfig.Logger = logger
		}
		if cbConfig != nil {
			cbConfig.Logger = logger
		}
	}

	// First wrap with circuit breaker (inner layer)
	withCB := NewCircuitBreakerWrapper(client, func(c *CircuitBreakerConfig) {
		if cbConfig != nil {
			*c = *cbConfig
		}
	})

	// Then wrap with retry (outer layer)
	withRetry := NewRetryWrapper(withCB, func(c *RetryConfig) {
		if retryConfig != nil {
			*c = *retryConfig
		}
	})

	return withRetry
}
