package browser

import (
	"context"
	"fmt"
	"time"

	"github.com/cbenitezpy-ueno/retrodash-server/internal/stream"
)

// CaptureService handles screenshot capture from the browser.
type CaptureService struct {
	browser  Browser
	encoder  *stream.Encoder
	sequence uint64
}

// NewCaptureService creates a new capture service.
func NewCaptureService(browser Browser, encoder *stream.Encoder) *CaptureService {
	return &CaptureService{
		browser: browser,
		encoder: encoder,
	}
}

// CaptureFrame captures a screenshot and returns it as a Frame.
func (s *CaptureService) CaptureFrame(ctx context.Context, quality int) (*stream.Frame, error) {
	data, err := s.browser.CaptureScreenshot(ctx, quality)
	if err != nil {
		return nil, fmt.Errorf("capture failed: %w", err)
	}

	s.sequence++
	return &stream.Frame{
		Data:      data,
		Timestamp: time.Now(),
		Sequence:  s.sequence,
		Quality:   quality,
	}, nil
}

// CaptureFrameWithQuality captures using a quality level.
func (s *CaptureService) CaptureFrameWithQuality(ctx context.Context, level string) (*stream.Frame, error) {
	var quality int
	if level == "low" {
		quality = s.encoder.QualityForLevel("low")
	} else {
		quality = s.encoder.QualityForLevel("high")
	}
	return s.CaptureFrame(ctx, quality)
}
