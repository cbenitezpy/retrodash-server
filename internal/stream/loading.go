package stream

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"log"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// LoadingImageGenerator creates loading placeholder images.
type LoadingImageGenerator struct {
	width   int
	height  int
	quality int
	cached  []byte
}

// NewLoadingImageGenerator creates a generator with the given dimensions.
func NewLoadingImageGenerator(width, height, quality int) *LoadingImageGenerator {
	g := &LoadingImageGenerator{
		width:   width,
		height:  height,
		quality: quality,
	}
	g.cached = g.generate("Loading...")
	return g
}

// GetLoadingImage returns the cached loading image.
func (g *LoadingImageGenerator) GetLoadingImage() []byte {
	return g.cached
}

// generate creates a loading image with the given text.
func (g *LoadingImageGenerator) generate(text string) []byte {
	img := image.NewRGBA(image.Rect(0, 0, g.width, g.height))

	// Fill with dark background
	bgColor := color.RGBA{30, 30, 30, 255}
	for y := 0; y < g.height; y++ {
		for x := 0; x < g.width; x++ {
			img.Set(x, y, bgColor)
		}
	}

	// Draw text centered (scaled up for visibility)
	g.drawCenteredText(img, text, color.RGBA{200, 200, 200, 255})

	// Draw a simple spinner/progress indicator
	g.drawSpinner(img)

	// Encode to JPEG
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: g.quality}); err != nil {
		log.Printf("failed to encode loading image: %v", err)
		return nil
	}
	return buf.Bytes()
}

// drawCenteredText draws text centered on the image.
func (g *LoadingImageGenerator) drawCenteredText(img *image.RGBA, text string, col color.RGBA) {
	face := basicfont.Face7x13
	scale := 4 // Scale up for better visibility

	// Calculate text dimensions
	charWidth := 7 * scale
	charHeight := 13 * scale
	textWidth := len(text) * charWidth

	// Center position
	startX := (g.width - textWidth) / 2
	startY := (g.height - charHeight) / 2

	// Draw each character scaled
	for i, ch := range text {
		x := startX + i*charWidth
		y := startY
		g.drawScaledChar(img, face, ch, x, y, scale, col)
	}
}

// drawScaledChar draws a single character scaled up.
func (g *LoadingImageGenerator) drawScaledChar(img *image.RGBA, face font.Face, char rune, x, y, scale int, col color.RGBA) {
	// Create small image for the character
	smallW := 7
	smallH := 13
	small := image.NewRGBA(image.Rect(0, 0, smallW, smallH))

	drawer := &font.Drawer{
		Dst:  small,
		Src:  image.NewUniform(col),
		Face: face,
		Dot:  fixed.Point26_6{X: 0, Y: fixed.I(smallH - 2)},
	}
	drawer.DrawString(string(char))

	// Scale up to target image
	for sy := 0; sy < smallH; sy++ {
		for sx := 0; sx < smallW; sx++ {
			c := small.RGBAAt(sx, sy)
			if c.A > 0 {
				for dy := 0; dy < scale; dy++ {
					for dx := 0; dx < scale; dx++ {
						px := x + sx*scale + dx
						py := y + sy*scale + dy
						if px < g.width && py < g.height {
							img.Set(px, py, c)
						}
					}
				}
			}
		}
	}
}

// drawSpinner draws a simple loading indicator below the text.
func (g *LoadingImageGenerator) drawSpinner(img *image.RGBA) {
	// Draw three dots below the text
	centerX := g.width / 2
	centerY := g.height/2 + 80 // Below the text
	dotRadius := 8
	spacing := 40
	dotColor := color.RGBA{100, 100, 100, 255}

	// Draw three dots
	for i := -1; i <= 1; i++ {
		dotX := centerX + i*spacing
		g.drawCircle(img, dotX, centerY, dotRadius, dotColor)
	}
}

// drawCircle draws a filled circle.
func (g *LoadingImageGenerator) drawCircle(img *image.RGBA, cx, cy, radius int, col color.RGBA) {
	for y := cy - radius; y <= cy+radius; y++ {
		for x := cx - radius; x <= cx+radius; x++ {
			dx := x - cx
			dy := y - cy
			if dx*dx+dy*dy <= radius*radius {
				if x >= 0 && x < g.width && y >= 0 && y < g.height {
					img.Set(x, y, col)
				}
			}
		}
	}
}
