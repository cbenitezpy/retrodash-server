package browser

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"

	"github.com/cbenitezpy-ueno/retrodash-server/internal/config"
	"github.com/cbenitezpy-ueno/retrodash-server/pkg/types"
)

var (
	// ErrBrowserNotReady indicates the browser hasn't finished starting.
	ErrBrowserNotReady = errors.New("browser not ready")
	// ErrBrowserStopped indicates the browser has been stopped.
	ErrBrowserStopped = errors.New("browser stopped")
	// ErrDashboardUnreachable indicates the dashboard URL cannot be reached.
	ErrDashboardUnreachable = errors.New("dashboard URL is unreachable")
	// ErrNavigationFailed indicates page navigation failed.
	ErrNavigationFailed = errors.New("navigation failed")
	// ErrMaxRestartsExceeded indicates recovery attempts have been exhausted.
	ErrMaxRestartsExceeded = errors.New("maximum browser restarts exceeded")
	// ErrLoginRequired indicates the dashboard requires authentication.
	ErrLoginRequired = errors.New("dashboard requires authentication")
)

// LoginRequiredError provides detailed help for authentication setup.
type LoginRequiredError struct {
	DashboardURL string
	DetectedURL  string
}

func (e *LoginRequiredError) Error() string {
	return fmt.Sprintf(`Authentication required. The dashboard redirected to login.

Detected URL: %s

To fix this, choose one of these options:

OPTION 1: Enable Grafana Anonymous Access
------------------------------------------
Edit your grafana.ini or environment variables:

  [auth.anonymous]
  enabled = true
  org_name = Main Org.
  org_role = Viewer

Then restart Grafana.

OPTION 2: Use a Grafana API Key
-------------------------------
1. In Grafana, go to: Configuration > API Keys
2. Click "Add API key"
3. Name: "retrodash", Role: "Viewer"
4. Copy the key and run:

  docker run ... -e GRAFANA_API_KEY=your_key_here ...

Or pass it in the stream URL:
  http://bridge:8080/stream?api_key=your_key_here
`, e.DetectedURL)
}

// ChromeBrowser implements Browser using ChromeDP.
type ChromeBrowser struct {
	cfg    *config.Config
	state  *BrowserState
	mu     sync.RWMutex
	cancel context.CancelFunc
	ctx    context.Context
}

// NewChromeBrowser creates a new ChromeDP-based browser.
func NewChromeBrowser(cfg *config.Config) *ChromeBrowser {
	return &ChromeBrowser{
		cfg:   cfg,
		state: NewBrowserState(),
	}
}

// Start initializes Chrome and navigates to the dashboard URL.
// The provided context is used for startup validation only - the browser lifecycle
// is managed independently and terminated via Stop().
func (b *ChromeBrowser) Start(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.state.SetStatus(types.BrowserStarting)

	// Build dashboard URL (add API key if configured)
	dashboardURL := b.buildDashboardURL()

	// Verify dashboard is reachable before starting Chrome (uses provided context for timeout)
	if err := b.checkDashboardReachable(ctx); err != nil {
		b.state.SetStatus(types.BrowserError)
		b.state.SetLastError(err)
		return err
	}

	// Create allocator with background context - browser lifecycle is independent
	// of the startup context and is managed via Stop()
	opts := ChromeOptions(b.cfg.ChromePath, b.cfg.ViewportWidth, b.cfg.ViewportHeight, !b.cfg.VerifyTLS, b.cfg.HostResolverRules)
	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)

	// Create browser context with custom logger to suppress CDP unmarshal errors
	browserCtx, browserCancel := chromedp.NewContext(allocCtx,
		chromedp.WithErrorf(func(format string, args ...interface{}) {
			msg := fmt.Sprintf(format, args...)
			// Suppress known CDP unmarshal errors that don't affect functionality
			if !strings.Contains(msg, "unmarshal event") {
				log.Printf("ERROR: %s", msg)
			}
		}),
	)

	// Store cancel functions
	b.cancel = func() {
		browserCancel()
		allocCancel()
	}
	b.ctx = browserCtx

	// Navigate to dashboard
	log.Printf("Navigating to dashboard: %s", dashboardURL)
	if err := chromedp.Run(browserCtx,
		chromedp.Navigate(dashboardURL),
		chromedp.WaitReady("body", chromedp.ByQuery),
	); err != nil {
		b.state.SetStatus(types.BrowserError)
		b.state.SetLastError(fmt.Errorf("%w: %v", ErrNavigationFailed, err))
		b.cancel()
		return b.state.LastError()
	}

	// Wait for dashboard to fully render
	// Strategy: Try to wait for Grafana-specific elements, then add small buffer for animations
	log.Println("Waiting for dashboard to render...")
	if err := b.waitForDashboardReady(browserCtx); err != nil {
		log.Printf("Dashboard wait completed with: %v (continuing anyway)", err)
	}

	// Check for login redirect
	var currentURL string
	if err := chromedp.Run(browserCtx, chromedp.Location(&currentURL)); err == nil {
		if b.isLoginPage(currentURL) {
			b.state.SetStatus(types.BrowserError)
			loginErr := &LoginRequiredError{
				DashboardURL: b.cfg.DashboardURL,
				DetectedURL:  currentURL,
			}
			b.state.SetLastError(loginErr)
			b.cancel()
			return loginErr
		}
	}

	b.state.SetCurrentURL(dashboardURL)
	b.state.SetStatus(types.BrowserReady)
	log.Println("Browser ready, dashboard loaded")

	return nil
}

// buildDashboardURL constructs the dashboard URL with optional API key and kiosk mode.
func (b *ChromeBrowser) buildDashboardURL() string {
	u, err := url.Parse(b.cfg.DashboardURL)
	if err != nil {
		return b.cfg.DashboardURL
	}

	q := u.Query()

	// Auto-detect Grafana and enable kiosk mode for fullscreen
	if isGrafanaDashboard(u.Path) {
		q.Set("kiosk", "1")
		log.Println("Grafana dashboard detected, enabling kiosk mode")
	}

	// Add timezone if configured (Grafana uses 'tz' parameter)
	if b.cfg.Timezone != "" {
		q.Set("tz", b.cfg.Timezone)
		log.Printf("Using timezone: %s", b.cfg.Timezone)
	}

	// Add API key if configured
	if b.cfg.GrafanaAPIKey != "" {
		q.Set("auth_token", b.cfg.GrafanaAPIKey)
		log.Println("Using Grafana API key for authentication")
	}

	u.RawQuery = q.Encode()
	return u.String()
}

// isGrafanaDashboard checks if the URL path indicates a Grafana dashboard.
func isGrafanaDashboard(path string) bool {
	// Grafana dashboard paths: /d/xxx/name or /dashboard/db/name
	return strings.Contains(path, "/d/") || strings.Contains(path, "/dashboard/")
}

// isLoginPage checks if the URL indicates a login page.
func (b *ChromeBrowser) isLoginPage(currentURL string) bool {
	u, err := url.Parse(currentURL)
	if err != nil {
		return false
	}

	// Common login path patterns
	loginPaths := []string{"/login", "/signin", "/auth", "/authenticate"}
	for _, path := range loginPaths {
		if strings.Contains(u.Path, path) {
			return true
		}
	}

	// Check for login query parameters
	if u.Query().Get("login") != "" || u.Query().Get("redirect") != "" {
		return true
	}

	return false
}

// Stop shuts down the browser.
func (b *ChromeBrowser) Stop() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.cancel != nil {
		b.cancel()
		b.cancel = nil
	}
	b.state.SetStatus(types.BrowserError) // Using Error as "stopped" state
	log.Println("Browser stopped")
	return nil
}

// Restart stops and restarts the browser.
func (b *ChromeBrowser) Restart(ctx context.Context) error {
	log.Println("Restarting browser...")
	b.Stop()                    //nolint:errcheck // best-effort cleanup before restart
	time.Sleep(2 * time.Second) // Give Chrome time to fully stop
	return b.Start(ctx)
}

// Navigate navigates the existing browser to a new URL without restarting Chrome.
// This is more efficient than Stop/Start for switching between dashboards.
func (b *ChromeBrowser) Navigate(ctx context.Context, url string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.state.Status() != types.BrowserReady {
		return ErrBrowserNotReady
	}

	if b.ctx == nil {
		return ErrBrowserStopped
	}

	// Update the config URL
	b.cfg.DashboardURL = url
	dashboardURL := b.buildDashboardURL()

	log.Printf("Navigating to: %s", dashboardURL)

	// Navigate to the new URL
	if err := chromedp.Run(b.ctx,
		chromedp.Navigate(dashboardURL),
		chromedp.WaitReady("body", chromedp.ByQuery),
	); err != nil {
		b.state.SetLastError(fmt.Errorf("%w: %v", ErrNavigationFailed, err))
		return b.state.LastError()
	}

	// Wait for dashboard content to load
	if err := b.waitForDashboardReady(b.ctx); err != nil {
		log.Printf("Dashboard wait completed with: %v (continuing anyway)", err)
	}

	b.state.SetCurrentURL(dashboardURL)
	log.Printf("Navigation complete: %s", dashboardURL)

	return nil
}

// Status returns the current browser state.
func (b *ChromeBrowser) Status() types.BrowserStatus {
	return b.state.Status()
}

// IsReady returns true if the browser is ready for capture.
func (b *ChromeBrowser) IsReady() bool {
	return b.state.Status() == types.BrowserReady
}

// LastError returns the most recent error.
func (b *ChromeBrowser) LastError() error {
	return b.state.LastError()
}

// CaptureScreenshot captures the current viewport as JPEG.
func (b *ChromeBrowser) CaptureScreenshot(ctx context.Context, quality int) ([]byte, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.state.Status() != types.BrowserReady {
		return nil, ErrBrowserNotReady
	}

	if b.ctx == nil {
		return nil, ErrBrowserStopped
	}

	// Use a channel to implement timeout without deriving from potentially corrupted context
	type result struct {
		data []byte
		err  error
	}
	resultCh := make(chan result, 1)

	go func() {
		var buf []byte
		err := chromedp.Run(b.ctx, chromedp.ActionFunc(func(ctx context.Context) error {
			var captureErr error
			buf, captureErr = page.CaptureScreenshot().
				WithFormat(page.CaptureScreenshotFormatJpeg).
				WithQuality(int64(quality)).
				Do(ctx)
			return captureErr
		}))
		resultCh <- result{data: buf, err: err}
	}()

	select {
	case res := <-resultCh:
		if res.err != nil {
			return nil, fmt.Errorf("screenshot capture failed: %w", res.err)
		}
		b.state.RecordCapture()
		return res.data, nil
	case <-time.After(3 * time.Second):
		return nil, fmt.Errorf("screenshot capture failed: %w", context.DeadlineExceeded)
	}
}

// Click simulates a mouse click at the given pixel coordinates.
func (b *ChromeBrowser) Click(ctx context.Context, x, y int) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.state.Status() != types.BrowserReady {
		return ErrBrowserNotReady
	}

	return chromedp.Run(b.ctx,
		chromedp.MouseClickXY(float64(x), float64(y)),
	)
}

// Drag simulates a mouse drag from start to end coordinates.
func (b *ChromeBrowser) Drag(ctx context.Context, startX, startY, endX, endY int) error {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.state.Status() != types.BrowserReady {
		return ErrBrowserNotReady
	}

	// Proper drag sequence: move to start, press, move to end, release
	return chromedp.Run(b.ctx,
		// Move to start position
		chromedp.ActionFunc(func(ctx context.Context) error {
			return input.DispatchMouseEvent(input.MouseMoved, float64(startX), float64(startY)).Do(ctx)
		}),
		// Press mouse button
		chromedp.ActionFunc(func(ctx context.Context) error {
			return input.DispatchMouseEvent(input.MousePressed, float64(startX), float64(startY)).
				WithButton(input.Left).
				WithClickCount(1).
				Do(ctx)
		}),
		// Move to end position (drag)
		chromedp.ActionFunc(func(ctx context.Context) error {
			return input.DispatchMouseEvent(input.MouseMoved, float64(endX), float64(endY)).
				WithButton(input.Left).
				Do(ctx)
		}),
		// Release mouse button
		chromedp.ActionFunc(func(ctx context.Context) error {
			return input.DispatchMouseEvent(input.MouseReleased, float64(endX), float64(endY)).
				WithButton(input.Left).
				WithClickCount(1).
				Do(ctx)
		}),
	)
}

// ViewportSize returns the configured viewport dimensions.
func (b *ChromeBrowser) ViewportSize() (width, height int) {
	return b.cfg.ViewportWidth, b.cfg.ViewportHeight
}

// checkDashboardReachable verifies the dashboard URL is accessible.
func (b *ChromeBrowser) checkDashboardReachable(ctx context.Context) error {
	// Parse and validate URL
	u, err := url.Parse(b.cfg.DashboardURL)
	if err != nil {
		return fmt.Errorf("%w: invalid URL: %v", ErrDashboardUnreachable, err)
	}

	// Create HTTP client with timeout
	transport := &http.Transport{}
	if !b.cfg.VerifyTLS {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	client := &http.Client{
		Timeout:   10 * time.Second,
		Transport: transport,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, u.String(), nil)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDashboardUnreachable, err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDashboardUnreachable, err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Accept any 2xx or 3xx status as reachable
	if resp.StatusCode >= 400 {
		return fmt.Errorf("%w: HTTP %d", ErrDashboardUnreachable, resp.StatusCode)
	}

	return nil
}

// State returns the browser state (for health checks).
func (b *ChromeBrowser) State() *BrowserState {
	return b.state
}

// waitForDashboardReady waits for the dashboard to be fully loaded.
// It uses a combination of strategies:
// 1. Try to wait for Grafana-specific elements (react-grid-layout)
// 2. Fall back to waiting for common dashboard indicators
// 3. Add a small buffer for animations
func (b *ChromeBrowser) waitForDashboardReady(ctx context.Context) error {
	// Create a timeout context for the wait operations
	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Try Grafana-specific selector first (dashboard panels container)
	grafanaSelectors := []string{
		".react-grid-layout",             // Grafana dashboard grid
		".panel-container",               // Individual panels
		"[data-testid='dashboard-grid']", // Newer Grafana versions
	}

	for _, selector := range grafanaSelectors {
		err := chromedp.Run(waitCtx, chromedp.WaitVisible(selector, chromedp.ByQuery))
		if err == nil {
			// Found a Grafana element, wait a bit more for data to load
			time.Sleep(1 * time.Second)
			return nil
		}
	}

	// If not Grafana, try generic content indicators
	genericSelectors := []string{
		"main",
		"#root",
		"#app",
		".dashboard",
		".content",
	}

	for _, selector := range genericSelectors {
		err := chromedp.Run(waitCtx, chromedp.WaitVisible(selector, chromedp.ByQuery))
		if err == nil {
			time.Sleep(1 * time.Second)
			return nil
		}
	}

	// Fallback: just wait a reasonable time for any dynamic content
	time.Sleep(2 * time.Second)
	return fmt.Errorf("no known dashboard elements found, using fallback wait")
}
