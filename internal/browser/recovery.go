// Package browser provides Chrome crash detection and auto-restart capabilities.
package browser

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/cbenitezpy-ueno/retrodash-server/pkg/types"
)

// RecoveryManager handles browser crash detection and recovery.
type RecoveryManager struct {
	browser       Browser
	config        RecoveryConfig
	mu            sync.RWMutex
	restartCount  int
	lastRestart   time.Time
	onRecovery    func(error)
	stopChan      chan struct{}
	monitorCancel context.CancelFunc
}

// RecoveryConfig holds recovery configuration.
type RecoveryConfig struct {
	// CheckInterval is how often to check browser health.
	CheckInterval time.Duration
	// MaxRestarts is the maximum number of restarts before giving up.
	MaxRestarts int
	// RestartCooldown is the minimum time between restarts.
	RestartCooldown time.Duration
	// RecoveryTimeout is the timeout for recovery operations.
	RecoveryTimeout time.Duration
}

// DefaultRecoveryConfig returns sensible defaults.
func DefaultRecoveryConfig() RecoveryConfig {
	return RecoveryConfig{
		CheckInterval:   5 * time.Second,
		MaxRestarts:     5,
		RestartCooldown: 30 * time.Second,
		RecoveryTimeout: 60 * time.Second,
	}
}

// NewRecoveryManager creates a new recovery manager.
func NewRecoveryManager(browser Browser, config RecoveryConfig) *RecoveryManager {
	return &RecoveryManager{
		browser:  browser,
		config:   config,
		stopChan: make(chan struct{}),
	}
}

// OnRecovery sets a callback for when recovery occurs.
func (r *RecoveryManager) OnRecovery(fn func(error)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onRecovery = fn
}

// Start begins monitoring the browser.
func (r *RecoveryManager) Start(ctx context.Context) {
	monitorCtx, cancel := context.WithCancel(ctx)
	r.mu.Lock()
	r.monitorCancel = cancel
	r.mu.Unlock()

	go r.monitor(monitorCtx)
}

// Stop stops the recovery monitor.
func (r *RecoveryManager) Stop() {
	r.mu.RLock()
	cancel := r.monitorCancel
	r.mu.RUnlock()

	if cancel != nil {
		cancel()
	}
	close(r.stopChan)
}

// RestartCount returns the number of restarts performed.
func (r *RecoveryManager) RestartCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.restartCount
}

// monitor continuously checks browser health.
func (r *RecoveryManager) monitor(ctx context.Context) {
	ticker := time.NewTicker(r.config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-r.stopChan:
			return
		case <-ticker.C:
			if !r.isBrowserHealthy() {
				if err := r.attemptRecovery(ctx); err != nil {
					log.Printf("Recovery failed: %v", err)
				}
			}
		}
	}
}

// isBrowserHealthy checks if the browser is in a healthy state.
func (r *RecoveryManager) isBrowserHealthy() bool {
	status := r.browser.Status()
	return status == types.BrowserReady
}

// attemptRecovery tries to restart the browser.
func (r *RecoveryManager) attemptRecovery(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check restart limits
	if r.restartCount >= r.config.MaxRestarts {
		log.Printf("Max restarts (%d) reached, giving up", r.config.MaxRestarts)
		return ErrMaxRestartsExceeded
	}

	// Check cooldown
	if time.Since(r.lastRestart) < r.config.RestartCooldown {
		log.Printf("Still in cooldown, skipping restart")
		return nil
	}

	lastErr := r.browser.LastError()
	log.Printf("Browser unhealthy (status: %s, error: %v), attempting recovery...",
		r.browser.Status(), lastErr)

	// Stop existing browser
	if err := r.browser.Stop(); err != nil {
		log.Printf("Error stopping browser: %v", err)
	}

	// Wait a moment
	time.Sleep(time.Second)

	// Restart with timeout
	recoveryCtx, cancel := context.WithTimeout(ctx, r.config.RecoveryTimeout)
	defer cancel()

	if err := r.browser.Start(recoveryCtx); err != nil {
		log.Printf("Failed to restart browser: %v", err)
		return err
	}

	r.restartCount++
	r.lastRestart = time.Now()
	log.Printf("Browser recovered successfully (restart #%d)", r.restartCount)

	// Call recovery callback
	if r.onRecovery != nil {
		go r.onRecovery(lastErr)
	}

	return nil
}

// ResetRestartCount resets the restart counter (e.g., after successful operation).
func (r *RecoveryManager) ResetRestartCount() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.restartCount = 0
}
