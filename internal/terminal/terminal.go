// Package terminal provides terminal command execution and rendering.
package terminal

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// Terminal represents a terminal emulator running a command.
type Terminal struct {
	cmd     string
	args    []string
	width   int
	height  int
	cols    int
	rows    int
	pty     *os.File
	process *exec.Cmd
	buffer  *ScreenBuffer
	mu      sync.RWMutex
	running bool
}

// Config holds terminal configuration.
type Config struct {
	// Command is the command to run (without arguments).
	Command string
	// Args are the command arguments (optional).
	Args []string
	// Width is the viewport width in pixels.
	Width int
	// Height is the viewport height in pixels.
	Height int
}

// ParseCommandURL parses a cmd:// URL and returns command and args.
// Formats supported:
//   - cmd://top
//   - cmd://htop -d 1
//   - cmd://htop?args=-d,1
//   - cmd://watch?args=-n,1,df,-h
func ParseCommandURL(cmdURL string) (command string, args []string, ok bool) {
	if !strings.HasPrefix(cmdURL, "cmd://") {
		return "", nil, false
	}

	cmdPart := strings.TrimPrefix(cmdURL, "cmd://")

	// Check for query params style: cmd://htop?args=-d,1
	if idx := strings.Index(cmdPart, "?"); idx != -1 {
		command = cmdPart[:idx]
		query := cmdPart[idx+1:]

		// Parse args=arg1,arg2
		for _, param := range strings.Split(query, "&") {
			if strings.HasPrefix(param, "args=") {
				argsStr := strings.TrimPrefix(param, "args=")
				if argsStr != "" {
					args = strings.Split(argsStr, ",")
				}
			}
		}
	} else {
		// Space-separated style: cmd://htop -d 1
		parts := strings.Fields(cmdPart)
		if len(parts) > 0 {
			command = parts[0]
			if len(parts) > 1 {
				args = parts[1:]
			}
		}
	}

	return command, args, command != ""
}

// IsCommandURL checks if the URL is a command URL.
func IsCommandURL(url string) bool {
	return strings.HasPrefix(url, "cmd://")
}

// New creates a new terminal with the given configuration.
// If cfg.Args is provided, it's used directly. Otherwise, cfg.Command is parsed
// to extract command and arguments (for backward compatibility).
func New(cfg *Config) *Terminal {
	// Calculate character grid based on scaled font size (14x26 = 7x13 * 2)
	// We use scale=2 in renderToImage for better readability
	scale := 2
	charWidth := 7 * scale   // 14
	charHeight := 13 * scale // 26
	cols := cfg.Width / charWidth
	rows := cfg.Height / charHeight

	// Use Args if provided, otherwise parse Command
	cmd := cfg.Command
	args := cfg.Args
	if args == nil && strings.Contains(cfg.Command, " ") {
		// Backward compatibility: parse command string
		parts := strings.Fields(cfg.Command)
		cmd = parts[0]
		if len(parts) > 1 {
			args = parts[1:]
		}
	}

	return &Terminal{
		cmd:    cmd,
		args:   args,
		width:  cfg.Width,
		height: cfg.Height,
		cols:   cols,
		rows:   rows,
		buffer: NewScreenBuffer(cols, rows),
	}
}

// Start starts the terminal command.
// Note: The context is only used for startup validation, not for the process lifecycle.
// Use Stop() to terminate the terminal process.
func (t *Terminal) Start(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Create command without context - process lifecycle is managed by Stop()
	// Using CommandContext would kill the process when the startup context is cancelled
	t.process = exec.Command(t.cmd, t.args...)
	t.process.Env = append(os.Environ(),
		"TERM=xterm-256color",
		fmt.Sprintf("COLUMNS=%d", t.cols),
		fmt.Sprintf("LINES=%d", t.rows),
	)

	// Start with PTY
	var err error
	t.pty, err = pty.StartWithSize(t.process, &pty.Winsize{
		Rows: uint16(t.rows),
		Cols: uint16(t.cols),
	})
	if err != nil {
		return fmt.Errorf("failed to start pty: %w", err)
	}

	t.running = true

	// Read output in background
	go t.readOutput()

	return nil
}

// readOutput continuously reads from PTY and updates buffer.
func (t *Terminal) readOutput() {
	buf := make([]byte, 4096)
	for {
		n, err := t.pty.Read(buf)
		if err != nil {
			if err != io.EOF {
				log.Printf("Terminal read error: %v", err)
			} else {
				log.Println("Terminal: PTY EOF, command exited")
			}
			t.mu.Lock()
			t.running = false
			t.mu.Unlock()
			return
		}

		if n > 0 {
			t.mu.Lock()
			t.buffer.Write(buf[:n])
			t.mu.Unlock()
		}
	}
}

// Stop stops the terminal gracefully.
// It first sends SIGINT and waits briefly for graceful exit,
// then forces SIGKILL if needed, and finally closes the PTY.
func (t *Terminal) Stop() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	var errs []error

	// First, try graceful shutdown with SIGINT
	if t.process != nil && t.process.Process != nil {
		if err := t.process.Process.Signal(os.Interrupt); err != nil {
			// Process might already be dead, try kill directly
			log.Printf("SIGINT failed: %v, forcing kill", err)
			if killErr := t.process.Process.Kill(); killErr != nil {
				errs = append(errs, fmt.Errorf("kill failed: %w", killErr))
			}
		} else {
			// Wait briefly for graceful exit
			done := make(chan struct{})
			proc := t.process // Capture before goroutine to avoid race
			go func() {
				proc.Wait() //nolint:errcheck // waiting for process exit, error irrelevant
				close(done)
			}()

			select {
			case <-done:
				// Process exited gracefully
			case <-time.After(500 * time.Millisecond):
				// Timeout, force kill
				log.Println("Process didn't exit gracefully, forcing kill")
				if err := t.process.Process.Kill(); err != nil {
					errs = append(errs, fmt.Errorf("force kill failed: %w", err))
				}
			}
		}
		t.process = nil
	}

	// Close PTY after process is stopped
	if t.pty != nil {
		if err := t.pty.Close(); err != nil {
			errs = append(errs, fmt.Errorf("pty close failed: %w", err))
		}
		t.pty = nil
	}

	t.running = false

	if len(errs) > 0 {
		return fmt.Errorf("stop errors: %v", errs)
	}
	return nil
}

// CaptureScreenshot captures the current terminal state as JPEG.
func (t *Terminal) CaptureScreenshot(ctx context.Context, quality int) ([]byte, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if !t.running {
		return nil, fmt.Errorf("terminal not running")
	}

	// Render buffer to image
	img := t.renderToImage()

	// Encode to JPEG
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return nil, fmt.Errorf("jpeg encode failed: %w", err)
	}

	return buf.Bytes(), nil
}

// renderToImage renders the screen buffer to an image.
func (t *Terminal) renderToImage() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, t.width, t.height))

	// Fill background with black using direct pixel buffer manipulation
	// This is much faster than img.Set() in nested loops
	t.fillRect(img, 0, 0, t.width, t.height, color.RGBA{0, 0, 0, 255})

	// Use basicfont but scale up for readability
	// Scale factor of 2 gives us 14x26 effective character size
	scale := 2
	face := basicfont.Face7x13
	baseCharWidth := 7
	baseCharHeight := 13
	charWidth := baseCharWidth * scale
	charHeight := baseCharHeight * scale

	// Draw each character
	for row := 0; row < t.rows && row < len(t.buffer.lines); row++ {
		line := t.buffer.lines[row]

		for col := 0; col < len(line.cells) && col < t.cols; col++ {
			cell := line.cells[col]
			cellX := col * charWidth
			cellY := row * charHeight

			// Draw cell background if not default
			if cell.bg.Type != ColorDefault {
				bgC := t.colorToRGBA(cell.bg)
				t.fillRect(img, cellX, cellY, charWidth, charHeight, bgC)
			}

			// Skip empty characters
			if cell.char == 0 || cell.char == ' ' {
				continue
			}

			// Draw character scaled up
			fgC := t.colorToRGBA(cell.fg)
			t.drawScaledChar(img, face, cell.char, cellX, cellY, charHeight, scale, fgC)
		}
	}

	// Draw cursor (blinking effect based on time)
	if t.buffer.cursorVisible {
		cursorBlink := (time.Now().UnixMilli()/500)%2 == 0
		if cursorBlink {
			cursorX := t.buffer.cursorX * charWidth
			cursorY := t.buffer.cursorY * charHeight
			cursorColor := color.RGBA{229, 229, 229, 255} // Standard terminal white

			// Draw block cursor - clamp to image bounds
			cursorW := charWidth
			cursorH := charHeight
			if cursorX+cursorW > t.width {
				cursorW = t.width - cursorX
			}
			if cursorY+cursorH > t.height {
				cursorH = t.height - cursorY
			}
			if cursorW > 0 && cursorH > 0 {
				t.fillRect(img, cursorX, cursorY, cursorW, cursorH, cursorColor)
			}
		}
	}

	return img
}

// fillRect fills a rectangle using direct pixel buffer manipulation.
// This is significantly faster than using img.Set() in nested loops.
func (t *Terminal) fillRect(img *image.RGBA, x, y, w, h int, c color.RGBA) {
	// Bounds checking
	if x < 0 {
		w += x
		x = 0
	}
	if y < 0 {
		h += y
		y = 0
	}
	if x+w > img.Bounds().Dx() {
		w = img.Bounds().Dx() - x
	}
	if y+h > img.Bounds().Dy() {
		h = img.Bounds().Dy() - y
	}
	if w <= 0 || h <= 0 {
		return
	}

	// Fill first row directly into pixel buffer
	stride := img.Stride
	firstRowOffset := y*stride + x*4
	for px := 0; px < w; px++ {
		offset := firstRowOffset + px*4
		img.Pix[offset] = c.R
		img.Pix[offset+1] = c.G
		img.Pix[offset+2] = c.B
		img.Pix[offset+3] = c.A
	}

	// Copy first row to remaining rows
	rowBytes := w * 4
	for py := 1; py < h; py++ {
		rowOffset := (y+py)*stride + x*4
		copy(img.Pix[rowOffset:rowOffset+rowBytes], img.Pix[firstRowOffset:firstRowOffset+rowBytes])
	}
}

// drawScaledChar draws a character scaled up using the font face.
func (t *Terminal) drawScaledChar(img *image.RGBA, face font.Face, char rune, x, y, charHeight, scale int, fg color.RGBA) {
	// Create a small image for the character
	smallW := 7
	smallH := 13
	small := image.NewRGBA(image.Rect(0, 0, smallW, smallH))

	drawer := &font.Drawer{
		Dst:  small,
		Src:  image.NewUniform(fg),
		Face: face,
		Dot:  fixed.Point26_6{X: 0, Y: fixed.I(smallH - 2)},
	}
	drawer.DrawString(string(char))

	// Scale up to target image using direct pixel buffer access
	imgStride := img.Stride
	imgPix := img.Pix
	imgWidth := t.width
	imgHeight := t.height

	for sy := 0; sy < smallH; sy++ {
		// Get source row offset
		srcRowOffset := sy * small.Stride

		for sx := 0; sx < smallW; sx++ {
			srcOffset := srcRowOffset + sx*4
			// Check alpha directly from pixel buffer (faster than RGBAAt)
			if small.Pix[srcOffset+3] > 0 {
				r, g, b, a := small.Pix[srcOffset], small.Pix[srcOffset+1], small.Pix[srcOffset+2], small.Pix[srcOffset+3]

				// Calculate base position for scaled pixel
				baseX := x + sx*scale
				baseY := y + sy*scale

				// Draw scaled block
				for dy := 0; dy < scale; dy++ {
					py := baseY + dy
					if py >= imgHeight {
						break
					}
					rowOffset := py * imgStride

					for dx := 0; dx < scale; dx++ {
						px := baseX + dx
						if px >= imgWidth {
							break
						}
						dstOffset := rowOffset + px*4
						imgPix[dstOffset] = r
						imgPix[dstOffset+1] = g
						imgPix[dstOffset+2] = b
						imgPix[dstOffset+3] = a
					}
				}
			}
		}
	}
}

// colorToRGBA converts a Color to color.RGBA.
func (t *Terminal) colorToRGBA(c Color) color.RGBA {
	switch c.Type {
	case ColorDefault:
		// Use standard terminal white (color 7) for default foreground
		// This ensures proper contrast and matches terminal conventions
		return color.RGBA{229, 229, 229, 255}
	case ColorBasic:
		return t.basicColor(c.Index)
	case Color256:
		return t.color256(c.Index)
	case ColorRGB:
		return color.RGBA{c.R, c.G, c.B, 255}
	default:
		// Fallback to standard terminal white
		return color.RGBA{229, 229, 229, 255}
	}
}

// basicColor returns basic 16 ANSI colors.
func (t *Terminal) basicColor(idx int) color.RGBA {
	colors := []color.RGBA{
		{0, 0, 0, 255},       // 0: Black
		{205, 0, 0, 255},     // 1: Red
		{0, 205, 0, 255},     // 2: Green
		{205, 205, 0, 255},   // 3: Yellow
		{0, 0, 238, 255},     // 4: Blue
		{205, 0, 205, 255},   // 5: Magenta
		{0, 205, 205, 255},   // 6: Cyan
		{229, 229, 229, 255}, // 7: White
		{127, 127, 127, 255}, // 8: Bright Black
		{255, 0, 0, 255},     // 9: Bright Red
		{0, 255, 0, 255},     // 10: Bright Green
		{255, 255, 0, 255},   // 11: Bright Yellow
		{92, 92, 255, 255},   // 12: Bright Blue
		{255, 0, 255, 255},   // 13: Bright Magenta
		{0, 255, 255, 255},   // 14: Bright Cyan
		{255, 255, 255, 255}, // 15: Bright White
	}
	if idx >= 0 && idx < len(colors) {
		return colors[idx]
	}
	// Fallback to standard terminal white (color 7)
	return color.RGBA{229, 229, 229, 255}
}

// color256 converts 256-color index to RGB.
func (t *Terminal) color256(idx int) color.RGBA {
	if idx < 16 {
		return t.basicColor(idx)
	}

	if idx < 232 {
		// 216 color cube (6x6x6)
		idx -= 16
		r := uint8((idx / 36) * 51)
		g := uint8(((idx / 6) % 6) * 51)
		b := uint8((idx % 6) * 51)
		return color.RGBA{r, g, b, 255}
	}

	// Grayscale (24 shades)
	gray := uint8((idx-232)*10 + 8)
	return color.RGBA{gray, gray, gray, 255}
}

// IsRunning returns true if the terminal is running.
func (t *Terminal) IsRunning() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.running
}

// IsReady returns true if the terminal is ready for capture.
func (t *Terminal) IsReady() bool {
	return t.IsRunning()
}

// ViewportSize returns the configured viewport dimensions.
func (t *Terminal) ViewportSize() (width, height int) {
	return t.width, t.height
}

// LastActivity returns the time of last activity.
func (t *Terminal) LastActivity() time.Time {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.buffer.lastUpdate
}
