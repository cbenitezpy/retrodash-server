package switching

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/cbenitezpy-ueno/retrodash-server/internal/browser"
	"github.com/cbenitezpy-ueno/retrodash-server/internal/config"
	"github.com/cbenitezpy-ueno/retrodash-server/internal/origins"
	"github.com/cbenitezpy-ueno/retrodash-server/internal/stream"
	"github.com/cbenitezpy-ueno/retrodash-server/internal/terminal"
)

var (
	// ErrNoActiveSource indicates no source is currently active.
	ErrNoActiveSource = errors.New("no active source")
	// ErrSwitchInProgress indicates a source switch is already in progress.
	ErrSwitchInProgress = errors.New("source switch already in progress")
)

// SourceSwitcher manages dynamic switching between frame providers.
type SourceSwitcher struct {
	cfg             *config.Config
	originManager   *origins.Manager
	currentProvider stream.FrameProvider
	chromeBrowser   *browser.ChromeBrowser
	terminal        *terminal.Terminal
	touchHandler    *browser.TouchHandler
	mu              sync.RWMutex
	switching       bool
}

// NewSourceSwitcher creates a new source switcher.
func NewSourceSwitcher(cfg *config.Config, originManager *origins.Manager) *SourceSwitcher {
	return &SourceSwitcher{
		cfg:           cfg,
		originManager: originManager,
	}
}

// StartInitialSource starts the initial source based on DASHBOARD_URL.
func (s *SourceSwitcher) StartInitialSource(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if terminal.IsCommandURL(s.cfg.DashboardURL) {
		return s.startTerminalLocked(ctx, s.cfg.DashboardURL)
	}
	return s.startBrowserLocked(ctx, s.cfg.DashboardURL)
}

// SwitchToOrigin switches the stream source to the specified origin.
// Uses granular locking to minimize lock contention during I/O operations.
func (s *SourceSwitcher) SwitchToOrigin(ctx context.Context, origin *origins.Origin) error {
	// Check if already switching (brief lock)
	s.mu.Lock()
	if s.switching {
		s.mu.Unlock()
		return ErrSwitchInProgress
	}
	s.switching = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.switching = false
		s.mu.Unlock()
	}()

	log.Printf("Switching to origin: %s (type: %s)", origin.Name, origin.Type)

	// Get the URL/command for the origin (no lock needed)
	targetURL := s.getOriginURL(origin)
	if targetURL == "" {
		return fmt.Errorf("origin %s has no valid URL or command", origin.Name)
	}

	// Determine current state (brief read lock)
	s.mu.RLock()
	isTerminalTarget := terminal.IsCommandURL(targetURL)
	isCurrentTerminal := s.terminal != nil && s.currentProvider == s.terminal
	hasBrowser := s.chromeBrowser != nil
	s.mu.RUnlock()

	// Handle same-type switches (can use existing optimized methods)
	if isTerminalTarget && isCurrentTerminal {
		// Terminal → Terminal: restart with new command
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.restartTerminalLocked(ctx, targetURL)
	}

	if !isTerminalTarget && !isCurrentTerminal && hasBrowser {
		// Browser → Browser: use Navigate (already optimized)
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.navigateBrowserLocked(ctx, targetURL)
	}

	// Type switch: create new provider OUTSIDE the lock to minimize contention
	if isTerminalTarget {
		// Browser → Terminal
		return s.switchToTerminal(ctx, targetURL)
	}
	// Terminal → Browser
	return s.switchToBrowser(ctx, targetURL)
}

// switchToTerminal switches from browser to terminal with minimal lock contention.
// Creates the terminal outside the lock, then swaps references briefly under lock.
func (s *SourceSwitcher) switchToTerminal(ctx context.Context, cmdURL string) error {
	// Create and start terminal OUTSIDE the lock
	newTerm, err := s.createTerminal(ctx, cmdURL)
	if err != nil {
		return err
	}

	// Brief lock to swap provider references
	s.mu.Lock()
	oldBrowser := s.chromeBrowser
	s.currentProvider = newTerm
	s.terminal = newTerm
	s.chromeBrowser = nil
	s.touchHandler = nil
	s.mu.Unlock()

	// Stop old browser OUTSIDE the lock
	if oldBrowser != nil {
		log.Println("Stopping old browser...")
		oldBrowser.Stop() //nolint:errcheck // best-effort cleanup
	}

	log.Println("Terminal started successfully")
	return nil
}

// switchToBrowser switches from terminal to browser with minimal lock contention.
// Creates the browser outside the lock, then swaps references briefly under lock.
func (s *SourceSwitcher) switchToBrowser(ctx context.Context, url string) error {
	// Create and start browser OUTSIDE the lock
	newBrowser, err := s.createBrowser(ctx, url)
	if err != nil {
		return err
	}

	// Brief lock to swap provider references
	s.mu.Lock()
	oldTerminal := s.terminal
	s.currentProvider = newBrowser
	s.chromeBrowser = newBrowser
	s.terminal = nil
	s.touchHandler = browser.NewTouchHandler(newBrowser)
	s.mu.Unlock()

	// Stop old terminal OUTSIDE the lock
	if oldTerminal != nil {
		log.Println("Stopping old terminal...")
		oldTerminal.Stop() //nolint:errcheck // best-effort cleanup
	}

	log.Println("Browser started successfully")
	return nil
}

// createTerminal creates and starts a new terminal without holding any locks.
func (s *SourceSwitcher) createTerminal(ctx context.Context, cmdURL string) (*terminal.Terminal, error) {
	cmd, args, ok := terminal.ParseCommandURL(cmdURL)
	if !ok {
		return nil, fmt.Errorf("invalid command URL: %s", cmdURL)
	}

	if len(args) > 0 {
		log.Printf("Starting terminal with command: %s %v", cmd, args)
	} else {
		log.Printf("Starting terminal with command: %s", cmd)
	}

	term := terminal.New(&terminal.Config{
		Command: cmd,
		Args:    args,
		Width:   s.cfg.ViewportWidth,
		Height:  s.cfg.ViewportHeight,
	})

	if err := term.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start terminal: %w", err)
	}

	return term, nil
}

// createBrowser creates and starts a new browser without holding any locks.
func (s *SourceSwitcher) createBrowser(ctx context.Context, url string) (*browser.ChromeBrowser, error) {
	log.Printf("Starting browser with URL: %s", url)

	// Create new config with the target URL
	browserCfg := *s.cfg
	browserCfg.DashboardURL = url

	b := browser.NewChromeBrowser(&browserCfg)

	if err := b.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start browser: %w", err)
	}

	return b, nil
}

// GetProvider returns the current frame provider.
func (s *SourceSwitcher) GetProvider() stream.FrameProvider {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentProvider
}

// GetTouchHandler returns the touch handler (only available in browser mode).
func (s *SourceSwitcher) GetTouchHandler() *browser.TouchHandler {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.touchHandler
}

// Stop stops all active providers.
func (s *SourceSwitcher) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.stopBrowserLocked()
	s.stopTerminalLocked()
}

// IsBrowserMode returns true if currently in browser mode.
func (s *SourceSwitcher) IsBrowserMode() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.chromeBrowser != nil && s.currentProvider == s.chromeBrowser
}

// getOriginURL returns the URL/command for an origin.
func (s *SourceSwitcher) getOriginURL(origin *origins.Origin) string {
	switch origin.Type {
	case origins.OriginTypeGrafana:
		if origin.Config.Grafana != nil {
			return origin.Config.Grafana.URL
		}
	case origins.OriginTypeCommand:
		if origin.Config.Command != nil {
			cmd := origin.Config.Command.Command
			// Don't add cmd:// prefix if already present
			if !strings.HasPrefix(cmd, "cmd://") {
				cmd = "cmd://" + cmd
			}
			return cmd
		}
	}
	return ""
}

// startBrowserLocked starts the browser with the given URL.
// Caller must hold the lock.
func (s *SourceSwitcher) startBrowserLocked(ctx context.Context, url string) error {
	log.Printf("Starting browser with URL: %s", url)

	// Create new config with the target URL
	browserCfg := *s.cfg
	browserCfg.DashboardURL = url

	s.chromeBrowser = browser.NewChromeBrowser(&browserCfg)

	if err := s.chromeBrowser.Start(ctx); err != nil {
		s.chromeBrowser = nil
		return fmt.Errorf("failed to start browser: %w", err)
	}

	s.currentProvider = s.chromeBrowser
	s.touchHandler = browser.NewTouchHandler(s.chromeBrowser)
	log.Println("Browser started successfully")
	return nil
}

// navigateBrowserLocked navigates the existing browser to a new URL.
// Caller must hold the lock.
func (s *SourceSwitcher) navigateBrowserLocked(ctx context.Context, url string) error {
	if s.chromeBrowser == nil {
		return s.startBrowserLocked(ctx, url)
	}

	log.Printf("Navigating browser to: %s", url)

	if err := s.chromeBrowser.Navigate(ctx, url); err != nil {
		log.Printf("Navigate failed (%v), falling back to restart", err)
		s.stopBrowserLocked()
		return s.startBrowserLocked(ctx, url)
	}

	return nil
}

// stopBrowserLocked stops the browser if running.
// Caller must hold the lock.
func (s *SourceSwitcher) stopBrowserLocked() {
	if s.chromeBrowser != nil {
		log.Println("Stopping browser...")
		s.chromeBrowser.Stop() //nolint:errcheck // best-effort cleanup
		s.chromeBrowser = nil
		s.touchHandler = nil
	}
}

// startTerminalLocked starts a terminal with the given command URL.
// Caller must hold the lock.
func (s *SourceSwitcher) startTerminalLocked(ctx context.Context, cmdURL string) error {
	cmd, args, ok := terminal.ParseCommandURL(cmdURL)
	if !ok {
		return fmt.Errorf("invalid command URL: %s", cmdURL)
	}

	if len(args) > 0 {
		log.Printf("Starting terminal with command: %s %v", cmd, args)
	} else {
		log.Printf("Starting terminal with command: %s", cmd)
	}

	s.terminal = terminal.New(&terminal.Config{
		Command: cmd,
		Args:    args,
		Width:   s.cfg.ViewportWidth,
		Height:  s.cfg.ViewportHeight,
	})

	if err := s.terminal.Start(ctx); err != nil {
		s.terminal = nil
		return fmt.Errorf("failed to start terminal: %w", err)
	}

	s.currentProvider = s.terminal
	s.touchHandler = nil // No touch support for terminal
	log.Println("Terminal started successfully")
	return nil
}

// restartTerminalLocked restarts the terminal with a new command.
// Caller must hold the lock.
func (s *SourceSwitcher) restartTerminalLocked(ctx context.Context, cmdURL string) error {
	s.stopTerminalLocked()
	return s.startTerminalLocked(ctx, cmdURL)
}

// stopTerminalLocked stops the terminal if running.
// Caller must hold the lock.
func (s *SourceSwitcher) stopTerminalLocked() {
	if s.terminal != nil {
		log.Println("Stopping terminal...")
		s.terminal.Stop() //nolint:errcheck // best-effort cleanup
		s.terminal = nil
	}
}

// IsReady returns true if the current provider is ready.
// This implements the health.StatusProvider interface.
// Returns false during provider switches or when no provider is active.
func (s *SourceSwitcher) IsReady() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Not ready during active switching
	if s.switching {
		return false
	}

	if s.currentProvider == nil {
		return false
	}
	return s.currentProvider.IsReady()
}
