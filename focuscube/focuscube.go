package focuscube

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// replyTimeout bounds how long command() waits for a "\n"-terminated reply. A var
// so tests can shrink it.
var replyTimeout = 2 * time.Second

// FocusCube is an opened Pegasus focuser.
type FocusCube struct {
	t    Transport
	info DeviceInfo

	mu     sync.Mutex
	dcMode bool // DC-motor focuser: moves use "G:" instead of "M:" (host-side flag)
}

// New wraps an already-open Transport. Most callers use OpenFirst / OpenPort; New
// is for a custom Transport (alternate backend, or a fake for testing).
func New(t Transport, info DeviceInfo) *FocusCube { return &FocusCube{t: t, info: info} }

// OpenFirst finds and opens the first attached Pegasus (FTDI) focuser.
func OpenFirst() (*FocusCube, error) {
	t, info, err := openFirst()
	if err != nil {
		return nil, err
	}
	return New(t, info), nil
}

// OpenPort opens the focuser on a specific serial port (from Enumerate).
func OpenPort(port string) (*FocusCube, error) {
	t, info, err := openPort(port)
	if err != nil {
		return nil, err
	}
	return New(t, info), nil
}

// OpenBySerial opens the FocusCube whose USB serial number matches serial
// (case-insensitive). The serial comes from the USB descriptor via the enumerator —
// read before opening — so it disambiguates several FTDI devices sharing VID 0x0403
// and binds the same physical unit across replug / port renumbering.
func OpenBySerial(serial string) (*FocusCube, error) {
	t, info, err := openBySerial(serial)
	if err != nil {
		return nil, err
	}
	return New(t, info), nil
}

func (f *FocusCube) Info() DeviceInfo { return f.info }
func (f *FocusCube) Close() error     { return f.t.Close() }

// SetDCMode selects DC-motor move semantics (host-side; affects subsequent MoveTo).
// FocusCube/stepper focusers use "M:"; DC-motor focusers use "G:".
func (f *FocusCube) SetDCMode(on bool) { f.mu.Lock(); f.dcMode = on; f.mu.Unlock() }

// command sends cmd+"\n" and returns the trimmed "\n"-terminated reply. Caller
// holds mu.
func (f *FocusCube) command(cmd string) (string, error) {
	if _, err := f.t.Write([]byte(cmd + "\n")); err != nil {
		return "", fmt.Errorf("focuscube: write %q: %w", cmd, err)
	}
	deadline := time.Now().Add(replyTimeout)
	var buf []byte
	tmp := make([]byte, 64)
	for time.Now().Before(deadline) {
		n, err := f.t.Read(tmp)
		if err != nil {
			return "", fmt.Errorf("focuscube: read reply to %q: %w", cmd, err)
		}
		if n == 0 {
			time.Sleep(5 * time.Millisecond)
			continue
		}
		buf = append(buf, tmp[:n]...)
		if i := bytes.IndexByte(buf, '\n'); i >= 0 {
			return strings.TrimRight(string(buf[:i]), "\r\n"), nil
		}
	}
	return "", fmt.Errorf("focuscube: timeout waiting for reply to %q", cmd)
}

// Command focuscube reads device status and provides diagnostic controls.
func (f *FocusCube) Command(cmd string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.command(cmd)
}

// Handshake sends "#" (identify) and returns the device id string (e.g. "OK_FC").
// A reply containing "OK" means connected.
func (f *FocusCube) Handshake() (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.command("#")
}

// Connected reports whether the device answers the "#" handshake with an OK id.
func (f *FocusCube) Connected() bool {
	id, err := f.Handshake()
	return err == nil && strings.Contains(id, "OK")
}

// Position returns the current position (steps). "P" -> integer string.
func (f *FocusCube) Position() (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, err := f.command("P")
	if err != nil {
		return 0, err
	}
	return parseInt(s)
}

// MoveTo commands an absolute move to position (steps). Uses "M:" (stepper) or
// "G:" (DC mode). Returns once acknowledged; poll IsMoving/Position for completion.
func (f *FocusCube) MoveTo(position int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cmd := "M:"
	if f.dcMode {
		cmd = "G:"
	}
	_, err := f.command(cmd + strconv.Itoa(position))
	return err
}

// Halt stops any in-progress motion ("H").
func (f *FocusCube) Halt() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, err := f.command("H")
	return err
}

// IsMoving reports motion state ("I" -> 0/1).
func (f *FocusCube) IsMoving() (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, err := f.command("I")
	if err != nil {
		return false, err
	}
	n, err := parseInt(s)
	return n != 0, err
}

// Temperature returns the probe temperature in °C ("T" -> double).
func (f *FocusCube) Temperature() (float64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, err := f.command("T")
	if err != nil {
		return 0, err
	}
	return strconv.ParseFloat(strings.TrimSpace(s), 64)
}

// SyncPosition sets the reported position to n without moving ("W:n").
func (f *FocusCube) SyncPosition(n int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, err := f.command("W:" + strconv.Itoa(n))
	return err
}

// SetReverse sets direction reversal ("N:1"/"N:0").
func (f *FocusCube) SetReverse(on bool) error { return f.flag("N", on) }

// SetLED enables/disables the LED ("L:1" on / "L:2" off).
func (f *FocusCube) SetLED(on bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cmd := "L:2"
	if on {
		cmd = "L:1"
	}
	_, err := f.command(cmd)
	return err
}

// SetKnobDisabled disables/enables the manual knob ("E:1" disable / "E:0" enable).
func (f *FocusCube) SetKnobDisabled(disabled bool) error { return f.flag("E", disabled) }

// SetBacklash sets backlash-compensation steps (0 disables) ("C:n").
func (f *FocusCube) SetBacklash(steps int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, err := f.command("C:" + strconv.Itoa(steps))
	return err
}

// flag sends a "<cmd>:1"/"<cmd>:0" boolean command.
func (f *FocusCube) flag(cmd string, on bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	v := "0"
	if on {
		v = "1"
	}
	_, err := f.command(cmd + ":" + v)
	return err
}

// parseInt parses an integer reply. The "P"/"I" replies are a bare integer, so we
// parse directly (no prefix guessing).
func parseInt(s string) (int, error) {
	return strconv.Atoi(strings.TrimSpace(s))
}
