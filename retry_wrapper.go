package resilience

import (
	"context"
	"crypto/rand"
	"errors"
	"log/slog"
	"math/big"
	"sync"
	"time"

	"github.com/sethvargo/go-retry"
)

// RetryWrapper wraps a ResilientClient with configurable retry logic.
// It uses exponential, constant, or fibonacci backoff strategies with jitter
// to prevent thundering herd problems.
type RetryWrapper[Req, Resp any] struct {
	client     ResilientClient[Req, Resp]
	config     *RetryConfig
	logger     *slog.Logger
	classifier ErrorClassifier
	stats      *retryStats
}

// retryStats tracks retry operation statistics.
type retryStats struct {
	mu              sync.RWMutex
	totalAttempts   int64
	totalRetries    int64
	totalSuccesses  int64
	totalFailures   int64
	lastAttemptTime time.Time
	lastError       error
}

// NewRetryWrapper creates a new retry wrapper around a ResilientClient.
// It applies the provided options to configure retry behavior.
//
// Example:
//
//	wrapper := resilience.NewRetryWrapper(
//	    client,
//	    resilience.WithMaxAttempts(5),
//	    resilience.WithExponentialBackoff(time.Second, 30*time.Second),
//	)
func NewRetryWrapper[Req, Resp any](
	client ResilientClient[Req, Resp],
	opts ...RetryOption,
) *RetryWrapper[Req, Resp] {
	config := DefaultRetryConfig()
	for _, opt := range opts {
		opt(config)
	}

	if config.Logger == nil {
		config.Logger = slog.Default()
	}

	if config.ErrorClassifier == nil {
		config.ErrorClassifier = DefaultErrorClassifier()
	}

	return &RetryWrapper[Req, Resp]{
		client:     client,
		config:     config,
		logger:     config.Logger,
		classifier: config.ErrorClassifier,
		stats:      &retryStats{},
	}
}

// Execute performs the request with retry logic.
// It will retry on retryable errors up to MaxAttempts times using the configured backoff strategy.
func (w *RetryWrapper[Req, Resp]) Execute(ctx context.Context, req Req) (Resp, error) {
	var zero Resp

	// Handle zero or negative max attempts - don't make any requests
	if w.config.MaxAttempts <= 0 {
		return zero, errors.New("max attempts must be positive")
	}

	// Check if parent context is already done before attempting any requests
	select {
	case <-ctx.Done():
		w.logger.Warn("context already done before request (expected condition)",
			"error", ctx.Err())
		return zero, ctx.Err()
	default:
	}

	var response Resp
	var attempts int

	backoff := w.getBackoffStrategy()

	err := retry.Do(ctx, backoff, func(ctx context.Context) error {
		attempts++

		// Track attempt and calculate retries (attempts after the first)
		w.stats.mu.Lock()
		w.stats.totalAttempts++
		if attempts > 1 {
			w.stats.totalRetries++
		}
		w.stats.lastAttemptTime = time.Now()
		w.stats.mu.Unlock()

		// Check if parent context is done before each retry attempt
		select {
		case <-ctx.Done():
			w.logger.Warn("context done before retry attempt (expected condition)",
				"attempt", attempts,
				"error", ctx.Err())
			return ctx.Err()
		default:
		}

		// Try the request
		resp, err := w.client.Execute(ctx, req)
		if err == nil {
			if attempts > 1 {
				w.logger.Info("request succeeded after retry",
					"attempts", attempts)
			}
			response = resp
			return nil
		}

		// Check if error is retryable
		if !w.classifier.IsRetryable(err) {
			w.logger.Debug("non-retryable error, giving up",
				"error", err,
				"attempts", attempts)
			return err
		}

		// Log retry
		w.logger.Debug("retrying request after delay",
			"attempt", attempts,
			"error", err)

		// Return retryable error to continue retry loop
		return retry.RetryableError(err)
	})
	if err != nil {
		w.logger.Warn("request failed after retries",
			"attempts", attempts,
			"error", err)
		// Track failure
		w.stats.mu.Lock()
		w.stats.totalFailures++
		w.stats.lastError = err
		w.stats.mu.Unlock()
		return zero, err
	}

	// Track success
	w.stats.mu.Lock()
	w.stats.totalSuccesses++
	w.stats.mu.Unlock()

	return response, nil
}

// getBackoffStrategy returns the appropriate backoff strategy based on configuration.
// Supports exponential, constant, and fibonacci backoff patterns with jitter to prevent thundering herd.
// Note: retry.Do() counts the initial attempt, so MaxAttempts-1 is passed to WithMaxRetries.
func (w *RetryWrapper[Req, Resp]) getBackoffStrategy() retry.Backoff {
	// Validate MaxAttempts to prevent overflow in conversions
	maxAttempts := w.config.MaxAttempts
	if maxAttempts < 0 {
		maxAttempts = 0
	}
	if maxAttempts > 1000 { // Cap at reasonable upper bound
		maxAttempts = 1000
	}

	// Calculate retry attempts (subtract 1 because Do() counts initial attempt)
	maxRetries := maxAttempts - 1
	if maxRetries < 0 {
		maxRetries = 0
	}

	switch w.config.Strategy {
	case RetryStrategyConstant:
		return retry.WithMaxRetries(
			uint64(maxRetries), // #nosec G115 - bounds checked above
			retry.BackoffFunc(func() (time.Duration, bool) {
				// Add jitter to prevent thundering herd using crypto/rand
				jitterMax := int64(w.config.InitialDelay / 10)
				if jitterMax <= 0 {
					jitterMax = 1
				}
				jitterBig, err := rand.Int(rand.Reader, big.NewInt(jitterMax))
				if err != nil {
					// Fallback to no jitter if crypto/rand fails
					return w.config.InitialDelay, false
				}
				jitter := time.Duration(jitterBig.Int64())
				return w.config.InitialDelay + jitter, false
			}),
		)

	case RetryStrategyFibonacci:
		return retry.WithMaxRetries(
			uint64(maxRetries), // #nosec G115 - bounds checked above
			retry.WithCappedDuration(
				w.config.MaxDelay,
				retry.WithJitter(
					w.config.InitialDelay/10,
					retry.NewFibonacci(w.config.InitialDelay),
				),
			),
		)

	case RetryStrategyExponential:
		return retry.WithMaxRetries(
			uint64(maxRetries), // #nosec G115 - bounds checked above
			retry.WithCappedDuration(
				w.config.MaxDelay,
				retry.WithJitter(
					w.config.InitialDelay/10,
					w.newConfigurableExponential(),
				),
			),
		)
	default:
		return retry.WithMaxRetries(
			uint64(maxRetries), // #nosec G115 - bounds checked above
			retry.WithCappedDuration(
				w.config.MaxDelay,
				retry.WithJitter(
					w.config.InitialDelay/10,
					w.newConfigurableExponential(),
				),
			),
		)
	}
}

// newConfigurableExponential creates a custom exponential backoff using the configured multiplier.
// Unlike retry.NewExponential which always doubles (2.0), this allows configurable growth rates.
// The delay for attempt N is: initialDelay * (multiplier ^ N)
func (w *RetryWrapper[Req, Resp]) newConfigurableExponential() retry.Backoff {
	// Get multiplier from config, default to 2.0 if not set or invalid
	multiplier := w.config.Multiplier
	if multiplier <= 0 {
		multiplier = 2.0
	}

	// For multiplier of exactly 2.0, use the optimized library implementation
	if multiplier == 2.0 {
		return retry.NewExponential(w.config.InitialDelay)
	}

	// For custom multipliers, implement custom backoff logic
	attempt := uint64(0)
	return retry.BackoffFunc(func() (time.Duration, bool) {
		// Calculate delay: initialDelay * (multiplier ^ attempt)
		delay := float64(w.config.InitialDelay)
		for i := uint64(0); i < attempt; i++ {
			delay *= multiplier
			// Prevent overflow
			if delay > float64(1<<63-1) {
				attempt++
				return time.Duration(1<<63 - 1), false
			}
		}
		attempt++
		return time.Duration(delay), false
	})
}

// RetryStats holds statistics about retry operations.
type RetryStats struct {
	// TotalAttempts is the total number of attempts made (including initial and retries)
	TotalAttempts int64

	// TotalRetries is the number of retry attempts (not including initial attempts)
	TotalRetries int64

	// TotalSuccesses is the number of successful operations
	TotalSuccesses int64

	// TotalFailures is the number of failed operations (after all retries exhausted)
	TotalFailures int64

	// LastAttemptTime is the time of the last attempt
	LastAttemptTime time.Time

	// LastError is the last error encountered (if any)
	LastError error
}

// GetRetryStats returns statistics about retry operations.
// This method is thread-safe and returns a snapshot of the current statistics.
func (w *RetryWrapper[Req, Resp]) GetRetryStats() RetryStats {
	w.stats.mu.RLock()
	defer w.stats.mu.RUnlock()

	return RetryStats{
		TotalAttempts:   w.stats.totalAttempts,
		TotalRetries:    w.stats.totalRetries,
		TotalSuccesses:  w.stats.totalSuccesses,
		TotalFailures:   w.stats.totalFailures,
		LastAttemptTime: w.stats.lastAttemptTime,
		LastError:       w.stats.lastError,
	}
}
