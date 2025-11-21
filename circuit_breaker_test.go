package resilience_test

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sony/gobreaker/v2"

	resilience "github.com/JohnPlummer/jp-go-resilience"
)

var _ = Describe("CircuitBreakerWrapper", func() {
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

	Describe("Default Configuration", func() {
		It("should create wrapper with default settings", func() {
			wrapper = resilience.NewCircuitBreakerWrapper(client)
			Expect(wrapper).NotTo(BeNil())
			Expect(wrapper.State()).To(Equal(resilience.StateClosed))
		})

		It("should have default ReadyToTrip function", func() {
			config := resilience.DefaultCircuitBreakerConfig()
			Expect(config.ReadyToTrip).NotTo(BeNil())

			// Test the 60% threshold with 3 requests
			counts := resilience.CircuitBreakerCounts{
				Requests:      3,
				TotalFailures: 2,
			}
			Expect(config.ReadyToTrip(counts)).To(BeTrue()) // 2/3 = 66.6% > 60%

			counts = resilience.CircuitBreakerCounts{
				Requests:      3,
				TotalFailures: 1,
			}
			Expect(config.ReadyToTrip(counts)).To(BeFalse()) // 1/3 = 33.3% < 60%
		})

		It("should have MaxRequests=3 in default config", func() {
			config := resilience.DefaultCircuitBreakerConfig()
			Expect(config.MaxRequests).To(Equal(uint32(3)))
		})

		It("should have Interval=10s in default config", func() {
			config := resilience.DefaultCircuitBreakerConfig()
			Expect(config.Interval).To(Equal(10 * time.Second))
		})

		It("should have Timeout=30s in default config", func() {
			config := resilience.DefaultCircuitBreakerConfig()
			Expect(config.Timeout).To(Equal(30 * time.Second))
		})
	})

	Describe("State Transitions", func() {
		Context("Closed to Open", func() {
			It("should trip circuit after 60% failure rate with 3+ requests", func() {
				// Use a classifier that considers all errors as circuit-tripping
				wrapper = resilience.NewCircuitBreakerWrapper(
					client,
					resilience.WithCircuitBreakerLogger(logger),
				)

				// Make 5 requests with 3 failures (60% failure rate)
				// Pattern: fail, fail, fail, success, success
				client.executeFunc = func(ctx context.Context, req string) (string, error) {
					return "", errors.New("error")
				}
				_, _ = wrapper.Execute(ctx, "test")
				_, _ = wrapper.Execute(ctx, "test")
				_, _ = wrapper.Execute(ctx, "test")

				client.executeFunc = func(ctx context.Context, req string) (string, error) {
					return "success", nil
				}
				_, _ = wrapper.Execute(ctx, "test")
				_, _ = wrapper.Execute(ctx, "test")

				// Circuit should now be open (tripped on 3rd failure)
				Expect(wrapper.State()).To(Equal(resilience.StateOpen))
			})

			It("should not trip circuit with less than 3 requests", func() {
				wrapper = resilience.NewCircuitBreakerWrapper(
					client,
					resilience.WithCircuitBreakerLogger(logger),
				)

				// Make 2 requests with 2 failures (100% failure rate)
				client.executeFunc = func(ctx context.Context, req string) (string, error) {
					return "", errors.New("error")
				}
				_, _ = wrapper.Execute(ctx, "test")
				_, _ = wrapper.Execute(ctx, "test")

				// Circuit should still be closed (not enough requests)
				Expect(wrapper.State()).To(Equal(resilience.StateClosed))
			})

			It("should not trip circuit with failure rate below 60%", func() {
				wrapper = resilience.NewCircuitBreakerWrapper(
					client,
					resilience.WithCircuitBreakerLogger(logger),
				)

				// Make 5 requests with 2 failures (40% failure rate)
				client.executeFunc = func(ctx context.Context, req string) (string, error) {
					return "", errors.New("error")
				}
				_, _ = wrapper.Execute(ctx, "test")
				_, _ = wrapper.Execute(ctx, "test")

				client.executeFunc = func(ctx context.Context, req string) (string, error) {
					return "success", nil
				}
				_, _ = wrapper.Execute(ctx, "test")
				_, _ = wrapper.Execute(ctx, "test")
				_, _ = wrapper.Execute(ctx, "test")

				// Circuit should still be closed (failure rate too low)
				Expect(wrapper.State()).To(Equal(resilience.StateClosed))
			})

			It("should trip circuit at exactly 60% failure rate", func() {
				wrapper = resilience.NewCircuitBreakerWrapper(
					client,
					resilience.WithCircuitBreakerLogger(logger),
				)

				// Make 5 requests with 3 failures (60% failure rate)
				client.executeFunc = func(ctx context.Context, req string) (string, error) {
					return "", errors.New("error")
				}
				_, _ = wrapper.Execute(ctx, "test")
				_, _ = wrapper.Execute(ctx, "test")
				_, _ = wrapper.Execute(ctx, "test")

				client.executeFunc = func(ctx context.Context, req string) (string, error) {
					return "success", nil
				}
				_, _ = wrapper.Execute(ctx, "test")
				_, _ = wrapper.Execute(ctx, "test")

				// Circuit should now be open
				Expect(wrapper.State()).To(Equal(resilience.StateOpen))
			})
		})

		Context("Open to Half-Open", func() {
			It("should transition to half-open after timeout", func() {
				wrapper = resilience.NewCircuitBreakerWrapper(
					client,
					resilience.WithTimeout(100*time.Millisecond),
					resilience.WithCircuitBreakerLogger(logger),
				)

				// Trip the circuit
				client.executeFunc = func(ctx context.Context, req string) (string, error) {
					return "", errors.New("error")
				}
				_, _ = wrapper.Execute(ctx, "test")
				_, _ = wrapper.Execute(ctx, "test")
				_, _ = wrapper.Execute(ctx, "test")

				Expect(wrapper.State()).To(Equal(resilience.StateOpen))

				// Wait for timeout
				time.Sleep(150 * time.Millisecond)

				// Make a successful request
				client.executeFunc = func(ctx context.Context, req string) (string, error) {
					return "success", nil
				}
				_, err := wrapper.Execute(ctx, "test")

				// Should transition to half-open (request allowed)
				Expect(err).To(BeNil())
				Expect(wrapper.State()).To(Equal(resilience.StateHalfOpen))
			})
		})

		Context("Half-Open to Closed", func() {
			It("should transition to closed after MaxRequests successes", func() {
				wrapper = resilience.NewCircuitBreakerWrapper(
					client,
					resilience.WithTimeout(100*time.Millisecond),
					resilience.WithMaxRequests(3),
					resilience.WithCircuitBreakerLogger(logger),
				)

				// Trip the circuit
				client.executeFunc = func(ctx context.Context, req string) (string, error) {
					return "", errors.New("error")
				}
				_, _ = wrapper.Execute(ctx, "test")
				_, _ = wrapper.Execute(ctx, "test")
				_, _ = wrapper.Execute(ctx, "test")

				Expect(wrapper.State()).To(Equal(resilience.StateOpen))

				// Wait for timeout
				time.Sleep(150 * time.Millisecond)

				// Make MaxRequests (3) successful requests
				client.executeFunc = func(ctx context.Context, req string) (string, error) {
					return "success", nil
				}
				_, _ = wrapper.Execute(ctx, "test")
				_, _ = wrapper.Execute(ctx, "test")
				_, _ = wrapper.Execute(ctx, "test")

				// Should transition to closed
				Expect(wrapper.State()).To(Equal(resilience.StateClosed))
			})
		})

		Context("Half-Open to Open", func() {
			It("should transition back to open on failure in half-open state", func() {
				wrapper = resilience.NewCircuitBreakerWrapper(
					client,
					resilience.WithTimeout(100*time.Millisecond),
					resilience.WithCircuitBreakerLogger(logger),
				)

				// Trip the circuit
				client.executeFunc = func(ctx context.Context, req string) (string, error) {
					return "", errors.New("error")
				}
				_, _ = wrapper.Execute(ctx, "test")
				_, _ = wrapper.Execute(ctx, "test")
				_, _ = wrapper.Execute(ctx, "test")

				Expect(wrapper.State()).To(Equal(resilience.StateOpen))

				// Wait for timeout
				time.Sleep(150 * time.Millisecond)

				// Make a successful request to enter half-open
				client.executeFunc = func(ctx context.Context, req string) (string, error) {
					return "success", nil
				}
				_, _ = wrapper.Execute(ctx, "test")
				Expect(wrapper.State()).To(Equal(resilience.StateHalfOpen))

				// Make a failing request
				client.executeFunc = func(ctx context.Context, req string) (string, error) {
					return "", errors.New("error")
				}
				_, _ = wrapper.Execute(ctx, "test")

				// Should transition back to open
				Expect(wrapper.State()).To(Equal(resilience.StateOpen))
			})
		})
	})

	Describe("MaxRequests Enforcement", func() {
		It("should allow MaxRequests in half-open state", func() {
			wrapper = resilience.NewCircuitBreakerWrapper(
				client,
				resilience.WithTimeout(100*time.Millisecond),
				resilience.WithMaxRequests(3),
				resilience.WithCircuitBreakerLogger(logger),
			)

			// Trip the circuit
			client.executeFunc = func(ctx context.Context, req string) (string, error) {
				return "", errors.New("error")
			}
			_, _ = wrapper.Execute(ctx, "test")
			_, _ = wrapper.Execute(ctx, "test")
			_, _ = wrapper.Execute(ctx, "test")

			// Wait for timeout
			time.Sleep(150 * time.Millisecond)

			// Reset call count
			client.resetCallCount()

			// Make MaxRequests successful requests
			client.executeFunc = func(ctx context.Context, req string) (string, error) {
				return "success", nil
			}
			for i := 0; i < 3; i++ {
				_, err := wrapper.Execute(ctx, "test")
				Expect(err).To(BeNil())
			}

			Expect(client.getCallCount()).To(Equal(3))
		})

		It("should reject requests exceeding MaxRequests in half-open state", func() {
			wrapper = resilience.NewCircuitBreakerWrapper(
				client,
				resilience.WithTimeout(100*time.Millisecond),
				resilience.WithMaxRequests(2),
				resilience.WithCircuitBreakerLogger(logger),
			)

			// Trip the circuit
			client.executeFunc = func(ctx context.Context, req string) (string, error) {
				return "", errors.New("error")
			}
			_, _ = wrapper.Execute(ctx, "test")
			_, _ = wrapper.Execute(ctx, "test")
			_, _ = wrapper.Execute(ctx, "test")

			// Wait for timeout
			time.Sleep(150 * time.Millisecond)

			// Set slow success function before spawning goroutines to avoid race
			client.executeFunc = func(ctx context.Context, req string) (string, error) {
				// Slow request to keep circuit in half-open
				time.Sleep(50 * time.Millisecond)
				return "success", nil
			}

			// Make MaxRequests + 1 requests concurrently
			var wg sync.WaitGroup
			results := make([]error, 5)
			for i := 0; i < 5; i++ {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()
					_, err := wrapper.Execute(ctx, "test")
					results[idx] = err
				}(i)
			}
			wg.Wait()

			// Count ErrTooManyRequests
			tooManyCount := 0
			for _, err := range results {
				if errors.Is(err, gobreaker.ErrTooManyRequests) {
					tooManyCount++
				}
			}

			// At least some requests should be rejected
			Expect(tooManyCount).To(BeNumerically(">", 0))
		})
	})

	Describe("Error Behavior", func() {
		It("should return ErrOpenState when circuit is open", func() {
			wrapper = resilience.NewCircuitBreakerWrapper(
				client,
				resilience.WithCircuitBreakerLogger(logger),
			)

			// Trip the circuit
			client.executeFunc = func(ctx context.Context, req string) (string, error) {
				return "", errors.New("error")
			}
			_, _ = wrapper.Execute(ctx, "test")
			_, _ = wrapper.Execute(ctx, "test")
			_, _ = wrapper.Execute(ctx, "test")

			Expect(wrapper.State()).To(Equal(resilience.StateOpen))

			// Try to make a request
			_, err := wrapper.Execute(ctx, "test")
			Expect(errors.Is(err, gobreaker.ErrOpenState)).To(BeTrue())
		})

		It("should not count rate limit errors as failures", func() {
			wrapper = resilience.NewCircuitBreakerWrapper(
				client,
				resilience.WithCircuitBreakerLogger(logger),
			)

			// Make 5 requests with rate limit errors (should not trip)
			client.executeFunc = func(ctx context.Context, req string) (string, error) {
				return "", resilience.NewStatusCodeError(429, errors.New("rate limited"))
			}
			for i := 0; i < 5; i++ {
				_, _ = wrapper.Execute(ctx, "test")
			}

			// Circuit should still be closed
			Expect(wrapper.State()).To(Equal(resilience.StateClosed))
		})

		It("should not count timeout errors as failures", func() {
			wrapper = resilience.NewCircuitBreakerWrapper(
				client,
				resilience.WithCircuitBreakerLogger(logger),
			)

			// Make 5 requests with timeout errors (should not trip)
			client.executeFunc = func(ctx context.Context, req string) (string, error) {
				return "", context.DeadlineExceeded
			}
			for i := 0; i < 5; i++ {
				_, _ = wrapper.Execute(ctx, "test")
			}

			// Circuit should still be closed
			Expect(wrapper.State()).To(Equal(resilience.StateClosed))
		})

		It("should count server errors as failures", func() {
			wrapper = resilience.NewCircuitBreakerWrapper(
				client,
				resilience.WithCircuitBreakerLogger(logger),
			)

			// Make 3 requests with 500 errors
			client.executeFunc = func(ctx context.Context, req string) (string, error) {
				return "", resilience.NewStatusCodeError(500, errors.New("server error"))
			}
			_, _ = wrapper.Execute(ctx, "test")
			_, _ = wrapper.Execute(ctx, "test")
			_, _ = wrapper.Execute(ctx, "test")

			// Circuit should be open
			Expect(wrapper.State()).To(Equal(resilience.StateOpen))
		})
	})

	Describe("Concurrent Requests", func() {
		It("should handle concurrent requests to closed circuit safely", func() {
			wrapper = resilience.NewCircuitBreakerWrapper(
				client,
				resilience.WithCircuitBreakerLogger(logger),
			)

			var wg sync.WaitGroup
			numGoroutines := 100

			client.executeFunc = func(ctx context.Context, req string) (string, error) {
				return "success", nil
			}

			for i := 0; i < numGoroutines; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					_, _ = wrapper.Execute(ctx, "test")
				}()
			}

			wg.Wait()
			Expect(client.getCallCount()).To(Equal(numGoroutines))
		})

		It("should handle concurrent requests to open circuit safely", func() {
			wrapper = resilience.NewCircuitBreakerWrapper(
				client,
				resilience.WithCircuitBreakerLogger(logger),
			)

			// Trip the circuit
			client.executeFunc = func(ctx context.Context, req string) (string, error) {
				return "", errors.New("error")
			}
			_, _ = wrapper.Execute(ctx, "test")
			_, _ = wrapper.Execute(ctx, "test")
			_, _ = wrapper.Execute(ctx, "test")

			Expect(wrapper.State()).To(Equal(resilience.StateOpen))

			// Make concurrent requests
			var wg sync.WaitGroup
			numGoroutines := 100
			rejectedCount := 0
			var mu sync.Mutex

			for i := 0; i < numGoroutines; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					_, err := wrapper.Execute(ctx, "test")
					if errors.Is(err, gobreaker.ErrOpenState) {
						mu.Lock()
						rejectedCount++
						mu.Unlock()
					}
				}()
			}

			wg.Wait()
			Expect(rejectedCount).To(Equal(numGoroutines))
		})

		It("should maintain accurate counts with concurrent requests", func() {
			// Use a never-trip function to ensure circuit stays closed during test
			wrapper = resilience.NewCircuitBreakerWrapper(
				client,
				resilience.WithInterval(0), // Disable interval clearing for test
				resilience.WithCircuitBreakerLogger(logger),
				resilience.WithReadyToTrip(func(counts resilience.CircuitBreakerCounts) bool {
					// Never trip - we want to count all requests
					return false
				}),
			)

			var wg sync.WaitGroup
			numGoroutines := 100
			successCount := 50
			failCount := 50

			// Pre-define functions to avoid race conditions
			successFunc := func(ctx context.Context, req string) (string, error) {
				return "success", nil
			}
			failFunc := func(ctx context.Context, req string) (string, error) {
				return "", errors.New("error")
			}

			for i := 0; i < successCount; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					client.setExecuteFunc(successFunc)
					_, _ = wrapper.Execute(ctx, "test")
				}()
			}

			for i := 0; i < failCount; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					client.setExecuteFunc(failFunc)
					_, _ = wrapper.Execute(ctx, "test")
				}()
			}

			wg.Wait()

			counts := wrapper.Counts()
			Expect(counts.Requests).To(Equal(uint32(numGoroutines)))
		})
	})

	Describe("GetHealth", func() {
		It("should return healthy status for closed circuit", func() {
			wrapper = resilience.NewCircuitBreakerWrapper(
				client,
				resilience.WithCircuitBreakerLogger(logger),
			)

			health := wrapper.GetHealth()
			Expect(health.Healthy).To(BeTrue())
			Expect(health.Status).To(Equal("closed"))
			Expect(health.State).To(Equal("closed"))
		})

		It("should return healthy status for half-open circuit", func() {
			wrapper = resilience.NewCircuitBreakerWrapper(
				client,
				resilience.WithTimeout(100*time.Millisecond),
				resilience.WithCircuitBreakerLogger(logger),
			)

			// Trip the circuit
			client.executeFunc = func(ctx context.Context, req string) (string, error) {
				return "", errors.New("error")
			}
			_, _ = wrapper.Execute(ctx, "test")
			_, _ = wrapper.Execute(ctx, "test")
			_, _ = wrapper.Execute(ctx, "test")

			// Wait for timeout
			time.Sleep(150 * time.Millisecond)

			// Make a successful request
			client.executeFunc = func(ctx context.Context, req string) (string, error) {
				return "success", nil
			}
			_, _ = wrapper.Execute(ctx, "test")

			health := wrapper.GetHealth()
			Expect(health.Healthy).To(BeTrue())
			Expect(health.Status).To(Equal("half-open"))
			Expect(health.State).To(Equal("half-open"))
		})

		It("should return unhealthy status for open circuit", func() {
			wrapper = resilience.NewCircuitBreakerWrapper(
				client,
				resilience.WithCircuitBreakerLogger(logger),
			)

			// Trip the circuit
			client.executeFunc = func(ctx context.Context, req string) (string, error) {
				return "", errors.New("error")
			}
			_, _ = wrapper.Execute(ctx, "test")
			_, _ = wrapper.Execute(ctx, "test")
			_, _ = wrapper.Execute(ctx, "test")

			health := wrapper.GetHealth()
			Expect(health.Healthy).To(BeFalse())
			Expect(health.Status).To(Equal("open"))
			Expect(health.State).To(Equal("open"))
		})

		It("should include accurate counts in health status", func() {
			wrapper = resilience.NewCircuitBreakerWrapper(
				client,
				resilience.WithInterval(0), // Disable interval clearing
				resilience.WithCircuitBreakerLogger(logger),
			)

			// Make 2 successful requests
			client.executeFunc = func(ctx context.Context, req string) (string, error) {
				return "success", nil
			}
			_, _ = wrapper.Execute(ctx, "test")
			_, _ = wrapper.Execute(ctx, "test")

			// Make 1 failed request
			client.executeFunc = func(ctx context.Context, req string) (string, error) {
				return "", errors.New("error")
			}
			_, _ = wrapper.Execute(ctx, "test")

			health := wrapper.GetHealth()
			Expect(health.Requests).To(Equal(uint32(3)))
			Expect(health.TotalSuccesses).To(Equal(uint32(2)))
			Expect(health.TotalFailures).To(Equal(uint32(1)))
		})
	})

	Describe("State and Counts Methods", func() {
		It("should return correct state", func() {
			wrapper = resilience.NewCircuitBreakerWrapper(
				client,
				resilience.WithCircuitBreakerLogger(logger),
			)

			Expect(wrapper.State()).To(Equal(resilience.StateClosed))

			// Trip the circuit
			client.executeFunc = func(ctx context.Context, req string) (string, error) {
				return "", errors.New("error")
			}
			_, _ = wrapper.Execute(ctx, "test")
			_, _ = wrapper.Execute(ctx, "test")
			_, _ = wrapper.Execute(ctx, "test")

			Expect(wrapper.State()).To(Equal(resilience.StateOpen))
		})

		It("should return correct counts", func() {
			wrapper = resilience.NewCircuitBreakerWrapper(
				client,
				resilience.WithInterval(0), // Disable interval clearing
				resilience.WithCircuitBreakerLogger(logger),
			)

			counts := wrapper.Counts()
			Expect(counts.Requests).To(Equal(uint32(0)))

			// Make some requests
			_, _ = wrapper.Execute(ctx, "test")
			_, _ = wrapper.Execute(ctx, "test")

			counts = wrapper.Counts()
			Expect(counts.Requests).To(Equal(uint32(2)))
			Expect(counts.TotalSuccesses).To(Equal(uint32(2)))
		})
	})

	Describe("Custom ReadyToTrip Function", func() {
		It("should use custom ReadyToTrip function", func() {
			tripped := false
			wrapper = resilience.NewCircuitBreakerWrapper(
				client,
				resilience.WithReadyToTrip(func(counts resilience.CircuitBreakerCounts) bool {
					// Trip after 5 consecutive failures
					tripped = counts.ConsecutiveFailures >= 5
					return tripped
				}),
				resilience.WithCircuitBreakerLogger(logger),
			)

			// Make 4 failures (should not trip)
			client.executeFunc = func(ctx context.Context, req string) (string, error) {
				return "", errors.New("error")
			}
			for i := 0; i < 4; i++ {
				_, _ = wrapper.Execute(ctx, "test")
			}
			Expect(wrapper.State()).To(Equal(resilience.StateClosed))

			// 5th failure should trip
			_, _ = wrapper.Execute(ctx, "test")
			Expect(wrapper.State()).To(Equal(resilience.StateOpen))
			Expect(tripped).To(BeTrue())
		})
	})

	Describe("OnStateChange Callback", func() {
		It("should call OnStateChange when state changes", func() {
			stateChanges := []string{}
			var mu sync.Mutex

			wrapper = resilience.NewCircuitBreakerWrapper(
				client,
				resilience.WithStateChangeHandler(func(name string, from, to resilience.CircuitBreakerState) {
					mu.Lock()
					defer mu.Unlock()
					stateChanges = append(stateChanges, from.String()+"->"+to.String())
				}),
				resilience.WithCircuitBreakerLogger(logger),
			)

			// Trip the circuit
			client.executeFunc = func(ctx context.Context, req string) (string, error) {
				return "", errors.New("error")
			}
			_, _ = wrapper.Execute(ctx, "test")
			_, _ = wrapper.Execute(ctx, "test")
			_, _ = wrapper.Execute(ctx, "test")

			mu.Lock()
			Expect(stateChanges).To(ContainElement("closed->open"))
			mu.Unlock()
		})
	})
})
