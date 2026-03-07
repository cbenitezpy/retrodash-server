package stream

import (
	"context"
	"log"
	"sync/atomic"
	"time"

	"github.com/cbenitezpy-ueno/retrodash-server/pkg/types"
)

// FrameProvider captures frames from a source (e.g., browser, terminal).
type FrameProvider interface {
	CaptureScreenshot(ctx context.Context, quality int) ([]byte, error)
	ViewportSize() (width, height int)
	IsReady() bool
}

// RestartableProvider is a FrameProvider that can be restarted.
type RestartableProvider interface {
	FrameProvider
	Restart(ctx context.Context) error
}

// ProviderGetter is a function that returns the current frame provider.
type ProviderGetter func() FrameProvider

// CaptureLoop continuously captures frames and broadcasts them.
type CaptureLoop struct {
	provider       FrameProvider
	providerGetter ProviderGetter
	broadcaster    *Broadcaster
	encoder        *Encoder
	fps            int
	running        int32 // atomic: 0 = stopped, 1 = running
	stopCh         chan struct{}
	sequence       uint64
	consecutiveErr int
	lastHighFrame  *Frame // Cache last valid frame for smooth transitions
	lastLowFrame   *Frame
	loadingFrame   *Frame // Static loading image for source transitions
}

// NewCaptureLoop creates a new capture loop with a static provider.
func NewCaptureLoop(provider FrameProvider, broadcaster *Broadcaster, encoder *Encoder, fps int) *CaptureLoop {
	return &CaptureLoop{
		provider:    provider,
		broadcaster: broadcaster,
		encoder:     encoder,
		fps:         fps,
		stopCh:      make(chan struct{}),
	}
}

// NewCaptureLoopWithGetter creates a new capture loop with a dynamic provider getter.
func NewCaptureLoopWithGetter(getter ProviderGetter, broadcaster *Broadcaster, encoder *Encoder, fps int) *CaptureLoop {
	return &CaptureLoop{
		providerGetter: getter,
		broadcaster:    broadcaster,
		encoder:        encoder,
		fps:            fps,
		stopCh:         make(chan struct{}),
	}
}

// Start begins the capture loop in a goroutine.
func (c *CaptureLoop) Start(ctx context.Context) {
	if !atomic.CompareAndSwapInt32(&c.running, 0, 1) {
		return // Already running
	}

	go c.run(ctx)
	log.Printf("Capture loop started at %d FPS", c.fps)
}

// Stop stops the capture loop.
func (c *CaptureLoop) Stop() {
	if !atomic.CompareAndSwapInt32(&c.running, 1, 0) {
		return // Already stopped
	}
	close(c.stopCh)
	log.Println("Capture loop stopped")
}

// IsRunning returns whether the capture loop is active.
func (c *CaptureLoop) IsRunning() bool {
	return atomic.LoadInt32(&c.running) == 1
}

// Sequence returns the current frame sequence number.
func (c *CaptureLoop) Sequence() uint64 {
	return atomic.LoadUint64(&c.sequence)
}

// SetLoadingImage sets the loading image to show during source transitions.
func (c *CaptureLoop) SetLoadingImage(data []byte, quality int) {
	c.loadingFrame = &Frame{
		Data:      data,
		Timestamp: time.Now(),
		Sequence:  0,
		Quality:   quality,
	}
}

// getProvider returns the current provider, using the getter if available.
func (c *CaptureLoop) getProvider() FrameProvider {
	if c.providerGetter != nil {
		return c.providerGetter()
	}
	return c.provider
}

func (c *CaptureLoop) run(ctx context.Context) {
	interval := time.Duration(1000/c.fps) * time.Millisecond
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			atomic.StoreInt32(&c.running, 0)
			return
		case <-c.stopCh:
			return
		case <-ticker.C:
			// Get current provider (may change dynamically)
			provider := c.getProvider()
			if provider == nil {
				continue
			}

			// Skip if no clients
			if c.broadcaster.ActiveClients() == 0 {
				continue
			}

			// If provider not ready, broadcast loading or cached frame for smooth transition
			if !provider.IsReady() {
				// Prefer loading image to show user we're working
				if c.loadingFrame != nil {
					c.broadcaster.Broadcast(c.loadingFrame)
				} else if c.lastHighFrame != nil {
					// Fall back to last captured frame
					if c.lastLowFrame != nil {
						c.broadcaster.BroadcastWithQuality(c.lastHighFrame, c.lastLowFrame)
					} else {
						// Low frame may be nil if capture failed, send high to all
						c.broadcaster.Broadcast(c.lastHighFrame)
					}
				}
				continue
			}

			// Capture high quality frame
			highQuality := c.encoder.QualityForLevel(types.QualityHigh)
			highData, err := provider.CaptureScreenshot(ctx, highQuality)
			if err != nil {
				c.consecutiveErr++
				if c.consecutiveErr <= 3 {
					log.Printf("Capture error (%d/10): %v", c.consecutiveErr, err)
				} else if c.consecutiveErr == 10 {
					log.Printf("Capture error (%d/10): %v - will restart browser", c.consecutiveErr, err)
				}

				// Auto-restart after 10 consecutive errors
				if c.consecutiveErr >= 10 {
					if restartable, ok := provider.(RestartableProvider); ok {
						log.Println("Too many consecutive capture errors, restarting browser...")
						if restartErr := restartable.Restart(ctx); restartErr != nil {
							log.Printf("Failed to restart browser: %v", restartErr)
						} else {
							c.consecutiveErr = 0
							log.Println("Browser restarted successfully")
						}
					}
				}
				continue
			}
			c.consecutiveErr = 0 // Reset on success

			seq := atomic.AddUint64(&c.sequence, 1)
			highFrame := &Frame{
				Data:      highData,
				Timestamp: time.Now(),
				Sequence:  seq,
				Quality:   highQuality,
			}

			// Capture low quality frame (if any low quality clients)
			lowQuality := c.encoder.QualityForLevel(types.QualityLow)
			lowData, err := provider.CaptureScreenshot(ctx, lowQuality)
			if err != nil {
				// Use high quality for all clients if low capture fails
				c.lastHighFrame = highFrame
				c.lastLowFrame = nil
				c.broadcaster.Broadcast(highFrame)
				continue
			}

			lowFrame := &Frame{
				Data:      lowData,
				Timestamp: time.Now(),
				Sequence:  seq,
				Quality:   lowQuality,
			}

			// Cache frames for smooth transitions during source switches
			c.lastHighFrame = highFrame
			c.lastLowFrame = lowFrame

			// Broadcast to clients based on their quality preference
			c.broadcaster.BroadcastWithQuality(highFrame, lowFrame)
		}
	}
}
