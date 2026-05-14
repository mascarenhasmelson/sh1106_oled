// Package sh1106 provides a driver for the SH1106 128x64 OLED display controller.
// Supports I2C and SPI interfaces via a simple Conn interface.
//
// Datasheet: https://cdn.velleman.eu/downloads/29/infosheets/sh1106_datasheet.pdf
//
// Wiring (I2C, most common):
//   VCC → 3.3V or 5V
//   GND → GND
//   SCL → I2C clock
//   SDA → I2C data
//
// Usage:
//
//	conn := NewI2CConn(i2cBus, 0x3C)
//	d := New(conn, Width128, Height64)
//	d.Init()
//	d.Clear()
//	d.DrawString(0, 0, "Hello, World!")
//	d.Display()
package sh1106

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const (
	Width128  = 128
	Height64  = 64
	Height32  = 32

	// SH1106 has 132 columns internally; visible 128 start at column offset 2.
	colOffset = 2

	// I2C control bytes
	i2cCmdByte  = 0x00 // Co=0, D/C#=0  → command stream
	i2cDataByte = 0x40 // Co=0, D/C#=1  → data stream
)

// SH1106 commands
const (
	cmdSetLowColumn        = 0x00
	cmdSetHighColumn       = 0x10
	cmdSetMemoryMode       = 0x20
	cmdSetStartLine        = 0x40
	cmdSetContrast         = 0x81
	cmdSetSegmentRemap0    = 0xA0
	cmdSetSegmentRemap1    = 0xA1
	cmdSetEntireDisplayOn  = 0xA4
	cmdSetEntireDisplayOff = 0xA5 // all pixels ON regardless of RAM
	cmdSetNormalDisplay    = 0xA6
	cmdSetInverseDisplay   = 0xA7
	cmdSetMultiplex        = 0xA8
	cmdDisplayOff          = 0xAE
	cmdDisplayOn           = 0xAF
	cmdSetPageAddress      = 0xB0
	cmdSetComScanDec       = 0xC8
	cmdSetComScanInc       = 0xC0
	cmdSetDisplayOffset    = 0xD3
	cmdSetDisplayClock     = 0xD5
	cmdSetPrecharge        = 0xD9
	cmdSetComPins          = 0xDA
	cmdSetVcomDetect       = 0xDB
	cmdChargePump          = 0x8D
)

// ---------------------------------------------------------------------------
// Conn interface — wire your platform's I2C/SPI here
// ---------------------------------------------------------------------------

// Conn is the low-level transport interface.
// Implement it for your platform (Linux periph.io, TinyGo machine.I2C, etc.)
type Conn interface {
	// WriteCmd sends one or more command bytes.
	WriteCmd(cmds ...byte) error
	// WriteData sends a block of pixel data bytes.
	WriteData(data []byte) error
}

// ---------------------------------------------------------------------------
// Device
// ---------------------------------------------------------------------------

// Device represents an SH1106 display.
type Device struct {
	conn   Conn
	width  int
	height int
	pages  int
	buf    []byte // frame buffer: pages × width bytes
}

// New creates a new SH1106 device.
// Typical call: New(conn, Width128, Height64)
func New(conn Conn, width, height int) *Device {
	pages := height / 8
	return &Device{
		conn:   conn,
		width:  width,
		height: height,
		pages:  pages,
		buf:    make([]byte, pages*width),
	}
}

// Init sends the initialisation sequence to the display.
func (d *Device) Init() error {
	cmds := []byte{
		cmdDisplayOff,
		cmdSetDisplayClock, 0x80,
		cmdSetMultiplex, byte(d.height - 1),
		cmdSetDisplayOffset, 0x00,
		cmdSetStartLine | 0x00,
		cmdChargePump, 0x14, // enable internal charge pump
		cmdSetSegmentRemap1,
		cmdSetComScanDec,
		cmdSetComPins, comPinsFor(d.height),
		cmdSetContrast, 0xCF,
		cmdSetPrecharge, 0xF1,
		cmdSetVcomDetect, 0x40,
		cmdSetEntireDisplayOn, // use RAM content
		cmdSetNormalDisplay,
		cmdDisplayOn,
	}
	for _, c := range cmds {
		if err := d.conn.WriteCmd(c); err != nil {
			return err
		}
	}
	return nil
}

func comPinsFor(height int) byte {
	if height == 32 {
		return 0x02
	}
	return 0x12 // 64-row
}

// ---------------------------------------------------------------------------
// Display control
// ---------------------------------------------------------------------------

// SetContrast sets display contrast 0–255.
func (d *Device) SetContrast(c byte) error {
	return d.conn.WriteCmd(cmdSetContrast, c)
}

// InvertDisplay turns on or off pixel inversion.
func (d *Device) InvertDisplay(inv bool) error {
	if inv {
		return d.conn.WriteCmd(cmdSetInverseDisplay)
	}
	return d.conn.WriteCmd(cmdSetNormalDisplay)
}

// DisplayOn turns the display on or off (without clearing RAM).
func (d *Device) DisplayOn(on bool) error {
	if on {
		return d.conn.WriteCmd(cmdDisplayOn)
	}
	return d.conn.WriteCmd(cmdDisplayOff)
}

// ---------------------------------------------------------------------------
// Frame buffer helpers
// ---------------------------------------------------------------------------

// Clear fills the frame buffer with zeros (black).
func (d *Device) Clear() {
	for i := range d.buf {
		d.buf[i] = 0
	}
}

// Fill fills the frame buffer (e.g. 0xFF = all pixels on).
func (d *Device) Fill(v byte) {
	for i := range d.buf {
		d.buf[i] = v
	}
}

// SetPixel sets or clears a single pixel at (x, y).
func (d *Device) SetPixel(x, y int, on bool) {
	if x < 0 || x >= d.width || y < 0 || y >= d.height {
		return
	}
	page := y / 8
	bit := uint(y % 8)
	idx := page*d.width + x
	if on {
		d.buf[idx] |= 1 << bit
	} else {
		d.buf[idx] &^= 1 << bit
	}
}

// Display flushes the frame buffer to the hardware page by page.
// SH1106 does NOT support continuous horizontal addressing across the full
// 128 columns (it uses page-mode with a 132-column internal memory), so we
// write each page individually.
func (d *Device) Display() error {
	for page := 0; page < d.pages; page++ {
		col := colOffset // start column (2 for SH1106 132→128 mapping)
		if err := d.conn.WriteCmd(
			cmdSetPageAddress|byte(page),
			cmdSetLowColumn|byte(col&0x0F),
			cmdSetHighColumn|byte(col>>4),
		); err != nil {
			return err
		}
		start := page * d.width
		if err := d.conn.WriteData(d.buf[start : start+d.width]); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Text rendering
// ---------------------------------------------------------------------------

// DrawChar draws a single ASCII character at pixel position (x, y).
// Uses the built-in 5×8 font (each char is 5 columns wide + 1 space).
func (d *Device) DrawChar(x, y int, ch byte) {
	if ch < 0x20 || ch > 0x7E {
		ch = '?'
	}
	glyph := font5x8[ch-0x20]
	for col := 0; col < 5; col++ {
		colData := glyph[col]
		for row := 0; row < 8; row++ {
			d.SetPixel(x+col, y+row, colData&(1<<uint(row)) != 0)
		}
	}
	// 1-pixel gap
	for row := 0; row < 8; row++ {
		d.SetPixel(x+5, y+row, false)
	}
}

// DrawString draws a string starting at pixel (x, y).
// Returns the x position after the last character.
func (d *Device) DrawString(x, y int, s string) int {
	for _, ch := range []byte(s) {
		d.DrawChar(x, y, ch)
		x += 6 // 5 pixels + 1 gap
		if x+6 > d.width {
			break // no wrapping — extend yourself if needed
		}
	}
	return x
}

// DrawStringWrapped draws a string with automatic line wrapping.
// lineHeight is typically 9 (8px font + 1px gap).
func (d *Device) DrawStringWrapped(x0, y int, s string, lineHeight int) {
	x := x0
	for _, ch := range []byte(s) {
		if x+6 > d.width {
			x = x0
			y += lineHeight
		}
		if y+8 > d.height {
			break
		}
		d.DrawChar(x, y, ch)
		x += 6
	}
}

// ---------------------------------------------------------------------------
// Basic drawing primitives
// ---------------------------------------------------------------------------

// DrawHLine draws a horizontal line.
func (d *Device) DrawHLine(x, y, w int) {
	for i := 0; i < w; i++ {
		d.SetPixel(x+i, y, true)
	}
}

// DrawVLine draws a vertical line.
func (d *Device) DrawVLine(x, y, h int) {
	for i := 0; i < h; i++ {
		d.SetPixel(x, y+i, true)
	}
}

// DrawRect draws a rectangle outline.
func (d *Device) DrawRect(x, y, w, h int) {
	d.DrawHLine(x, y, w)
	d.DrawHLine(x, y+h-1, w)
	d.DrawVLine(x, y, h)
	d.DrawVLine(x+w-1, y, h)
}

// FillRect fills a rectangle.
func (d *Device) FillRect(x, y, w, h int) {
	for row := 0; row < h; row++ {
		d.DrawHLine(x, y+row, w)
	}
}

// ---------------------------------------------------------------------------
// 5×8 ASCII font  (printable 0x20–0x7E)
// Each entry: [5]byte, one byte per column, LSB = top row
// ---------------------------------------------------------------------------

// font5x8 holds the 5-column bitmap for each printable ASCII character.
var font5x8 = [95][5]byte{
	{0x00, 0x00, 0x00, 0x00, 0x00}, // ' ' 0x20
	{0x00, 0x00, 0x5F, 0x00, 0x00}, // '!' 0x21
	{0x00, 0x07, 0x00, 0x07, 0x00}, // '"' 0x22
	{0x14, 0x7F, 0x14, 0x7F, 0x14}, // '#' 0x23
	{0x24, 0x2A, 0x7F, 0x2A, 0x12}, // '$' 0x24
	{0x23, 0x13, 0x08, 0x64, 0x62}, // '%' 0x25
	{0x36, 0x49, 0x55, 0x22, 0x50}, // '&' 0x26
	{0x00, 0x05, 0x03, 0x00, 0x00}, // ''' 0x27
	{0x00, 0x1C, 0x22, 0x41, 0x00}, // '(' 0x28
	{0x00, 0x41, 0x22, 0x1C, 0x00}, // ')' 0x29
	{0x14, 0x08, 0x3E, 0x08, 0x14}, // '*' 0x2A
	{0x08, 0x08, 0x3E, 0x08, 0x08}, // '+' 0x2B
	{0x00, 0x50, 0x30, 0x00, 0x00}, // ',' 0x2C
	{0x08, 0x08, 0x08, 0x08, 0x08}, // '-' 0x2D
	{0x00, 0x60, 0x60, 0x00, 0x00}, // '.' 0x2E
	{0x20, 0x10, 0x08, 0x04, 0x02}, // '/' 0x2F
	{0x3E, 0x51, 0x49, 0x45, 0x3E}, // '0' 0x30
	{0x00, 0x42, 0x7F, 0x40, 0x00}, // '1' 0x31
	{0x42, 0x61, 0x51, 0x49, 0x46}, // '2' 0x32
	{0x21, 0x41, 0x45, 0x4B, 0x31}, // '3' 0x33
	{0x18, 0x14, 0x12, 0x7F, 0x10}, // '4' 0x34
	{0x27, 0x45, 0x45, 0x45, 0x39}, // '5' 0x35
	{0x3C, 0x4A, 0x49, 0x49, 0x30}, // '6' 0x36
	{0x01, 0x71, 0x09, 0x05, 0x03}, // '7' 0x37
	{0x36, 0x49, 0x49, 0x49, 0x36}, // '8' 0x38
	{0x06, 0x49, 0x49, 0x29, 0x1E}, // '9' 0x39
	{0x00, 0x36, 0x36, 0x00, 0x00}, // ':' 0x3A
	{0x00, 0x56, 0x36, 0x00, 0x00}, // ';' 0x3B
	{0x08, 0x14, 0x22, 0x41, 0x00}, // '<' 0x3C
	{0x14, 0x14, 0x14, 0x14, 0x14}, // '=' 0x3D
	{0x00, 0x41, 0x22, 0x14, 0x08}, // '>' 0x3E
	{0x02, 0x01, 0x51, 0x09, 0x06}, // '?' 0x3F
	{0x32, 0x49, 0x79, 0x41, 0x3E}, // '@' 0x40
	{0x7E, 0x11, 0x11, 0x11, 0x7E}, // 'A' 0x41
	{0x7F, 0x49, 0x49, 0x49, 0x36}, // 'B' 0x42
	{0x3E, 0x41, 0x41, 0x41, 0x22}, // 'C' 0x43
	{0x7F, 0x41, 0x41, 0x22, 0x1C}, // 'D' 0x44
	{0x7F, 0x49, 0x49, 0x49, 0x41}, // 'E' 0x45
	{0x7F, 0x09, 0x09, 0x09, 0x01}, // 'F' 0x46
	{0x3E, 0x41, 0x49, 0x49, 0x7A}, // 'G' 0x47
	{0x7F, 0x08, 0x08, 0x08, 0x7F}, // 'H' 0x48
	{0x00, 0x41, 0x7F, 0x41, 0x00}, // 'I' 0x49
	{0x20, 0x40, 0x41, 0x3F, 0x01}, // 'J' 0x4A
	{0x7F, 0x08, 0x14, 0x22, 0x41}, // 'K' 0x4B
	{0x7F, 0x40, 0x40, 0x40, 0x40}, // 'L' 0x4C
	{0x7F, 0x02, 0x0C, 0x02, 0x7F}, // 'M' 0x4D
	{0x7F, 0x04, 0x08, 0x10, 0x7F}, // 'N' 0x4E
	{0x3E, 0x41, 0x41, 0x41, 0x3E}, // 'O' 0x4F
	{0x7F, 0x09, 0x09, 0x09, 0x06}, // 'P' 0x50
	{0x3E, 0x41, 0x51, 0x21, 0x5E}, // 'Q' 0x51
	{0x7F, 0x09, 0x19, 0x29, 0x46}, // 'R' 0x52
	{0x46, 0x49, 0x49, 0x49, 0x31}, // 'S' 0x53
	{0x01, 0x01, 0x7F, 0x01, 0x01}, // 'T' 0x54
	{0x3F, 0x40, 0x40, 0x40, 0x3F}, // 'U' 0x55
	{0x1F, 0x20, 0x40, 0x20, 0x1F}, // 'V' 0x56
	{0x3F, 0x40, 0x38, 0x40, 0x3F}, // 'W' 0x57
	{0x63, 0x14, 0x08, 0x14, 0x63}, // 'X' 0x58
	{0x07, 0x08, 0x70, 0x08, 0x07}, // 'Y' 0x59
	{0x61, 0x51, 0x49, 0x45, 0x43}, // 'Z' 0x5A
	{0x00, 0x7F, 0x41, 0x41, 0x00}, // '[' 0x5B
	{0x02, 0x04, 0x08, 0x10, 0x20}, // '\' 0x5C
	{0x00, 0x41, 0x41, 0x7F, 0x00}, // ']' 0x5D
	{0x04, 0x02, 0x01, 0x02, 0x04}, // '^' 0x5E
	{0x40, 0x40, 0x40, 0x40, 0x40}, // '_' 0x5F
	{0x00, 0x01, 0x02, 0x04, 0x00}, // '`' 0x60
	{0x20, 0x54, 0x54, 0x54, 0x78}, // 'a' 0x61
	{0x7F, 0x48, 0x44, 0x44, 0x38}, // 'b' 0x62
	{0x38, 0x44, 0x44, 0x44, 0x20}, // 'c' 0x63
	{0x38, 0x44, 0x44, 0x48, 0x7F}, // 'd' 0x64
	{0x38, 0x54, 0x54, 0x54, 0x18}, // 'e' 0x65
	{0x08, 0x7E, 0x09, 0x01, 0x02}, // 'f' 0x66
	{0x0C, 0x52, 0x52, 0x52, 0x3E}, // 'g' 0x67
	{0x7F, 0x08, 0x04, 0x04, 0x78}, // 'h' 0x68
	{0x00, 0x44, 0x7D, 0x40, 0x00}, // 'i' 0x69
	{0x20, 0x40, 0x44, 0x3D, 0x00}, // 'j' 0x6A
	{0x7F, 0x10, 0x28, 0x44, 0x00}, // 'k' 0x6B
	{0x00, 0x41, 0x7F, 0x40, 0x00}, // 'l' 0x6C
	{0x7C, 0x04, 0x18, 0x04, 0x78}, // 'm' 0x6D
	{0x7C, 0x08, 0x04, 0x04, 0x78}, // 'n' 0x6E
	{0x38, 0x44, 0x44, 0x44, 0x38}, // 'o' 0x6F
	{0x7C, 0x14, 0x14, 0x14, 0x08}, // 'p' 0x70
	{0x08, 0x14, 0x14, 0x18, 0x7C}, // 'q' 0x71
	{0x7C, 0x08, 0x04, 0x04, 0x08}, // 'r' 0x72
	{0x48, 0x54, 0x54, 0x54, 0x20}, // 's' 0x73
	{0x04, 0x3F, 0x44, 0x40, 0x20}, // 't' 0x74
	{0x3C, 0x40, 0x40, 0x40, 0x7C}, // 'u' 0x75
	{0x1C, 0x20, 0x40, 0x20, 0x1C}, // 'v' 0x76
	{0x3C, 0x40, 0x30, 0x40, 0x3C}, // 'w' 0x77
	{0x44, 0x28, 0x10, 0x28, 0x44}, // 'x' 0x78
	{0x0C, 0x50, 0x50, 0x50, 0x3C}, // 'y' 0x79
	{0x44, 0x64, 0x54, 0x4C, 0x44}, // 'z' 0x7A
	{0x00, 0x08, 0x36, 0x41, 0x00}, // '{' 0x7B
	{0x00, 0x00, 0x7F, 0x00, 0x00}, // '|' 0x7C
	{0x00, 0x41, 0x36, 0x08, 0x00}, // '}' 0x7D
	{0x10, 0x08, 0x08, 0x10, 0x08}, // '~' 0x7E
}
