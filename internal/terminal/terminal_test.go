package terminal

import (
	"context"
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestParseCommandURL(t *testing.T) {
	tests := []struct {
		url  string
		cmd  string
		args []string
		ok   bool
	}{
		{"cmd://top", "top", nil, true},
		{"cmd://htop -d 1", "htop", []string{"-d", "1"}, true},
		{"cmd://htop?args=-d,1", "htop", []string{"-d", "1"}, true},
		{"http://example.com", "", nil, false},
		{"cmd://watch?args=-n,1,df,-h", "watch", []string{"-n", "1", "df", "-h"}, true},
	}

	for _, tt := range tests {
		cmd, args, ok := ParseCommandURL(tt.url)
		assert.Equal(t, tt.cmd, cmd)
		assert.Equal(t, tt.args, args)
		assert.Equal(t, tt.ok, ok)
	}
}

func TestIsCommandURL(t *testing.T) {
	assert.True(t, IsCommandURL("cmd://top"))
	assert.False(t, IsCommandURL("http://google.com"))
}

func TestNew(t *testing.T) {
	cfg := &Config{
		Command: "top",
		Width:   800,
		Height:  600,
	}
	term := New(cfg)

	assert.Equal(t, "top", term.cmd)
	assert.Equal(t, 800, term.width)
	assert.Equal(t, 600, term.height)
	assert.NotNil(t, term.buffer)
}

func TestTerminal_Lifecycle(t *testing.T) {
	// This requires `echo` to be available, which should be on linux
	cfg := &Config{
		Command: "echo hello",
		Width:   100,
		Height:  100,
	}
	term := New(cfg)

	ctx := context.Background()
	err := term.Start(ctx)
	assert.NoError(t, err)
	assert.True(t, term.IsRunning())

	// Wait for output
	time.Sleep(100 * time.Millisecond)

	// Verify output in buffer
	term.mu.RLock()
	found := false
	for _, line := range term.buffer.lines {
		str := ""
		for _, cell := range line.cells {
			if cell.char != 0 {
				str += string(cell.char)
			}
		}
		if str == "hello" || str == "hello\r" || str == "hello\r\n" {
			found = true
			break
		}
		// Also check partial matches if echo appends newline
		if len(str) >= 5 && str[:5] == "hello" {
			found = true
			break
		}
	}
	term.mu.RUnlock()
	assert.True(t, found, "Output 'hello' not found in buffer")

	err = term.Stop()
	assert.NoError(t, err)
	assert.False(t, term.IsRunning())
}

func TestTerminal_CaptureScreenshot(t *testing.T) {
	cfg := &Config{
		Command: "sh",
		Args:    []string{"-c", "echo test && sleep 10"},
		Width:   100,
		Height:  100,
	}
	term := New(cfg)
	ctx := context.Background()

	// Should fail if not running
	_, err := term.CaptureScreenshot(ctx, 80)
	assert.Error(t, err)

	term.Start(ctx)
	defer term.Stop()
	time.Sleep(100 * time.Millisecond) // Give time for echo output

	imgData, err := term.CaptureScreenshot(ctx, 80)
	assert.NoError(t, err)
	assert.NotEmpty(t, imgData)

	width, height := term.ViewportSize()
	assert.Equal(t, 100, width)
	assert.Equal(t, 100, height)

	assert.True(t, term.IsReady())

	last := term.LastActivity()
	assert.False(t, last.IsZero())
}

func TestTerminal_ColorConversion(t *testing.T) {
	term := New(&Config{Command: "echo", Width: 100, Height: 100})

	// Basic
	c := term.colorToRGBA(Color{Type: ColorBasic, Index: 1}) // Red
	assert.Equal(t, uint8(205), c.R)
	assert.Equal(t, uint8(0), c.G)

	// RGB
	c = term.colorToRGBA(Color{Type: ColorRGB, R: 10, G: 20, B: 30})
	assert.Equal(t, uint8(10), c.R)
	assert.Equal(t, uint8(20), c.G)
	assert.Equal(t, uint8(30), c.B)

	// 256
	c = term.colorToRGBA(Color{Type: Color256, Index: 196}) // Red-ish
	assert.NotZero(t, c.R)

	// Default - uses standard terminal white (color 7)
	c = term.colorToRGBA(Color{Type: ColorDefault})
	assert.Equal(t, uint8(229), c.R)
}

func TestTerminal_Color256(t *testing.T) {
	term := New(&Config{Command: "echo", Width: 100, Height: 100})

	// Standard
	c := term.color256(10)
	assert.Equal(t, term.basicColor(10), c)

	// Cube
	c = term.color256(16) // 0,0,0
	assert.Equal(t, uint8(0), c.R)

	// Grayscale
	c = term.color256(232)
	assert.Equal(t, uint8(8), c.R)
	assert.Equal(t, uint8(8), c.G)
}

func TestTerminal_RenderToImage(t *testing.T) {
	// Test internal render method
	term := New(&Config{Command: "echo", Width: 100, Height: 100})

	// Write some content
	term.buffer.Write([]byte("A"))
	term.buffer.lines[0].cells[0].bg = Color{Type: ColorBasic, Index: 1} // Red BG
	term.buffer.lines[0].cells[0].fg = Color{Type: ColorBasic, Index: 2} // Green FG

	img := term.renderToImage()
	assert.NotNil(t, img)
	assert.Equal(t, 100, img.Bounds().Dx())
	assert.Equal(t, 100, img.Bounds().Dy())

	// Check background pixel (approximate location)
	bg := img.RGBAAt(0, 0)
	// Red is 205, 0, 0
	assert.Equal(t, uint8(205), bg.R)

	// Check cursor rendering (enable cursor blink mock if possible, or wait)
	// Cursor blink depends on time, might be on or off.
	// Since we can't control time easily without injection, we skip exact cursor pixel check.
}

func TestTerminal_FillRect(t *testing.T) {
	term := New(&Config{Command: "echo", Width: 100, Height: 100})
	red := color.RGBA{255, 0, 0, 255}

	t.Run("normal fill", func(t *testing.T) {
		img := image.NewRGBA(image.Rect(0, 0, 50, 50))
		term.fillRect(img, 10, 10, 20, 20, red)
		// Check a pixel inside the filled area
		c := img.RGBAAt(15, 15)
		assert.Equal(t, uint8(255), c.R)
		assert.Equal(t, uint8(0), c.G)
	})

	t.Run("negative x clipping", func(t *testing.T) {
		img := image.NewRGBA(image.Rect(0, 0, 50, 50))
		term.fillRect(img, -5, 10, 20, 10, red)
		// Should fill from x=0
		c := img.RGBAAt(0, 15)
		assert.Equal(t, uint8(255), c.R)
		// x=15 (original end) should still be filled since w was 20, clipped to 15
		c = img.RGBAAt(14, 15)
		assert.Equal(t, uint8(255), c.R)
	})

	t.Run("negative y clipping", func(t *testing.T) {
		img := image.NewRGBA(image.Rect(0, 0, 50, 50))
		term.fillRect(img, 10, -5, 10, 20, red)
		// Should fill from y=0
		c := img.RGBAAt(15, 0)
		assert.Equal(t, uint8(255), c.R)
	})

	t.Run("right edge clipping", func(t *testing.T) {
		img := image.NewRGBA(image.Rect(0, 0, 50, 50))
		term.fillRect(img, 40, 10, 20, 10, red)
		// Should fill from x=40 to x=49 (edge of image)
		c := img.RGBAAt(49, 15)
		assert.Equal(t, uint8(255), c.R)
	})

	t.Run("bottom edge clipping", func(t *testing.T) {
		img := image.NewRGBA(image.Rect(0, 0, 50, 50))
		term.fillRect(img, 10, 40, 10, 20, red)
		// Should fill from y=40 to y=49 (edge of image)
		c := img.RGBAAt(15, 49)
		assert.Equal(t, uint8(255), c.R)
	})

	t.Run("zero width after clipping", func(t *testing.T) {
		img := image.NewRGBA(image.Rect(0, 0, 50, 50))
		// Rect entirely to the left of image
		term.fillRect(img, -20, 10, 10, 10, red)
		// Nothing should be drawn - check a pixel
		c := img.RGBAAt(0, 15)
		assert.Equal(t, uint8(0), c.R) // Should be black (default)
	})

	t.Run("zero height after clipping", func(t *testing.T) {
		img := image.NewRGBA(image.Rect(0, 0, 50, 50))
		// Rect entirely above image
		term.fillRect(img, 10, -20, 10, 10, red)
		// Nothing should be drawn
		c := img.RGBAAt(15, 0)
		assert.Equal(t, uint8(0), c.R)
	})

	t.Run("rect beyond right edge", func(t *testing.T) {
		img := image.NewRGBA(image.Rect(0, 0, 50, 50))
		// Rect entirely to the right of image
		term.fillRect(img, 60, 10, 10, 10, red)
		// Nothing should be drawn
		c := img.RGBAAt(49, 15)
		assert.Equal(t, uint8(0), c.R)
	})

	t.Run("rect beyond bottom edge", func(t *testing.T) {
		img := image.NewRGBA(image.Rect(0, 0, 50, 50))
		// Rect entirely below image
		term.fillRect(img, 10, 60, 10, 10, red)
		// Nothing should be drawn
		c := img.RGBAAt(15, 49)
		assert.Equal(t, uint8(0), c.R)
	})
}

func TestTerminal_RenderCursorClipping(t *testing.T) {
	// Test cursor rendering when it's at the edge of the viewport
	term := New(&Config{Command: "echo", Width: 100, Height: 100})

	// Position cursor near edge (will be clipped)
	term.buffer.cursorX = 10 // Large column number
	term.buffer.cursorY = 5
	term.buffer.cursorVisible = true

	// Should not panic even if cursor is partially outside viewport
	img := term.renderToImage()
	assert.NotNil(t, img)
}

func TestScreenBuffer_IsEscapeComplete(t *testing.T) {
	b := NewScreenBuffer(80, 24)

	// Empty sequence
	b.escapeSeq = ""
	assert.False(t, b.isEscapeComplete())

	// Single character escape (non-CSI)
	b.escapeSeq = "M"
	assert.True(t, b.isEscapeComplete())

	// CSI sequence incomplete
	b.escapeSeq = "[1"
	assert.False(t, b.isEscapeComplete())

	// CSI sequence complete with uppercase letter
	b.escapeSeq = "[1A"
	assert.True(t, b.isEscapeComplete())

	// CSI sequence complete with lowercase letter
	b.escapeSeq = "[1m"
	assert.True(t, b.isEscapeComplete())
}

func TestScreenBuffer_EraseDisplay(t *testing.T) {
	// Test erase from cursor to end (mode 0)
	b := NewScreenBuffer(10, 5)
	// Manually set content on multiple lines
	for i := 0; i < 10; i++ {
		b.lines[0].cells[i].char = 'A'
		b.lines[1].cells[i].char = 'B'
		b.lines[2].cells[i].char = 'C'
	}
	b.cursorX = 5
	b.cursorY = 1

	b.eraseDisplay("0")
	// Row 0 should be intact
	assert.Equal(t, 'A', b.lines[0].cells[0].char)
	// Row 1 from column 5 onwards should be cleared
	assert.Equal(t, rune(0), b.lines[1].cells[5].char)
	// Row 2 should be cleared
	assert.Equal(t, rune(0), b.lines[2].cells[0].char)

	// Test erase from start to cursor (mode 1)
	b = NewScreenBuffer(10, 5)
	for i := 0; i < 10; i++ {
		b.lines[0].cells[i].char = 'A'
		b.lines[1].cells[i].char = 'B'
		b.lines[2].cells[i].char = 'C'
	}
	b.cursorX = 5
	b.cursorY = 1

	b.eraseDisplay("1")
	// Row 0 should be cleared
	assert.Equal(t, rune(0), b.lines[0].cells[0].char)
	// Row 1 up to column 5 should be cleared
	assert.Equal(t, rune(0), b.lines[1].cells[0].char)
	// Content after cursor should be intact
	assert.Equal(t, 'B', b.lines[1].cells[6].char)

	// Test erase entire screen (mode 2)
	b = NewScreenBuffer(10, 5)
	for i := 0; i < 10; i++ {
		b.lines[0].cells[i].char = 'A'
	}
	b.cursorX = 5
	b.cursorY = 0

	b.eraseDisplay("2")
	assert.Equal(t, rune(0), b.lines[0].cells[0].char)
	assert.Equal(t, 0, b.cursorX)
	assert.Equal(t, 0, b.cursorY)

	// Test mode 3 (also clears entire screen)
	b = NewScreenBuffer(10, 5)
	for i := 0; i < 10; i++ {
		b.lines[0].cells[i].char = 'A'
	}
	b.eraseDisplay("3")
	assert.Equal(t, rune(0), b.lines[0].cells[0].char)
}

func TestScreenBuffer_EraseLine(t *testing.T) {
	b := NewScreenBuffer(10, 5)
	b.Write([]byte("AAAAAAAAAA"))
	b.cursorX = 5
	b.cursorY = 0

	// Erase from cursor to end (mode 0)
	b.eraseLine("0")
	assert.Equal(t, 'A', b.lines[0].cells[0].char)
	assert.Equal(t, rune(0), b.lines[0].cells[5].char)

	// Reset
	b = NewScreenBuffer(10, 5)
	b.Write([]byte("AAAAAAAAAA"))
	b.cursorX = 5
	b.cursorY = 0

	// Erase from start to cursor (mode 1)
	b.eraseLine("1")
	assert.Equal(t, rune(0), b.lines[0].cells[0].char)
	assert.Equal(t, 'A', b.lines[0].cells[6].char)

	// Reset
	b = NewScreenBuffer(10, 5)
	b.Write([]byte("AAAAAAAAAA"))
	b.cursorX = 5
	b.cursorY = 0

	// Erase entire line (mode 2)
	b.eraseLine("2")
	assert.Equal(t, rune(0), b.lines[0].cells[0].char)
	assert.Equal(t, rune(0), b.lines[0].cells[9].char)
}

func TestScreenBuffer_SGR(t *testing.T) {
	b := NewScreenBuffer(10, 5)

	// Test reset
	b.sgr("")
	assert.Equal(t, ColorBasic, b.currentFg.Type)
	assert.Equal(t, 7, b.currentFg.Index)

	// Test bold
	b.sgr("1")
	assert.True(t, b.bold)

	// Test normal intensity
	b.sgr("22")
	assert.False(t, b.bold)

	// Test blink off (should be ignored)
	b.sgr("25")

	// Test foreground color with bold
	b.bold = true
	b.sgr("31")                           // Red
	assert.Equal(t, 9, b.currentFg.Index) // Bold adds 8

	// Test default foreground
	b.sgr("39")
	assert.Equal(t, 7, b.currentFg.Index)

	// Test background color
	b.sgr("41") // Red background
	assert.Equal(t, ColorBasic, b.currentBg.Type)
	assert.Equal(t, 1, b.currentBg.Index)

	// Test default background
	b.sgr("49")
	assert.Equal(t, ColorDefault, b.currentBg.Type)

	// Test bright foreground
	b.sgr("91") // Bright red
	assert.Equal(t, 9, b.currentFg.Index)

	// Test bright background
	b.sgr("101") // Bright red background
	assert.Equal(t, 9, b.currentBg.Index)

	// Test 256 color foreground
	b.sgr("38;5;196")
	assert.Equal(t, Color256, b.currentFg.Type)
	assert.Equal(t, 196, b.currentFg.Index)

	// Test 256 color background
	b.sgr("48;5;220")
	assert.Equal(t, Color256, b.currentBg.Type)
	assert.Equal(t, 220, b.currentBg.Index)

	// Test RGB foreground
	b.sgr("38;2;100;150;200")
	assert.Equal(t, ColorRGB, b.currentFg.Type)
	assert.Equal(t, uint8(100), b.currentFg.R)
	assert.Equal(t, uint8(150), b.currentFg.G)
	assert.Equal(t, uint8(200), b.currentFg.B)

	// Test RGB background
	b.sgr("48;2;50;60;70")
	assert.Equal(t, ColorRGB, b.currentBg.Type)
	assert.Equal(t, uint8(50), b.currentBg.R)
}

func TestScreenBuffer_ClearLine(t *testing.T) {
	b := NewScreenBuffer(10, 5)
	b.Write([]byte("AAAAAAAAAA"))

	// Clear valid line
	b.clearLine(0, 0, 5)
	assert.Equal(t, rune(0), b.lines[0].cells[0].char)
	assert.Equal(t, 'A', b.lines[0].cells[5].char)

	// Clear with invalid y (negative)
	b.clearLine(-1, 0, 5) // Should not panic

	// Clear with invalid y (too large)
	b.clearLine(100, 0, 5) // Should not panic
}

func TestTerminal_BasicColorOutOfRange(t *testing.T) {
	term := New(&Config{Command: "echo", Width: 100, Height: 100})

	// Valid index
	c := term.basicColor(0)
	assert.Equal(t, uint8(0), c.R)

	// Out of range index - should return fallback (standard white)
	c = term.basicColor(100)
	assert.Equal(t, uint8(229), c.R)

	// Negative index - should return fallback
	c = term.basicColor(-1)
	assert.Equal(t, uint8(229), c.R)
}

func TestTerminal_ColorToRGBAUnknownType(t *testing.T) {
	term := New(&Config{Command: "echo", Width: 100, Height: 100})

	// Unknown color type - should return fallback (standard white)
	c := term.colorToRGBA(Color{Type: ColorType(99)})
	assert.Equal(t, uint8(229), c.R)
}

func TestScreenBuffer_ProcessEscapeNonCSI(t *testing.T) {
	b := NewScreenBuffer(10, 5)

	// Non-CSI escape sequence (doesn't start with '[')
	b.escapeSeq = "M"
	b.processEscape() // Should not panic

	// Empty CSI sequence
	b.escapeSeq = "["
	b.processEscape() // Should not panic
}

func TestScreenBuffer_SGREdgeCases(t *testing.T) {
	b := NewScreenBuffer(10, 5)

	// Test 38 without enough params
	b.sgr("38")
	// Should not panic

	// Test 38;5 without color index
	b.sgr("38;5")
	// Should not panic

	// Test 38;2 without RGB values
	b.sgr("38;2")
	b.sgr("38;2;100")
	b.sgr("38;2;100;150")
	// Should not panic

	// Test 48 without enough params
	b.sgr("48")
	// Should not panic

	// Test 48;5 without color index
	b.sgr("48;5")
	// Should not panic

	// Test 48;2 without RGB values
	b.sgr("48;2")
	b.sgr("48;2;100")
	b.sgr("48;2;100;150")
	// Should not panic
}
