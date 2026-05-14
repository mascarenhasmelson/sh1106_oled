// cmd/demo/main.go — SH1106 driver demo
//
// Build for Raspberry Pi (Linux/arm64):
//
//	GOARCH=arm64 GOOS=linux go build -o sh1106-demo ./cmd/demo
//
// Run on the board:
//
//	./sh1106-demo
package main

import (
	"fmt"
	"log"
	"time"
	"sh1106/sh1106" // replace with your module path
)

func main() {
	// --- 1. Open I2C bus 1, address 0x3C ---
	conn, err := sh1106.NewLinuxI2CConn(1, 0x3C)
	if err != nil {
		log.Fatalf("open i2c: %v", err)
	}
	defer conn.Close()

	// --- 2. Create device and initialise ---
	d := sh1106.New(conn, sh1106.Width128, sh1106.Height64)
	if err := d.Init(); err != nil {
		log.Fatalf("init: %v", err)
	}

	// --- 3. Draw a border ---
	d.Clear()
	d.DrawRect(0, 0, 128, 64)

	// --- 4. Draw some text ---
	d.DrawString(4, 4, "SH1106 Driver")
	d.DrawString(4, 16, "Go lang rocks!")
	d.DrawString(4, 28, fmt.Sprintf("cols=128 rows=64"))

	// --- 5. Flush to display ---
	if err := d.Display(); err != nil {
		log.Fatalf("display: %v", err)
	}

	time.Sleep(2 * time.Second)

	// --- 6. Scroll a message across the screen ---
	msg := "Melson Mascarenhas"
	for i := 0; i < len(msg)*6; i++ {
		d.Clear()
		d.DrawString(128-i, 28, msg)
		if err := d.Display(); err != nil {
			log.Fatalf("display: %v", err)
		}
		time.Sleep(30 * time.Millisecond)
	}

	// --- 7. Invert, pause, revert ---
	_ = d.InvertDisplay(true)
	time.Sleep(1 * time.Second)
	_ = d.InvertDisplay(false)

	// --- 8. Done ---
	d.Clear()
	d.DrawStringWrapped(0, 0, "Done! Goodbye.", 10)
	_ = d.Display()
}
