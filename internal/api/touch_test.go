package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cbenitezpy-ueno/retrodash-server/internal/browser"
	"github.com/cbenitezpy-ueno/retrodash-server/internal/health"
	"github.com/cbenitezpy-ueno/retrodash-server/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockTouchModeProvider implements health.ModeProvider for touch tests
type mockTouchModeProvider struct {
	browserMode bool
}

func (m *mockTouchModeProvider) IsBrowserMode() bool {
	return m.browserMode
}

// mockTouchBrowser implements browser.Browser for touch testing.
type mockTouchBrowser struct {
	status     types.BrowserStatus
	lastClickX int
	lastClickY int
	width      int
	height     int
}

func newMockTouchBrowser() *mockTouchBrowser {
	return &mockTouchBrowser{
		status: types.BrowserReady,
		width:  1920,
		height: 1080,
	}
}

func (m *mockTouchBrowser) Start(ctx context.Context) error { return nil }
func (m *mockTouchBrowser) Stop() error                     { return nil }
func (m *mockTouchBrowser) Status() types.BrowserStatus     { return m.status }
func (m *mockTouchBrowser) LastError() error                { return nil }
func (m *mockTouchBrowser) CaptureScreenshot(ctx context.Context, quality int) ([]byte, error) {
	return nil, nil
}
func (m *mockTouchBrowser) Click(ctx context.Context, x, y int) error {
	if m.status != types.BrowserReady {
		return browser.ErrBrowserNotReady
	}
	m.lastClickX = x
	m.lastClickY = y
	return nil
}
func (m *mockTouchBrowser) Drag(ctx context.Context, startX, startY, endX, endY int) error {
	if m.status != types.BrowserReady {
		return browser.ErrBrowserNotReady
	}
	return nil
}
func (m *mockTouchBrowser) ViewportSize() (width, height int) {
	return m.width, m.height
}

func TestTouchHandler_ValidTouch(t *testing.T) {
	mockBrowser := newMockTouchBrowser()
	touchHandler := browser.NewTouchHandler(mockBrowser)
	handlers := NewHandlers(health.NewChecker(nil, nil, &mockTouchModeProvider{browserMode: true}))
	handlers.SetTouchHandler(touchHandler)

	// Send TouchStart first
	startEvent := types.TouchEvent{
		X:    0.5,
		Y:    0.5,
		Type: types.TouchStart,
	}
	body, _ := json.Marshal(startEvent)
	req := httptest.NewRequest(http.MethodPost, "/touch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handlers.TouchHandler(rec, req)
	assert.Equal(t, http.StatusNoContent, rec.Code)

	// Send TouchEnd to complete the tap
	endEvent := types.TouchEvent{
		X:    0.5,
		Y:    0.5,
		Type: types.TouchEnd,
	}
	body, _ = json.Marshal(endEvent)
	req = httptest.NewRequest(http.MethodPost, "/touch", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handlers.TouchHandler(rec, req)
	assert.Equal(t, http.StatusNoContent, rec.Code)

	// Verify click coordinates (0.5 * 1920 = 960, 0.5 * 1080 = 540)
	assert.Equal(t, 960, mockBrowser.lastClickX)
	assert.Equal(t, 540, mockBrowser.lastClickY)
}

func TestTouchHandler_MethodNotAllowed(t *testing.T) {
	handlers := NewHandlers(health.NewChecker(nil, nil, &mockTouchModeProvider{browserMode: true}))

	req := httptest.NewRequest(http.MethodGet, "/touch", nil)
	rec := httptest.NewRecorder()

	handlers.TouchHandler(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestTouchHandler_NoHandler(t *testing.T) {
	handlers := NewHandlers(health.NewChecker(nil, nil, &mockTouchModeProvider{browserMode: true}))
	// touchHandler not set

	event := types.TouchEvent{X: 0.5, Y: 0.5, Type: types.TouchStart}
	body, _ := json.Marshal(event)

	req := httptest.NewRequest(http.MethodPost, "/touch", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	handlers.TouchHandler(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestTouchHandler_InvalidJSON(t *testing.T) {
	mockBrowser := newMockTouchBrowser()
	touchHandler := browser.NewTouchHandler(mockBrowser)
	handlers := NewHandlers(health.NewChecker(nil, nil, &mockTouchModeProvider{browserMode: true}))
	handlers.SetTouchHandler(touchHandler)

	req := httptest.NewRequest(http.MethodPost, "/touch", bytes.NewReader([]byte("invalid json")))
	rec := httptest.NewRecorder()

	handlers.TouchHandler(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp types.ErrorResponse
	json.NewDecoder(rec.Body).Decode(&resp)
	assert.Equal(t, "INVALID_REQUEST", resp.Code)
}

func TestTouchHandler_InvalidCoordinates(t *testing.T) {
	mockBrowser := newMockTouchBrowser()
	touchHandler := browser.NewTouchHandler(mockBrowser)
	handlers := NewHandlers(health.NewChecker(nil, nil, &mockTouchModeProvider{browserMode: true}))
	handlers.SetTouchHandler(touchHandler)

	tests := []struct {
		name  string
		event types.TouchEvent
	}{
		{"x too low", types.TouchEvent{X: -0.1, Y: 0.5, Type: types.TouchStart}},
		{"x too high", types.TouchEvent{X: 1.1, Y: 0.5, Type: types.TouchStart}},
		{"y too low", types.TouchEvent{X: 0.5, Y: -0.1, Type: types.TouchStart}},
		{"y too high", types.TouchEvent{X: 0.5, Y: 1.1, Type: types.TouchStart}},
		{"invalid type", types.TouchEvent{X: 0.5, Y: 0.5, Type: "invalid"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, _ := json.Marshal(tt.event)
			req := httptest.NewRequest(http.MethodPost, "/touch", bytes.NewReader(body))
			rec := httptest.NewRecorder()

			handlers.TouchHandler(rec, req)

			assert.Equal(t, http.StatusBadRequest, rec.Code)

			var resp types.ErrorResponse
			json.NewDecoder(rec.Body).Decode(&resp)
			assert.Equal(t, "INVALID_TOUCH", resp.Code)
		})
	}
}

func TestTouchHandler_ValidCoordinates(t *testing.T) {
	tests := []struct {
		name      string
		x         float64
		y         float64
		expectedX int
		expectedY int
	}{
		{"center", 0.5, 0.5, 960, 540},
		{"top-left", 0.0, 0.0, 0, 0},
		{"bottom-right", 1.0, 1.0, 1920, 1080},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Fresh browser and handler for each test
			mockBrowser := newMockTouchBrowser()
			touchHandler := browser.NewTouchHandler(mockBrowser)
			handlers := NewHandlers(health.NewChecker(nil, nil, &mockTouchModeProvider{browserMode: true}))
			handlers.SetTouchHandler(touchHandler)

			// Send TouchStart
			startEvent := types.TouchEvent{X: tt.x, Y: tt.y, Type: types.TouchStart}
			body, _ := json.Marshal(startEvent)
			req := httptest.NewRequest(http.MethodPost, "/touch", bytes.NewReader(body))
			rec := httptest.NewRecorder()
			handlers.TouchHandler(rec, req)
			require.Equal(t, http.StatusNoContent, rec.Code)

			// Send TouchEnd to trigger the click
			endEvent := types.TouchEvent{X: tt.x, Y: tt.y, Type: types.TouchEnd}
			body, _ = json.Marshal(endEvent)
			req = httptest.NewRequest(http.MethodPost, "/touch", bytes.NewReader(body))
			rec = httptest.NewRecorder()
			handlers.TouchHandler(rec, req)
			require.Equal(t, http.StatusNoContent, rec.Code)

			assert.Equal(t, tt.expectedX, mockBrowser.lastClickX)
			assert.Equal(t, tt.expectedY, mockBrowser.lastClickY)
		})
	}
}

func TestTouchHandler_AllEventTypes(t *testing.T) {
	mockBrowser := newMockTouchBrowser()
	touchHandler := browser.NewTouchHandler(mockBrowser)
	handlers := NewHandlers(health.NewChecker(nil, nil, &mockTouchModeProvider{browserMode: true}))
	handlers.SetTouchHandler(touchHandler)

	eventTypes := []types.TouchEventType{types.TouchStart, types.TouchMove, types.TouchEnd}

	for _, eventType := range eventTypes {
		t.Run(string(eventType), func(t *testing.T) {
			event := types.TouchEvent{X: 0.5, Y: 0.5, Type: eventType}
			body, _ := json.Marshal(event)

			req := httptest.NewRequest(http.MethodPost, "/touch", bytes.NewReader(body))
			rec := httptest.NewRecorder()

			handlers.TouchHandler(rec, req)

			assert.Equal(t, http.StatusNoContent, rec.Code)
		})
	}
}
