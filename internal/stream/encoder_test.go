package stream

import (
	"image"
	"image/color"
	"testing"

	"github.com/cbenitezpy-ueno/retrodash-server/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewEncoder(t *testing.T) {
	enc := NewEncoder(85, 50)
	assert.NotNil(t, enc)
	assert.Equal(t, 85, enc.qualityHigh)
	assert.Equal(t, 50, enc.qualityLow)
}

func TestDefaultEncoder(t *testing.T) {
	enc := DefaultEncoder()
	assert.Equal(t, 85, enc.qualityHigh)
	assert.Equal(t, 50, enc.qualityLow)
}

func TestEncoder_QualityForLevel(t *testing.T) {
	enc := NewEncoder(90, 40)

	assert.Equal(t, 90, enc.QualityForLevel(types.QualityHigh))
	assert.Equal(t, 40, enc.QualityForLevel(types.QualityLow))
}

func TestEncoder_Encode(t *testing.T) {
	enc := DefaultEncoder()

	// Create a simple test image
	img := createTestImage(100, 100)

	data, err := enc.Encode(img, 80)
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// Verify JPEG magic bytes
	assert.Equal(t, byte(0xFF), data[0])
	assert.Equal(t, byte(0xD8), data[1])
}

func TestEncoder_EncodeWithLevel(t *testing.T) {
	enc := NewEncoder(90, 40)
	img := createTestImage(100, 100)

	t.Run("high quality", func(t *testing.T) {
		data, err := enc.EncodeWithLevel(img, types.QualityHigh)
		require.NoError(t, err)
		assert.NotEmpty(t, data)
	})

	t.Run("low quality", func(t *testing.T) {
		data, err := enc.EncodeWithLevel(img, types.QualityLow)
		require.NoError(t, err)
		assert.NotEmpty(t, data)
	})
}

func TestEncoder_QualityAffectsSize(t *testing.T) {
	enc := NewEncoder(95, 30)
	img := createTestImage(200, 200)

	highData, err := enc.EncodeWithLevel(img, types.QualityHigh)
	require.NoError(t, err)

	lowData, err := enc.EncodeWithLevel(img, types.QualityLow)
	require.NoError(t, err)

	// High quality should produce larger files (usually)
	// Note: This might not always be true for very simple images
	assert.NotEqual(t, len(highData), len(lowData))
}

// createTestImage creates a simple colored test image.
func createTestImage(width, height int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			// Create a gradient pattern
			r := uint8((x * 255) / width)
			g := uint8((y * 255) / height)
			b := uint8(128)
			img.Set(x, y, color.RGBA{R: r, G: g, B: b, A: 255})
		}
	}
	return img
}
