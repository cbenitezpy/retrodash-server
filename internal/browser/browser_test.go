package browser

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"testing"
	"time"

	"github.com/cbenitezpy-ueno/retrodash-server/internal/config"
	"github.com/cbenitezpy-ueno/retrodash-server/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// isChromeAvailable checks if Chrome is installed on the system.
// Tests that require a real Chrome browser should skip if this returns false.
func isChromeAvailable() bool {
	// Check common Chrome executable names
	for _, name := range []string{"google-chrome", "chromium", "chromium-browser"} {
		if _, err := exec.LookPath(name); err == nil {
			return true
		}
	}
	return false
}

func TestNewChromeBrowser(t *testing.T) {
	cfg := &config.Config{
		DashboardURL:   "http://localhost:3000",
		ViewportWidth:  1920,
		ViewportHeight: 1080,
	}

	browser := NewChromeBrowser(cfg)
	require.NotNil(t, browser)
	assert.Equal(t, types.BrowserStarting, browser.Status())
}

func TestBrowserState(t *testing.T) {
	state := NewBrowserState()

	// Initial state
	assert.Equal(t, types.BrowserStarting, state.Status())
	assert.Empty(t, state.CurrentURL())
	assert.Nil(t, state.LastError())
	assert.Zero(t, state.RestartCount())

	// Update status
	state.SetStatus(types.BrowserReady)
	assert.Equal(t, types.BrowserReady, state.Status())

	// Update URL
	state.SetCurrentURL("http://test.com")
	assert.Equal(t, "http://test.com", state.CurrentURL())

	// Record capture
	state.RecordCapture()
	assert.False(t, state.LastCaptureTime().IsZero())

	// Increment restart
	state.IncrementRestartCount()
	assert.Equal(t, 1, state.RestartCount())

	// Uptime should be positive
	assert.Greater(t, state.Uptime().Nanoseconds(), int64(0))
}

func TestChromeBrowser_ViewportSize(t *testing.T) {
	cfg := &config.Config{
		DashboardURL:   "http://localhost:3000",
		ViewportWidth:  1280,
		ViewportHeight: 720,
	}

	browser := NewChromeBrowser(cfg)
	width, height := browser.ViewportSize()

	assert.Equal(t, 1280, width)
	assert.Equal(t, 720, height)
}

func TestChromeBrowser_CaptureScreenshot_NotReady(t *testing.T) {
	cfg := &config.Config{
		DashboardURL:   "http://localhost:3000",
		ViewportWidth:  1920,
		ViewportHeight: 1080,
	}

	browser := NewChromeBrowser(cfg)

	// Browser not started yet
	_, err := browser.CaptureScreenshot(context.Background(), 80)
	assert.ErrorIs(t, err, ErrBrowserNotReady)
}

func TestChromeBrowser_Click_NotReady(t *testing.T) {
	cfg := &config.Config{
		DashboardURL:   "http://localhost:3000",
		ViewportWidth:  1920,
		ViewportHeight: 1080,
	}

	browser := NewChromeBrowser(cfg)

	err := browser.Click(context.Background(), 100, 100)
	assert.ErrorIs(t, err, ErrBrowserNotReady)
}

func TestChromeBrowser_Drag_NotReady(t *testing.T) {
	cfg := &config.Config{
		DashboardURL:   "http://localhost:3000",
		ViewportWidth:  1920,
		ViewportHeight: 1080,
	}

	browser := NewChromeBrowser(cfg)

	err := browser.Drag(context.Background(), 100, 100, 200, 200)
	assert.ErrorIs(t, err, ErrBrowserNotReady)
}

func TestChromeBrowser_checkDashboardReachable(t *testing.T) {
	// Create a test server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	cfg := &config.Config{
		DashboardURL:   ts.URL,
		ViewportWidth:  1920,
		ViewportHeight: 1080,
	}

	browser := NewChromeBrowser(cfg)
	err := browser.checkDashboardReachable(context.Background())
	assert.NoError(t, err)
}

func TestChromeBrowser_checkDashboardReachable_Unreachable(t *testing.T) {
	cfg := &config.Config{
		DashboardURL:   "http://localhost:59999", // Non-existent port
		ViewportWidth:  1920,
		ViewportHeight: 1080,
	}

	browser := NewChromeBrowser(cfg)
	err := browser.checkDashboardReachable(context.Background())
	assert.ErrorIs(t, err, ErrDashboardUnreachable)
}

func TestChromeBrowser_checkDashboardReachable_404(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	cfg := &config.Config{
		DashboardURL:   ts.URL,
		ViewportWidth:  1920,
		ViewportHeight: 1080,
	}

	browser := NewChromeBrowser(cfg)
	err := browser.checkDashboardReachable(context.Background())
	assert.ErrorIs(t, err, ErrDashboardUnreachable)
}

func TestChromeOptions(t *testing.T) {
	opts := ChromeOptions("", 1920, 1080, false, "")
	assert.NotEmpty(t, opts)
}

func TestChromeOptions_WithCustomPath(t *testing.T) {
	opts := ChromeOptions("/usr/bin/chromium", 1280, 720, false, "")
	assert.NotEmpty(t, opts)
}

func TestChromeOptions_WithHostResolverRules(t *testing.T) {
	opts := ChromeOptions("", 1920, 1080, false, "MAP grafana.local 192.168.1.100")
	assert.NotEmpty(t, opts)
}

func TestDefaultChromeOptions(t *testing.T) {
	opts := DefaultChromeOptions("")
	assert.NotEmpty(t, opts)
}

func TestIsGrafanaDashboard(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{"Grafana dashboard /d/", "/d/abc123/my-dashboard", true},
		{"Grafana dashboard /dashboard/", "/dashboard/db/my-dashboard", true},
		{"Not Grafana - root", "/", false},
		{"Not Grafana - api", "/api/dashboards", false},
		{"Not Grafana - login", "/login", false},
		{"Empty path", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isGrafanaDashboard(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestChromeBrowser_isLoginPage(t *testing.T) {
	cfg := &config.Config{
		DashboardURL:   "http://localhost:3000",
		ViewportWidth:  1920,
		ViewportHeight: 1080,
	}
	browser := NewChromeBrowser(cfg)

	tests := []struct {
		name     string
		url      string
		expected bool
	}{
		{"Login path", "http://localhost:3000/login", true},
		{"Signin path", "http://localhost:3000/signin", true},
		{"Auth path", "http://localhost:3000/auth/login", true},
		{"Authenticate path", "http://localhost:3000/authenticate", true},
		{"Login query param", "http://localhost:3000?login=true", true},
		{"Redirect query param", "http://localhost:3000?redirect=/dashboard", true},
		{"Dashboard path", "http://localhost:3000/d/abc/dashboard", false},
		{"Root path", "http://localhost:3000/", false},
		{"Invalid URL", "not-a-valid-url", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := browser.isLoginPage(tt.url)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestChromeBrowser_buildDashboardURL(t *testing.T) {
	tests := []struct {
		name        string
		cfg         *config.Config
		expectedURL string
		checkKiosk  bool
		checkTZ     bool
		checkAuth   bool
	}{
		{
			name: "Grafana dashboard with kiosk",
			cfg: &config.Config{
				DashboardURL:   "http://localhost:3000/d/abc/dashboard",
				ViewportWidth:  1920,
				ViewportHeight: 1080,
			},
			checkKiosk: true,
		},
		{
			name: "With timezone",
			cfg: &config.Config{
				DashboardURL:   "http://localhost:3000/d/abc/dashboard",
				ViewportWidth:  1920,
				ViewportHeight: 1080,
				Timezone:       "America/New_York",
			},
			checkTZ: true,
		},
		{
			name: "With API key",
			cfg: &config.Config{
				DashboardURL:   "http://localhost:3000/d/abc/dashboard",
				ViewportWidth:  1920,
				ViewportHeight: 1080,
				GrafanaAPIKey:  "secret-key",
			},
			checkAuth: true,
		},
		{
			name: "Invalid URL returns original",
			cfg: &config.Config{
				DashboardURL:   "://invalid",
				ViewportWidth:  1920,
				ViewportHeight: 1080,
			},
			expectedURL: "://invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			browser := NewChromeBrowser(tt.cfg)
			result := browser.buildDashboardURL()

			if tt.expectedURL != "" {
				assert.Equal(t, tt.expectedURL, result)
			}
			if tt.checkKiosk {
				assert.Contains(t, result, "kiosk=1")
			}
			if tt.checkTZ {
				assert.Contains(t, result, "tz=America%2FNew_York")
			}
			if tt.checkAuth {
				assert.Contains(t, result, "auth_token=secret-key")
			}
		})
	}
}

func TestLoginRequiredError(t *testing.T) {
	err := &LoginRequiredError{
		DashboardURL: "http://grafana.local/d/abc/dashboard",
		DetectedURL:  "http://grafana.local/login",
	}

	errMsg := err.Error()
	assert.Contains(t, errMsg, "Authentication required")
	assert.Contains(t, errMsg, "http://grafana.local/login")
	assert.Contains(t, errMsg, "[auth.anonymous]")
	assert.Contains(t, errMsg, "GRAFANA_API_KEY")
}

func TestChromeBrowser_IsReady(t *testing.T) {
	cfg := &config.Config{
		DashboardURL:   "http://localhost:3000",
		ViewportWidth:  1920,
		ViewportHeight: 1080,
	}
	browser := NewChromeBrowser(cfg)

	// Initially not ready
	assert.False(t, browser.IsReady())

	// Set status to ready
	browser.state.SetStatus(types.BrowserReady)
	assert.True(t, browser.IsReady())

	// Set status to error
	browser.state.SetStatus(types.BrowserError)
	assert.False(t, browser.IsReady())
}

func TestChromeBrowser_LastError(t *testing.T) {
	cfg := &config.Config{
		DashboardURL:   "http://localhost:3000",
		ViewportWidth:  1920,
		ViewportHeight: 1080,
	}
	browser := NewChromeBrowser(cfg)

	// Initially nil
	assert.Nil(t, browser.LastError())

	// Set an error
	testErr := ErrBrowserNotReady
	browser.state.SetLastError(testErr)
	assert.Equal(t, testErr, browser.LastError())
}

func TestChromeBrowser_State(t *testing.T) {
	cfg := &config.Config{
		DashboardURL:   "http://localhost:3000",
		ViewportWidth:  1920,
		ViewportHeight: 1080,
	}
	browser := NewChromeBrowser(cfg)

	state := browser.State()
	assert.NotNil(t, state)
	assert.Equal(t, browser.state, state)
}

func TestChromeBrowser_Stop_NoBrowser(t *testing.T) {
	cfg := &config.Config{
		DashboardURL:   "http://localhost:3000",
		ViewportWidth:  1920,
		ViewportHeight: 1080,
	}
	browser := NewChromeBrowser(cfg)

	// Should not panic when cancel is nil
	err := browser.Stop()
	assert.NoError(t, err)
	assert.Equal(t, types.BrowserError, browser.Status())
}

func TestBrowserState_SetLastError(t *testing.T) {
	state := NewBrowserState()

	// Initially nil
	assert.Nil(t, state.LastError())

	// Set error
	testErr := ErrDashboardUnreachable
	state.SetLastError(testErr)
	assert.Equal(t, testErr, state.LastError())

	// Clear error
	state.SetLastError(nil)
	assert.Nil(t, state.LastError())
}

func TestChromeBrowser_Navigate_NotReady(t *testing.T) {
	cfg := &config.Config{
		DashboardURL:   "http://localhost:3000",
		ViewportWidth:  1920,
		ViewportHeight: 1080,
	}

	b := NewChromeBrowser(cfg)

	// Browser not started yet (status is BrowserStarting)
	err := b.Navigate(context.Background(), "http://localhost:3000/d/new/dashboard")
	assert.ErrorIs(t, err, ErrBrowserNotReady)
}

func TestChromeBrowser_Navigate_BrowserStopped(t *testing.T) {
	cfg := &config.Config{
		DashboardURL:   "http://localhost:3000",
		ViewportWidth:  1920,
		ViewportHeight: 1080,
	}

	b := NewChromeBrowser(cfg)

	// Set status to ready but ctx is nil (simulates stopped browser)
	b.state.SetStatus(types.BrowserReady)
	b.ctx = nil

	err := b.Navigate(context.Background(), "http://localhost:3000/d/new/dashboard")
	assert.ErrorIs(t, err, ErrBrowserStopped)
}

func TestChromeBrowser_Navigate_Success(t *testing.T) {
	if !isChromeAvailable() {
		t.Skip("Chrome not available in CI environment")
	}
	// Create two test servers to navigate between
	ts1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html><body><div id='root'>Page 1</div></body></html>"))
	}))
	defer ts1.Close()

	ts2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html><body><div id='root'>Page 2</div></body></html>"))
	}))
	defer ts2.Close()

	cfg := &config.Config{
		DashboardURL:   ts1.URL,
		ViewportWidth:  800,
		ViewportHeight: 600,
	}

	b := NewChromeBrowser(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Start the browser
	err := b.Start(ctx)
	require.NoError(t, err)
	defer b.Stop()

	assert.Equal(t, types.BrowserReady, b.Status())

	// Navigate to second page
	err = b.Navigate(ctx, ts2.URL)
	assert.NoError(t, err)
	assert.Contains(t, b.state.CurrentURL(), ts2.URL)
}

func TestChromeBrowser_Navigate_NavigationFailed(t *testing.T) {
	if !isChromeAvailable() {
		t.Skip("Chrome not available in CI environment")
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html><body>OK</body></html>"))
	}))
	defer ts.Close()

	cfg := &config.Config{
		DashboardURL:   ts.URL,
		ViewportWidth:  800,
		ViewportHeight: 600,
	}

	b := NewChromeBrowser(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Start the browser
	err := b.Start(ctx)
	require.NoError(t, err)

	// Stop the browser to invalidate the internal context, but keep status as Ready
	// This simulates a browser whose Chrome process has died
	b.cancel()
	b.cancel = nil
	// ctx is now cancelled, so chromedp.Run will fail

	err = b.Navigate(ctx, "http://localhost:59999/nonexistent")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrNavigationFailed)
}

func TestChromeBrowser_CaptureScreenshot_BrowserStopped(t *testing.T) {
	cfg := &config.Config{
		DashboardURL:   "http://localhost:3000",
		ViewportWidth:  1920,
		ViewportHeight: 1080,
	}
	browser := NewChromeBrowser(cfg)

	// Set status to ready but ctx is nil
	browser.state.SetStatus(types.BrowserReady)
	browser.ctx = nil

	_, err := browser.CaptureScreenshot(context.Background(), 80)
	assert.ErrorIs(t, err, ErrBrowserStopped)
}

// Integration test - requires Chrome/Chromium installed
// Uncomment to run locally with Chrome available
/*
func TestChromeBrowser_StartStop(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html><body>Test</body></html>"))
	}))
	defer ts.Close()

	cfg := &config.Config{
		DashboardURL:   ts.URL,
		ViewportWidth:  800,
		ViewportHeight: 600,
	}

	browser := NewChromeBrowser(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := browser.Start(ctx)
	require.NoError(t, err)
	assert.Equal(t, types.BrowserReady, browser.Status())

	// Capture screenshot
	data, err := browser.CaptureScreenshot(ctx, 80)
	require.NoError(t, err)
	assert.NotEmpty(t, data)
	// JPEG starts with 0xFF 0xD8
	assert.Equal(t, byte(0xFF), data[0])
	assert.Equal(t, byte(0xD8), data[1])

	err = browser.Stop()
	assert.NoError(t, err)
}
*/
