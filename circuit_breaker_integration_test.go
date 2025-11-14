package resilience_test

import (
	"context"
	"errors"
	"log/slog"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	resilience "github.com/JohnPlummer/jp-go-resilience"
)

var _ = Describe("CircuitBreaker ErrorClassifier Integration", func() {
	var (
		client  *mockCircuitBreakerClient
		wrapper *resilience.CircuitBreakerWrapper[string, string]
		ctx     context.Context
		logger  *slog.Logger
	)

	BeforeEach(func() {
		client = &mockCircuitBreakerClient{
			executeFunc: func(ctx context.Context, req string) (string, error) {
				return "success", nil
			},
		}
		ctx = context.Background()
		logger = slog.Default()
	})

	Describe("Default HTTPStatusClassifier", func() {
		BeforeEach(func() {
			wrapper = resilience.NewCircuitBreakerWrapper(
				client,
				resilience.WithCircuitBreakerLogger(logger),
			)
		})

		Context("Rate Limit Errors (429)", func() {
			It("should not trip circuit on rate limit errors", func() {
				// Make 5 requests with 429 errors
				client.executeFunc = func(ctx context.Context, req string) (string, error) {
					return "", resilience.NewStatusCodeError(429, errors.New("rate limited"))
				}
				for i := 0; i < 5; i++ {
					_, _ = wrapper.Execute(ctx, "test")
				}

				// Circuit should still be closed
				Expect(wrapper.State()).To(Equal(resilience.StateClosed))
			})
		})

		Context("Timeout Errors", func() {
			It("should not trip circuit on context deadline exceeded", func() {
				// Make 5 requests with timeout errors
				client.executeFunc = func(ctx context.Context, req string) (string, error) {
					return "", context.DeadlineExceeded
				}
				for i := 0; i < 5; i++ {
					_, _ = wrapper.Execute(ctx, "test")
				}

				// Circuit should still be closed
				Expect(wrapper.State()).To(Equal(resilience.StateClosed))
			})

			It("should not trip circuit on context canceled", func() {
				// Make 5 requests with canceled errors
				client.executeFunc = func(ctx context.Context, req string) (string, error) {
					return "", context.Canceled
				}
				for i := 0; i < 5; i++ {
					_, _ = wrapper.Execute(ctx, "test")
				}

				// Circuit should still be closed
				Expect(wrapper.State()).To(Equal(resilience.StateClosed))
			})
		})

		Context("Authentication Errors (401, 403)", func() {
			It("should trip circuit on 401 errors", func() {
				// Make 3 requests with 401 errors
				client.executeFunc = func(ctx context.Context, req string) (string, error) {
					return "", resilience.NewStatusCodeError(401, errors.New("unauthorized"))
				}
				_, _ = wrapper.Execute(ctx, "test")
				_, _ = wrapper.Execute(ctx, "test")
				_, _ = wrapper.Execute(ctx, "test")

				// Circuit should be open
				Expect(wrapper.State()).To(Equal(resilience.StateOpen))
			})

			It("should trip circuit on 403 errors", func() {
				// Make 3 requests with 403 errors
				client.executeFunc = func(ctx context.Context, req string) (string, error) {
					return "", resilience.NewStatusCodeError(403, errors.New("forbidden"))
				}
				_, _ = wrapper.Execute(ctx, "test")
				_, _ = wrapper.Execute(ctx, "test")
				_, _ = wrapper.Execute(ctx, "test")

				// Circuit should be open
				Expect(wrapper.State()).To(Equal(resilience.StateOpen))
			})
		})

		Context("Server Errors (5xx)", func() {
			It("should trip circuit on 500 errors", func() {
				// Make 3 requests with 500 errors
				client.executeFunc = func(ctx context.Context, req string) (string, error) {
					return "", resilience.NewStatusCodeError(500, errors.New("internal server error"))
				}
				_, _ = wrapper.Execute(ctx, "test")
				_, _ = wrapper.Execute(ctx, "test")
				_, _ = wrapper.Execute(ctx, "test")

				// Circuit should be open
				Expect(wrapper.State()).To(Equal(resilience.StateOpen))
			})

			It("should trip circuit on 502 errors", func() {
				// Make 3 requests with 502 errors
				client.executeFunc = func(ctx context.Context, req string) (string, error) {
					return "", resilience.NewStatusCodeError(502, errors.New("bad gateway"))
				}
				_, _ = wrapper.Execute(ctx, "test")
				_, _ = wrapper.Execute(ctx, "test")
				_, _ = wrapper.Execute(ctx, "test")

				// Circuit should be open
				Expect(wrapper.State()).To(Equal(resilience.StateOpen))
			})

			It("should trip circuit on 503 errors", func() {
				// Make 3 requests with 503 errors
				client.executeFunc = func(ctx context.Context, req string) (string, error) {
					return "", resilience.NewStatusCodeError(503, errors.New("service unavailable"))
				}
				_, _ = wrapper.Execute(ctx, "test")
				_, _ = wrapper.Execute(ctx, "test")
				_, _ = wrapper.Execute(ctx, "test")

				// Circuit should be open
				Expect(wrapper.State()).To(Equal(resilience.StateOpen))
			})

			It("should trip circuit on 504 errors", func() {
				// Make 3 requests with 504 errors
				client.executeFunc = func(ctx context.Context, req string) (string, error) {
					return "", resilience.NewStatusCodeError(504, errors.New("gateway timeout"))
				}
				_, _ = wrapper.Execute(ctx, "test")
				_, _ = wrapper.Execute(ctx, "test")
				_, _ = wrapper.Execute(ctx, "test")

				// Circuit should be open
				Expect(wrapper.State()).To(Equal(resilience.StateOpen))
			})
		})

		Context("Client Errors (4xx)", func() {
			It("should NOT trip circuit on 400 errors", func() {
				// Make 5 requests with 400 errors (client errors don't trip by default)
				client.executeFunc = func(ctx context.Context, req string) (string, error) {
					return "", resilience.NewStatusCodeError(400, errors.New("bad request"))
				}
				for i := 0; i < 5; i++ {
					_, _ = wrapper.Execute(ctx, "test")
				}

				// Circuit should still be closed (400 is not in default trip list)
				Expect(wrapper.State()).To(Equal(resilience.StateClosed))
			})

			It("should NOT trip circuit on 404 errors", func() {
				// Make 5 requests with 404 errors (client errors don't trip by default)
				client.executeFunc = func(ctx context.Context, req string) (string, error) {
					return "", resilience.NewStatusCodeError(404, errors.New("not found"))
				}
				for i := 0; i < 5; i++ {
					_, _ = wrapper.Execute(ctx, "test")
				}

				// Circuit should still be closed (404 is not in default trip list)
				Expect(wrapper.State()).To(Equal(resilience.StateClosed))
			})
		})

		Context("Unknown Errors", func() {
			It("should trip circuit on unknown errors", func() {
				// Make 3 requests with unknown errors (no status code)
				client.executeFunc = func(ctx context.Context, req string) (string, error) {
					return "", errors.New("unknown error")
				}
				_, _ = wrapper.Execute(ctx, "test")
				_, _ = wrapper.Execute(ctx, "test")
				_, _ = wrapper.Execute(ctx, "test")

				// Circuit should be open
				Expect(wrapper.State()).To(Equal(resilience.StateOpen))
			})
		})
	})

	Describe("Custom ErrorClassifier", func() {
		It("should use custom classifier to determine circuit trip behavior", func() {
			// Custom classifier that only trips on specific error message
			customClassifier := &customCircuitBreakerClassifier{
				tripMessage: "critical error",
			}

			// Create fresh client and wrapper
			freshClient := &mockCircuitBreakerClient{
				executeFunc: func(ctx context.Context, req string) (string, error) {
					return "success", nil
				},
			}

			customWrapper := resilience.NewCircuitBreakerWrapper(
				freshClient,
				resilience.WithCircuitBreakerErrorClassifier(customClassifier),
				resilience.WithCircuitBreakerLogger(logger),
				resilience.WithInterval(0), // Disable count clearing
			)

			// Make 3 requests with critical errors (classifier says these SHOULD trip)
			freshClient.executeFunc = func(ctx context.Context, req string) (string, error) {
				return "", errors.New("critical error")
			}
			_, _ = customWrapper.Execute(ctx, "test")
			_, _ = customWrapper.Execute(ctx, "test")
			_, _ = customWrapper.Execute(ctx, "test")

			// Circuit should be open because classifier says "critical error" should trip
			Expect(customWrapper.State()).To(Equal(resilience.StateOpen))
		})
	})
})

// customCircuitBreakerClassifier is a test classifier that trips on specific error messages.
type customCircuitBreakerClassifier struct {
	tripMessage string
}

func (c *customCircuitBreakerClassifier) ShouldTripCircuit(err error) bool {
	if err == nil {
		return false
	}
	return err.Error() == c.tripMessage
}
