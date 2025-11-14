package resilience_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	pkgerrors "github.com/JohnPlummer/jp-go-errors"
	resilience "github.com/JohnPlummer/jp-go-resilience"
)

var _ = Describe("RetryWrapper Integration", func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc
		client *mockClient
		logger *slog.Logger
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
		client = &mockClient{}
		logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelError,
		}))
	})

	AfterEach(func() {
		cancel()
	})

	Describe("HTTPStatusClassifier Integration", func() {
		var classifier *resilience.HTTPStatusClassifier

		BeforeEach(func() {
			classifier = resilience.NewHTTPStatusClassifier()
		})

		Context("with retryable HTTP status codes", func() {
			DescribeTable("retries on retryable status codes",
				func(statusCode int, errorMsg string) {
					attemptCount := 0
					client.executeFunc = func(ctx context.Context, req string) (string, error) {
						attemptCount++
						if attemptCount < 3 {
							return "", resilience.NewStatusCodeError(statusCode, errors.New(errorMsg))
						}
						return "success", nil
					}

					wrapper := resilience.NewRetryWrapper(
						client,
						resilience.WithMaxAttempts(5),
						resilience.WithConstantBackoff(10*time.Millisecond),
						resilience.WithErrorClassifier(classifier),
						resilience.WithRetryLogger(logger),
					)

					resp, err := wrapper.Execute(ctx, "test")
					Expect(err).NotTo(HaveOccurred())
					Expect(resp).To(Equal("success"))
					Expect(client.getCallCount()).To(Equal(3))
				},
				Entry("429 rate limit", 429, "rate limit exceeded"),
				Entry("500 internal server error", 500, "internal server error"),
				Entry("502 bad gateway", 502, "bad gateway"),
				Entry("503 service unavailable", 503, "service unavailable"),
				Entry("504 gateway timeout", 504, "gateway timeout"),
			)
		})

		Context("with non-retryable HTTP status codes", func() {
			DescribeTable("does not retry on non-retryable status codes",
				func(statusCode int, errorMsg string) {
					client.executeFunc = func(ctx context.Context, req string) (string, error) {
						return "", resilience.NewStatusCodeError(statusCode, errors.New(errorMsg))
					}

					wrapper := resilience.NewRetryWrapper(
						client,
						resilience.WithMaxAttempts(5),
						resilience.WithConstantBackoff(10*time.Millisecond),
						resilience.WithErrorClassifier(classifier),
						resilience.WithRetryLogger(logger),
					)

					resp, err := wrapper.Execute(ctx, "test")
					Expect(err).To(HaveOccurred())
					Expect(resp).To(Equal(""))
					Expect(client.getCallCount()).To(Equal(1))
				},
				Entry("400 bad request", 400, "bad request"),
				Entry("401 unauthorized", 401, "unauthorized"),
				Entry("403 forbidden", 403, "forbidden"),
				Entry("404 not found", 404, "not found"),
			)
		})

		Context("with jp-go-errors sentinel errors", func() {
			It("retries on ErrRateLimited", func() {
				attemptCount := 0
				client.executeFunc = func(ctx context.Context, req string) (string, error) {
					attemptCount++
					if attemptCount < 3 {
						return "", pkgerrors.ErrRateLimited
					}
					return "success", nil
				}

				wrapper := resilience.NewRetryWrapper(
					client,
					resilience.WithMaxAttempts(5),
					resilience.WithConstantBackoff(10*time.Millisecond),
					resilience.WithErrorClassifier(classifier),
					resilience.WithRetryLogger(logger),
				)

				resp, err := wrapper.Execute(ctx, "test")
				Expect(err).NotTo(HaveOccurred())
				Expect(resp).To(Equal("success"))
				Expect(client.getCallCount()).To(Equal(3))
			})

			It("retries on timeout errors", func() {
				attemptCount := 0
				client.executeFunc = func(ctx context.Context, req string) (string, error) {
					attemptCount++
					if attemptCount < 3 {
						return "", pkgerrors.NewTimeoutError("operation timeout", "test_operation", 5*time.Second)
					}
					return "success", nil
				}

				wrapper := resilience.NewRetryWrapper(
					client,
					resilience.WithMaxAttempts(5),
					resilience.WithConstantBackoff(10*time.Millisecond),
					resilience.WithErrorClassifier(classifier),
					resilience.WithRetryLogger(logger),
				)

				resp, err := wrapper.Execute(ctx, "test")
				Expect(err).NotTo(HaveOccurred())
				Expect(resp).To(Equal("success"))
				Expect(client.getCallCount()).To(Equal(3))
			})

			It("does not retry on context errors", func() {
				client.executeFunc = func(ctx context.Context, req string) (string, error) {
					return "", context.DeadlineExceeded
				}

				wrapper := resilience.NewRetryWrapper(
					client,
					resilience.WithMaxAttempts(5),
					resilience.WithConstantBackoff(10*time.Millisecond),
					resilience.WithErrorClassifier(classifier),
					resilience.WithRetryLogger(logger),
				)

				resp, err := wrapper.Execute(ctx, "test")
				Expect(err).To(Equal(context.DeadlineExceeded))
				Expect(resp).To(Equal(""))
				Expect(client.getCallCount()).To(Equal(1))
			})

			It("does not retry on context.Canceled", func() {
				client.executeFunc = func(ctx context.Context, req string) (string, error) {
					return "", context.Canceled
				}

				wrapper := resilience.NewRetryWrapper(
					client,
					resilience.WithMaxAttempts(5),
					resilience.WithConstantBackoff(10*time.Millisecond),
					resilience.WithErrorClassifier(classifier),
					resilience.WithRetryLogger(logger),
				)

				resp, err := wrapper.Execute(ctx, "test")
				Expect(err).To(Equal(context.Canceled))
				Expect(resp).To(Equal(""))
				Expect(client.getCallCount()).To(Equal(1))
			})
		})

		Context("with custom status code configurations", func() {
			It("uses custom retryable status codes", func() {
				customClassifier := &resilience.HTTPStatusClassifier{
					RetryableStatuses: []int{418}, // Only retry on "I'm a teapot"
				}

				client.executeFunc = func(ctx context.Context, req string) (string, error) {
					return "", resilience.NewStatusCodeError(500, errors.New("server error"))
				}

				wrapper := resilience.NewRetryWrapper(
					client,
					resilience.WithMaxAttempts(5),
					resilience.WithConstantBackoff(10*time.Millisecond),
					resilience.WithErrorClassifier(customClassifier),
					resilience.WithRetryLogger(logger),
				)

				resp, err := wrapper.Execute(ctx, "test")
				Expect(err).To(HaveOccurred())
				Expect(resp).To(Equal(""))
				Expect(client.getCallCount()).To(Equal(1)) // No retry on 500
			})

			It("retries on custom status code", func() {
				customClassifier := &resilience.HTTPStatusClassifier{
					RetryableStatuses: []int{418},
				}

				attemptCount := 0
				client.executeFunc = func(ctx context.Context, req string) (string, error) {
					attemptCount++
					if attemptCount < 3 {
						return "", resilience.NewStatusCodeError(418, errors.New("I'm a teapot"))
					}
					return "success", nil
				}

				wrapper := resilience.NewRetryWrapper(
					client,
					resilience.WithMaxAttempts(5),
					resilience.WithConstantBackoff(10*time.Millisecond),
					resilience.WithErrorClassifier(customClassifier),
					resilience.WithRetryLogger(logger),
				)

				resp, err := wrapper.Execute(ctx, "test")
				Expect(err).NotTo(HaveOccurred())
				Expect(resp).To(Equal("success"))
				Expect(client.getCallCount()).To(Equal(3))
			})
		})

		Context("with unknown errors", func() {
			It("retries on unknown errors by default", func() {
				attemptCount := 0
				client.executeFunc = func(ctx context.Context, req string) (string, error) {
					attemptCount++
					if attemptCount < 3 {
						return "", errors.New("unknown network error")
					}
					return "success", nil
				}

				wrapper := resilience.NewRetryWrapper(
					client,
					resilience.WithMaxAttempts(5),
					resilience.WithConstantBackoff(10*time.Millisecond),
					resilience.WithErrorClassifier(classifier),
					resilience.WithRetryLogger(logger),
				)

				resp, err := wrapper.Execute(ctx, "test")
				Expect(err).NotTo(HaveOccurred())
				Expect(resp).To(Equal("success"))
				Expect(client.getCallCount()).To(Equal(3))
			})
		})
	})

	Describe("DefaultErrorClassifier", func() {
		It("uses default error classifier with sensible defaults", func() {
			classifier := resilience.DefaultErrorClassifier()

			wrapper := resilience.NewRetryWrapper(
				client,
				resilience.WithMaxAttempts(5),
				resilience.WithConstantBackoff(10*time.Millisecond),
				resilience.WithErrorClassifier(classifier),
				resilience.WithRetryLogger(logger),
			)

			attemptCount := 0
			client.executeFunc = func(ctx context.Context, req string) (string, error) {
				attemptCount++
				if attemptCount < 3 {
					return "", resilience.NewStatusCodeError(503, errors.New("service unavailable"))
				}
				return "success", nil
			}

			resp, err := wrapper.Execute(ctx, "test")
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).To(Equal("success"))
			Expect(client.getCallCount()).To(Equal(3))
		})
	})

	Describe("StatusCodeError", func() {
		It("wraps errors with status codes", func() {
			baseErr := errors.New("base error")
			statusErr := resilience.NewStatusCodeError(500, baseErr)

			Expect(statusErr.Error()).To(Equal("base error"))
			Expect(errors.Unwrap(statusErr)).To(Equal(baseErr))
		})

		It("works with error classification", func() {
			classifier := resilience.NewHTTPStatusClassifier()

			statusErr := resilience.NewStatusCodeError(503, errors.New("service unavailable"))
			Expect(classifier.IsRetryable(statusErr)).To(BeTrue())

			statusErr = resilience.NewStatusCodeError(400, errors.New("bad request"))
			Expect(classifier.IsRetryable(statusErr)).To(BeFalse())
		})
	})

	Describe("Real-world scenarios", func() {
		It("handles intermittent network failures", func() {
			attemptCount := 0
			client.executeFunc = func(ctx context.Context, req string) (string, error) {
				attemptCount++
				// Simulate intermittent failures
				if attemptCount%2 == 1 && attemptCount < 5 {
					return "", resilience.NewStatusCodeError(503, errors.New("service unavailable"))
				}
				return "success", nil
			}

			wrapper := resilience.NewRetryWrapper(
				client,
				resilience.WithMaxAttempts(10),
				resilience.WithExponentialBackoff(10*time.Millisecond, 200*time.Millisecond),
				resilience.WithRetryLogger(logger),
			)

			resp, err := wrapper.Execute(ctx, "test")
			Expect(err).NotTo(HaveOccurred())
			Expect(resp).To(Equal("success"))
		})

		It("handles rate limiting with backoff", func() {
			attemptCount := 0
			client.executeFunc = func(ctx context.Context, req string) (string, error) {
				attemptCount++
				if attemptCount < 4 {
					return "", pkgerrors.ErrRateLimited
				}
				return "success", nil
			}

			wrapper := resilience.NewRetryWrapper(
				client,
				resilience.WithMaxAttempts(5),
				resilience.WithExponentialBackoff(20*time.Millisecond, 500*time.Millisecond),
				resilience.WithRetryLogger(logger),
			)

			start := time.Now()
			resp, err := wrapper.Execute(ctx, "test")
			elapsed := time.Since(start)

			Expect(err).NotTo(HaveOccurred())
			Expect(resp).To(Equal("success"))
			Expect(client.getCallCount()).To(Equal(4))
			// Should have exponential delays
			Expect(elapsed).To(BeNumerically(">=", 50*time.Millisecond))
		})

		It("gives up on permanent errors quickly", func() {
			client.executeFunc = func(ctx context.Context, req string) (string, error) {
				return "", resilience.NewStatusCodeError(401, errors.New("unauthorized"))
			}

			wrapper := resilience.NewRetryWrapper(
				client,
				resilience.WithMaxAttempts(10),
				resilience.WithExponentialBackoff(100*time.Millisecond, time.Second),
				resilience.WithRetryLogger(logger),
			)

			start := time.Now()
			resp, err := wrapper.Execute(ctx, "test")
			elapsed := time.Since(start)

			Expect(err).To(HaveOccurred())
			Expect(resp).To(Equal(""))
			Expect(client.getCallCount()).To(Equal(1))
			// Should fail immediately without retries
			Expect(elapsed).To(BeNumerically("<", 50*time.Millisecond))
		})
	})
})
