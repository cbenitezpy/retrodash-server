// Package health provides health check functionality.
package health

import (
	"time"

	"github.com/cbenitezpy-ueno/retrodash-server/pkg/types"
)

// Version is the server version.
const Version = "1.0.0"

// StatusProvider provides status for health checks.
type StatusProvider interface {
	IsReady() bool
}

// ErrorProvider optionally provides last error.
type ErrorProvider interface {
	LastError() error
}

// ClientCounter provides active client count.
type ClientCounter interface {
	ActiveClients() int
}

// ModeProvider provides current mode information.
type ModeProvider interface {
	IsBrowserMode() bool
}

// Checker performs health checks.
type Checker struct {
	startTime     time.Time
	provider      StatusProvider
	clientCounter ClientCounter
	modeProvider  ModeProvider
}

// NewChecker creates a new health checker.
// The modeProvider should implement IsBrowserMode() for dynamic mode detection.
func NewChecker(provider StatusProvider, clients ClientCounter, modeProvider ModeProvider) *Checker {
	return &Checker{
		startTime:     time.Now(),
		provider:      provider,
		clientCounter: clients,
		modeProvider:  modeProvider,
	}
}

// Check returns the current health status.
func (c *Checker) Check() types.HealthResponse {
	resp := types.HealthResponse{
		Version: Version,
		Uptime:  int64(time.Since(c.startTime).Seconds()),
	}

	// Set current mode dynamically
	if c.modeProvider != nil {
		if c.modeProvider.IsBrowserMode() {
			resp.Mode = "browser"
		} else {
			resp.Mode = "terminal"
		}
	}

	// Check provider status
	if c.provider != nil {
		if c.provider.IsReady() {
			resp.Status = "ok"
			resp.BrowserStatus = "ready"
		} else {
			resp.Status = "error"
			resp.BrowserStatus = "not_ready"
			// Try to get error if provider supports it
			if ep, ok := c.provider.(ErrorProvider); ok {
				if err := ep.LastError(); err != nil {
					resp.LastError = err.Error()
				}
			}
		}
	} else {
		resp.Status = "ok"
	}

	// Add client count if available
	if c.clientCounter != nil {
		resp.ActiveClients = c.clientCounter.ActiveClients()
	}

	return resp
}

// IsHealthy returns true if the server is healthy.
func (c *Checker) IsHealthy() bool {
	if c.provider == nil {
		return true
	}
	return c.provider.IsReady()
}
