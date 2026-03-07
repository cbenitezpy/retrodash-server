package browser

import (
	"context"
	"sync"

	"github.com/cbenitezpy-ueno/retrodash-server/pkg/types"
)

// TouchHandler manages touch event translation and execution.
type TouchHandler struct {
	browser        Browser
	viewportWidth  int
	viewportHeight int

	// Touch state tracking for drag detection
	mu            sync.Mutex
	touchActive   bool
	startX        float64
	startY        float64
	lastX         float64
	lastY         float64
	hasMoved      bool
	moveThreshold float64 // minimum movement to be considered a drag
}

// NewTouchHandler creates a new touch handler.
func NewTouchHandler(browser Browser) *TouchHandler {
	width, height := browser.ViewportSize()
	return &TouchHandler{
		browser:        browser,
		viewportWidth:  width,
		viewportHeight: height,
		moveThreshold:  0.01, // 1% of screen = drag threshold
	}
}

// TranslateCoordinates converts normalized (0-1) coordinates to pixel coordinates.
func (h *TouchHandler) TranslateCoordinates(x, y float64) (pixelX, pixelY int) {
	pixelX = int(x * float64(h.viewportWidth))
	pixelY = int(y * float64(h.viewportHeight))
	return
}

// HandleTouch processes a touch event with drag detection.
func (h *TouchHandler) HandleTouch(ctx context.Context, event types.TouchEvent) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	switch event.Type {
	case types.TouchStart:
		// Start tracking touch
		h.touchActive = true
		h.startX = event.X
		h.startY = event.Y
		h.lastX = event.X
		h.lastY = event.Y
		h.hasMoved = false
		// Don't click yet - wait for TouchEnd to determine if tap or drag
		return nil

	case types.TouchMove:
		if !h.touchActive {
			return nil
		}
		// Check if movement exceeds threshold
		dx := event.X - h.startX
		dy := event.Y - h.startY
		if abs(dx) > h.moveThreshold || abs(dy) > h.moveThreshold {
			h.hasMoved = true
		}
		h.lastX = event.X
		h.lastY = event.Y
		return nil

	case types.TouchEnd:
		if !h.touchActive {
			return nil
		}
		h.touchActive = false

		if h.hasMoved {
			// This was a drag - execute drag from start to end position
			return h.browser.Drag(ctx,
				int(h.startX*float64(h.viewportWidth)),
				int(h.startY*float64(h.viewportHeight)),
				int(h.lastX*float64(h.viewportWidth)),
				int(h.lastY*float64(h.viewportHeight)),
			)
		}
		// This was a tap - execute click at start position
		pixelX, pixelY := h.TranslateCoordinates(h.startX, h.startY)
		return h.browser.Click(ctx, pixelX, pixelY)

	default:
		return nil
	}
}

// abs returns absolute value of float64
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// HandleDrag processes a drag sequence (start -> move(s) -> end).
func (h *TouchHandler) HandleDrag(ctx context.Context, startX, startY, endX, endY float64) error {
	startPixelX, startPixelY := h.TranslateCoordinates(startX, startY)
	endPixelX, endPixelY := h.TranslateCoordinates(endX, endY)

	return h.browser.Drag(ctx, startPixelX, startPixelY, endPixelX, endPixelY)
}

// ValidateCoordinates checks if coordinates are in valid range (0-1).
func ValidateCoordinates(x, y float64) bool {
	return x >= 0 && x <= 1 && y >= 0 && y <= 1
}

// ValidateTouchEvent validates a touch event.
func ValidateTouchEvent(event types.TouchEvent) bool {
	// Validate coordinates
	if !ValidateCoordinates(event.X, event.Y) {
		return false
	}

	// Validate event type
	switch event.Type {
	case types.TouchStart, types.TouchMove, types.TouchEnd:
		return true
	default:
		return false
	}
}
