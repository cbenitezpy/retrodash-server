package stream

import (
	"bytes"
	"image"
	"image/jpeg"

	"github.com/cbenitezpy-ueno/retrodash-server/pkg/types"
)

// Encoder handles JPEG encoding with configurable quality levels.
type Encoder struct {
	qualityHigh int
	qualityLow  int
}

// NewEncoder creates a new JPEG encoder.
func NewEncoder(qualityHigh, qualityLow int) *Encoder {
	return &Encoder{
		qualityHigh: qualityHigh,
		qualityLow:  qualityLow,
	}
}

// QualityForLevel returns the JPEG quality value for a quality level.
func (e *Encoder) QualityForLevel(level types.StreamQuality) int {
	if level == types.QualityLow {
		return e.qualityLow
	}
	return e.qualityHigh
}

// Encode encodes an image to JPEG with the specified quality.
func (e *Encoder) Encode(img image.Image, quality int) ([]byte, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// EncodeWithLevel encodes an image using a quality level.
func (e *Encoder) EncodeWithLevel(img image.Image, level types.StreamQuality) ([]byte, error) {
	return e.Encode(img, e.QualityForLevel(level))
}

// DefaultEncoder returns an encoder with default quality settings.
func DefaultEncoder() *Encoder {
	return NewEncoder(85, 50)
}
