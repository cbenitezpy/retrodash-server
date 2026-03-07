package terminal

import (
	"strconv"
	"strings"
	"time"
)

// Color represents a terminal color (basic, 256, or RGB).
type Color struct {
	Type    ColorType
	Index   int   // For basic (0-15) or 256 (0-255) colors
	R, G, B uint8 // For RGB colors
}

// ColorType indicates the type of color.
type ColorType int

const (
	ColorDefault ColorType = iota
	ColorBasic             // 0-15 (standard ANSI)
	Color256               // 0-255 (extended)
	ColorRGB               // 24-bit RGB
)

// Cell represents a single character cell in the terminal.
type Cell struct {
	char rune
	fg   Color
	bg   Color
	bold bool
}

// Line represents a line of cells.
type Line struct {
	cells []Cell
}

// ScreenBuffer holds the terminal screen state.
type ScreenBuffer struct {
	lines         []Line
	cols          int
	rows          int
	cursorX       int
	cursorY       int
	cursorVisible bool
	currentFg     Color
	currentBg     Color
	bold          bool
	lastUpdate    time.Time

	// Scroll-back buffer
	scrollBack    []Line
	maxScrollBack int

	// ANSI parser state
	inEscape  bool
	escapeSeq string
}

// NewScreenBuffer creates a new screen buffer.
func NewScreenBuffer(cols, rows int) *ScreenBuffer {
	lines := make([]Line, rows)
	for i := range lines {
		lines[i] = Line{cells: make([]Cell, cols)}
	}

	return &ScreenBuffer{
		lines:         lines,
		cols:          cols,
		rows:          rows,
		cursorVisible: true,
		currentFg:     Color{Type: ColorBasic, Index: 7}, // Default white
		currentBg:     Color{Type: ColorDefault},         // Default background
		maxScrollBack: 1000,                              // Keep 1000 lines of history
	}
}

// Write processes bytes and updates the buffer.
func (b *ScreenBuffer) Write(data []byte) {
	b.lastUpdate = time.Now()

	for _, c := range data {
		b.processByte(c)
	}
}

// processByte handles a single byte of input.
func (b *ScreenBuffer) processByte(c byte) {
	if b.inEscape {
		b.escapeSeq += string(c)

		// Check if escape sequence is complete
		if b.isEscapeComplete() {
			b.processEscape()
			b.inEscape = false
			b.escapeSeq = ""
		}
		return
	}

	switch c {
	case 0x1b: // ESC
		b.inEscape = true
		b.escapeSeq = ""
	case '\r': // Carriage return
		b.cursorX = 0
	case '\n': // Line feed
		b.newLine()
	case '\b': // Backspace
		if b.cursorX > 0 {
			b.cursorX--
		}
	case '\t': // Tab
		b.cursorX = ((b.cursorX / 8) + 1) * 8
		if b.cursorX >= b.cols {
			b.cursorX = b.cols - 1
		}
	default:
		if c >= 32 && c < 127 {
			b.putChar(rune(c))
		}
	}
}

// isEscapeComplete checks if the current escape sequence is complete.
func (b *ScreenBuffer) isEscapeComplete() bool {
	if len(b.escapeSeq) == 0 {
		return false
	}

	// CSI sequences end with a letter
	if strings.HasPrefix(b.escapeSeq, "[") {
		last := b.escapeSeq[len(b.escapeSeq)-1]
		return (last >= 'A' && last <= 'Z') || (last >= 'a' && last <= 'z')
	}

	// Single character escapes
	return len(b.escapeSeq) == 1
}

// processEscape handles an escape sequence.
func (b *ScreenBuffer) processEscape() {
	if !strings.HasPrefix(b.escapeSeq, "[") {
		return
	}

	seq := b.escapeSeq[1:] // Remove '['
	if len(seq) == 0 {
		return
	}

	cmd := seq[len(seq)-1]
	params := seq[:len(seq)-1]

	switch cmd {
	case 'H', 'f': // Cursor position
		b.cursorPosition(params)
	case 'A': // Cursor up
		b.cursorUp(params)
	case 'B': // Cursor down
		b.cursorDown(params)
	case 'C': // Cursor forward
		b.cursorForward(params)
	case 'D': // Cursor back
		b.cursorBack(params)
	case 'J': // Erase display
		b.eraseDisplay(params)
	case 'K': // Erase line
		b.eraseLine(params)
	case 'm': // SGR (colors/attributes)
		b.sgr(params)
	}
}

// cursorPosition sets cursor to row;col.
func (b *ScreenBuffer) cursorPosition(params string) {
	parts := strings.Split(params, ";")
	row, col := 1, 1

	if len(parts) >= 1 && parts[0] != "" {
		if r, err := strconv.Atoi(parts[0]); err == nil {
			row = r
		}
	}
	if len(parts) >= 2 && parts[1] != "" {
		if c, err := strconv.Atoi(parts[1]); err == nil {
			col = c
		}
	}

	b.cursorY = clamp(row-1, 0, b.rows-1)
	b.cursorX = clamp(col-1, 0, b.cols-1)
}

// cursorUp moves cursor up n rows.
func (b *ScreenBuffer) cursorUp(params string) {
	n := parseParam(params, 1)
	b.cursorY = clamp(b.cursorY-n, 0, b.rows-1)
}

// cursorDown moves cursor down n rows.
func (b *ScreenBuffer) cursorDown(params string) {
	n := parseParam(params, 1)
	b.cursorY = clamp(b.cursorY+n, 0, b.rows-1)
}

// cursorForward moves cursor forward n cols.
func (b *ScreenBuffer) cursorForward(params string) {
	n := parseParam(params, 1)
	b.cursorX = clamp(b.cursorX+n, 0, b.cols-1)
}

// cursorBack moves cursor back n cols.
func (b *ScreenBuffer) cursorBack(params string) {
	n := parseParam(params, 1)
	b.cursorX = clamp(b.cursorX-n, 0, b.cols-1)
}

// eraseDisplay clears parts of the screen.
func (b *ScreenBuffer) eraseDisplay(params string) {
	n := parseParam(params, 0)

	switch n {
	case 0: // Clear from cursor to end
		b.clearLine(b.cursorY, b.cursorX, b.cols)
		for y := b.cursorY + 1; y < b.rows; y++ {
			b.clearLine(y, 0, b.cols)
		}
	case 1: // Clear from start to cursor
		for y := 0; y < b.cursorY; y++ {
			b.clearLine(y, 0, b.cols)
		}
		b.clearLine(b.cursorY, 0, b.cursorX+1)
	case 2, 3: // Clear entire screen
		for y := 0; y < b.rows; y++ {
			b.clearLine(y, 0, b.cols)
		}
		b.cursorX, b.cursorY = 0, 0
	}
}

// eraseLine clears parts of the current line.
func (b *ScreenBuffer) eraseLine(params string) {
	n := parseParam(params, 0)

	switch n {
	case 0: // Clear from cursor to end
		b.clearLine(b.cursorY, b.cursorX, b.cols)
	case 1: // Clear from start to cursor
		b.clearLine(b.cursorY, 0, b.cursorX+1)
	case 2: // Clear entire line
		b.clearLine(b.cursorY, 0, b.cols)
	}
}

// sgr handles Select Graphic Rendition (colors/attributes).
func (b *ScreenBuffer) sgr(params string) {
	if params == "" {
		params = "0"
	}

	parts := strings.Split(params, ";")
	for i := 0; i < len(parts); i++ {
		n := parseParam(parts[i], 0)

		switch {
		case n == 0: // Reset
			b.currentFg = Color{Type: ColorBasic, Index: 7}
			b.currentBg = Color{Type: ColorDefault}
			b.bold = false
		case n == 1: // Bold
			b.bold = true
		case n == 22: // Normal intensity
			b.bold = false
		case n == 25: // Blink off
			// Ignore
		case n >= 30 && n <= 37: // Foreground color
			idx := n - 30
			if b.bold {
				idx += 8
			}
			b.currentFg = Color{Type: ColorBasic, Index: idx}
		case n == 38: // Extended foreground color
			if i+1 < len(parts) {
				mode := parseParam(parts[i+1], 0)
				if mode == 5 && i+2 < len(parts) {
					// 256 color: ESC[38;5;⟨n⟩m
					colorIdx := parseParam(parts[i+2], 0)
					b.currentFg = Color{Type: Color256, Index: colorIdx}
					i += 2
				} else if mode == 2 && i+4 < len(parts) {
					// RGB color: ESC[38;2;⟨r⟩;⟨g⟩;⟨b⟩m
					r := parseParam(parts[i+2], 0)
					g := parseParam(parts[i+3], 0)
					b_ := parseParam(parts[i+4], 0)
					b.currentFg = Color{Type: ColorRGB, R: uint8(r), G: uint8(g), B: uint8(b_)}
					i += 4
				}
			}
		case n == 39: // Default foreground
			b.currentFg = Color{Type: ColorBasic, Index: 7}
		case n >= 40 && n <= 47: // Background color
			b.currentBg = Color{Type: ColorBasic, Index: n - 40}
		case n == 48: // Extended background color
			if i+1 < len(parts) {
				mode := parseParam(parts[i+1], 0)
				if mode == 5 && i+2 < len(parts) {
					// 256 color
					colorIdx := parseParam(parts[i+2], 0)
					b.currentBg = Color{Type: Color256, Index: colorIdx}
					i += 2
				} else if mode == 2 && i+4 < len(parts) {
					// RGB color
					r := parseParam(parts[i+2], 0)
					g := parseParam(parts[i+3], 0)
					b_ := parseParam(parts[i+4], 0)
					b.currentBg = Color{Type: ColorRGB, R: uint8(r), G: uint8(g), B: uint8(b_)}
					i += 4
				}
			}
		case n == 49: // Default background
			b.currentBg = Color{Type: ColorDefault}
		case n >= 90 && n <= 97: // Bright foreground
			b.currentFg = Color{Type: ColorBasic, Index: n - 90 + 8}
		case n >= 100 && n <= 107: // Bright background
			b.currentBg = Color{Type: ColorBasic, Index: n - 100 + 8}
		}
	}
}

// putChar places a character at the current position.
func (b *ScreenBuffer) putChar(c rune) {
	if b.cursorY >= 0 && b.cursorY < b.rows && b.cursorX >= 0 && b.cursorX < b.cols {
		b.lines[b.cursorY].cells[b.cursorX] = Cell{
			char: c,
			fg:   b.currentFg,
			bg:   b.currentBg,
			bold: b.bold,
		}
	}

	b.cursorX++
	if b.cursorX >= b.cols {
		b.cursorX = 0
		b.newLine()
	}
}

// newLine moves to the next line, scrolling if necessary.
func (b *ScreenBuffer) newLine() {
	b.cursorY++
	if b.cursorY >= b.rows {
		// Save top line to scroll-back buffer
		if b.maxScrollBack > 0 {
			// Make a copy of the line being scrolled out
			oldLine := Line{cells: make([]Cell, len(b.lines[0].cells))}
			copy(oldLine.cells, b.lines[0].cells)
			b.scrollBack = append(b.scrollBack, oldLine)

			// Trim scroll-back if too large
			if len(b.scrollBack) > b.maxScrollBack {
				b.scrollBack = b.scrollBack[1:]
			}
		}

		// Scroll up
		copy(b.lines[:b.rows-1], b.lines[1:])
		b.lines[b.rows-1] = Line{cells: make([]Cell, b.cols)}
		b.cursorY = b.rows - 1
	}
}

// ScrollBackLines returns the number of lines in scroll-back buffer.
func (b *ScreenBuffer) ScrollBackLines() int {
	return len(b.scrollBack)
}

// GetScrollBackLine returns a line from scroll-back buffer (0 = oldest).
func (b *ScreenBuffer) GetScrollBackLine(idx int) []Cell {
	if idx < 0 || idx >= len(b.scrollBack) {
		return nil
	}
	return b.scrollBack[idx].cells
}

// clearLine clears cells in a line from start to end.
func (b *ScreenBuffer) clearLine(y, start, end int) {
	if y < 0 || y >= b.rows {
		return
	}
	for x := start; x < end && x < b.cols; x++ {
		b.lines[y].cells[x] = Cell{}
	}
}

// parseParam parses a numeric parameter with default value.
func parseParam(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

// clamp restricts a value to a range.
func clamp(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
