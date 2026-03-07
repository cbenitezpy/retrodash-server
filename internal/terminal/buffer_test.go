package terminal

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewScreenBuffer(t *testing.T) {
	cols, rows := 80, 24
	buf := NewScreenBuffer(cols, rows)

	assert.Equal(t, cols, buf.cols)
	assert.Equal(t, rows, buf.rows)
	assert.Equal(t, rows, len(buf.lines))
	assert.Equal(t, cols, len(buf.lines[0].cells))
	assert.Equal(t, 0, buf.cursorX)
	assert.Equal(t, 0, buf.cursorY)
}

func TestScreenBuffer_Write_Plain(t *testing.T) {
	buf := NewScreenBuffer(10, 5)
	buf.Write([]byte("Hello"))

	assert.Equal(t, 'H', buf.lines[0].cells[0].char)
	assert.Equal(t, 'e', buf.lines[0].cells[1].char)
	assert.Equal(t, 'o', buf.lines[0].cells[4].char)
	assert.Equal(t, 5, buf.cursorX)
}

func TestScreenBuffer_Write_Newline(t *testing.T) {
	buf := NewScreenBuffer(10, 5)
	buf.Write([]byte("Line 1"))
	buf.Write([]byte("\r\n"))
	buf.Write([]byte("Line 2"))

	assert.Equal(t, 'L', buf.lines[0].cells[0].char)
	assert.Equal(t, 'L', buf.lines[1].cells[0].char)
}

func TestScreenBuffer_Write_Wrapping(t *testing.T) {
	buf := NewScreenBuffer(5, 5)
	buf.Write([]byte("123456"))

	assert.Equal(t, '1', buf.lines[0].cells[0].char)
	assert.Equal(t, '5', buf.lines[0].cells[4].char)
	assert.Equal(t, '6', buf.lines[1].cells[0].char)
	assert.Equal(t, 1, buf.cursorY)
	assert.Equal(t, 1, buf.cursorX)
}

func TestScreenBuffer_Write_Scrolling(t *testing.T) {
	buf := NewScreenBuffer(10, 2)
	buf.Write([]byte("Line 1\r\nLine 2\r\nLine 3"))

	assert.Equal(t, 'L', buf.lines[0].cells[0].char)
	assert.Equal(t, '2', buf.lines[0].cells[5].char) // Line 2 is now at index 0
	assert.Equal(t, 'L', buf.lines[1].cells[0].char)
	assert.Equal(t, '3', buf.lines[1].cells[5].char) // Line 3 is now at index 1

	// Check scrollback
	assert.Equal(t, 1, buf.ScrollBackLines())
	line := buf.GetScrollBackLine(0)
	assert.Equal(t, 'L', line[0].char)
	assert.Equal(t, '1', line[5].char)
}

func TestScreenBuffer_ScrollBack_Limit(t *testing.T) {
	buf := NewScreenBuffer(10, 2)
	buf.maxScrollBack = 2
	buf.Write([]byte("1\r\n2\r\n3\r\n4\r\n5"))

	assert.Equal(t, 2, buf.ScrollBackLines())

	line0 := buf.GetScrollBackLine(0)
	assert.Equal(t, '2', line0[0].char)
	line1 := buf.GetScrollBackLine(1)
	assert.Equal(t, '3', line1[0].char)
}

func TestScreenBuffer_ControlChars(t *testing.T) {
	buf := NewScreenBuffer(10, 5)

	// CR
	buf.Write([]byte("Hello"))
	buf.Write([]byte("\rWorld"))
	assert.Equal(t, 'W', buf.lines[0].cells[0].char)
	assert.Equal(t, 'd', buf.lines[0].cells[4].char)

	// Tab
	buf.cursorX = 0
	buf.Write([]byte("\tA"))
	assert.Equal(t, 8, buf.cursorX-1)
	assert.Equal(t, 'A', buf.lines[0].cells[8].char)

	// Backspace
	buf.Write([]byte("\bB"))
	assert.Equal(t, 'B', buf.lines[0].cells[8].char)
}

func TestScreenBuffer_ANSI_Cursor(t *testing.T) {
	buf := NewScreenBuffer(10, 10)

	// Position
	buf.Write([]byte("\x1b[5;5H")) // 1-based, so 4,4
	assert.Equal(t, 4, buf.cursorY)
	assert.Equal(t, 4, buf.cursorX)

	// Up
	buf.Write([]byte("\x1b[2A"))
	assert.Equal(t, 2, buf.cursorY)

	// Down
	buf.Write([]byte("\x1b[3B"))
	assert.Equal(t, 5, buf.cursorY)

	// Forward
	buf.Write([]byte("\x1b[2C"))
	assert.Equal(t, 6, buf.cursorX)

	// Back
	buf.Write([]byte("\x1b[3D"))
	assert.Equal(t, 3, buf.cursorX)
}

func TestScreenBuffer_ANSI_Erase(t *testing.T) {
	buf := NewScreenBuffer(10, 5)

	// Fill screen
	for i := 0; i < 5; i++ {
		buf.Write([]byte("XXXXXXXXXX\r\n"))
	}

	// Reset cursor to 0,0
	buf.cursorX = 0
	buf.cursorY = 0

	// Clear screen (2J)
	buf.Write([]byte("\x1b[2J"))
	assert.Equal(t, rune(0), buf.lines[0].cells[0].char)
	assert.Equal(t, 0, buf.cursorX)
	assert.Equal(t, 0, buf.cursorY)

	// Fill line
	buf.Write([]byte("XXXXXXXXXX"))
	buf.cursorX = 5

	// Clear line to end (0K)
	// We must ensure we are on the correct line.
	// Writing 10 chars wrapped, so cursorY became 1.
	buf.cursorY = 0
	buf.cursorX = 5

	buf.Write([]byte("\x1b[0K"))
	assert.Equal(t, 'X', buf.lines[0].cells[4].char)
	assert.Equal(t, rune(0), buf.lines[0].cells[5].char)

	// Clear line start to cursor (1K)
	buf.cursorY = 0
	buf.cursorX = 5
	buf.Write([]byte("XXXXXXXXXX")) // Restore

	buf.cursorY = 0
	buf.cursorX = 5
	buf.Write([]byte("\x1b[1K"))
	assert.Equal(t, rune(0), buf.lines[0].cells[4].char)
	assert.Equal(t, 'X', buf.lines[0].cells[6].char)
}

func TestScreenBuffer_ANSI_SGR(t *testing.T) {
	buf := NewScreenBuffer(10, 5)

	// Bold
	buf.Write([]byte("\x1b[1mB"))
	assert.True(t, buf.lines[0].cells[0].bold)

	// Reset
	buf.Write([]byte("\x1b[0mR"))
	assert.False(t, buf.lines[0].cells[1].bold)

	// Red FG
	buf.Write([]byte("\x1b[31mC"))
	assert.Equal(t, ColorBasic, buf.lines[0].cells[2].fg.Type)
	assert.Equal(t, 1, buf.lines[0].cells[2].fg.Index)

	// Green BG
	buf.Write([]byte("\x1b[42mD"))
	assert.Equal(t, ColorBasic, buf.lines[0].cells[3].bg.Type)
	assert.Equal(t, 2, buf.lines[0].cells[3].bg.Index)

	// 256 color FG
	buf.Write([]byte("\x1b[38;5;200mE"))
	assert.Equal(t, Color256, buf.lines[0].cells[4].fg.Type)
	assert.Equal(t, 200, buf.lines[0].cells[4].fg.Index)

	// RGB color BG
	buf.Write([]byte("\x1b[48;2;10;20;30mF"))
	assert.Equal(t, ColorRGB, buf.lines[0].cells[5].bg.Type)
	assert.Equal(t, uint8(10), buf.lines[0].cells[5].bg.R)
}

func TestParseParam(t *testing.T) {
	assert.Equal(t, 10, parseParam("10", 0))
	assert.Equal(t, 0, parseParam("", 0))
	assert.Equal(t, 5, parseParam("invalid", 5))
}

func TestClamp(t *testing.T) {
	assert.Equal(t, 5, clamp(5, 0, 10))
	assert.Equal(t, 0, clamp(-5, 0, 10))
	assert.Equal(t, 10, clamp(15, 0, 10))
}

func TestGetScrollBackLine_Invalid(t *testing.T) {
	buf := NewScreenBuffer(10, 5)
	assert.Nil(t, buf.GetScrollBackLine(0))
}
