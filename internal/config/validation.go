package config

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/cbenitezpy-ueno/retrodash-server/internal/origins"
)

var (
	// ErrMissingDashboardURL indicates DASHBOARD_URL is not set.
	ErrMissingDashboardURL = errors.New("DASHBOARD_URL is required")
	// ErrInvalidDashboardURL indicates DASHBOARD_URL is not a valid URL.
	ErrInvalidDashboardURL = errors.New("DASHBOARD_URL must be a valid HTTP(S) URL or cmd:// command")
	// ErrInvalidPort indicates PORT is out of range.
	ErrInvalidPort = errors.New("PORT must be between 1 and 65535")
	// ErrInvalidFPS indicates FPS is out of range.
	ErrInvalidFPS = errors.New("FPS must be between 1 and 30")
	// ErrInvalidJPEGQuality indicates JPEG quality is out of range.
	ErrInvalidJPEGQuality = errors.New("JPEG quality must be between 1 and 100")
	// ErrInvalidQualityOrder indicates high quality is less than low quality.
	ErrInvalidQualityOrder = errors.New("JPEG_QUALITY_HIGH must be >= JPEG_QUALITY_LOW")
	// ErrInvalidViewport indicates viewport dimensions are invalid.
	ErrInvalidViewport = errors.New("viewport dimensions must be between 320x240 and 3840x2160")
	// ErrInvalidAspectRatio indicates viewport aspect ratio is unreasonable.
	ErrInvalidAspectRatio = errors.New("viewport aspect ratio must be between 0.5 and 3.0")
)

// Validate checks the configuration for errors.
func Validate(cfg *Config) error {
	// Required: DASHBOARD_URL
	if cfg.DashboardURL == "" {
		return ErrMissingDashboardURL
	}

	// Validate URL format (http(s):// or cmd://)
	if strings.HasPrefix(cfg.DashboardURL, "cmd://") {
		// Validate command against allowlist and check for injection
		if err := origins.ValidateCommand(cfg.DashboardURL); err != nil {
			return fmt.Errorf("%w: %s (%v)", ErrInvalidDashboardURL, cfg.DashboardURL, err)
		}
	} else {
		// Parse as regular URL for http(s)://
		u, err := url.Parse(cfg.DashboardURL)
		if err != nil {
			return fmt.Errorf("%w: %s", ErrInvalidDashboardURL, cfg.DashboardURL)
		}

		switch u.Scheme {
		case "http", "https":
			if u.Host == "" {
				return fmt.Errorf("%w: %s", ErrInvalidDashboardURL, cfg.DashboardURL)
			}
		default:
			return fmt.Errorf("%w: %s", ErrInvalidDashboardURL, cfg.DashboardURL)
		}
	}

	// Validate PORT
	if cfg.Port < 1 || cfg.Port > 65535 {
		return fmt.Errorf("%w: %d", ErrInvalidPort, cfg.Port)
	}

	// Validate FPS
	if cfg.FPS < 1 || cfg.FPS > 30 {
		return fmt.Errorf("%w: %d", ErrInvalidFPS, cfg.FPS)
	}

	// Validate JPEG quality
	if cfg.JPEGQualityHigh < 1 || cfg.JPEGQualityHigh > 100 {
		return fmt.Errorf("%w: JPEG_QUALITY_HIGH=%d", ErrInvalidJPEGQuality, cfg.JPEGQualityHigh)
	}
	if cfg.JPEGQualityLow < 1 || cfg.JPEGQualityLow > 100 {
		return fmt.Errorf("%w: JPEG_QUALITY_LOW=%d", ErrInvalidJPEGQuality, cfg.JPEGQualityLow)
	}
	if cfg.JPEGQualityHigh < cfg.JPEGQualityLow {
		return fmt.Errorf("%w: high=%d, low=%d", ErrInvalidQualityOrder, cfg.JPEGQualityHigh, cfg.JPEGQualityLow)
	}

	// Validate viewport dimensions
	if cfg.ViewportWidth < 320 || cfg.ViewportWidth > 3840 ||
		cfg.ViewportHeight < 240 || cfg.ViewportHeight > 2160 {
		return fmt.Errorf("%w: %dx%d", ErrInvalidViewport, cfg.ViewportWidth, cfg.ViewportHeight)
	}

	// Validate aspect ratio (width/height between 0.5 and 3.0)
	aspectRatio := float64(cfg.ViewportWidth) / float64(cfg.ViewportHeight)
	if aspectRatio < 0.5 || aspectRatio > 3.0 {
		return fmt.Errorf("%w: %.2f", ErrInvalidAspectRatio, aspectRatio)
	}

	return nil
}
