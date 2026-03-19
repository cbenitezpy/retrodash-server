// Package main is the entry point for the Bridge Server.
package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/cbenitezpy-ueno/retrodash-server/internal/api"
	"github.com/cbenitezpy-ueno/retrodash-server/internal/config"
	"github.com/cbenitezpy-ueno/retrodash-server/internal/health"
	"github.com/cbenitezpy-ueno/retrodash-server/internal/logging"
	"github.com/cbenitezpy-ueno/retrodash-server/internal/origins"
	"github.com/cbenitezpy-ueno/retrodash-server/internal/stream"
	"github.com/cbenitezpy-ueno/retrodash-server/internal/switching"
	"github.com/cbenitezpy-ueno/retrodash-server/internal/terminal"
)

func main() {
	// Initialize structured JSON logging
	logging.Setup(nil, "bridge")
	log.Println("Starting RetroDash Bridge Server...")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Configuration error: %v", err)
	}

	log.Printf("Configuration loaded:")
	log.Printf("  Dashboard URL: %s", cfg.DashboardURL)
	log.Printf("  Port: %d", cfg.Port)
	log.Printf("  FPS: %d", cfg.FPS)
	log.Printf("  Viewport: %dx%d", cfg.ViewportWidth, cfg.ViewportHeight)
	log.Printf("  Origins file: %s", cfg.OriginsFile)

	// Initialize origin manager for multi-origin mode
	originStore := origins.NewJSONStore(cfg.OriginsFile)
	originManager := origins.NewManager(originStore)

	// Load persisted origins
	if err := originManager.Load(); err != nil {
		log.Printf("Warning: Could not load origins: %v (starting with empty list)", err)
	} else {
		log.Printf("Loaded %d origins from %s", originManager.Count(), cfg.OriginsFile)
	}

	// Determine initial mode
	var mode string
	switch {
	case cfg.DashboardURL == "":
		mode = "standby"
		log.Println("Standby mode: waiting for origin configuration via API")
	case terminal.IsCommandURL(cfg.DashboardURL):
		mode = "terminal"
		cmd, _, _ := terminal.ParseCommandURL(cfg.DashboardURL)
		log.Printf("Terminal mode: running command '%s'", cmd)
	default:
		mode = "browser"
		log.Println("Browser mode: rendering dashboard")
	}

	// Create source switcher for dynamic origin switching
	sourceSwitcher := switching.NewSourceSwitcher(cfg, originManager)

	// Start initial source with timeout (no-op in standby mode)
	if cfg.DashboardURL != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		if err := sourceSwitcher.StartInitialSource(ctx); err != nil {
			cancel()
			log.Fatalf("Failed to start initial source: %v", err)
		}
		cancel()
	}

	// Create stream components
	encoder := stream.NewEncoder(cfg.JPEGQualityHigh, cfg.JPEGQualityLow)
	broadcaster := stream.NewBroadcaster(encoder)

	// Create and start capture loop with dynamic provider getter
	captureLoop := stream.NewCaptureLoopWithGetter(sourceSwitcher.GetProvider, broadcaster, encoder, cfg.FPS)

	// Create loading image for smooth source transitions
	loadingGen := stream.NewLoadingImageGenerator(cfg.ViewportWidth, cfg.ViewportHeight, cfg.JPEGQualityHigh)
	captureLoop.SetLoadingImage(loadingGen.GetLoadingImage(), cfg.JPEGQualityHigh)
	log.Println("Loading placeholder image generated")

	captureCtx, captureCancel := context.WithCancel(context.Background())
	go captureLoop.Start(captureCtx)

	// Create health checker with source switcher (implements StatusProvider and ModeProvider)
	healthChecker := health.NewChecker(sourceSwitcher, broadcaster, sourceSwitcher)

	// Create Prometheus metrics and start background updater
	metrics := health.NewMetrics()
	metrics.SetBrowserReady(sourceSwitcher.IsReady())
	metrics.SetActiveStreams(broadcaster.ActiveClients())
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-captureCtx.Done():
				return
			case <-ticker.C:
				metrics.UpdateMemory()
				metrics.SetBrowserReady(sourceSwitcher.IsReady())
				metrics.SetActiveStreams(broadcaster.ActiveClients())
			}
		}
	}()

	// Create HTTP server
	srv := api.NewServer(cfg)
	srv.SetMetrics(metrics)
	srv.RegisterHealthRoutes(sourceSwitcher, sourceSwitcher)

	// Create handlers
	handlers := api.NewHandlers(healthChecker)
	handlers.SetBroadcaster(broadcaster)
	handlers.SetOriginManager(originManager)
	handlers.SetSourceSwitcher(sourceSwitcher)
	if touchHandler := sourceSwitcher.GetTouchHandler(); touchHandler != nil {
		handlers.SetTouchHandler(touchHandler)
	}

	// Register routes
	srv.RegisterHandler("/health", handlers.HealthHandler)
	srv.RegisterHandler("/stream", handlers.StreamHandler)
	srv.RegisterHandler("/touch", handlers.TouchHandler)
	srv.RegisterHandler("/api/origins", handlers.OriginsHandler)
	srv.RegisterHandler("/api/origins/", handlers.OriginItemHandler)

	log.Printf("Routes registered: /health, /healthz, /readyz, /metrics, /stream, /touch, /api/origins, /api/origins/{id} (mode: %s)", mode)
	log.Printf("Origin manager initialized with %d origins", originManager.Count())

	// Start server with graceful shutdown
	if err := srv.StartWithGracefulShutdown(func() {
		log.Println("Stopping capture loop...")
		captureCancel()
		log.Println("Stopping source switcher...")
		sourceSwitcher.Stop()
	}); err != nil {
		log.Printf("Server error: %v", err)
		os.Exit(1)
	}

	log.Println("Server stopped")
}
