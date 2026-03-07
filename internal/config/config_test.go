package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.Equal(t, 8080, cfg.Port)
	assert.Equal(t, 15, cfg.FPS)
	assert.Equal(t, 85, cfg.JPEGQualityHigh)
	assert.Equal(t, 50, cfg.JPEGQualityLow)
	assert.Equal(t, 1920, cfg.ViewportWidth)
	assert.Equal(t, 1080, cfg.ViewportHeight)
	assert.Empty(t, cfg.DashboardURL)
	assert.Empty(t, cfg.ChromePath)
	assert.Equal(t, "./data/origins.json", cfg.OriginsFile)
}

func TestLoad_ValidConfig(t *testing.T) {
	// Set environment variables
	os.Setenv("DASHBOARD_URL", "http://grafana:3000/dashboard")
	os.Setenv("PORT", "9090")
	os.Setenv("FPS", "20")
	os.Setenv("JPEG_QUALITY_HIGH", "90")
	os.Setenv("JPEG_QUALITY_LOW", "40")
	os.Setenv("VIEWPORT_WIDTH", "1280")
	os.Setenv("VIEWPORT_HEIGHT", "720")
	os.Setenv("CHROME_PATH", "/usr/bin/chromium")
	defer func() {
		os.Unsetenv("DASHBOARD_URL")
		os.Unsetenv("PORT")
		os.Unsetenv("FPS")
		os.Unsetenv("JPEG_QUALITY_HIGH")
		os.Unsetenv("JPEG_QUALITY_LOW")
		os.Unsetenv("VIEWPORT_WIDTH")
		os.Unsetenv("VIEWPORT_HEIGHT")
		os.Unsetenv("CHROME_PATH")
	}()

	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, "http://grafana:3000/dashboard", cfg.DashboardURL)
	assert.Equal(t, 9090, cfg.Port)
	assert.Equal(t, 20, cfg.FPS)
	assert.Equal(t, 90, cfg.JPEGQualityHigh)
	assert.Equal(t, 40, cfg.JPEGQualityLow)
	assert.Equal(t, 1280, cfg.ViewportWidth)
	assert.Equal(t, 720, cfg.ViewportHeight)
	assert.Equal(t, "/usr/bin/chromium", cfg.ChromePath)
}

func TestLoad_MissingDashboardURL(t *testing.T) {
	os.Unsetenv("DASHBOARD_URL")

	_, err := Load()
	assert.ErrorIs(t, err, ErrMissingDashboardURL)
}

func TestLoad_InvalidDashboardURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"no scheme", "grafana:3000/dashboard"},
		{"ftp scheme", "ftp://grafana:3000/dashboard"},
		{"no host", "http:///dashboard"},
		{"malformed", "://invalid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv("DASHBOARD_URL", tt.url)
			defer os.Unsetenv("DASHBOARD_URL")

			_, err := Load()
			assert.ErrorIs(t, err, ErrInvalidDashboardURL)
		})
	}
}

func TestLoad_DefaultsWhenEnvMissing(t *testing.T) {
	os.Setenv("DASHBOARD_URL", "http://localhost:3000")
	defer os.Unsetenv("DASHBOARD_URL")

	// Clear all optional env vars
	os.Unsetenv("PORT")
	os.Unsetenv("FPS")
	os.Unsetenv("JPEG_QUALITY_HIGH")
	os.Unsetenv("JPEG_QUALITY_LOW")
	os.Unsetenv("VIEWPORT_WIDTH")
	os.Unsetenv("VIEWPORT_HEIGHT")
	os.Unsetenv("CHROME_PATH")

	cfg, err := Load()
	require.NoError(t, err)

	// Should use defaults
	assert.Equal(t, 8080, cfg.Port)
	assert.Equal(t, 15, cfg.FPS)
	assert.Equal(t, 85, cfg.JPEGQualityHigh)
	assert.Equal(t, 50, cfg.JPEGQualityLow)
	assert.Equal(t, 1920, cfg.ViewportWidth)
	assert.Equal(t, 1080, cfg.ViewportHeight)
	assert.Empty(t, cfg.ChromePath)
}

func TestValidate_InvalidPort(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DashboardURL = "http://localhost:3000"

	cfg.Port = 0
	assert.ErrorIs(t, Validate(cfg), ErrInvalidPort)

	cfg.Port = 65536
	assert.ErrorIs(t, Validate(cfg), ErrInvalidPort)
}

func TestValidate_InvalidFPS(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DashboardURL = "http://localhost:3000"

	cfg.FPS = 0
	assert.ErrorIs(t, Validate(cfg), ErrInvalidFPS)

	cfg.FPS = 31
	assert.ErrorIs(t, Validate(cfg), ErrInvalidFPS)
}

func TestValidate_InvalidJPEGQuality(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DashboardURL = "http://localhost:3000"

	cfg.JPEGQualityHigh = 0
	assert.ErrorIs(t, Validate(cfg), ErrInvalidJPEGQuality)

	cfg.JPEGQualityHigh = 101
	assert.ErrorIs(t, Validate(cfg), ErrInvalidJPEGQuality)

	cfg.JPEGQualityHigh = 85
	cfg.JPEGQualityLow = 0
	assert.ErrorIs(t, Validate(cfg), ErrInvalidJPEGQuality)
}

func TestValidate_QualityOrder(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DashboardURL = "http://localhost:3000"
	cfg.JPEGQualityHigh = 40
	cfg.JPEGQualityLow = 60 // Low > High is invalid

	assert.ErrorIs(t, Validate(cfg), ErrInvalidQualityOrder)
}

func TestValidate_InvalidViewport(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DashboardURL = "http://localhost:3000"

	cfg.ViewportWidth = 100 // Too small
	assert.ErrorIs(t, Validate(cfg), ErrInvalidViewport)

	cfg.ViewportWidth = 1920
	cfg.ViewportHeight = 100 // Too small
	assert.ErrorIs(t, Validate(cfg), ErrInvalidViewport)

	cfg.ViewportWidth = 5000 // Too large
	cfg.ViewportHeight = 1080
	assert.ErrorIs(t, Validate(cfg), ErrInvalidViewport)
}

func TestValidate_InvalidAspectRatio(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DashboardURL = "http://localhost:3000"

	// Very tall (aspect ratio < 0.5)
	cfg.ViewportWidth = 320
	cfg.ViewportHeight = 1000
	assert.ErrorIs(t, Validate(cfg), ErrInvalidAspectRatio)

	// Very wide (aspect ratio > 3.0)
	cfg.ViewportWidth = 3840
	cfg.ViewportHeight = 500
	assert.ErrorIs(t, Validate(cfg), ErrInvalidAspectRatio)
}

func TestValidate_ValidConfig(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DashboardURL = "http://localhost:3000"

	assert.NoError(t, Validate(cfg))
}

func TestValidate_HTTPSSupported(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DashboardURL = "https://secure.example.com/dashboard"

	assert.NoError(t, Validate(cfg))
}

func TestLoad_OriginsFile(t *testing.T) {
	os.Setenv("DASHBOARD_URL", "http://localhost:3000")
	defer os.Unsetenv("DASHBOARD_URL")

	t.Run("default value when ORIGINS_FILE not set", func(t *testing.T) {
		os.Unsetenv("ORIGINS_FILE")

		cfg, err := Load()
		require.NoError(t, err)
		assert.Equal(t, "./data/origins.json", cfg.OriginsFile)
	})

	t.Run("custom value when ORIGINS_FILE set", func(t *testing.T) {
		os.Setenv("ORIGINS_FILE", "/custom/path/origins.json")
		defer os.Unsetenv("ORIGINS_FILE")

		cfg, err := Load()
		require.NoError(t, err)
		assert.Equal(t, "/custom/path/origins.json", cfg.OriginsFile)
	})
}
