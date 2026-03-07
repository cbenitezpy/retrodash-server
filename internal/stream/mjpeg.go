package stream

import (
	"context"
	"fmt"
	"net/http"
)

const (
	// MJPEGBoundary is the boundary string for multipart MJPEG.
	MJPEGBoundary = "frame"
	// MJPEGContentType is the content type for MJPEG streams.
	MJPEGContentType = "multipart/x-mixed-replace; boundary=" + MJPEGBoundary
)

// MJPEGWriter writes MJPEG frames to an HTTP response.
type MJPEGWriter struct {
	w       http.ResponseWriter
	flusher http.Flusher
}

// NewMJPEGWriter creates a new MJPEG writer.
func NewMJPEGWriter(w http.ResponseWriter) (*MJPEGWriter, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("response writer does not support flushing")
	}

	return &MJPEGWriter{
		w:       w,
		flusher: flusher,
	}, nil
}

// WriteHeaders sets the appropriate HTTP headers for MJPEG streaming.
func (m *MJPEGWriter) WriteHeaders() {
	m.w.Header().Set("Content-Type", MJPEGContentType)
	m.w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	m.w.Header().Set("Pragma", "no-cache")
	m.w.Header().Set("Expires", "0")
	m.w.Header().Set("Connection", "keep-alive")
}

// WriteFrame writes a single JPEG frame to the stream.
func (m *MJPEGWriter) WriteFrame(data []byte) error {
	// Write boundary
	if _, err := fmt.Fprintf(m.w, "--%s\r\n", MJPEGBoundary); err != nil {
		return err
	}

	// Write headers
	if _, err := fmt.Fprintf(m.w, "Content-Type: image/jpeg\r\n"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(m.w, "Content-Length: %d\r\n\r\n", len(data)); err != nil {
		return err
	}

	// Write frame data
	if _, err := m.w.Write(data); err != nil {
		return err
	}

	// Write trailing newline
	if _, err := m.w.Write([]byte("\r\n")); err != nil {
		return err
	}

	// Flush to send immediately
	m.flusher.Flush()

	return nil
}

// StreamToClient streams frames from a client's channel until context is cancelled.
func StreamToClient(ctx context.Context, client *Client, writer *MJPEGWriter) error {
	for {
		select {
		case <-ctx.Done():
			// Client disconnected or server shutting down
			return ctx.Err()
		case frame, ok := <-client.FrameChan:
			if !ok {
				// Channel closed
				return nil
			}
			if err := writer.WriteFrame(frame.Data); err != nil {
				return err
			}
			client.RecordFrameSent(len(frame.Data))
		}
	}
}
