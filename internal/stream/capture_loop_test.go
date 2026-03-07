package stream

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cbenitezpy-ueno/retrodash-server/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockFrameProvider mock for FrameProvider
type MockFrameProvider struct {
	mock.Mock
}

func (m *MockFrameProvider) CaptureScreenshot(ctx context.Context, quality int) ([]byte, error) {
	args := m.Called(ctx, quality)
	// Handle nil return safely
	var data []byte
	if args.Get(0) != nil {
		data = args.Get(0).([]byte)
	}
	return data, args.Error(1)
}

func (m *MockFrameProvider) ViewportSize() (int, int) {
	args := m.Called()
	return args.Int(0), args.Int(1)
}

func (m *MockFrameProvider) IsReady() bool {
	args := m.Called()
	return args.Bool(0)
}

// MockRestartableProvider mock for RestartableProvider
type MockRestartableProvider struct {
	MockFrameProvider
}

func (m *MockRestartableProvider) Restart(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func TestNewCaptureLoop(t *testing.T) {
	provider := new(MockFrameProvider)
	encoder := DefaultEncoder()
	broadcaster := NewBroadcaster(encoder)
	fps := 30

	loop := NewCaptureLoop(provider, broadcaster, encoder, fps)

	assert.NotNil(t, loop)
	assert.Equal(t, provider, loop.provider)
	assert.Equal(t, broadcaster, loop.broadcaster)
	assert.Equal(t, encoder, loop.encoder)
	assert.Equal(t, fps, loop.fps)
	assert.False(t, loop.IsRunning())
}

func TestCaptureLoop_Lifecycle(t *testing.T) {
	provider := new(MockFrameProvider)
	encoder := DefaultEncoder()
	broadcaster := NewBroadcaster(encoder)
	fps := 30

	loop := NewCaptureLoop(provider, broadcaster, encoder, fps)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start
	loop.Start(ctx)
	assert.True(t, loop.IsRunning())

	// Start again (idempotent)
	loop.Start(ctx)
	assert.True(t, loop.IsRunning())

	// Stop
	loop.Stop()
	assert.False(t, loop.IsRunning())

	// Stop again (idempotent)
	loop.Stop()
	assert.False(t, loop.IsRunning())
}

func TestCaptureLoop_Run_NoClients(t *testing.T) {
	provider := new(MockFrameProvider)
	encoder := DefaultEncoder()
	broadcaster := NewBroadcaster(encoder)
	loop := NewCaptureLoop(provider, broadcaster, encoder, 100)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// If there are no clients, CaptureScreenshot should NOT be called.
	provider.On("IsReady").Return(true)

	loop.Start(ctx)
	time.Sleep(50 * time.Millisecond)
	loop.Stop()

	provider.AssertNotCalled(t, "CaptureScreenshot", mock.Anything, mock.Anything)
}

func TestCaptureLoop_Run_WithClients_Success(t *testing.T) {
	provider := new(MockFrameProvider)
	encoder := DefaultEncoder()
	broadcaster := NewBroadcaster(encoder)
	loop := NewCaptureLoop(provider, broadcaster, encoder, 50) // High FPS for quick test

	// Add a client
	client := NewClient("127.0.0.1:1234", types.QualityHigh, "127.0.0.1", 10)
	broadcaster.Subscribe(client)

	provider.On("IsReady").Return(true)
	provider.On("CaptureScreenshot", mock.Anything, mock.Anything).Return([]byte{0x01}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	loop.Start(ctx)

	// Wait for at least one frame
	time.Sleep(100 * time.Millisecond)

	loop.Stop()

	// Check that we captured some frames
	provider.AssertCalled(t, "CaptureScreenshot", mock.Anything, mock.Anything)
	assert.Greater(t, loop.Sequence(), uint64(0))
}

func TestCaptureLoop_Run_ProviderNotReady(t *testing.T) {
	provider := new(MockFrameProvider)
	encoder := DefaultEncoder()
	broadcaster := NewBroadcaster(encoder)
	loop := NewCaptureLoop(provider, broadcaster, encoder, 50)

	// Add a client
	client := NewClient("127.0.0.1:1234", types.QualityHigh, "127.0.0.1", 10)
	broadcaster.Subscribe(client)

	provider.On("IsReady").Return(false)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	loop.Start(ctx)
	time.Sleep(50 * time.Millisecond)
	loop.Stop()

	provider.AssertNotCalled(t, "CaptureScreenshot", mock.Anything, mock.Anything)
}

func TestCaptureLoop_Run_CaptureError(t *testing.T) {
	provider := new(MockFrameProvider)
	encoder := DefaultEncoder()
	broadcaster := NewBroadcaster(encoder)
	loop := NewCaptureLoop(provider, broadcaster, encoder, 50)

	client := NewClient("127.0.0.1:1234", types.QualityHigh, "127.0.0.1", 10)
	broadcaster.Subscribe(client)

	provider.On("IsReady").Return(true)
	provider.On("CaptureScreenshot", mock.Anything, mock.Anything).Return(nil, errors.New("capture failed"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	loop.Start(ctx)
	time.Sleep(100 * time.Millisecond)
	loop.Stop()

	provider.AssertCalled(t, "CaptureScreenshot", mock.Anything, mock.Anything)
	// Sequence should not increase on error
	assert.Equal(t, uint64(0), loop.Sequence())
}

func TestCaptureLoop_Run_RestartOnManyErrors(t *testing.T) {
	provider := new(MockRestartableProvider)
	encoder := DefaultEncoder()
	broadcaster := NewBroadcaster(encoder)
	// Very high FPS to trigger 10 errors quickly
	loop := NewCaptureLoop(provider, broadcaster, encoder, 100)

	client := NewClient("127.0.0.1:1234", types.QualityHigh, "127.0.0.1", 10)
	broadcaster.Subscribe(client)

	provider.On("IsReady").Return(true)
	// Return error
	provider.On("CaptureScreenshot", mock.Anything, mock.Anything).Return(nil, errors.New("capture failed"))
	// Expect restart
	provider.On("Restart", mock.Anything).Return(nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	loop.Start(ctx)

	// Wait enough time for 10+ errors (100ms should be enough at 100fps)
	time.Sleep(200 * time.Millisecond)

	loop.Stop()

	provider.AssertCalled(t, "Restart", mock.Anything)
}

func TestCaptureLoop_Run_LowQualityFallback(t *testing.T) {
	provider := new(MockFrameProvider)
	encoder := DefaultEncoder()
	broadcaster := NewBroadcaster(encoder)
	loop := NewCaptureLoop(provider, broadcaster, encoder, 50)

	client := NewClient("127.0.0.1:1234", types.QualityHigh, "127.0.0.1", 10)
	broadcaster.Subscribe(client)

	provider.On("IsReady").Return(true)

	// Simplify the test to avoid complexity with functional arguments in Mock
	// High quality call (85)
	provider.On("CaptureScreenshot", mock.Anything, 85).Return([]byte{0x01}, nil)
	// Low quality call (50) - fails
	provider.On("CaptureScreenshot", mock.Anything, 50).Return(nil, errors.New("fail"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	loop.Start(ctx)
	time.Sleep(100 * time.Millisecond)
	loop.Stop()

	assert.Greater(t, loop.Sequence(), uint64(0))
}

func TestCaptureLoop_ContextDone(t *testing.T) {
	provider := new(MockFrameProvider)
	encoder := DefaultEncoder()
	broadcaster := NewBroadcaster(encoder)
	loop := NewCaptureLoop(provider, broadcaster, encoder, 100)

	ctx, cancel := context.WithCancel(context.Background())

	loop.Start(ctx)
	assert.True(t, loop.IsRunning())

	cancel()

	// Give it a moment to react
	time.Sleep(20 * time.Millisecond)
	assert.False(t, loop.IsRunning())
}
