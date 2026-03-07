// Package config provides configuration management via environment variables.
package config

import (
	"log"
	"os"
	"strconv"
)

// Config holds all server configuration.
type Config struct {
	// DashboardURL is the URL of the dashboard to render (required for single-origin mode).
	DashboardURL string
	// Port is the HTTP server port.
	Port int
	// OriginsFile is the path to the origins JSON file for persistence.
	OriginsFile string
	// FPS is the target frames per second.
	FPS int
	// JPEGQualityHigh is the JPEG quality for high quality streams.
	JPEGQualityHigh int
	// JPEGQualityLow is the JPEG quality for low quality streams.
	JPEGQualityLow int
	// ViewportWidth is the browser viewport width in pixels.
	ViewportWidth int
	// ViewportHeight is the browser viewport height in pixels.
	ViewportHeight int
	// ChromePath is the path to Chrome/Chromium binary (auto-detect if empty).
	ChromePath string
	// AuthCookies is optional cookies for authenticated dashboards.
	AuthCookies string
	// GrafanaAPIKey is an optional API key for Grafana authentication.
	GrafanaAPIKey string
	// VerifyTLS enables TLS certificate verification (disabled by default for homelab use).
	VerifyTLS bool
	// Timezone is the IANA timezone for dashboard display (e.g., "America/Argentina/Buenos_Aires").
	Timezone string
	// HostResolverRules is optional Chrome host resolver rules for mDNS/local hostname resolution.
	// Format: "MAP hostname ip_address" (e.g., "MAP grafana.local 192.168.1.100")
	HostResolverRules string
}

// DefaultConfig returns a Config with default values.
func DefaultConfig() *Config {
	return &Config{
		Port:            8080,
		OriginsFile:     "./data/origins.json",
		FPS:             15,
		JPEGQualityHigh: 85,
		JPEGQualityLow:  50,
		ViewportWidth:   1920,
		ViewportHeight:  1080,
	}
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	cfg := DefaultConfig()

	// Required
	cfg.DashboardURL = os.Getenv("DASHBOARD_URL")

	// Optional with defaults
	if v := os.Getenv("PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.Port = port
		} else {
			log.Printf("WARN: invalid PORT value %q, using default %d", v, cfg.Port)
		}
	}

	if v := os.Getenv("FPS"); v != "" {
		if fps, err := strconv.Atoi(v); err == nil {
			cfg.FPS = fps
		} else {
			log.Printf("WARN: invalid FPS value %q, using default %d", v, cfg.FPS)
		}
	}

	if v := os.Getenv("JPEG_QUALITY_HIGH"); v != "" {
		if q, err := strconv.Atoi(v); err == nil {
			cfg.JPEGQualityHigh = q
		} else {
			log.Printf("WARN: invalid JPEG_QUALITY_HIGH value %q, using default %d", v, cfg.JPEGQualityHigh)
		}
	}

	if v := os.Getenv("JPEG_QUALITY_LOW"); v != "" {
		if q, err := strconv.Atoi(v); err == nil {
			cfg.JPEGQualityLow = q
		} else {
			log.Printf("WARN: invalid JPEG_QUALITY_LOW value %q, using default %d", v, cfg.JPEGQualityLow)
		}
	}

	if v := os.Getenv("VIEWPORT_WIDTH"); v != "" {
		if w, err := strconv.Atoi(v); err == nil {
			cfg.ViewportWidth = w
		} else {
			log.Printf("WARN: invalid VIEWPORT_WIDTH value %q, using default %d", v, cfg.ViewportWidth)
		}
	}

	if v := os.Getenv("VIEWPORT_HEIGHT"); v != "" {
		if h, err := strconv.Atoi(v); err == nil {
			cfg.ViewportHeight = h
		} else {
			log.Printf("WARN: invalid VIEWPORT_HEIGHT value %q, using default %d", v, cfg.ViewportHeight)
		}
	}

	cfg.ChromePath = os.Getenv("CHROME_PATH")
	cfg.AuthCookies = os.Getenv("AUTH_COOKIES")
	cfg.GrafanaAPIKey = os.Getenv("GRAFANA_API_KEY")
	cfg.Timezone = os.Getenv("TIMEZONE")
	cfg.HostResolverRules = os.Getenv("HOST_RESOLVER_RULES")

	// Origins file path (for multi-origin mode)
	if v := os.Getenv("ORIGINS_FILE"); v != "" {
		cfg.OriginsFile = v
	}

	// Parse VERIFY_TLS - only verify if explicitly enabled (homelab default: no verification)
	if v := os.Getenv("VERIFY_TLS"); v == "true" || v == "1" {
		cfg.VerifyTLS = true
	}

	if err := Validate(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}
