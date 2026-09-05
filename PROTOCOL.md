# FocusCube protocol

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
