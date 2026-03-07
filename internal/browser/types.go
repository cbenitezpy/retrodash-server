// Package browser provides headless Chrome/Chromium browser management via ChromeDP.
package browser

import (
	"context"
	"sync"
	"time"

	"github.com/cbenitezpy-ueno/retrodash-server/pkg/types"
)

// Browser represents a managed headless browser instance.
type Browser interface {
	// Start initializes and navigates to the dashboard URL.
	Start(ctx context.Context) error
	// Stop shuts down the browser.
	Stop() error
	// Status returns the current browser state.
	Status() types.BrowserStatus
	// LastError returns the most recent error.
	LastError() error
	// CaptureScreenshot captures the current viewport.
	CaptureScreenshot(ctx context.Context, quality int) ([]byte, error)
	// Click simulates a click at pixel coordinates.
	Click(ctx context.Context, x, y int) error
	// Drag simulates a drag from start to end coordinates.
	Drag(ctx context.Context, startX, startY, endX, endY int) error
	// ViewportSize returns current viewport dimensions.
	ViewportSize() (width, height int)
}

// BrowserState holds the runtime state of the browser.
type BrowserState struct {
	mu           sync.RWMutex
	status       types.BrowserStatus
	currentURL   string
	lastError    error
	lastCapture  time.Time
	startTime    time.Time
	restartCount int
}

// NewBrowserState creates a new BrowserState.
func NewBrowserState() *BrowserState {
	return &BrowserState{
		status:    types.BrowserStarting,
		startTime: time.Now(),
	}
}

// Status returns the current status.
func (s *BrowserState) Status() types.BrowserStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}

// SetStatus updates the status.
func (s *BrowserState) SetStatus(status types.BrowserStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = status
}

// CurrentURL returns the loaded URL.
func (s *BrowserState) CurrentURL() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentURL
}

// SetCurrentURL updates the URL.
func (s *BrowserState) SetCurrentURL(url string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.currentURL = url
}

// LastError returns the last error.
func (s *BrowserState) LastError() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastError
}

// SetLastError updates the last error.
func (s *BrowserState) SetLastError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastError = err
}

// RecordCapture updates the last capture time.
func (s *BrowserState) RecordCapture() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastCapture = time.Now()
}

// LastCaptureTime returns when the last capture occurred.
func (s *BrowserState) LastCaptureTime() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastCapture
}

// IncrementRestartCount increments the restart counter.
func (s *BrowserState) IncrementRestartCount() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.restartCount++
}

// RestartCount returns the number of restarts.
func (s *BrowserState) RestartCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.restartCount
}

// Uptime returns duration since browser started.
func (s *BrowserState) Uptime() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return time.Since(s.startTime)
}
