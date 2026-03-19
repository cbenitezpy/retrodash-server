package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cbenitezpy-ueno/retrodash-server/internal/health"
	"github.com/cbenitezpy-ueno/retrodash-server/internal/stream"
	"github.com/stretchr/testify/assert"
)

// mockStreamModeProvider implements health.ModeProvider for stream tests
type mockStreamModeProvider struct {
	mode string
}

func (m *mockStreamModeProvider) Mode() string {
	if m.mode != "" {
		return m.mode
	}
	return "browser"
}

func TestStreamHandler_NoBroadcaster(t *testing.T) {
	modeProvider := &mockStreamModeProvider{mode: "browser"}
	handlers := NewHandlers(health.NewChecker(nil, nil, modeProvider))
	// broadcaster not set

	req := httptest.NewRequest(http.MethodGet, "/stream", nil)
	rec := httptest.NewRecorder()

	handlers.StreamHandler(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestStreamHandler_MethodNotAllowed(t *testing.T) {
	modeProvider := &mockStreamModeProvider{mode: "browser"}
	handlers := NewHandlers(health.NewChecker(nil, nil, modeProvider))

	req := httptest.NewRequest(http.MethodPost, "/stream", nil)
	rec := httptest.NewRecorder()

	handlers.StreamHandler(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestStreamHandler_Quality(t *testing.T) {
	broadcaster := stream.NewBroadcaster(stream.DefaultEncoder())
	modeProvider := &mockStreamModeProvider{mode: "browser"}
	handlers := NewHandlers(health.NewChecker(nil, nil, modeProvider))
	handlers.SetBroadcaster(broadcaster)

	// Use a fixed client ID for testing
	handlers.clientIDGen = func() string { return "test-client" }

	t.Run("default quality is high", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/stream", nil)
		rec := httptest.NewRecorder()

		// Cancel context quickly to end the stream
		ctx, cancel := context.WithTimeout(req.Context(), 50*time.Millisecond)
		defer cancel()
		req = req.WithContext(ctx)

		handlers.StreamHandler(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Header().Get("Content-Type"), "multipart/x-mixed-replace")
	})

	t.Run("low quality param", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/stream?quality=low", nil)
		rec := httptest.NewRecorder()

		ctx, cancel := context.WithTimeout(req.Context(), 50*time.Millisecond)
		defer cancel()
		req = req.WithContext(ctx)

		handlers.StreamHandler(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})
}

func TestStreamHandler_Headers(t *testing.T) {
	broadcaster := stream.NewBroadcaster(stream.DefaultEncoder())
	modeProvider := &mockStreamModeProvider{mode: "browser"}
	handlers := NewHandlers(health.NewChecker(nil, nil, modeProvider))
	handlers.SetBroadcaster(broadcaster)
	handlers.clientIDGen = func() string { return "test" }

	req := httptest.NewRequest(http.MethodGet, "/stream", nil)
	rec := httptest.NewRecorder()

	ctx, cancel := context.WithTimeout(req.Context(), 50*time.Millisecond)
	defer cancel()
	req = req.WithContext(ctx)

	handlers.StreamHandler(rec, req)

	// Check MJPEG headers
	contentType := rec.Header().Get("Content-Type")
	assert.True(t, strings.HasPrefix(contentType, "multipart/x-mixed-replace"))
	assert.Contains(t, contentType, "boundary=")

	assert.Contains(t, rec.Header().Get("Cache-Control"), "no-cache")
}

func TestStreamHandler_ClientCleanup(t *testing.T) {
	broadcaster := stream.NewBroadcaster(stream.DefaultEncoder())
	modeProvider := &mockStreamModeProvider{mode: "browser"}
	handlers := NewHandlers(health.NewChecker(nil, nil, modeProvider))
	handlers.SetBroadcaster(broadcaster)
	handlers.clientIDGen = func() string { return "cleanup-test" }

	req := httptest.NewRequest(http.MethodGet, "/stream", nil)
	rec := httptest.NewRecorder()

	// Very short context to test cleanup
	ctx, cancel := context.WithTimeout(req.Context(), 10*time.Millisecond)
	defer cancel()
	req = req.WithContext(ctx)

	// Before handler
	assert.Equal(t, 0, broadcaster.ActiveClients())

	handlers.StreamHandler(rec, req)

	// After handler - client should be cleaned up
	assert.Equal(t, 0, broadcaster.ActiveClients())
}

func TestStreamHandler_ReceivesFrames(t *testing.T) {
	broadcaster := stream.NewBroadcaster(stream.DefaultEncoder())
	modeProvider := &mockStreamModeProvider{mode: "browser"}
	handlers := NewHandlers(health.NewChecker(nil, nil, modeProvider))
	handlers.SetBroadcaster(broadcaster)
	handlers.clientIDGen = func() string { return "frame-test" }

	// Create a request with cancellable context
	req := httptest.NewRequest(http.MethodGet, "/stream", nil)
	rec := httptest.NewRecorder()

	ctx, cancel := context.WithCancel(req.Context())
	req = req.WithContext(ctx)

	// Start handler in goroutine
	done := make(chan struct{})
	go func() {
		handlers.StreamHandler(rec, req)
		close(done)
	}()

	// Wait for client to connect
	time.Sleep(20 * time.Millisecond)

	// Broadcast a frame
	frame := &stream.Frame{
		Data:      []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10}, // JPEG header
		Timestamp: time.Now(),
		Sequence:  1,
		Quality:   85,
	}
	broadcaster.Broadcast(frame)

	// Give time for frame to be written
	time.Sleep(20 * time.Millisecond)

	// Cancel to stop the handler
	cancel()

	// Wait for handler to finish
	select {
	case <-done:
		// OK
	case <-time.After(500 * time.Millisecond):
		t.Fatal("handler did not finish")
	}

	// Check that we got some output
	body := rec.Body.String()
	assert.Contains(t, body, "--frame")
	assert.Contains(t, body, "Content-Type: image/jpeg")
}

func TestMJPEGWriter(t *testing.T) {
	rec := httptest.NewRecorder()

	writer, err := stream.NewMJPEGWriter(rec)
	assert.NoError(t, err)
	assert.NotNil(t, writer)

	writer.WriteHeaders()
	assert.Contains(t, rec.Header().Get("Content-Type"), "multipart/x-mixed-replace")

	// Write a frame
	frameData := []byte{0xFF, 0xD8, 0xFF, 0xE0} // JPEG magic bytes
	err = writer.WriteFrame(frameData)
	assert.NoError(t, err)

	body := rec.Body.String()
	assert.Contains(t, body, "--frame")
	assert.Contains(t, body, "Content-Type: image/jpeg")
	assert.Contains(t, body, "Content-Length: 4")
}

func TestGenerateClientID(t *testing.T) {
	id1 := generateClientID()
	time.Sleep(time.Nanosecond)
	id2 := generateClientID()

	assert.NotEmpty(t, id1)
	assert.NotEmpty(t, id2)
	assert.NotEqual(t, id1, id2)
}
