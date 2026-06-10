# pegasus-astro

Pure-Go driver for Pegasus Astro focusers — **FocusCube / FocusCube 2 / SMFC / DMFC /
ScopsOAG** — package `focuscube/`.

These controllers are an **FTDI USB-serial** bridge (VID `0x0403`) exposed by the OS as
a virtual COM port. The protocol is plain ASCII — each command terminated with `\n`,
replies `\n`-terminated — the Pegasus DMFC / FocusCube serial command set (published by
Pegasus as the *DMFC Serial Command Table*). The driver opens the port at **19200 8N1**
and does line I/O via [`go.bug.st/serial`](https://pkg.go.dev/go.bug.st/serial): no
vendor library, no raw-USB FTDI reimplementation. It uses only the library's pure-Go
paths, so it **builds for any target with `CGO_ENABLED=0`**.

> **Note:** the newer **FocusCube 3** uses a different, `F`-prefixed command set
> (`FM:`, `FH`, `FT`, identity `FC3…`) and is **not** targeted by this driver.

## Protocol

Each command is sent as ASCII terminated by `\n`; the reply is a single
`\n`-terminated line.

| Command | Meaning | Reply |
|---|---|---|
| `#` | identify / handshake | id string (e.g. `OK_FC`) |
| `P` | get position | integer |
| `M:<n>` | absolute move (stepper) | echo |
| `G:<n>` | absolute move (DC motor) | echo |
| `W:<n>` | sync position (no move) | echo |
| `H` | halt | echo |
| `I` | is-moving | `0` / `1` |
| `T` | temperature (°C) | float (`-127` = no probe) |
| `N:1` / `N:0` | reverse on / off | echo |
| `E:1` / `E:0` | knob disable / enable | echo |
| `C:<n>` | backlash steps (0 = off) | echo |
| `L:1` / `L:2` | LED on / off | echo |

`MaxStep` / `StepSize` are host-side settings, not device-queried — the controller
enforces no mechanical travel limit. Stepper focusers move with `M:`; DC-motor focusers
use `G:` (select via `SetDCMode`).

## Usage

```go
d, err := focuscube.OpenFirst() // or focuscube.OpenPort("/dev/ttyUSB0")
if err != nil {
	log.Fatal(err)
}
defer d.Close()

if !d.Connected() { // "#" handshake replies OK_…
	log.Fatal("focuser not responding")
}
pos, _ := d.Position()
_ = d.MoveTo(12000) // stepper; d.SetDCMode(true) switches to "G:"
temp, _ := d.Temperature()
```

## fcprobe

`cmd/fcprobe` is a CLI to inspect and exercise a focuser:

```sh
fcprobe -list                 # candidate serial ports
fcprobe                       # read-only: id / position / moving / temperature
fcprobe -moveto 12000         # absolute move, watch settle
fcprobe -in 200 / -out 200    # relative move by N steps
fcprobe -stoptest 40000       # move far, then halt mid-flight
fcprobe -halt                 # stop motion
fcprobe -sync 0               # set reported position without moving (W:)
fcprobe -reverse on|off       # direction reversal (N:)
fcprobe -led on|off           # status LED (L:)
fcprobe -knob enable|disable  # manual knob / encoder (E:)
fcprobe -backlash 50          # backlash steps, 0 = off (C:)
fcprobe -dc                   # DC-motor move semantics (G:)
fcprobe -raw "P"              # send a raw command, show the reply
fcprobe -selftest             # non-destructive motion test (returns to start)
fcprobe -watch                # poll position + moving
```

## Layout

```
focuscube/
  focuscube.go        device logic + ASCII command framing (pure Go, all platforms)
  transport.go        Transport seam (Write/Read/Close) + DeviceInfo + VID/Baud
  serial.go           go.bug.st/serial port open (pure Go, all OSes)
  enum_other.go       !darwin — FTDI (VID 0x0403) discovery via the pure-Go enumerator
  enum_darwin.go      darwin  — FTDI VCP discovery by device name (cgo-free)
  focuscube_test.go   protocol tests over a fake Transport
cmd/fcprobe/          inspect / drive a focuser
```

## Portability

The sole dependency is `go.bug.st/serial`, used only on its pure-Go paths, so the driver
compiles for any target with `CGO_ENABLED=0` (linux/darwin/windows × amd64/arm64/arm).
macOS device discovery is name-based to avoid the enumerator's lone cgo (IOKit) path.

## Build & test

```sh
go test ./focuscube/
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o fcprobe ./cmd/fcprobe   # e.g. Raspberry Pi
```

## License

[MIT](LICENSE) © 2026 mikefsq
