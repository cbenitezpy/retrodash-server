package health

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// MockStatusProvider implements StatusProvider and ErrorProvider
type MockStatusProvider struct {
	ready     bool
	lastError error
}

func (m *MockStatusProvider) IsReady() bool {
	return m.ready
}

func (m *MockStatusProvider) LastError() error {
	return m.lastError
}

// MockClientCounter implements ClientCounter
type MockClientCounter struct {
	count int
}

func (m *MockClientCounter) ActiveClients() int {
	return m.count
}

// MockModeProvider implements ModeProvider
type MockModeProvider struct {
	browserMode bool
}

func (m *MockModeProvider) IsBrowserMode() bool {
	return m.browserMode
}

func TestNewChecker(t *testing.T) {
	provider := &MockStatusProvider{ready: true}
	clients := &MockClientCounter{count: 5}
	modeProvider := &MockModeProvider{browserMode: true}

	checker := NewChecker(provider, clients, modeProvider)
	assert.NotNil(t, checker)
	assert.Equal(t, provider, checker.provider)
	assert.Equal(t, clients, checker.clientCounter)
	assert.Equal(t, modeProvider, checker.modeProvider)
	assert.False(t, checker.startTime.IsZero())
}

func TestCheck(t *testing.T) {
	t.Run("Healthy with all dependencies", func(t *testing.T) {
		provider := &MockStatusProvider{ready: true}
		clients := &MockClientCounter{count: 5}
		modeProvider := &MockModeProvider{browserMode: true}
		checker := NewChecker(provider, clients, modeProvider)
		// Simulate some uptime
		time.Sleep(10 * time.Millisecond)

		resp := checker.Check()

		assert.Equal(t, Version, resp.Version)
		assert.GreaterOrEqual(t, resp.Uptime, int64(0))
		assert.Equal(t, "ok", resp.Status)
		assert.Equal(t, "browser", resp.Mode)
		assert.Equal(t, "ready", resp.BrowserStatus)
		assert.Equal(t, 5, resp.ActiveClients)
		assert.Empty(t, resp.LastError)
	})

	t.Run("Unhealthy provider", func(t *testing.T) {
		provider := &MockStatusProvider{ready: false, lastError: errors.New("browser crashed")}
		clients := &MockClientCounter{count: 0}
		modeProvider := &MockModeProvider{browserMode: true}
		checker := NewChecker(provider, clients, modeProvider)

		resp := checker.Check()

		assert.Equal(t, "error", resp.Status)
		assert.Equal(t, "not_ready", resp.BrowserStatus)
		assert.Equal(t, "browser crashed", resp.LastError)
	})

	t.Run("No provider", func(t *testing.T) {
		modeProvider := &MockModeProvider{browserMode: false}
		checker := NewChecker(nil, nil, modeProvider)
		resp := checker.Check()

		assert.Equal(t, "ok", resp.Status)
		assert.Equal(t, "terminal", resp.Mode)
		assert.Equal(t, 0, resp.ActiveClients)
	})
}

// SimpleStatusProvider only implements StatusProvider
type SimpleStatusProvider struct {
	ready bool
}

func (s *SimpleStatusProvider) IsReady() bool {
	return s.ready
}

func TestCheck_SimpleProvider(t *testing.T) {
	provider := &SimpleStatusProvider{ready: false}
	modeProvider := &MockModeProvider{browserMode: true}
	checker := NewChecker(provider, nil, modeProvider)

	resp := checker.Check()
	assert.Equal(t, "error", resp.Status)
	assert.Equal(t, "not_ready", resp.BrowserStatus)
	assert.Empty(t, resp.LastError)
}

func TestIsHealthy(t *testing.T) {
	t.Run("Provider ready", func(t *testing.T) {
		provider := &MockStatusProvider{ready: true}
		modeProvider := &MockModeProvider{browserMode: true}
		checker := NewChecker(provider, nil, modeProvider)
		assert.True(t, checker.IsHealthy())
	})

	t.Run("Provider not ready", func(t *testing.T) {
		provider := &MockStatusProvider{ready: false}
		modeProvider := &MockModeProvider{browserMode: true}
		checker := NewChecker(provider, nil, modeProvider)
		assert.False(t, checker.IsHealthy())
	})

	t.Run("No provider", func(t *testing.T) {
		modeProvider := &MockModeProvider{browserMode: false}
		checker := NewChecker(nil, nil, modeProvider)
		assert.True(t, checker.IsHealthy())
	})
}
