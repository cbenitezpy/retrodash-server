// Package types provides shared types for the Bridge Server.
package types

import "time"

// StreamQuality represents the quality level for JPEG compression.
type StreamQuality string

const (
	// QualityHigh uses higher JPEG quality (less compression, better image).
	QualityHigh StreamQuality = "high"
	// QualityLow uses lower JPEG quality (more compression, smaller size).
	QualityLow StreamQuality = "low"
)

// TouchEventType represents the type of touch interaction.
type TouchEventType string

const (
	// TouchStart indicates the beginning of a touch/click.
	TouchStart TouchEventType = "start"
	// TouchMove indicates movement during a touch (drag).
	TouchMove TouchEventType = "move"
	// TouchEnd indicates the end of a touch/click.
	TouchEnd TouchEventType = "end"
)

// TouchEvent represents a touch interaction from the client.
type TouchEvent struct {
	// X is the normalized X coordinate (0.0 to 1.0).
	X float64 `json:"x"`
	// Y is the normalized Y coordinate (0.0 to 1.0).
	Y float64 `json:"y"`
	// Type is the touch event type (start, move, end).
	Type TouchEventType `json:"type"`
}

// HealthResponse represents the response from the /health endpoint.
type HealthResponse struct {
	// Status is "ok" or "error".
	Status string `json:"status"`
	// Version is the server version.
	Version string `json:"version,omitempty"`
	// Uptime is seconds since server start.
	Uptime int64 `json:"uptime,omitempty"`
	// Mode is the current source mode ("browser" or "terminal").
	Mode string `json:"mode,omitempty"`
	// BrowserStatus is the current browser state (optional).
	BrowserStatus string `json:"browserStatus,omitempty"`
	// ActiveClients is the number of connected stream clients (optional).
	ActiveClients int `json:"activeClients,omitempty"`
	// LastError is the most recent error message (optional).
	LastError string `json:"lastError,omitempty"`
}

// ErrorResponse represents an error response from the API.
type ErrorResponse struct {
	// Message is the error description.
	Message string `json:"message"`
	// Code is an optional error code.
	Code string `json:"code,omitempty"`
}

// BrowserStatus represents the state of the headless browser.
type BrowserStatus string

const (
	// BrowserStarting indicates the browser is initializing.
	BrowserStarting BrowserStatus = "starting"
	// BrowserReady indicates the browser is ready and page is loaded.
	BrowserReady BrowserStatus = "ready"
	// BrowserError indicates the browser encountered an error.
	BrowserError BrowserStatus = "error"
	// BrowserRestarting indicates the browser is being restarted.
	BrowserRestarting BrowserStatus = "restarting"
)

// Frame represents a captured screenshot.
type Frame struct {
	// Data is the JPEG-encoded image data.
	Data []byte
	// Timestamp is when the frame was captured.
	Timestamp time.Time
	// Width is the frame width in pixels.
	Width int
	// Height is the frame height in pixels.
	Height int
	// Quality is the JPEG quality used.
	Quality int
	// Sequence is a monotonic frame counter.
	Sequence uint64
}

// StreamClient represents an active stream connection.
type StreamClient struct {
	// ID is a unique client identifier.
	ID string
	// Quality is the requested stream quality.
	Quality StreamQuality
	// ConnectedAt is when the client connected.
	ConnectedAt time.Time
	// FramesSent is the total frames delivered.
	FramesSent uint64
	// BytesSent is the total bytes delivered.
	BytesSent uint64
	// FramesDropped is frames dropped due to slow client.
	FramesDropped uint64
	// RemoteAddr is the client IP:port.
	RemoteAddr string
}
