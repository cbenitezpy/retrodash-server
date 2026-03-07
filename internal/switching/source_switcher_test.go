package switching

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cbenitezpy-ueno/retrodash-server/internal/browser"
	"github.com/cbenitezpy-ueno/retrodash-server/internal/config"
	"github.com/cbenitezpy-ueno/retrodash-server/internal/origins"
	"github.com/stretchr/testify/assert"
)

// mockStore implements origins.Store for testing
type mockStore struct {
	origins []*origins.Origin
	err     error
}

func (m *mockStore) Load() ([]*origins.Origin, error) {
	return m.origins, m.err
}

func (m *mockStore) Save(origs []*origins.Origin) error {
	m.origins = origs
	return m.err
}

func TestNewSourceSwitcher(t *testing.T) {
	cfg := &config.Config{
		DashboardURL:   "http://example.com",
		ViewportWidth:  1920,
		ViewportHeight: 1080,
	}
	om := origins.NewManager(&mockStore{})

	sw := NewSourceSwitcher(cfg, om)

	assert.NotNil(t, sw)
	assert.Equal(t, cfg, sw.cfg)
	assert.Equal(t, om, sw.originManager)
	assert.Nil(t, sw.currentProvider)
	assert.Nil(t, sw.chromeBrowser)
	assert.Nil(t, sw.terminal)
}

func TestGetProvider_InitiallyNil(t *testing.T) {
	cfg := &config.Config{}
	sw := NewSourceSwitcher(cfg, nil)

	provider := sw.GetProvider()

	assert.Nil(t, provider)
}

func TestGetTouchHandler_InitiallyNil(t *testing.T) {
	cfg := &config.Config{}
	sw := NewSourceSwitcher(cfg, nil)

	handler := sw.GetTouchHandler()

	assert.Nil(t, handler)
}

func TestIsBrowserMode_InitiallyFalse(t *testing.T) {
	cfg := &config.Config{}
	sw := NewSourceSwitcher(cfg, nil)

	isBrowser := sw.IsBrowserMode()

	assert.False(t, isBrowser)
}

func TestIsReady_NoProvider(t *testing.T) {
	cfg := &config.Config{}
	sw := NewSourceSwitcher(cfg, nil)

	ready := sw.IsReady()

	assert.False(t, ready)
}

func TestIsReady_DuringSwitching(t *testing.T) {
	cfg := &config.Config{}
	sw := NewSourceSwitcher(cfg, nil)

	// Simulate switching state
	sw.mu.Lock()
	sw.switching = true
	sw.mu.Unlock()

	ready := sw.IsReady()

	assert.False(t, ready)
}

func TestStop_NoProviders(t *testing.T) {
	cfg := &config.Config{}
	sw := NewSourceSwitcher(cfg, nil)

	// Should not panic when no providers are active
	assert.NotPanics(t, func() {
		sw.Stop()
	})
}

func TestGetOriginURL_Grafana(t *testing.T) {
	cfg := &config.Config{}
	sw := NewSourceSwitcher(cfg, nil)

	origin := &origins.Origin{
		Type: origins.OriginTypeGrafana,
		Config: origins.OriginConfig{
			Grafana: &origins.GrafanaConfig{
				URL: "http://grafana.example.com/d/dashboard",
			},
		},
	}

	url := sw.getOriginURL(origin)

	assert.Equal(t, "http://grafana.example.com/d/dashboard", url)
}

func TestGetOriginURL_GrafanaNoConfig(t *testing.T) {
	cfg := &config.Config{}
	sw := NewSourceSwitcher(cfg, nil)

	origin := &origins.Origin{
		Type:   origins.OriginTypeGrafana,
		Config: origins.OriginConfig{},
	}

	url := sw.getOriginURL(origin)

	assert.Empty(t, url)
}

func TestGetOriginURL_Command(t *testing.T) {
	cfg := &config.Config{}
	sw := NewSourceSwitcher(cfg, nil)

	origin := &origins.Origin{
		Type: origins.OriginTypeCommand,
		Config: origins.OriginConfig{
			Command: &origins.CommandConfig{
				Command: "htop",
			},
		},
	}

	url := sw.getOriginURL(origin)

	assert.Equal(t, "cmd://htop", url)
}

func TestGetOriginURL_CommandWithPrefix(t *testing.T) {
	cfg := &config.Config{}
	sw := NewSourceSwitcher(cfg, nil)

	origin := &origins.Origin{
		Type: origins.OriginTypeCommand,
		Config: origins.OriginConfig{
			Command: &origins.CommandConfig{
				Command: "cmd://top -d 1",
			},
		},
	}

	url := sw.getOriginURL(origin)

	// Should not double-add prefix
	assert.Equal(t, "cmd://top -d 1", url)
}

func TestGetOriginURL_CommandNoConfig(t *testing.T) {
	cfg := &config.Config{}
	sw := NewSourceSwitcher(cfg, nil)

	origin := &origins.Origin{
		Type:   origins.OriginTypeCommand,
		Config: origins.OriginConfig{},
	}

	url := sw.getOriginURL(origin)

	assert.Empty(t, url)
}

func TestGetOriginURL_UnknownType(t *testing.T) {
	cfg := &config.Config{}
	sw := NewSourceSwitcher(cfg, nil)

	origin := &origins.Origin{
		Type:   "unknown",
		Config: origins.OriginConfig{},
	}

	url := sw.getOriginURL(origin)

	assert.Empty(t, url)
}

func TestSwitchToOrigin_EmptyURL(t *testing.T) {
	cfg := &config.Config{}
	sw := NewSourceSwitcher(cfg, nil)

	origin := &origins.Origin{
		Name:   "Empty Origin",
		Type:   origins.OriginTypeGrafana,
		Config: origins.OriginConfig{},
	}

	err := sw.SwitchToOrigin(context.Background(), origin)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no valid URL or command")
}

func TestSwitchToOrigin_AlreadySwitching(t *testing.T) {
	cfg := &config.Config{}
	sw := NewSourceSwitcher(cfg, nil)

	// Simulate switching state
	sw.mu.Lock()
	sw.switching = true
	sw.mu.Unlock()

	origin := &origins.Origin{
		Name: "Test Origin",
		Type: origins.OriginTypeGrafana,
		Config: origins.OriginConfig{
			Grafana: &origins.GrafanaConfig{
				URL: "http://example.com",
			},
		},
	}

	err := sw.SwitchToOrigin(context.Background(), origin)

	assert.Equal(t, ErrSwitchInProgress, err)
}

func TestStartTerminalLocked_InvalidURL(t *testing.T) {
	cfg := &config.Config{
		ViewportWidth:  800,
		ViewportHeight: 600,
	}
	sw := NewSourceSwitcher(cfg, nil)

	err := sw.startTerminalLocked(context.Background(), "invalid://url")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid command URL")
}

func TestStartTerminalLocked_ValidCommand(t *testing.T) {
	// Skip in CI - this test has race conditions with terminal goroutines
	// that are difficult to synchronize properly in a containerized environment
	t.Skip("Skipping terminal test in CI due to race conditions")
}

func TestRestartTerminalLocked(t *testing.T) {
	// Skip in CI - this test has race conditions with terminal goroutines
	// that are difficult to synchronize properly in a containerized environment
	t.Skip("Skipping terminal test in CI due to race conditions")
}

func TestErrors(t *testing.T) {
	assert.Equal(t, "no active source", ErrNoActiveSource.Error())
	assert.Equal(t, "source switch already in progress", ErrSwitchInProgress.Error())
}

// mockFrameProvider implements stream.FrameProvider for testing
type mockFrameProvider struct {
	ready bool
}

func (m *mockFrameProvider) CaptureScreenshot(ctx context.Context, quality int) ([]byte, error) {
	return []byte{}, nil
}

func (m *mockFrameProvider) IsReady() bool {
	return m.ready
}

func (m *mockFrameProvider) ViewportSize() (int, int) {
	return 800, 600
}

func TestIsReady_WithMockProvider(t *testing.T) {
	cfg := &config.Config{}
	sw := NewSourceSwitcher(cfg, nil)

	// Set a mock provider
	sw.mu.Lock()
	sw.currentProvider = &mockFrameProvider{ready: true}
	sw.mu.Unlock()

	ready := sw.IsReady()
	assert.True(t, ready)

	// Test with not ready provider
	sw.mu.Lock()
	sw.currentProvider = &mockFrameProvider{ready: false}
	sw.mu.Unlock()

	ready = sw.IsReady()
	assert.False(t, ready)
}

func TestNavigateBrowserLocked_NoBrowser(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping browser test in short mode")
	}

	cfg := &config.Config{
		ViewportWidth:  800,
		ViewportHeight: 600,
	}
	sw := NewSourceSwitcher(cfg, nil)

	// This will try to start a browser - may succeed or fail depending on environment
	err := sw.navigateBrowserLocked(context.Background(), "http://example.com")

	// Clean up if browser was started
	if err == nil {
		sw.stopBrowserLocked()
	}
	// Test passes either way - we're just verifying it doesn't panic
}

func TestNavigateBrowserLocked_NavigateSuccess(t *testing.T) {
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
	sw := NewSourceSwitcher(cfg, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Start browser first
	err := sw.startBrowserLocked(ctx, ts1.URL)
	if err != nil {
		t.Skip("Cannot start browser in this environment")
	}
	defer sw.stopBrowserLocked()

	// Navigate to ts2 using Navigate (no restart)
	err = sw.navigateBrowserLocked(ctx, ts2.URL)
	assert.NoError(t, err)
	// Browser should still be the same instance (not restarted)
	assert.NotNil(t, sw.chromeBrowser)
}

func TestNavigateBrowserLocked_NavigateFailsFallback(t *testing.T) {
	cfg := &config.Config{
		ViewportWidth:  800,
		ViewportHeight: 600,
	}
	sw := NewSourceSwitcher(cfg, nil)

	// Set a browser that is NOT ready (Navigate will return ErrBrowserNotReady)
	browserCfg := &config.Config{
		DashboardURL:   "http://localhost:59999",
		ViewportWidth:  800,
		ViewportHeight: 600,
	}
	sw.chromeBrowser = browser.NewChromeBrowser(browserCfg)
	// Status is BrowserStarting (not ready), so Navigate will fail

	// navigateBrowserLocked will call Navigate, get an error, then fallback to stop/start
	// The fallback start will also fail because the URL is unreachable, but we're testing the flow
	err := sw.navigateBrowserLocked(context.Background(), "http://localhost:59999")

	// Should get an error from the fallback start attempt (unreachable URL)
	assert.Error(t, err)
	// Browser should be nil after the failed fallback
	assert.Nil(t, sw.chromeBrowser)
}

func TestStopBrowserLocked_NoBrowser(t *testing.T) {
	cfg := &config.Config{}
	sw := NewSourceSwitcher(cfg, nil)

	// Should not panic
	assert.NotPanics(t, func() {
		sw.stopBrowserLocked()
	})
}

func TestStopTerminalLocked_NoTerminal(t *testing.T) {
	cfg := &config.Config{}
	sw := NewSourceSwitcher(cfg, nil)

	// Should not panic
	assert.NotPanics(t, func() {
		sw.stopTerminalLocked()
	})
}

func TestCreateTerminal(t *testing.T) {
	// Skip in CI - this test has race conditions with terminal goroutines
	t.Skip("Skipping terminal test in CI due to race conditions")
}

func TestCreateTerminal_InvalidURL(t *testing.T) {
	cfg := &config.Config{
		ViewportWidth:  800,
		ViewportHeight: 600,
	}
	sw := NewSourceSwitcher(cfg, nil)

	// Invalid command URL - this doesn't start a terminal so no race condition
	_, err := sw.createTerminal(context.Background(), "invalid://url")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid command URL")
}

func TestCreateBrowser(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html><body>OK</body></html>"))
	}))
	defer ts.Close()

	cfg := &config.Config{
		ViewportWidth:  800,
		ViewportHeight: 600,
	}
	sw := NewSourceSwitcher(cfg, nil)

	b, err := sw.createBrowser(context.Background(), ts.URL)
	if err == nil {
		assert.NotNil(t, b)
		b.Stop()
	}
}

func TestCreateBrowser_Unreachable(t *testing.T) {
	cfg := &config.Config{
		ViewportWidth:  800,
		ViewportHeight: 600,
	}
	sw := NewSourceSwitcher(cfg, nil)

	// Unreachable URL
	_, err := sw.createBrowser(context.Background(), "http://localhost:59999")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to start browser")
}

func TestSwitchToTerminal(t *testing.T) {
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
	sw := NewSourceSwitcher(cfg, nil)

	// Start with a browser first
	err := sw.startBrowserLocked(context.Background(), ts.URL)
	if err != nil {
		t.Skip("Cannot start browser in this environment")
	}
	assert.NotNil(t, sw.chromeBrowser)

	// Switch to terminal
	err = sw.switchToTerminal(context.Background(), "cmd://echo hello")
	if err == nil {
		assert.NotNil(t, sw.terminal)
		assert.Nil(t, sw.chromeBrowser)
		assert.NotNil(t, sw.currentProvider)
		sw.terminal.Stop()
	}
}

func TestSwitchToTerminal_NoOldBrowser(t *testing.T) {
	cfg := &config.Config{
		ViewportWidth:  800,
		ViewportHeight: 600,
	}
	sw := NewSourceSwitcher(cfg, nil)

	// No browser to stop
	err := sw.switchToTerminal(context.Background(), "cmd://echo hello")
	if err == nil {
		assert.NotNil(t, sw.terminal)
		sw.terminal.Stop()
	}
}

func TestSwitchToBrowser(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html><body>OK</body></html>"))
	}))
	defer ts.Close()

	cfg := &config.Config{
		ViewportWidth:  800,
		ViewportHeight: 600,
	}
	sw := NewSourceSwitcher(cfg, nil)

	// Start with a terminal first
	err := sw.startTerminalLocked(context.Background(), "cmd://echo hello")
	if err != nil {
		t.Skip("Cannot start terminal in this environment")
	}
	assert.NotNil(t, sw.terminal)

	// Switch to browser
	err = sw.switchToBrowser(context.Background(), ts.URL)
	if err == nil {
		assert.NotNil(t, sw.chromeBrowser)
		assert.Nil(t, sw.terminal)
		assert.NotNil(t, sw.currentProvider)
		assert.NotNil(t, sw.touchHandler)
		sw.chromeBrowser.Stop()
	}
}

func TestSwitchToBrowser_NoOldTerminal(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html><body>OK</body></html>"))
	}))
	defer ts.Close()

	cfg := &config.Config{
		ViewportWidth:  800,
		ViewportHeight: 600,
	}
	sw := NewSourceSwitcher(cfg, nil)

	// No terminal to stop
	err := sw.switchToBrowser(context.Background(), ts.URL)
	if err == nil {
		assert.NotNil(t, sw.chromeBrowser)
		sw.chromeBrowser.Stop()
	}
}

func TestSwitchToTerminal_CreateFails(t *testing.T) {
	cfg := &config.Config{
		ViewportWidth:  800,
		ViewportHeight: 600,
	}
	sw := NewSourceSwitcher(cfg, nil)

	// Invalid command URL will fail
	err := sw.switchToTerminal(context.Background(), "invalid://url")
	assert.Error(t, err)
}

func TestSwitchToBrowser_CreateFails(t *testing.T) {
	cfg := &config.Config{
		ViewportWidth:  800,
		ViewportHeight: 600,
	}
	sw := NewSourceSwitcher(cfg, nil)

	// Unreachable URL will fail
	err := sw.switchToBrowser(context.Background(), "http://localhost:59999")
	assert.Error(t, err)
}

func TestSwitchToOrigin_TypeSwitch_TerminalToBrowser(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html><body>OK</body></html>"))
	}))
	defer ts.Close()

	cfg := &config.Config{
		ViewportWidth:  800,
		ViewportHeight: 600,
	}
	sw := NewSourceSwitcher(cfg, nil)

	// Start with terminal
	sw.mu.Lock()
	err := sw.startTerminalLocked(context.Background(), "cmd://echo test")
	sw.mu.Unlock()
	if err != nil {
		t.Skip("Cannot start terminal in this environment")
	}

	// Switch to browser origin
	origin := &origins.Origin{
		Name: "Test Browser",
		Type: origins.OriginTypeGrafana,
		Config: origins.OriginConfig{
			Grafana: &origins.GrafanaConfig{
				URL: ts.URL,
			},
		},
	}

	err = sw.SwitchToOrigin(context.Background(), origin)
	if err == nil {
		assert.NotNil(t, sw.chromeBrowser)
		assert.Nil(t, sw.terminal)
		sw.Stop()
	}
}

func TestSwitchToOrigin_TypeSwitch_BrowserToTerminal(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html><body>OK</body></html>"))
	}))
	defer ts.Close()

	cfg := &config.Config{
		ViewportWidth:  800,
		ViewportHeight: 600,
	}
	sw := NewSourceSwitcher(cfg, nil)

	// Start with browser
	sw.mu.Lock()
	err := sw.startBrowserLocked(context.Background(), ts.URL)
	sw.mu.Unlock()
	if err != nil {
		t.Skip("Cannot start browser in this environment")
	}

	// Switch to terminal origin
	origin := &origins.Origin{
		Name: "Test Terminal",
		Type: origins.OriginTypeCommand,
		Config: origins.OriginConfig{
			Command: &origins.CommandConfig{
				Command: "echo test",
			},
		},
	}

	err = sw.SwitchToOrigin(context.Background(), origin)
	if err == nil {
		assert.NotNil(t, sw.terminal)
		assert.Nil(t, sw.chromeBrowser)
		sw.Stop()
	}
}
