# The Lenovo SmartClock 2

LANovo on the **Lenovo SmartClock 2** (board `helios`). The clock must be
rooted with Magisk: `lanovod` runs resident from `/data/adb/lanovod`, started at
boot by a Magisk `service.d` script, and takes the screen, audio, buttons and
Wi-Fi from Android the same way it does on the Smart Displays.

## The device

| | |
|---|---|
| Model | Lenovo SmartClock 2, CD-24502F |
| SoC | MediaTek MT8167, quad Cortex-A35, **armv7 userspace, 32-bit only** |
| GPU | PowerVR GE8100 |
| Panel | 4-inch, 480×800 framebuffer shown landscape (800×480) by SurfaceFlinger; fts_ts touch; **no camera** |
| Audio | TI TAS5805M amplifier on i2c-2 at 0x2d, run by the kernel's `clock2` codec driver; mono speaker; two PDM microphones on the SoC's DMIC input |
| OS | Android 10 (API 29), kernel 4.14 |
| Controls | volume buttons and mic switch (`odm:gpio-keys`: keys 115, 114, 248) |

## Installing

Needs Magisk root, with su granted to the adb shell, and adb to the clock:
either a cable to the debug port or adb over Wi-Fi (`adb connect` first). The
install is the same as on the Smart Displays:

```sh
lanovo install --name bedroom                     # the released host CLI
lanovo install --serial 192.168.1.50:5555         # the same over the network
```

or, from a checkout, `make dist` and then `bin/lanovo install`. After that,
`make install-lanovod` (`DEVICE=<ip>:5555` over the network) pushes a new
`lanovod` and lets the keeper restart it; it stops if `lanovo install` has not
set the clock up yet. `lanovo install` sees the board, skips
the boot-image patch, the Wi-Fi setup and the system-partition install, and
puts `lanovod` under `/data/adb` instead. The clock keeps its Wi-Fi: the
networks the stock UI saved are carried over.

The same `lanovod` runs on every board. It carries the native helpers for each
board's API (the clock's are API 28, the Smart Displays' API 27) and extracts
the ones its board needs on start. On the clock it installs:

| On the clock | From |
|---|---|
| `/data/adb/lanovod/bin/lanovod` | `bin/lanovod` |
| `/data/adb/lanovod/lib/liblanovo-shim.so` | `shim.c`, extracted by lanovod |
| `/data/adb/lanovod/wificonf.awk` | `wificonf.awk` |
| `/data/adb/service.d/lanovod.sh` | `lanovod.sh` |

The swap and restart run on the clock from `install.sh`, detached from adb and
only after the pushed binary's SHA-256 checks out, because stopping `lanovod`
drops the clock off Wi-Fi: an install over adb-over-Wi-Fi loses its connection
for a moment, then the new `lanovod` brings the network back and `lanovo
install` reconnects. `lanovod.sh` keeps `lanovod` and the surface helper
running, restarting either if it exits, so updates from the panel work as on
the Smart Displays.

For development:

```sh
make logs           # follow lanovod's log
make clock-state    # what is running
make clock-restart  # restart lanovod and the surface helper
make clock-stop     # stop them
make remove-clock   # remove the install; reboot to get the stock UI back
```

State (settings, keys) lives in `/data/misc/lanovo` and survives a removal.
LANovo's own log is `/data/adb/lanovod/lanovod.log`.

## How it works

- **The surface helper on Android 10.** The shared native helpers build for API
  28 at most. The API-28 `lanovo-surface` runs with `liblanovo-shim.so`
  preloaded. The shim exports the five libgui/libui symbols Android 10 renamed
  or dropped and forwards them to their Android 10 replacements, then fixes the
  layout of the helper's hand-built `Transaction`. The comments in `shim.c` give
  the AOSP sources for each mapping. The proper fix is an API-29 build in
  libcountertop.
- **Taking over from Android.** `lanovod run` stops zygote (so the stock UI,
  system_server and the stock audio and Wi-Fi management go away). The service
  script covers what system_server used to do:
  - keeps the stock boot animation from covering LANovo;
  - powers the Wi-Fi chip on (`/dev/wmtWifi`) and runs its own `wpa_supplicant`
    with a control socket in `/data/misc/wifi/sockets`, its config seeded once
    from the networks the stock UI saved (`WifiConfigStore.xml`);
  - adds an `ip rule` so traffic uses the main routing table, which lanovod's
    DHCP fills (netd's per-network tables stay empty with no framework);
  - turns off Magisk's su logging (each logged su starts `am`, which hangs with
    no framework) and tells Magisk the boot completed, since
    `sys.boot_completed` never comes and Magisk would otherwise drop into safe
    mode (modules and `service.d` off) on the third boot.
- **Display.** SurfaceFlinger already rotates the portrait framebuffer to
  landscape, so the board is an upright 800×480 panel (`FBWidth`/`FBHeight`
  keep detection on the 480×800 framebuffer mode).
- **Audio.** The speaker needs the stock HAL's speaker path (`I2S O03_O04
  Switch`) before DL1 will prepare. Unlike the Smart Displays, the TAS5805M is
  left alone: the kernel's `clock2` codec driver sets it up for this speaker
  on each stream start. Loading the Smart Display amp tables over it trips the
  amp's overcurrent fault and makes it hiss. `lanovod tools amp --bus 2 --addr
  45` reads its state and faults. The microphones are PDM mics read on
  MultiMedia1_Capture with no ADC to load.
- **Charging base.** The night light is in the wireless charging base, not the
  clock: a WCH CH554 on USB (`1a86:5722`) that shows up as `/dev/ttyACM0`, the
  port the stock `charge_base` service drives. LANovo offers the base to Home
  Assistant as a dimmable "Night light" (levels 1 to 9; the firmware ignores
  10) and a "Charging" sensor for a phone on the pad. The light's setting is
  kept across restarts, read back from the base, and set again when the base
  comes back dark (the clock lifted off it, or the base reset). The base
  blinks the light by itself when a phone is set down, with a cooldown.
  `lanovod tools dock status` and `lanovod tools dock light 0-9` talk to it by
  hand. The protocol and what is known of the firmware are in
  `internal/hardware/chargebase`. The clock's own red, green and blue LEDs
  (`/sys/class/leds`) are not the night light.

## Current limitations

- **No camera, no protected video** (Widevine is not provisioned for LANovo).
- **adb over Wi-Fi** goes away with the framework (the magisk-wifiadb module
  needs it). Use the debug port, or LANovo's own setting (`access.adb`), which
  opens an unauthenticated root shell on port 5555 to the whole network.
- **The IP address changes** when LANovo takes over, because its DHCP client is
  not Android's. Reserve an address on the router.
- **Wi-Fi networks** are copied from the stock UI once. To change them later,
  use `lanovod wifi` on the clock or edit `/data/misc/wifi/lanovo_wpa.conf`.
- **Microphone sensitivity** is borrowed from the Smart Display 7, and the
  speaker is assumed mono; neither is calibrated.
- **Root is required.** Without it lanovod cannot stop zygote or open the sound
  card, the framebuffer or SurfaceFlinger.
