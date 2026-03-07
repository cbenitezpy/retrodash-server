package stream

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cbenitezpy-ueno/retrodash-server/pkg/types"
	"github.com/stretchr/testify/assert"
)

func TestNewMJPEGWriter(t *testing.T) {
	t.Run("Valid writer", func(t *testing.T) {
		w := httptest.NewRecorder()
		mw, err := NewMJPEGWriter(w)
		assert.NoError(t, err)
		assert.NotNil(t, mw)
	})

	t.Run("Invalid writer (no flusher)", func(t *testing.T) {
		// BasicResponseWriter does not implement Flusher
		// But httptest.Recorder DOES implement Flusher.
		// We need a custom writer that doesn't.
		type noFlushWriter struct {
			http.ResponseWriter
		}
		w := &noFlushWriter{httptest.NewRecorder()}
		mw, err := NewMJPEGWriter(w)
		assert.Error(t, err)
		assert.Nil(t, mw)
	})
}

func TestMJPEGWriter_WriteHeaders(t *testing.T) {
	w := httptest.NewRecorder()
	mw, _ := NewMJPEGWriter(w)

	mw.WriteHeaders()

	assert.Equal(t, MJPEGContentType, w.Header().Get("Content-Type"))
	assert.Equal(t, "no-cache, no-store, must-revalidate", w.Header().Get("Cache-Control"))
	assert.Equal(t, "no-cache", w.Header().Get("Pragma"))
	assert.Equal(t, "0", w.Header().Get("Expires"))
	assert.Equal(t, "keep-alive", w.Header().Get("Connection"))
}

func TestMJPEGWriter_WriteFrame(t *testing.T) {
	w := httptest.NewRecorder()
	mw, _ := NewMJPEGWriter(w)

	data := []byte("fake-jpeg-data")
	err := mw.WriteFrame(data)

	assert.NoError(t, err)

	body := w.Body.String()
	assert.Contains(t, body, "--"+MJPEGBoundary)
	assert.Contains(t, body, "Content-Type: image/jpeg")
	assert.Contains(t, body, "Content-Length: 14")
	assert.Contains(t, body, "fake-jpeg-data")
}

func TestStreamToClient(t *testing.T) {
	t.Run("Stream frames", func(t *testing.T) {
		w := httptest.NewRecorder()
		mw, _ := NewMJPEGWriter(w)
		client := NewClient("test", types.QualityHigh, "127.0.0.1", 10)
		ctx, cancel := context.WithCancel(context.Background())

		// Start streaming in background
		errCh := make(chan error)
		go func() {
			errCh <- StreamToClient(ctx, client, mw)
		}()

		// Send a frame
		client.FrameChan <- &Frame{Data: []byte("frame1")}

		// Allow some time for processing
		time.Sleep(10 * time.Millisecond)

		// Send another
		client.FrameChan <- &Frame{Data: []byte("frame2")}
		time.Sleep(10 * time.Millisecond)

		cancel()

		err := <-errCh
		assert.ErrorIs(t, err, context.Canceled)

		body := w.Body.String()
		assert.Contains(t, body, "frame1")
		assert.Contains(t, body, "frame2")

		// Check stats
		sent, _, _ := client.Stats()
		assert.Equal(t, uint64(2), sent)
	})

	t.Run("Channel closed", func(t *testing.T) {
		w := httptest.NewRecorder()
		mw, _ := NewMJPEGWriter(w)
		client := NewClient("test", types.QualityHigh, "127.0.0.1", 10)
		ctx := context.Background()

		close(client.FrameChan)

		err := StreamToClient(ctx, client, mw)
		assert.NoError(t, err)
	})

	// Testing WriteFrame error is hard with httptest.Recorder as it doesn't fail on Write easily.
	// Would need a failing writer mock.
}

type FailingWriter struct {
	http.ResponseWriter
	failOnWrite bool
}

func (f *FailingWriter) Write(p []byte) (int, error) {
	if f.failOnWrite {
		return 0, errors.New("write failed")
	}
	return f.ResponseWriter.Write(p)
}

func (f *FailingWriter) Flush() {}

func TestStreamToClient_WriteError(t *testing.T) {
	// Need to bypass NewMJPEGWriter check for Flusher, or implement it
	recorder := httptest.NewRecorder()
	fw := &FailingWriter{ResponseWriter: recorder, failOnWrite: true}

	// Construct MJPEGWriter manually or ensure FailingWriter implements Flusher
	mw := &MJPEGWriter{w: fw, flusher: fw}

	client := NewClient("test", types.QualityHigh, "127.0.0.1", 10)
	ctx := context.Background()

	go func() {
		client.FrameChan <- &Frame{Data: []byte("frame1")}
	}()

	err := StreamToClient(ctx, client, mw)
	assert.Error(t, err)
	assert.Equal(t, "write failed", err.Error())
}
