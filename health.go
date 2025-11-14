package resilience

// HealthStatus represents the health status of a circuit breaker.
// It provides a strongly-typed alternative to map[string]interface{} for health checks.
type HealthStatus struct {
	// Healthy indicates whether the circuit breaker is in a healthy state.
	// True for closed and half-open states, false for open state.
	Healthy bool `json:"healthy"`

	// Status is a short string description of the state ("closed", "half-open", "open", "unknown").
	Status string `json:"status"`

	// State is the full string representation of the circuit breaker state.
	State string `json:"state"`

	// Requests is the total number of requests in the current interval.
	Requests uint32 `json:"requests"`

	// TotalSuccesses is the total number of successful requests.
	TotalSuccesses uint32 `json:"total_successes"`

	// TotalFailures is the total number of failed requests.
	TotalFailures uint32 `json:"total_failures"`

	// ConsecutiveFailures is the number of consecutive failures.
	ConsecutiveFailures uint32 `json:"consecutive_failures"`

	// ConsecutiveSuccesses is the number of consecutive successes.
	ConsecutiveSuccesses uint32 `json:"consecutive_successes"`
}
