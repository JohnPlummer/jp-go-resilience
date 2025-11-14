package resilience_test

import (
	"context"
	"sync"
)

type mockCircuitBreakerClient struct {
	executeFunc func(ctx context.Context, req string) (string, error)
	mu          sync.Mutex
	callCount   int
}

func (m *mockCircuitBreakerClient) Execute(ctx context.Context, req string) (string, error) {
	m.mu.Lock()
	m.callCount++
	m.mu.Unlock()
	return m.executeFunc(ctx, req)
}

func (m *mockCircuitBreakerClient) getCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount
}

func (m *mockCircuitBreakerClient) resetCallCount() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callCount = 0
}
