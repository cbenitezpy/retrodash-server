// Package stream provides MJPEG streaming functionality.
package stream

import (
	"sync"
	"time"

	"github.com/cbenitezpy-ueno/retrodash-server/pkg/types"
)

// Frame represents a captured screenshot ready for streaming.
type Frame struct {
	// Data is the JPEG-encoded image data.
	Data []byte
	// Timestamp is when the frame was captured.
	Timestamp time.Time
	// Sequence is a monotonic frame counter.
	Sequence uint64
	// Quality is the JPEG quality used.
	Quality int
}

// Client represents an active stream client connection.
type Client struct {
	// ID is a unique client identifier.
	ID string
	// Quality is the requested stream quality.
	Quality types.StreamQuality
	// FrameChan receives frames for this client.
	FrameChan chan *Frame
	// ConnectedAt is when the client connected.
	ConnectedAt time.Time
	// stats
	mu            sync.Mutex
	framesSent    uint64
	bytesSent     uint64
	framesDropped uint64
	remoteAddr    string
}

// NewClient creates a new stream client.
func NewClient(id string, quality types.StreamQuality, remoteAddr string, bufferSize int) *Client {
	return &Client{
		ID:          id,
		Quality:     quality,
		FrameChan:   make(chan *Frame, bufferSize),
		ConnectedAt: time.Now(),
		remoteAddr:  remoteAddr,
	}
}

// RecordFrameSent updates statistics for a sent frame.
func (c *Client) RecordFrameSent(bytes int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.framesSent++
	c.bytesSent += uint64(bytes)
}

// RecordFrameDropped updates the dropped frame count.
func (c *Client) RecordFrameDropped() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.framesDropped++
}

// Stats returns client statistics.
func (c *Client) Stats() (sent, dropped, bytes uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.framesSent, c.framesDropped, c.bytesSent
}

// RemoteAddr returns the client's remote address.
func (c *Client) RemoteAddr() string {
	return c.remoteAddr
}

// Close closes the frame channel.
func (c *Client) Close() {
	close(c.FrameChan)
}
