package browser

import (
	"context"
	"testing"

	"github.com/cbenitezpy-ueno/retrodash-server/internal/stream"
	"github.com/cbenitezpy-ueno/retrodash-server/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockBrowser implements Browser for testing.
type mockBrowser struct {
	status      types.BrowserStatus
	lastError   error
	screenshot  []byte
	width       int
	height      int
	clickCalled bool
	dragCalled  bool
}

func newMockBrowser() *mockBrowser {
	return &mockBrowser{
		status: types.BrowserReady,
		// JPEG magic bytes + minimal data
		screenshot: []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46},
		width:      1920,
		height:     1080,
	}
}

func (m *mockBrowser) Start(ctx context.Context) error {
	m.status = types.BrowserReady
	return nil
}

func (m *mockBrowser) Stop() error {
	m.status = types.BrowserError
	return nil
}

func (m *mockBrowser) Status() types.BrowserStatus {
	return m.status
}

func (m *mockBrowser) LastError() error {
	return m.lastError
}

func (m *mockBrowser) CaptureScreenshot(ctx context.Context, quality int) ([]byte, error) {
	if m.status != types.BrowserReady {
		return nil, ErrBrowserNotReady
	}
	return m.screenshot, nil
}

func (m *mockBrowser) Click(ctx context.Context, x, y int) error {
	if m.status != types.BrowserReady {
		return ErrBrowserNotReady
	}
	m.clickCalled = true
	return nil
}

func (m *mockBrowser) Drag(ctx context.Context, startX, startY, endX, endY int) error {
	if m.status != types.BrowserReady {
		return ErrBrowserNotReady
	}
	m.dragCalled = true
	return nil
}

func (m *mockBrowser) ViewportSize() (width, height int) {
	return m.width, m.height
}

func TestCaptureService_CaptureFrame(t *testing.T) {
	browser := newMockBrowser()
	encoder := stream.DefaultEncoder()
	service := NewCaptureService(browser, encoder)

	frame, err := service.CaptureFrame(context.Background(), 80)
	require.NoError(t, err)
	require.NotNil(t, frame)

	assert.Equal(t, browser.screenshot, frame.Data)
	assert.Equal(t, 80, frame.Quality)
	assert.Equal(t, uint64(1), frame.Sequence)
	assert.False(t, frame.Timestamp.IsZero())
}

func TestCaptureService_CaptureFrame_Sequence(t *testing.T) {
	browser := newMockBrowser()
	encoder := stream.DefaultEncoder()
	service := NewCaptureService(browser, encoder)

	// Capture multiple frames
	for i := 1; i <= 5; i++ {
		frame, err := service.CaptureFrame(context.Background(), 80)
		require.NoError(t, err)
		assert.Equal(t, uint64(i), frame.Sequence)
	}
}

func TestCaptureService_CaptureFrame_BrowserNotReady(t *testing.T) {
	browser := newMockBrowser()
	browser.status = types.BrowserStarting
	encoder := stream.DefaultEncoder()
	service := NewCaptureService(browser, encoder)

	_, err := service.CaptureFrame(context.Background(), 80)
	assert.Error(t, err)
}

func TestCaptureService_CaptureFrameWithQuality(t *testing.T) {
	browser := newMockBrowser()
	encoder := stream.NewEncoder(90, 40)
	service := NewCaptureService(browser, encoder)

	t.Run("high quality", func(t *testing.T) {
		frame, err := service.CaptureFrameWithQuality(context.Background(), "high")
		require.NoError(t, err)
		// The mock returns the same data regardless of quality
		assert.NotNil(t, frame)
	})

	t.Run("low quality", func(t *testing.T) {
		frame, err := service.CaptureFrameWithQuality(context.Background(), "low")
		require.NoError(t, err)
		assert.NotNil(t, frame)
	})
}
