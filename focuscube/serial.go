package focuscube

import (
	"errors"
	"fmt"
	"strings"
	"time"

	bugst "go.bug.st/serial"
)

// readTimeout is the per-read timeout set on the port. With it, Read returns
// promptly when bytes arrive and returns (0, nil) once idle past the timeout,
// which command()'s deadline loop handles.
const readTimeout = 100 * time.Millisecond

// openPort opens dev at the FocusCube line speed (8N1) as a Transport. The
// go.bug.st/serial port already satisfies Transport (Read/Write/Close) and its
// port I/O is pure Go on every OS, so the driver cross-compiles to any target.
func openPort(dev string) (Transport, DeviceInfo, error) {
	port, err := bugst.Open(dev, &bugst.Mode{
		BaudRate: Baud,
		DataBits: 8,
		Parity:   bugst.NoParity,
		StopBits: bugst.OneStopBit,
	})
	if err != nil {
		return nil, DeviceInfo{}, fmt.Errorf("focuscube: open %s: %w", dev, err)
	}
	if err := port.SetReadTimeout(readTimeout); err != nil {
		port.Close()
		return nil, DeviceInfo{}, fmt.Errorf("focuscube: set read timeout on %s: %w", dev, err)
	}
	return port, DeviceInfo{Port: dev}, nil
}

// Enumerate lists attached FocusCube FTDI serial ports. Matching is per-OS:
// enum_other.go uses the USB VID via the pure-Go enumerator; enum_darwin.go matches
// the FTDI device-name convention, deliberately avoiding the enumerator's macOS cgo
// (IOKit) path so the driver builds for any target with CGO_ENABLED=0.
func Enumerate() ([]DeviceInfo, error) { return enumeratePorts() }

// openFirst opens the first attached port that IDENTIFIES as a FocusCube, not merely the first
// port with FTDI's vendor id.
//
// VID 0403 is a generic USB-serial bridge shared by mounts, focusers, sky-quality meters and
// anything else that needed a UART, so a rig commonly carries several. Taking ports[0] on the
// strength of the VID alone binds whichever the OS enumerated first, holds its port against the
// driver that actually owns it, and talks a protocol it does not speak. OpenBySerial exists for
// the same reason and says so; this path simply did not honour it.
func openFirst() (Transport, DeviceInfo, error) {
	ports, err := enumeratePorts()
	if err != nil {
		return nil, DeviceInfo{}, err
	}
	if len(ports) == 0 {
		return nil, DeviceInfo{}, errors.New("focuscube: no FTDI serial port found")
	}
	for _, d := range ports {
		t, info, err := openInfo(d)
		if err != nil {
			continue // busy (its real driver holds it) or not openable: no evidence about what it is
		}
		// The "#" handshake must answer with an OK id — a device that is not a FocusCube stays
		// silent, so a parsed reply is identification rather than "something was listening".
		if New(t, info).Connected() {
			return t, info, nil
		}
		t.Close()
	}
	return nil, DeviceInfo{}, fmt.Errorf("focuscube: none of %d candidate port(s) answered the handshake", len(ports))
}

// openInfo opens the port named by an enumerated DeviceInfo and returns that same info
// (so the USB serial/product captured during enumeration is preserved on the handle).
func openInfo(d DeviceInfo) (Transport, DeviceInfo, error) {
	t, _, err := openPort(d.Port)
	if err != nil {
		return nil, DeviceInfo{}, err
	}
	return t, d, nil
}

// openBySerial opens the FocusCube whose USB serial number matches serial
// (case-insensitive, trimmed), as reported by the enumerator before the port is opened.
// This is the stable per-unit identity that disambiguates several FTDI devices sharing
// VID 0x0403 and survives replug / port renumbering.
func openBySerial(serial string) (Transport, DeviceInfo, error) {
	want := strings.TrimSpace(strings.ToLower(serial))
	if want == "" {
		return nil, DeviceInfo{}, errors.New("focuscube: empty serial")
	}
	ports, err := enumeratePorts()
	if err != nil {
		return nil, DeviceInfo{}, err
	}
	for _, d := range ports {
		if strings.ToLower(strings.TrimSpace(d.Serial)) == want {
			return openInfo(d)
		}
	}
	return nil, DeviceInfo{}, fmt.Errorf("focuscube: no FocusCube with serial %q found", serial)
}
