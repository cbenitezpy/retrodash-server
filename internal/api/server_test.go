package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cbenitezpy-ueno/retrodash-server/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewServer(t *testing.T) {
	cfg := &config.Config{
		Port:            8080,
		DashboardURL:    "http://localhost:3000",
		FPS:             15,
		JPEGQualityHigh: 85,
		JPEGQualityLow:  50,
		ViewportWidth:   1920,
		ViewportHeight:  1080,
	}

	srv := NewServer(cfg)
	require.NotNil(t, srv)
	assert.NotNil(t, srv.httpServer)
	assert.NotNil(t, srv.mux)
}

func TestServer_RegisterHandler(t *testing.T) {
	cfg := &config.Config{
		Port:         8080,
		DashboardURL: "http://localhost:3000",
	}

	srv := NewServer(cfg)

	// Register a test handler
	srv.RegisterHandler("/test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test response"))
	})

	// Test the handler through the mux
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	srv.mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "test response", rec.Body.String())
}

func TestServer_Uptime(t *testing.T) {
	cfg := &config.Config{
		Port:         8080,
		DashboardURL: "http://localhost:3000",
	}

	srv := NewServer(cfg)

	// Wait a bit
	time.Sleep(10 * time.Millisecond)

	uptime := srv.Uptime()
	assert.Greater(t, uptime.Milliseconds(), int64(0))
}

func TestLoggingMiddleware(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := LoggingMiddleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	wrapped.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRecoveryMiddleware(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	})

	wrapped := RecoveryMiddleware(handler)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	// Should not panic
	wrapped.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestCORSMiddleware(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := CORSMiddleware(handler)

	t.Run("adds CORS headers", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()

		wrapped.ServeHTTP(rec, req)

		assert.Equal(t, "*", rec.Header().Get("Access-Control-Allow-Origin"))
		assert.Contains(t, rec.Header().Get("Access-Control-Allow-Methods"), "GET")
		assert.Contains(t, rec.Header().Get("Access-Control-Allow-Methods"), "POST")
	})

	t.Run("handles OPTIONS preflight", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodOptions, "/test", nil)
		rec := httptest.NewRecorder()

		wrapped.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNoContent, rec.Code)
	})
}

func TestResponseWriter_Flush(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := newResponseWriter(rec)

	// Write some data
	rw.Write([]byte("test"))

	// Flush should not panic
	rw.Flush()

	assert.Equal(t, "test", rec.Body.String())
}

func TestResponseWriter_WriteHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := newResponseWriter(rec)

	// First WriteHeader should work
	rw.WriteHeader(http.StatusCreated)
	assert.Equal(t, http.StatusCreated, rw.status)

	// Second WriteHeader should be ignored
	rw.WriteHeader(http.StatusOK)
	assert.Equal(t, http.StatusCreated, rw.status)
}

func TestMiddlewareChain(t *testing.T) {
	cfg := &config.Config{
		Port:         8080,
		DashboardURL: "http://localhost:3000",
	}

	srv := NewServer(cfg)

	// Register a handler that returns JSON
	srv.RegisterHandler("/api/test", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, `{"status":"ok"}`)
	})

	// Create a test server with middleware applied
	ts := httptest.NewServer(srv.applyMiddleware(srv.mux))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/test")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "*", resp.Header.Get("Access-Control-Allow-Origin"))

	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, `{"status":"ok"}`, string(body))
}
