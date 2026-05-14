// Package sh1106 — i2c_linux.go
// Provides a ready-to-use I2C Conn backed by Go's standard "golang.org/x/periph"
// library OR a raw /dev/i2c-N file descriptor.
//
// For TinyGo (embedded), implement the Conn interface directly using
// machine.I2C.Tx() — see the example in example_tinygo.go.

package  sh1106

import (
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

// ---------------------------------------------------------------------------
// Raw Linux /dev/i2c-N connector (no extra libraries)
// ---------------------------------------------------------------------------

const (
	i2cSlave = 0x0703 // ioctl request: set slave address
)

// LinuxI2CConn communicates with the SH1106 over a raw Linux I2C device file.
// This has zero external dependencies beyond "golang.org/x/sys/unix".
type LinuxI2CConn struct {
	fd   int
	addr uint8
}

// NewLinuxI2CConn opens /dev/i2c-<bus> and targets the given 7-bit address.
// Typical address for SH1106: 0x3C (SA0 low) or 0x3D (SA0 high).
//
//	conn, err := sh1106.NewLinuxI2CConn(1, 0x3C)  // /dev/i2c-1
func NewLinuxI2CConn(bus int, addr uint8) (*LinuxI2CConn, error) {
	path := fmt.Sprintf("/dev/i2c-%d", bus)
	fd, err := unix.Open(path, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("sh1106: open %s: %w", path, err)
	}
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL,
		uintptr(fd), i2cSlave, uintptr(addr)); errno != 0 {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("sh1106: ioctl I2C_SLAVE: %w", errno)
	}
	return &LinuxI2CConn{fd: fd, addr: addr}, nil
}

// Close releases the file descriptor.
func (c *LinuxI2CConn) Close() error {
	return unix.Close(c.fd)
}

// WriteCmd implements Conn.
func (c *LinuxI2CConn) WriteCmd(cmds ...byte) error {
	buf := make([]byte, len(cmds)+1)
	buf[0] = i2cCmdByte
	copy(buf[1:], cmds)
	return c.write(buf)
}

// WriteData implements Conn.
func (c *LinuxI2CConn) WriteData(data []byte) error {
	buf := make([]byte, len(data)+1)
	buf[0] = i2cDataByte
	copy(buf[1:], data)
	return c.write(buf)
}

func (c *LinuxI2CConn) write(buf []byte) error {
	n, err := unix.Write(c.fd, buf)
	if err != nil {
		return fmt.Errorf("sh1106: i2c write: %w", err)
	}
	if n != len(buf) {
		return fmt.Errorf("sh1106: i2c short write: %d/%d", n, len(buf))
	}
	return nil
}

// Ensure LinuxI2CConn satisfies Conn at compile time.
var _ Conn = (*LinuxI2CConn)(nil)

// ---------------------------------------------------------------------------
// Dummy for unsafe import suppression
// ---------------------------------------------------------------------------

var _ = unsafe.Sizeof(0) // keep import happy when not building on Linux
