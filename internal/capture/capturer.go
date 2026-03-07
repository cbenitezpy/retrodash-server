// Package capture defines the interface for screen capture sources.
package capture

import (
	"context"
)

// Capturer is the interface for screen capture sources (browser, terminal, etc).
type Capturer interface {
	// Start initializes the capture source.
	Start(ctx context.Context) error
	// Stop shuts down the capture source.
	Stop() error
	// CaptureScreenshot captures the current screen as JPEG.
	CaptureScreenshot(ctx context.Context, quality int) ([]byte, error)
	// ViewportSize returns the configured viewport dimensions.
	ViewportSize() (width, height int)
	// IsReady returns true if the capture source is ready.
	IsReady() bool
}
