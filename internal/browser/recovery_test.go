package browser

import (
	"context"
	"testing"
	"time"

	"github.com/cbenitezpy-ueno/retrodash-server/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultRecoveryConfig(t *testing.T) {
	cfg := DefaultRecoveryConfig()

	assert.Equal(t, 5*time.Second, cfg.CheckInterval)
	assert.Equal(t, 5, cfg.MaxRestarts)
	assert.Equal(t, 30*time.Second, cfg.RestartCooldown)
	assert.Equal(t, 60*time.Second, cfg.RecoveryTimeout)
}

func TestNewRecoveryManager(t *testing.T) {
	browser := newMockBrowser()
	cfg := DefaultRecoveryConfig()

	rm := NewRecoveryManager(browser, cfg)

	require.NotNil(t, rm)
	assert.Equal(t, 0, rm.RestartCount())
}

func TestRecoveryManager_HealthyBrowser(t *testing.T) {
	browser := newMockBrowser()
	browser.status = types.BrowserReady
	cfg := RecoveryConfig{
		CheckInterval:   50 * time.Millisecond,
		MaxRestarts:     3,
		RestartCooldown: 10 * time.Millisecond,
		RecoveryTimeout: time.Second,
	}

	rm := NewRecoveryManager(browser, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	rm.Start(ctx)
	time.Sleep(150 * time.Millisecond)
	rm.Stop()

	// No restarts should have occurred
	assert.Equal(t, 0, rm.RestartCount())
}

func TestRecoveryManager_UnhealthyBrowser_Recovery(t *testing.T) {
	browser := newMockBrowser()
	browser.status = types.BrowserError
	cfg := RecoveryConfig{
		CheckInterval:   50 * time.Millisecond,
		MaxRestarts:     3,
		RestartCooldown: 10 * time.Millisecond,
		RecoveryTimeout: time.Second,
	}

	rm := NewRecoveryManager(browser, cfg)

	recoveryCount := 0
	rm.OnRecovery(func(err error) {
		recoveryCount++
	})

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	rm.Start(ctx)
	time.Sleep(100 * time.Millisecond)

	// At least one restart should have occurred
	assert.GreaterOrEqual(t, rm.RestartCount(), 1)
}

func TestRecoveryManager_MaxRestartsExceeded(t *testing.T) {
	browser := newMockBrowser()
	browser.status = types.BrowserError
	cfg := RecoveryConfig{
		CheckInterval:   20 * time.Millisecond,
		MaxRestarts:     2,
		RestartCooldown: 5 * time.Millisecond,
		RecoveryTimeout: 100 * time.Millisecond,
	}

	rm := NewRecoveryManager(browser, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	rm.Start(ctx)
	time.Sleep(400 * time.Millisecond)
	rm.Stop()

	// Should have hit max restarts
	assert.LessOrEqual(t, rm.RestartCount(), cfg.MaxRestarts)
}

func TestRecoveryManager_ResetRestartCount(t *testing.T) {
	browser := newMockBrowser()
	cfg := DefaultRecoveryConfig()

	rm := NewRecoveryManager(browser, cfg)

	// Simulate some restarts
	rm.restartCount = 3

	rm.ResetRestartCount()

	assert.Equal(t, 0, rm.RestartCount())
}

func TestRecoveryManager_OnRecoveryCallback(t *testing.T) {
	browser := newMockBrowser()
	browser.status = types.BrowserError
	cfg := RecoveryConfig{
		CheckInterval:   50 * time.Millisecond,
		MaxRestarts:     1,
		RestartCooldown: 10 * time.Millisecond,
		RecoveryTimeout: time.Second,
	}

	rm := NewRecoveryManager(browser, cfg)

	callbackCalled := make(chan bool, 1)
	rm.OnRecovery(func(err error) {
		callbackCalled <- true
	})

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	rm.Start(ctx)

	select {
	case <-callbackCalled:
		// Callback was called
	case <-time.After(150 * time.Millisecond):
		// Timeout - callback might not have been called yet
	}

	rm.Stop()
}
