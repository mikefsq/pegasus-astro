package focuscube

import (
	"strings"
	"sync"
	"testing"
	"time"
)

// fake is an in-memory serial Transport: it records each command line and, on
// write, queues the canned reply for that command so Read returns it. Commands
// match by full text ("P") or by "prefix:" ("W:") for parameterized ones.
type fake struct {
	mu      sync.Mutex
	replies map[string]string
	written []string
	out     []byte
	failW   bool
}

func newFake(r map[string]string) *fake { return &fake{replies: r} }

func (f *fake) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failW {
		return 0, errGone
	}
	cmd := strings.TrimRight(string(p), "\n")
	f.written = append(f.written, cmd)
	reply, ok := f.replies[cmd]
	if !ok {
		if i := strings.IndexByte(cmd, ':'); i >= 0 {
			reply, ok = f.replies[cmd[:i+1]] // "W:" etc.
		}
	}
	if ok {
		f.out = append(f.out, []byte(reply+"\n")...)
	}
	return len(p), nil
}

func (f *fake) Read(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.out) == 0 {
		return 0, nil
	}
	n := copy(p, f.out)
	f.out = f.out[n:]
	return n, nil
}

func (f *fake) Close() error { return nil }

func (f *fake) last() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.written) == 0 {
		return ""
	}
	return f.written[len(f.written)-1]
}

type goneErr struct{}

func (goneErr) Error() string { return "device removed" }

var errGone = goneErr{}

func dev(r map[string]string) *FocusCube { return New(newFake(r), DeviceInfo{Port: "fake"}) }

func TestEncodeCommands(t *testing.T) {
	replies := map[string]string{
		"#": "OK_FC", "P": "1234", "I": "0", "T": "20.5", "H": "H:1",
		"M:": "M:1", "G:": "G:1", "W:": "W:1", "N:": "N:1", "L:1": "L:1",
		"L:2": "L:2", "E:": "E:1", "C:": "C:1",
	}
	cases := []struct {
		name string
		do   func(*FocusCube)
		want string
	}{
		{"handshake", func(d *FocusCube) { d.Handshake() }, "#"},
		{"position", func(d *FocusCube) { d.Position() }, "P"},
		{"ismoving", func(d *FocusCube) { d.IsMoving() }, "I"},
		{"temperature", func(d *FocusCube) { d.Temperature() }, "T"},
		{"moveto stepper", func(d *FocusCube) { d.MoveTo(5000) }, "M:5000"},
		{"moveto dc", func(d *FocusCube) { d.SetDCMode(true); d.MoveTo(5000) }, "G:5000"},
		{"halt", func(d *FocusCube) { d.Halt() }, "H"},
		{"sync", func(d *FocusCube) { d.SyncPosition(0) }, "W:0"},
		{"reverse on", func(d *FocusCube) { d.SetReverse(true) }, "N:1"},
		{"led off", func(d *FocusCube) { d.SetLED(false) }, "L:2"},
		{"knob disable", func(d *FocusCube) { d.SetKnobDisabled(true) }, "E:1"},
		{"backlash", func(d *FocusCube) { d.SetBacklash(40) }, "C:40"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFake(replies)
			c.do(New(f, DeviceInfo{}))
			if got := f.last(); got != c.want {
				t.Errorf("sent %q, want %q", got, c.want)
			}
		})
	}
}

func TestDecode(t *testing.T) {
	d := dev(map[string]string{"#": "OK_FC2", "P": "12345", "I": "1", "T": "19.75"})
	if id, err := d.Handshake(); err != nil || id != "OK_FC2" {
		t.Fatalf("Handshake()=%q,%v", id, err)
	}
	if !d.Connected() {
		t.Error("Connected()=false want true")
	}
	if p, err := d.Position(); err != nil || p != 12345 {
		t.Fatalf("Position()=%d,%v want 12345", p, err)
	}
	if m, err := d.IsMoving(); err != nil || !m {
		t.Fatalf("IsMoving()=%v,%v want true", m, err)
	}
	if tmp, err := d.Temperature(); err != nil || tmp != 19.75 {
		t.Fatalf("Temperature()=%v,%v want 19.75", tmp, err)
	}
}

func TestTimeout(t *testing.T) {
	old := replyTimeout
	replyTimeout = 50 * time.Millisecond
	defer func() { replyTimeout = old }()
	d := dev(map[string]string{}) // no reply staged
	if _, err := d.Position(); err == nil {
		t.Error("want timeout error when device gives no reply")
	}
}

func TestDeviceRemoved(t *testing.T) {
	f := newFake(map[string]string{})
	f.failW = true
	if _, err := New(f, DeviceInfo{}).Position(); err == nil {
		t.Error("want error on removed device")
	}
}

// TestOpenBySerialEmpty: an empty serial is rejected immediately, before any port
// enumeration or I/O.
func TestOpenBySerialEmpty(t *testing.T) {
	if _, err := OpenBySerial("   "); err == nil {
		t.Error("OpenBySerial(blank) = nil err, want an empty-serial error")
	}
}

// TestDeviceInfoCarriesSerial: New preserves the USB serial/product captured at
// enumeration on the opened handle.
func TestDeviceInfoCarriesSerial(t *testing.T) {
	d := New(newFake(map[string]string{}), DeviceInfo{Port: "/dev/x", Serial: "FT1ABCDE", Product: "FocusCube"})
	if got := d.Info(); got.Serial != "FT1ABCDE" || got.Product != "FocusCube" {
		t.Errorf("Info() = %+v, want Serial=FT1ABCDE Product=FocusCube", got)
	}
}

// The identification test openFirst relies on: a FocusCube answers "#" with an OK id, anything
// else on the bus does not.
//
// VID 0403 is a generic FTDI bridge shared by mounts, meters and focusers, so binding on the VID
// alone takes whichever device enumerated first — holding its port against the driver that owns
// it and speaking a protocol it does not understand. This is the check that prevents that; if it
// ever passed for a silent device, openFirst would go back to grabbing neighbours.
func TestConnectedIdentifiesTheDevice(t *testing.T) {
	cases := []struct {
		name    string
		replies map[string]string
		want    bool
	}{
		{"a FocusCube answers with an OK id", map[string]string{"#": "OK_FC"}, true},
		{"a different Pegasus unit still identifies", map[string]string{"#": "OK_FC2"}, true},
		{"a silent device is not one", map[string]string{}, false},
		{"a device that replies without OK is not one", map[string]string{"#": "GARBAGE"}, false},
		{"another instrument's chatter is not one", map[string]string{"#": "19.85,1013.2"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := New(newFake(c.replies), DeviceInfo{Port: "/dev/ttyUSB0"})
			if got := f.Connected(); got != c.want {
				t.Errorf("Connected() = %v, want %v", got, c.want)
			}
		})
	}
}
