# pegasus-astro

Go driver for Pegasus Astro FocusCube, FocusCube 2, SMFC, DMFC, and ScopsOAG
focusers over USB-serial. No vendor SDK or cgo is required.
FocusCube 3 uses a different command set and is unsupported.

## Build and run

Requires Go 1.25 or later.

```sh
go build -o fcprobe ./cmd/fcprobe
./fcprobe -list
./fcprobe -serial YOUR_USB_SERIAL
```

The default run reads identity, position, motion state, and temperature.
Use `-port` for an explicit serial port or `-serial` for a stable USB identifier.

```sh
./fcprobe -serial YOUR_USB_SERIAL -moveto 12000
./fcprobe -serial YOUR_USB_SERIAL -halt
```

The driver does not enforce a mechanical travel limit. Choose positions
appropriate for the focuser. `-dc` selects DC-motor commands; the default is
stepper mode. `-selftest` moves the focuser and attempts to return it to the
starting position. Use `-help` for settings and diagnostic options.

## Use the library

```go
package main

import (
    "fmt"
    "log"

    "github.com/mikefsq/pegasus-astro/focuscube"
)

func run() error {
    device, err := focuscube.OpenFirst()
    if err != nil {
        return err
    }
    defer device.Close()

    value, err := device.Position()
    if err != nil {
        return err
    }
    fmt.Println(value)
    return nil
}

func main() {
    if err := run(); err != nil {
        log.Fatal(err)
    }
}
```

`MoveTo` starts an absolute move. `SetDCMode(true)` selects DC-motor motion.
`Temperature` reads the optional probe; the controller reports -127°C when
no probe is attached.

## Platforms and development

Serial I/O supports Linux, macOS, and Windows without cgo. On macOS,
discovery uses device names; elsewhere it uses USB IDs. The service user must
have permission to open the serial port.

```sh
go test -race ./...
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build ./...
```

See [PROTOCOL.md](PROTOCOL.md) for wire commands. Protocol tests use a fake
`Transport` and do not require hardware.

## License

[MIT](LICENSE).
