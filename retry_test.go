package resilience_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	resilience "github.com/JohnPlummer/jp-go-resilience"
)

// mockClient implements ResilientClient for testing
type mockClient struct {
	executeFunc func(ctx context.Context, req string) (string, error)
	callCount   atomic.Int32
}

func (m *mockClient) Execute(ctx context.Context, req string) (string, error) {
	m.callCount.Add(1)
	return m.executeFunc(ctx, req)
}

func (m *mockClient) getCallCount() int {
	return int(m.callCount.Load())
}

// mockErrorClassifier for testing
type mockErrorClassifier struct {
	isRetryableFunc func(err error) bool
}

func (m *mockErrorClassifier) IsRetryable(err error) bool {
	return m.isRetryableFunc(err)
}

var _ = Describe("RetryWrapper", func() {
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
			Level: slog.LevelError, // Quiet during tests
		}))
	})

	AfterEach(func() {
		cancel()
	})

	Describe("NewRetryWrapper", func() {
		It("creates a wrapper with default config", func() {
			wrapper := resilience.NewRetryWrapper(client)
			Expect(wrapper).NotTo(BeNil())
		})

		It("creates a wrapper with custom options", func() {
			wrapper := resilience.NewRetryWrapper(
				client,
				resilience.WithMaxAttempts(5),
				resilience.WithExponentialBackoff(time.Millisecond, 100*time.Millisecond),
				resilience.WithRetryLogger(logger),
			)
			Expect(wrapper).NotTo(BeNil())
		})
	})

	Describe("Execute", func() {
		Context("successful request", func() {
			It("returns response on first attempt", func() {
				client.executeFunc = func(ctx context.Context, req string) (string, error) {
					return "success", nil
				}

				wrapper := resilience.NewRetryWrapper(
					client,
					resilience.WithMaxAttempts(3),
					resilience.WithConstantBackoff(10*time.Millisecond),
					resilience.WithRetryLogger(logger),
				)

				resp, err := wrapper.Execute(ctx, "test")
				Expect(err).NotTo(HaveOccurred())
				Expect(resp).To(Equal("success"))
				Expect(client.getCallCount()).To(Equal(1))

				// Check stats
				stats := wrapper.GetRetryStats()
				Expect(stats.TotalAttempts).To(Equal(int64(1)))
				Expect(stats.TotalRetries).To(Equal(int64(0)))
				Expect(stats.TotalSuccesses).To(Equal(int64(1)))
				Expect(stats.TotalFailures).To(Equal(int64(0)))
			})
		})

		Context("retryable errors", func() {
			It("retries on retryable error and succeeds", func() {
				attemptCount := 0
				client.executeFunc = func(ctx context.Context, req string) (string, error) {
					attemptCount++
					if attemptCount < 3 {
						return "", resilience.NewStatusCodeError(503, errors.New("service unavailable"))
					}
					return "success", nil
				}

				wrapper := resilience.NewRetryWrapper(
					client,
					resilience.WithMaxAttempts(5),
					resilience.WithConstantBackoff(10*time.Millisecond),
					resilience.WithRetryLogger(logger),
				)

				resp, err := wrapper.Execute(ctx, "test")
				Expect(err).NotTo(HaveOccurred())
				Expect(resp).To(Equal("success"))
				Expect(client.getCallCount()).To(Equal(3))

				// Check stats
				stats := wrapper.GetRetryStats()
				Expect(stats.TotalAttempts).To(Equal(int64(3)))
				Expect(stats.TotalRetries).To(Equal(int64(2)))
				Expect(stats.TotalSuccesses).To(Equal(int64(1)))
				Expect(stats.TotalFailures).To(Equal(int64(0)))
			})

			It("exhausts retries on persistent error", func() {
				client.executeFunc = func(ctx context.Context, req string) (string, error) {
					return "", resilience.NewStatusCodeError(503, errors.New("service unavailable"))
				}

				wrapper := resilience.NewRetryWrapper(
					client,
					resilience.WithMaxAttempts(3),
					resilience.WithConstantBackoff(10*time.Millisecond),
					resilience.WithRetryLogger(logger),
				)

				resp, err := wrapper.Execute(ctx, "test")
				Expect(err).To(HaveOccurred())
				Expect(resp).To(Equal(""))
				Expect(client.getCallCount()).To(Equal(3))

				// Check stats
				stats := wrapper.GetRetryStats()
				Expect(stats.TotalAttempts).To(Equal(int64(3)))
				Expect(stats.TotalRetries).To(Equal(int64(2)))
				Expect(stats.TotalSuccesses).To(Equal(int64(0)))
				Expect(stats.TotalFailures).To(Equal(int64(1)))
				Expect(stats.LastError).To(HaveOccurred())
			})
		})

		Context("non-retryable errors", func() {
			It("does not retry on non-retryable error", func() {
				client.executeFunc = func(ctx context.Context, req string) (string, error) {
					return "", resilience.NewStatusCodeError(400, errors.New("bad request"))
				}

				wrapper := resilience.NewRetryWrapper(
					client,
					resilience.WithMaxAttempts(3),
					resilience.WithConstantBackoff(10*time.Millisecond),
					resilience.WithRetryLogger(logger),
				)

				resp, err := wrapper.Execute(ctx, "test")
				Expect(err).To(HaveOccurred())
				Expect(resp).To(Equal(""))
				Expect(client.getCallCount()).To(Equal(1))

				// Check stats
				stats := wrapper.GetRetryStats()
				Expect(stats.TotalAttempts).To(Equal(int64(1)))
				Expect(stats.TotalRetries).To(Equal(int64(0)))
				Expect(stats.TotalSuccesses).To(Equal(int64(0)))
				Expect(stats.TotalFailures).To(Equal(int64(1)))
			})
		})

		Context("context cancellation", func() {
			It("returns immediately when context is already done", func() {
				canceledCtx, cancel := context.WithCancel(context.Background())
				cancel() // Cancel immediately

				client.executeFunc = func(ctx context.Context, req string) (string, error) {
					return "success", nil
				}

				wrapper := resilience.NewRetryWrapper(
					client,
					resilience.WithMaxAttempts(3),
					resilience.WithConstantBackoff(10*time.Millisecond),
					resilience.WithRetryLogger(logger),
				)

				resp, err := wrapper.Execute(canceledCtx, "test")
				Expect(err).To(Equal(context.Canceled))
				Expect(resp).To(Equal(""))
				Expect(client.getCallCount()).To(Equal(0))
			})

			It("stops retrying when context is canceled during retry", func() {
				attemptCount := 0
				client.executeFunc = func(ctx context.Context, req string) (string, error) {
					attemptCount++
					if attemptCount == 2 {
						cancel() // Cancel after second attempt
						time.Sleep(50 * time.Millisecond)
					}
					return "", resilience.NewStatusCodeError(503, errors.New("service unavailable"))
				}

				wrapper := resilience.NewRetryWrapper(
					client,
					resilience.WithMaxAttempts(5),
					resilience.WithConstantBackoff(10*time.Millisecond),
					resilience.WithRetryLogger(logger),
				)

				resp, err := wrapper.Execute(ctx, "test")
				Expect(err).To(Equal(context.Canceled))
				Expect(resp).To(Equal(""))
				Expect(client.getCallCount()).To(BeNumerically("<=", 3))
			})

			It("handles context deadline exceeded", func() {
				shortCtx, shortCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
				defer shortCancel()

				client.executeFunc = func(ctx context.Context, req string) (string, error) {
					time.Sleep(100 * time.Millisecond)
					return "", resilience.NewStatusCodeError(503, errors.New("service unavailable"))
				}

				wrapper := resilience.NewRetryWrapper(
					client,
					resilience.WithMaxAttempts(5),
					resilience.WithConstantBackoff(10*time.Millisecond),
					resilience.WithRetryLogger(logger),
				)

				resp, err := wrapper.Execute(shortCtx, "test")
				Expect(err).To(Or(Equal(context.DeadlineExceeded), MatchError(ContainSubstring("deadline exceeded"))))
				Expect(resp).To(Equal(""))
			})
		})

		Context("backoff strategies", func() {
			It("uses exponential backoff", func() {
				attemptCount := 0
				client.executeFunc = func(ctx context.Context, req string) (string, error) {
					attemptCount++
					if attemptCount < 3 {
						return "", resilience.NewStatusCodeError(503, errors.New("service unavailable"))
					}
					return "success", nil
				}

				start := time.Now()
				wrapper := resilience.NewRetryWrapper(
					client,
					resilience.WithMaxAttempts(3),
					resilience.WithExponentialBackoff(50*time.Millisecond, 500*time.Millisecond),
					resilience.WithRetryLogger(logger),
				)

				resp, err := wrapper.Execute(ctx, "test")
				elapsed := time.Since(start)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp).To(Equal("success"))
				// Should have delays: ~50ms, ~100ms (with jitter)
				Expect(elapsed).To(BeNumerically(">=", 100*time.Millisecond))
				Expect(elapsed).To(BeNumerically("<", 300*time.Millisecond))
			})

			It("uses constant backoff", func() {
				attemptCount := 0
				client.executeFunc = func(ctx context.Context, req string) (string, error) {
					attemptCount++
					if attemptCount < 3 {
						return "", resilience.NewStatusCodeError(503, errors.New("service unavailable"))
					}
					return "success", nil
				}

				start := time.Now()
				wrapper := resilience.NewRetryWrapper(
					client,
					resilience.WithMaxAttempts(3),
					resilience.WithConstantBackoff(50*time.Millisecond),
					resilience.WithRetryLogger(logger),
				)

				resp, err := wrapper.Execute(ctx, "test")
				elapsed := time.Since(start)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp).To(Equal("success"))
				// Should have delays: ~50ms, ~50ms (with jitter)
				Expect(elapsed).To(BeNumerically(">=", 80*time.Millisecond))
				Expect(elapsed).To(BeNumerically("<", 150*time.Millisecond))
			})

			It("uses fibonacci backoff", func() {
				attemptCount := 0
				client.executeFunc = func(ctx context.Context, req string) (string, error) {
					attemptCount++
					if attemptCount < 3 {
						return "", resilience.NewStatusCodeError(503, errors.New("service unavailable"))
					}
					return "success", nil
				}

				start := time.Now()
				wrapper := resilience.NewRetryWrapper(
					client,
					resilience.WithMaxAttempts(3),
					resilience.WithFibonacciBackoff(50*time.Millisecond, 500*time.Millisecond),
					resilience.WithRetryLogger(logger),
				)

				resp, err := wrapper.Execute(ctx, "test")
				elapsed := time.Since(start)

				Expect(err).NotTo(HaveOccurred())
				Expect(resp).To(Equal("success"))
				// Should have delays: ~50ms, ~50ms (fibonacci: 1, 1, 2, 3, 5...)
				Expect(elapsed).To(BeNumerically(">=", 80*time.Millisecond))
				Expect(elapsed).To(BeNumerically("<", 200*time.Millisecond))
			})
		})

		Context("max attempts enforcement", func() {
			It("enforces max attempts limit", func() {
				client.executeFunc = func(ctx context.Context, req string) (string, error) {
					return "", resilience.NewStatusCodeError(503, errors.New("service unavailable"))
				}

				wrapper := resilience.NewRetryWrapper(
					client,
					resilience.WithMaxAttempts(5),
					resilience.WithConstantBackoff(10*time.Millisecond),
					resilience.WithRetryLogger(logger),
				)

				_, err := wrapper.Execute(ctx, "test")
				Expect(err).To(HaveOccurred())
				Expect(client.getCallCount()).To(Equal(5))
			})

			It("caps max attempts at 1000", func() {
				client.executeFunc = func(ctx context.Context, req string) (string, error) {
					// Succeed immediately to avoid long test
					return "success", nil
				}

				wrapper := resilience.NewRetryWrapper(
					client,
					resilience.WithMaxAttempts(2000), // Should be capped at 1000
					resilience.WithConstantBackoff(time.Microsecond),
					resilience.WithRetryLogger(logger),
				)

				resp, err := wrapper.Execute(ctx, "test")
				Expect(err).NotTo(HaveOccurred())
				Expect(resp).To(Equal("success"))
			})

			It("handles zero max attempts", func() {
				client.executeFunc = func(ctx context.Context, req string) (string, error) {
					return "", resilience.NewStatusCodeError(503, errors.New("service unavailable"))
				}

				wrapper := resilience.NewRetryWrapper(
					client,
					resilience.WithMaxAttempts(0),
					resilience.WithConstantBackoff(10*time.Millisecond),
					resilience.WithRetryLogger(logger),
				)

				_, err := wrapper.Execute(ctx, "test")
				Expect(err).To(HaveOccurred())
				Expect(client.getCallCount()).To(Equal(0))
			})

			It("handles negative max attempts", func() {
				client.executeFunc = func(ctx context.Context, req string) (string, error) {
					return "", resilience.NewStatusCodeError(503, errors.New("service unavailable"))
				}

				wrapper := resilience.NewRetryWrapper(
					client,
					resilience.WithMaxAttempts(-1),
					resilience.WithConstantBackoff(10*time.Millisecond),
					resilience.WithRetryLogger(logger),
				)

				_, err := wrapper.Execute(ctx, "test")
				Expect(err).To(HaveOccurred())
				Expect(client.getCallCount()).To(Equal(0))
			})
		})

		Context("custom error classifier", func() {
			It("uses custom error classifier", func() {
				customErr := errors.New("custom error")
				client.executeFunc = func(ctx context.Context, req string) (string, error) {
					return "", customErr
				}

				classifier := &mockErrorClassifier{
					isRetryableFunc: func(err error) bool {
						return errors.Is(err, customErr)
					},
				}

				wrapper := resilience.NewRetryWrapper(
					client,
					resilience.WithMaxAttempts(3),
					resilience.WithConstantBackoff(10*time.Millisecond),
					resilience.WithErrorClassifier(classifier),
					resilience.WithRetryLogger(logger),
				)

				_, err := wrapper.Execute(ctx, "test")
				Expect(err).To(Equal(customErr))
				Expect(client.getCallCount()).To(Equal(3))
			})
		})

		Context("thread safety", func() {
			It("handles concurrent requests safely", func() {
				successCount := atomic.Int32{}
				client.executeFunc = func(ctx context.Context, req string) (string, error) {
					successCount.Add(1)
					return "success", nil
				}

				wrapper := resilience.NewRetryWrapper(
					client,
					resilience.WithMaxAttempts(3),
					resilience.WithConstantBackoff(10*time.Millisecond),
					resilience.WithRetryLogger(logger),
				)

				const concurrency = 100
				var wg sync.WaitGroup
				wg.Add(concurrency)

				for i := 0; i < concurrency; i++ {
					go func() {
						defer wg.Done()
						resp, err := wrapper.Execute(ctx, "test")
						Expect(err).NotTo(HaveOccurred())
						Expect(resp).To(Equal("success"))
					}()
				}

				wg.Wait()
				Expect(int(successCount.Load())).To(Equal(concurrency))

				// Check stats are consistent
				stats := wrapper.GetRetryStats()
				Expect(stats.TotalAttempts).To(Equal(int64(concurrency)))
				Expect(stats.TotalSuccesses).To(Equal(int64(concurrency)))
			})

			It("tracks stats correctly with concurrent failures and retries", func() {
				attemptCounts := sync.Map{}
				client.executeFunc = func(ctx context.Context, req string) (string, error) {
					// Each goroutine succeeds on third attempt
					count, _ := attemptCounts.Load(req)
					if count == nil {
						count = 0
					}
					attemptCounts.Store(req, count.(int)+1)

					if count.(int) < 2 {
						return "", resilience.NewStatusCodeError(503, errors.New("service unavailable"))
					}
					return "success", nil
				}

				wrapper := resilience.NewRetryWrapper(
					client,
					resilience.WithMaxAttempts(5),
					resilience.WithConstantBackoff(10*time.Millisecond),
					resilience.WithRetryLogger(logger),
				)

				const concurrency = 50
				var wg sync.WaitGroup
				wg.Add(concurrency)

				for i := 0; i < concurrency; i++ {
					go func(id int) {
						defer wg.Done()
						resp, err := wrapper.Execute(ctx, "test")
						Expect(err).NotTo(HaveOccurred())
						Expect(resp).To(Equal("success"))
					}(i)
				}

				wg.Wait()

				// Check stats
				stats := wrapper.GetRetryStats()
				Expect(stats.TotalSuccesses).To(Equal(int64(concurrency)))
				Expect(stats.TotalFailures).To(Equal(int64(0)))
			})
		})

		Context("GetRetryStats", func() {
			It("returns accurate statistics", func() {
				attemptCount := 0
				client.executeFunc = func(ctx context.Context, req string) (string, error) {
					attemptCount++
					if attemptCount < 3 {
						return "", resilience.NewStatusCodeError(503, errors.New("service unavailable"))
					}
					return "success", nil
				}

				wrapper := resilience.NewRetryWrapper(
					client,
					resilience.WithMaxAttempts(5),
					resilience.WithConstantBackoff(10*time.Millisecond),
					resilience.WithRetryLogger(logger),
				)

				// First request succeeds after 2 retries
				resp, err := wrapper.Execute(ctx, "test1")
				Expect(err).NotTo(HaveOccurred())
				Expect(resp).To(Equal("success"))

				stats := wrapper.GetRetryStats()
				Expect(stats.TotalAttempts).To(Equal(int64(3)))
				Expect(stats.TotalRetries).To(Equal(int64(2)))
				Expect(stats.TotalSuccesses).To(Equal(int64(1)))
				Expect(stats.TotalFailures).To(Equal(int64(0)))
				Expect(stats.LastAttemptTime).NotTo(BeZero())
				Expect(stats.LastError).To(BeNil())

				// Second request fails
				attemptCount = 0
				client.executeFunc = func(ctx context.Context, req string) (string, error) {
					return "", resilience.NewStatusCodeError(503, errors.New("service unavailable"))
				}

				_, err = wrapper.Execute(ctx, "test2")
				Expect(err).To(HaveOccurred())

				stats = wrapper.GetRetryStats()
				Expect(stats.TotalAttempts).To(Equal(int64(8))) // 3 + 5
				Expect(stats.TotalRetries).To(Equal(int64(6)))  // 2 + 4
				Expect(stats.TotalSuccesses).To(Equal(int64(1)))
				Expect(stats.TotalFailures).To(Equal(int64(1)))
				Expect(stats.LastError).To(HaveOccurred())
			})
		})
	})
})
