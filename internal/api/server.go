// Package api provides the HTTP server and handlers.
package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cbenitezpy-ueno/retrodash-server/internal/config"
)

// Server represents the HTTP server.
type Server struct {
	httpServer *http.Server
	cfg        *config.Config
	mux        *http.ServeMux
	startTime  time.Time
}

// NewServer creates a new HTTP server.
func NewServer(cfg *config.Config) *Server {
	mux := http.NewServeMux()

	srv := &Server{
		cfg:       cfg,
		mux:       mux,
		startTime: time.Now(),
	}

	srv.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      srv.applyMiddleware(mux),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 0, // Disabled for streaming
		IdleTimeout:  120 * time.Second,
	}

	return srv
}

// RegisterHandler registers a handler for a pattern.
func (s *Server) RegisterHandler(pattern string, handler http.HandlerFunc) {
	s.mux.HandleFunc(pattern, handler)
}

// Start begins listening for HTTP requests.
func (s *Server) Start() error {
	log.Printf("Starting server on port %d", s.cfg.Port)
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	log.Println("Shutting down server...")
	return s.httpServer.Shutdown(ctx)
}

// StartWithGracefulShutdown starts the server and handles shutdown signals.
func (s *Server) StartWithGracefulShutdown(onShutdown func()) error {
	// Start server in goroutine
	errCh := make(chan error, 1)
	go func() {
		if err := s.Start(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		return err
	case <-quit:
		log.Println("Received shutdown signal")
	}

	// Call shutdown callback
	if onShutdown != nil {
		onShutdown()
	}

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return s.Shutdown(ctx)
}

// Uptime returns the duration since server start.
func (s *Server) Uptime() time.Duration {
	return time.Since(s.startTime)
}

// applyMiddleware wraps the handler with middleware.
func (s *Server) applyMiddleware(handler http.Handler) http.Handler {
	// Apply in reverse order (last applied runs first)
	handler = CORSMiddleware(handler)
	handler = RecoveryMiddleware(handler)
	handler = LoggingMiddleware(handler)
	return handler
}
