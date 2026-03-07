package browser

import (
	"context"
	"testing"

	"github.com/cbenitezpy-ueno/retrodash-server/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTouchHandler(t *testing.T) {
	browser := newMockBrowser()
	browser.width = 1920
	browser.height = 1080

	handler := NewTouchHandler(browser)
	require.NotNil(t, handler)
	assert.Equal(t, 1920, handler.viewportWidth)
	assert.Equal(t, 1080, handler.viewportHeight)
}

func TestTouchHandler_TranslateCoordinates(t *testing.T) {
	browser := newMockBrowser()
	browser.width = 1920
	browser.height = 1080
	handler := NewTouchHandler(browser)

	tests := []struct {
		name      string
		x, y      float64
		expectedX int
		expectedY int
	}{
		{"center", 0.5, 0.5, 960, 540},
		{"top-left", 0.0, 0.0, 0, 0},
		{"bottom-right", 1.0, 1.0, 1920, 1080},
		{"quarter", 0.25, 0.75, 480, 810},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pixelX, pixelY := handler.TranslateCoordinates(tt.x, tt.y)
			assert.Equal(t, tt.expectedX, pixelX)
			assert.Equal(t, tt.expectedY, pixelY)
		})
	}
}

func TestTouchHandler_HandleTouch_Start(t *testing.T) {
	browser := newMockBrowser()
	handler := NewTouchHandler(browser)

	event := types.TouchEvent{
		X:    0.5,
		Y:    0.5,
		Type: types.TouchStart,
	}

	err := handler.HandleTouch(context.Background(), event)
	assert.NoError(t, err)
}

func TestTouchHandler_HandleTouch_Move(t *testing.T) {
	browser := newMockBrowser()
	handler := NewTouchHandler(browser)

	event := types.TouchEvent{
		X:    0.6,
		Y:    0.6,
		Type: types.TouchMove,
	}

	err := handler.HandleTouch(context.Background(), event)
	assert.NoError(t, err)
}

func TestTouchHandler_HandleTouch_End(t *testing.T) {
	browser := newMockBrowser()
	handler := NewTouchHandler(browser)

	event := types.TouchEvent{
		X:    0.5,
		Y:    0.5,
		Type: types.TouchEnd,
	}

	err := handler.HandleTouch(context.Background(), event)
	assert.NoError(t, err)
}

func TestTouchHandler_HandleDrag(t *testing.T) {
	browser := newMockBrowser()
	handler := NewTouchHandler(browser)

	err := handler.HandleDrag(context.Background(), 0.1, 0.1, 0.9, 0.9)
	assert.NoError(t, err)
}

func TestValidateCoordinates(t *testing.T) {
	tests := []struct {
		name     string
		x, y     float64
		expected bool
	}{
		{"valid center", 0.5, 0.5, true},
		{"valid zero", 0.0, 0.0, true},
		{"valid one", 1.0, 1.0, true},
		{"invalid negative x", -0.1, 0.5, false},
		{"invalid negative y", 0.5, -0.1, false},
		{"invalid x > 1", 1.1, 0.5, false},
		{"invalid y > 1", 0.5, 1.1, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateCoordinates(tt.x, tt.y)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValidateTouchEvent(t *testing.T) {
	tests := []struct {
		name     string
		event    types.TouchEvent
		expected bool
	}{
		{
			"valid start",
			types.TouchEvent{X: 0.5, Y: 0.5, Type: types.TouchStart},
			true,
		},
		{
			"valid move",
			types.TouchEvent{X: 0.5, Y: 0.5, Type: types.TouchMove},
			true,
		},
		{
			"valid end",
			types.TouchEvent{X: 0.5, Y: 0.5, Type: types.TouchEnd},
			true,
		},
		{
			"invalid type",
			types.TouchEvent{X: 0.5, Y: 0.5, Type: "invalid"},
			false,
		},
		{
			"invalid x",
			types.TouchEvent{X: 1.5, Y: 0.5, Type: types.TouchStart},
			false,
		},
		{
			"invalid y",
			types.TouchEvent{X: 0.5, Y: -0.1, Type: types.TouchStart},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateTouchEvent(tt.event)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTouchHandler_BrowserNotReady(t *testing.T) {
	browser := newMockBrowser()
	browser.status = types.BrowserStarting
	handler := NewTouchHandler(browser)

	// TouchStart only stores state, no error expected
	startEvent := types.TouchEvent{
		X:    0.5,
		Y:    0.5,
		Type: types.TouchStart,
	}
	err := handler.HandleTouch(context.Background(), startEvent)
	assert.NoError(t, err)

	// TouchEnd tries to click, which should fail if browser not ready
	endEvent := types.TouchEvent{
		X:    0.5,
		Y:    0.5,
		Type: types.TouchEnd,
	}
	err = handler.HandleTouch(context.Background(), endEvent)
	assert.Error(t, err)
}

func TestAbs(t *testing.T) {
	tests := []struct {
		name     string
		input    float64
		expected float64
	}{
		{"positive", 5.0, 5.0},
		{"negative", -5.0, 5.0},
		{"zero", 0.0, 0.0},
		{"small negative", -0.001, 0.001},
		{"large positive", 1000.5, 1000.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := abs(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTouchHandler_DragSequence(t *testing.T) {
	browser := newMockBrowser()
	browser.width = 1920
	browser.height = 1080
	handler := NewTouchHandler(browser)

	// Start touch
	startEvent := types.TouchEvent{
		X:    0.1,
		Y:    0.1,
		Type: types.TouchStart,
	}
	err := handler.HandleTouch(context.Background(), startEvent)
	assert.NoError(t, err)

	// Move significantly (> threshold of 1%)
	moveEvent := types.TouchEvent{
		X:    0.5,
		Y:    0.5,
		Type: types.TouchMove,
	}
	err = handler.HandleTouch(context.Background(), moveEvent)
	assert.NoError(t, err)

	// End touch - should trigger drag, not click
	endEvent := types.TouchEvent{
		X:    0.5,
		Y:    0.5,
		Type: types.TouchEnd,
	}
	err = handler.HandleTouch(context.Background(), endEvent)
	assert.NoError(t, err)
	// Verify drag was called (not click)
	assert.True(t, browser.dragCalled)
	assert.False(t, browser.clickCalled)
}

func TestTouchHandler_TapSequence(t *testing.T) {
	browser := newMockBrowser()
	browser.width = 1920
	browser.height = 1080
	handler := NewTouchHandler(browser)

	// Start touch
	startEvent := types.TouchEvent{
		X:    0.5,
		Y:    0.5,
		Type: types.TouchStart,
	}
	err := handler.HandleTouch(context.Background(), startEvent)
	assert.NoError(t, err)

	// End touch without moving - should trigger click
	endEvent := types.TouchEvent{
		X:    0.5,
		Y:    0.5,
		Type: types.TouchEnd,
	}
	err = handler.HandleTouch(context.Background(), endEvent)
	assert.NoError(t, err)
	// Verify click was called (not drag)
	assert.True(t, browser.clickCalled)
	assert.False(t, browser.dragCalled)
}

func TestTouchHandler_MoveWithoutStart(t *testing.T) {
	browser := newMockBrowser()
	handler := NewTouchHandler(browser)

	// Move without start should be no-op
	moveEvent := types.TouchEvent{
		X:    0.5,
		Y:    0.5,
		Type: types.TouchMove,
	}
	err := handler.HandleTouch(context.Background(), moveEvent)
	assert.NoError(t, err)
}

func TestTouchHandler_EndWithoutStart(t *testing.T) {
	browser := newMockBrowser()
	handler := NewTouchHandler(browser)

	// End without start should be no-op
	endEvent := types.TouchEvent{
		X:    0.5,
		Y:    0.5,
		Type: types.TouchEnd,
	}
	err := handler.HandleTouch(context.Background(), endEvent)
	assert.NoError(t, err)
	assert.False(t, browser.clickCalled)
	assert.False(t, browser.dragCalled)
}

func TestTouchHandler_UnknownEventType(t *testing.T) {
	browser := newMockBrowser()
	handler := NewTouchHandler(browser)

	// Unknown event type should be no-op
	unknownEvent := types.TouchEvent{
		X:    0.5,
		Y:    0.5,
		Type: "unknown",
	}
	err := handler.HandleTouch(context.Background(), unknownEvent)
	assert.NoError(t, err)
}
