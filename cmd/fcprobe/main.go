// Command fcprobe reads device status and provides diagnostic controls.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mikefsq/pegasus-astro/focuscube"
)

func main() {
	list := flag.Bool("list", false, "list candidate serial ports (with USB serial) and exit")
	port := flag.String("port", "", "serial port to open (default: first found)")
	serial := flag.String("serial", "", "open the FocusCube with this USB serial number (from -list)")
	dc := flag.Bool("dc", false, "use DC-motor move semantics (G:)")

	moveTo := flag.Int("moveto", -1, "absolute move to position, then watch; -1 = skip")
	in := flag.Int("in", -1, "relative move IN by N steps (decreasing), then watch; -1 = skip")
	out := flag.Int("out", -1, "relative move OUT by N steps (increasing), then watch; -1 = skip")
	stopTest := flag.Int("stoptest", -1, "move to this far target, run ~1s, then halt; report halted position")
	halt := flag.Bool("halt", false, "stop motion")
	sync := flag.Int("sync", -1, "set reported position to N without moving (W:); -1 = skip")

	reverse := flag.String("reverse", "", "direction reversal on|off (N:)")
	led := flag.String("led", "", "status LED on|off (L:)")
	knob := flag.String("knob", "", "manual knob/encoder enable|disable (E:)")
	backlash := flag.Int("backlash", -1, "backlash steps, 0 = off (C:); -1 = skip")

	raw := flag.String("raw", "", "send a raw command (e.g. P) and print the reply")
	selfTest := flag.Bool("selftest", false, "non-destructive motion test: small move + halt-mid-flight, returned to start")
	watch := flag.Bool("watch", false, "poll position+moving repeatedly")
	flag.Parse()

	if *list {
		ports, err := focuscube.Enumerate()
		if err != nil {
			fmt.Fprintln(os.Stderr, "enumerate:", err)
			os.Exit(1)
		}
		for _, p := range ports {
			fmt.Printf("%s\tserial=%q product=%q\n", p.Port, p.Serial, p.Product)
		}
		return
	}

	var (
		d   *focuscube.FocusCube
		err error
	)
	switch {
	case *serial != "":
		d, err = focuscube.OpenBySerial(*serial)
	case *port != "":
		d, err = focuscube.OpenPort(*port)
	default:
		d, err = focuscube.OpenFirst()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "open:", err)
		os.Exit(1)
	}
	defer d.Close()
	if *dc {
		d.SetDCMode(true)
	}
	fmt.Printf("opened %s\n", d.Info().Port)

	if *raw != "" {
		reply, err := d.Command(*raw)
		if err != nil {
			fmt.Fprintf(os.Stderr, "raw %q: %v\n", *raw, err)
			os.Exit(1)
		}
		fmt.Printf("raw %q -> %q\n", *raw, reply)
		return
	}

	dumpAll(d)

	switch {
	case *selfTest:
		runSelfTest(d)
	case *stopTest >= 0:
		p0, _ := d.Position()
		fmt.Printf("\nstoptest: at %d, MoveTo %d then halt mid-flight...\n", p0, *stopTest)
		report("MoveTo", d.MoveTo(*stopTest))
		for i := 0; i < 40; i++ { // wait until it's actually moving
			if m, _ := d.IsMoving(); m {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		time.Sleep(1 * time.Second)
		report("Halt", d.Halt())
		time.Sleep(300 * time.Millisecond)
		p, _ := d.Position()
		m, _ := d.IsMoving()
		fmt.Printf("after halt: position=%d moving=%v (target was %d; halted short = Halt works)\n", p, m, *stopTest)
	case *out >= 0:
		p0, _ := d.Position()
		tgt := p0 + *out
		fmt.Printf("\nat %d, out %d -> MoveTo %d...\n", p0, *out, tgt)
		report("MoveTo", d.MoveTo(tgt))
		watchSettle(d)
	case *in >= 0:
		p0, _ := d.Position()
		tgt := p0 - *in
		if tgt < 0 {
			tgt = 0
		}
		fmt.Printf("\nat %d, in %d -> MoveTo %d...\n", p0, *in, tgt)
		report("MoveTo", d.MoveTo(tgt))
		watchSettle(d)
	case *moveTo >= 0:
		fmt.Printf("\nmoving to %d...\n", *moveTo)
		report("MoveTo", d.MoveTo(*moveTo))
		watchSettle(d)
	case *halt:
		report("Halt", d.Halt())
	case *sync >= 0:
		report("SyncPosition", d.SyncPosition(*sync))
		p, _ := d.Position()
		fmt.Printf("position now %d\n", p)
	case *reverse != "":
		report("SetReverse", d.SetReverse(onoff(*reverse)))
	case *led != "":
		report("SetLED", d.SetLED(onoff(*led)))
	case *knob != "":
		report("SetKnobDisabled", d.SetKnobDisabled(strings.EqualFold(*knob, "disable")))
	case *backlash >= 0:
		report("SetBacklash", d.SetBacklash(*backlash))
	case *watch:
		fmt.Println("\nwatching (Ctrl-C to stop)...")
		for {
			p, _ := d.Position()
			m, _ := d.IsMoving()
			fmt.Printf("position=%d moving=%v\n", p, m)
			time.Sleep(500 * time.Millisecond)
		}
	}
}

func dumpAll(d *focuscube.FocusCube) {
	fmt.Println("\n-- identity --")
	if id, err := d.Handshake(); err == nil {
		fmt.Printf("id          : %s\n", id)
	} else {
		fmt.Fprintln(os.Stderr, "handshake:", err)
	}

	fmt.Println("\n-- status --")
	if p, err := d.Position(); err == nil {
		fmt.Printf("position    : %d\n", p)
	}
	if m, err := d.IsMoving(); err == nil {
		fmt.Printf("moving      : %v\n", m)
	}
	if t, err := d.Temperature(); err == nil {
		note := ""
		if t <= -100 {
			note = "  (no probe attached)"
		}
		fmt.Printf("temperature : %.2f °C%s\n", t, note)
	}
}

// runSelfTest exercises MoveTo and Halt non-destructively: it records the start
// position, makes a small move and returns, then halts a longer move mid-flight and
// returns. Config setters (reverse/LED/knob/backlash) are not touched — the protocol
// has no way to read them back, so they can't be safely round-tripped.
func runSelfTest(d *focuscube.FocusCube) {
	p0, err := d.Position()
	if err != nil {
		fmt.Fprintln(os.Stderr, "selftest: position:", err)
		return
	}
	fmt.Printf("\n-- self-test (start position %d) --\n", p0)

	// 1) small absolute move out and back.
	tgt := p0 + 200
	if err := d.MoveTo(tgt); err != nil {
		fmt.Fprintln(os.Stderr, "selftest MoveTo:", err)
		return
	}
	waitStill(d)
	p, _ := d.Position()
	fmt.Printf("MoveTo +200          %s (at %d, want ~%d)\n", passFail(abs(p-tgt) <= 5), p, tgt)
	d.MoveTo(p0)
	waitStill(d)

	// 2) halt a longer move mid-flight.
	far := p0 + 8000
	if err := d.MoveTo(far); err != nil {
		fmt.Fprintln(os.Stderr, "selftest MoveTo(far):", err)
		return
	}
	for i := 0; i < 40; i++ {
		if m, _ := d.IsMoving(); m {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	time.Sleep(1 * time.Second)
	d.Halt()
	waitStill(d) // let the motor finish its deceleration ramp before judging
	p, _ = d.Position()
	m, _ := d.IsMoving()
	fmt.Printf("Halt mid-flight      %s (stopped at %d short of %d, now moving=%v)\n", passFail(!m && p < far), p, far, m)

	// 3) return to start.
	d.MoveTo(p0)
	waitStill(d)
	p, _ = d.Position()
	fmt.Printf("returned to start    %s (at %d, want %d)\n", passFail(abs(p-p0) <= 5), p, p0)
}

func report(label string, err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", label, err)
		return
	}
	fmt.Printf("%s: ok\n", label)
}

// watchSettle waits for an in-progress move to finish, then prints the settled
// position.
func watchSettle(d *focuscube.FocusCube) {
	waitStill(d)
	p, _ := d.Position()
	fmt.Printf("settled at %d\n", p)
}

// waitStill first waits briefly for motion to actually begin (the controller can
// report not-moving for a moment right after accepting a move) so it can't report
// "settled" before the focuser has moved, then waits for it to stop.
func waitStill(d *focuscube.FocusCube) {
	for i := 0; i < 20; i++ {
		if m, _ := d.IsMoving(); m {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	for i := 0; i < 600; i++ {
		m, err := d.IsMoving()
		if err != nil {
			fmt.Fprintln(os.Stderr, "ismoving:", err)
			return
		}
		if !m {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	fmt.Println("gave up waiting for the focuser to settle")
}

func onoff(s string) bool {
	switch strings.ToLower(s) {
	case "on", "1", "true", "yes", "enable":
		return true
	}
	return false
}

func passFail(ok bool) string {
	if ok {
		return "PASS"
	}
	return "FAIL"
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
