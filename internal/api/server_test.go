package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cbenitezpy-ueno/retrodash-server/internal/config"
	"github.com/cbenitezpy-ueno/retrodash-server/internal/health"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockStatusProvider struct{ ready bool }

func (m *mockStatusProvider) IsReady() bool { return m.ready }

type mockServerModeProvider struct{ mode string }

func (m *mockServerModeProvider) Mode() string {
	if m.mode != "" {
		return m.mode
	}
	return "browser"
}

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
		_, _ = w.Write([]byte("test response"))
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
	_, _ = rw.Write([]byte("test"))

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
		_, _ = io.WriteString(w, `{"status":"ok"}`)
	})

	// Create a test server with middleware applied
	ts := httptest.NewServer(srv.applyMiddleware(srv.mux))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/test")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "*", resp.Header.Get("Access-Control-Allow-Origin"))

	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, `{"status":"ok"}`, string(body))
}

func TestServer_SetMetrics(t *testing.T) {
	cfg := &config.Config{
		Port:         8080,
		DashboardURL: "http://localhost:3000",
	}

	srv := NewServer(cfg)
	assert.Nil(t, srv.metrics)

	m := health.NewMetrics()
	srv.SetMetrics(m)
	assert.Equal(t, m, srv.metrics)
}

func TestPrometheusMiddleware(t *testing.T) {
	m := health.NewMetrics()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := PrometheusMiddleware(m)(handler)

	t.Run("increments counter for regular path", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		rec := httptest.NewRecorder()
		wrapped.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		// Verify the http_requests counter was incremented (1 time-series created)
		count := testutil.CollectAndCount(m.Registry(), "retrodash_http_requests_total")
		assert.Equal(t, 1, count)
	})

	t.Run("does not count /stream path", func(t *testing.T) {
		// Record the number of counter series before the stream request
		before := testutil.CollectAndCount(m.Registry(), "retrodash_http_requests_total")

		streamReq := httptest.NewRequest(http.MethodGet, "/stream", nil)
		streamRec := httptest.NewRecorder()
		wrapped.ServeHTTP(streamRec, streamReq)

		// No new label combination for /stream should appear
		after := testutil.CollectAndCount(m.Registry(), "retrodash_http_requests_total")
		assert.Equal(t, before, after)
	})
}

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/healthz", "/healthz"},
		{"/readyz", "/readyz"},
		{"/metrics", "/metrics"},
		{"/health", "/health"},
		{"/api/origins", "/api/origins"},
		{"/api/origins/", "/api/origins/"},
		{"/api/origins/abc-123-def", "/api/origins/{id}"},
		{"/api/origins/abc-123/connect", "/api/origins/{id}/connect"},
		{"/api/origins/allowed-commands", "/api/origins/allowed-commands"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, normalizePath(tt.input))
		})
	}
}

func TestServer_RegisterHealthRoutes(t *testing.T) {
	cfg := &config.Config{
		Port:         8080,
		DashboardURL: "http://localhost:3000",
	}

	srv := NewServer(cfg)
	m := health.NewMetrics()
	srv.SetMetrics(m)

	provider := &mockStatusProvider{ready: true}
	srv.RegisterHealthRoutes(provider, &mockServerModeProvider{mode: "browser"})

	t.Run("GET /healthz returns 200 ok", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "ok", rec.Body.String())
	})

	t.Run("GET /readyz with ready provider returns 200 ok", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "ok", rec.Body.String())
	})

	t.Run("GET /metrics returns 200 with prometheus content", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		rec := httptest.NewRecorder()
		srv.mux.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.True(t, strings.Contains(rec.Body.String(), "retrodash_"))
	})
}

func TestPrometheusMiddleware_disabled(t *testing.T) {
	cfg := &config.Config{
		Port:         8080,
		DashboardURL: "http://localhost:3000",
	}

	// metrics is nil — applyMiddleware should still work without panicking
	srv := NewServer(cfg)
	assert.Nil(t, srv.metrics)

	srv.RegisterHandler("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("pong"))
	})

	ts := httptest.NewServer(srv.applyMiddleware(srv.mux))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/ping")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "pong", string(body))
}

func TestMiddlewareChain_WithMetrics(t *testing.T) {
	cfg := &config.Config{
		Port:         8080,
		DashboardURL: "http://localhost:3000",
	}

	srv := NewServer(cfg)
	m := health.NewMetrics()
	srv.SetMetrics(m)

	srv.RegisterHandler("/api/metrics-test", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	ts := httptest.NewServer(srv.applyMiddleware(srv.mux))
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/api/metrics-test")
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Prometheus counter should have been incremented
	count := testutil.CollectAndCount(m.Registry(), "retrodash_http_requests_total")
	assert.Equal(t, 1, count)
}
