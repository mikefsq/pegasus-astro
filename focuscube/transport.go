// Package focuscube is a pure-Go driver for Pegasus Astro focusers
// (FocusCube / FocusCube2 / SMFC / DMFC / ScopsOAG) over their FTDI USB-serial
// link. Unlike the ZWO/Astroasis accessories (USB-HID), these are an FTDI VCP
// bridge presented as an OS serial port; the protocol is plain ASCII commands
// (each terminated with "\n"; replies are "\n"-terminated).
//
// The command set is the Pegasus DMFC / FocusCube serial protocol (published by
// Pegasus as the DMFC Serial Command Table). The transport opens the FTDI virtual
// COM port at 19200 8N1 and does line I/O via go.bug.st/serial — no vendor library, no
// raw-USB FTDI reimplementation. It uses only the library's pure-Go paths (port I/O
// everywhere; the USB-VID enumerator off macOS, device-name matching on macOS), so
// it builds for any target with CGO_ENABLED=0.
package focuscube

// FTDI vendor ID (the FocusCube bridge) and the FocusCube serial line speed.
const (
	VID  uint16 = 0x0403
	Baud        = 19200
)

// Transport is a byte-level serial channel to the FTDI VCP port (satisfied by a
// go.bug.st/serial port); the device logic frames ASCII commands over it. Read
// should block up to a short timeout and return 0 bytes (not an error) when
// nothing is available, so command() can poll to a deadline.
type Transport interface {
	Write(p []byte) (int, error)
	Read(p []byte) (int, error)
	Close() error
}

// DeviceInfo identifies an opened serial port plus the USB-descriptor properties the
// enumerator reports for it before the port is opened. Serial is the FTDI bridge's USB
// iSerialNumber — a stable per-unit identity that disambiguates several FTDI devices
// sharing VID 0x0403 and survives replug / port renumbering.
type DeviceInfo struct {
	Port    string // e.g. /dev/cu.usbserial-XXXX, /dev/ttyUSB0, COM3
	Serial  string // USB iSerialNumber (from the enumerator); "" if unavailable
	Product string // USB iProduct string (from the enumerator); "" if unavailable
}
