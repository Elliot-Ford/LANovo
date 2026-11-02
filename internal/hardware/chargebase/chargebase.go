// Package chargebase drives the SmartClock 2's wireless charging base, whose
// CH554 runs the night light and reports on the charging pad.
//
// The base takes frames on its USB CDC port: 0x80, a command, the frame's
// length, its arguments, then the 16-bit big-endian sum of the bytes before.
// It answers each with a frame of the same shape. Sets answer with a 1 in the
// argument slot that was set. The answer's command byte has been seen with
// 0x20 added (and the checksum with it) for hours at a time, for no reason
// found; like the stock HAL, only the arguments are read.
//
// The commands are those of the stock heliosbase HAL; their meaning comes from
// the CH554's own firmware (/system/xbin/user.bin). The set command's second
// argument, the HAL's power_ctrl, latches a flag that drives three of the
// CH554's pins from the charger's state, a current or temperature reading and
// a timer. Turning it off changed nothing seen: wireless charging, fast
// wireless charging and the USB port on the back all carried on. It is left as
// the base comes up, on.
package chargebase

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

const (
	start = 0x80

	cmdSet    = 0x52
	cmdStatus = 0x51

	// keep leaves the other setting in a set frame alone.
	keep = 0xff

	// The longest answer seen is a version's, 9 bytes.
	maxFrame = 16
)

// MaxLevel is the brightest night light. The firmware drives the light's PWM
// at 25 a level and drops 10 and above without answering; 0 turns it off.
const MaxLevel = 9

// Blinking is the light's level while the base flashes it for a phone set
// down, which it does on its own, with a cooldown between flashes.
const Blinking = 0x0b

// The stock service waits two seconds; the base answers within a few
// milliseconds.
var answerWithin = time.Second

var ErrNoAnswer = errors.New("chargebase: the base did not answer")

// Pad is the charger chip's state as the status reports it.
type Pad byte

// Seen as a phone was set down and lifted off; the firmware also handles 6
// and 7, which have not been seen.
const (
	PadIdle     Pad = 1
	PadDetected Pad = 2
	PadCharging Pad = 3
)

// Charging groups the states the way the firmware does.
func (p Pad) Charging() bool { return p == 2 || p == 3 || p == 6 }

func (p Pad) String() string {
	switch p {
	case PadIdle:
		return "idle"
	case PadDetected:
		return "detected"
	case PadCharging:
		return "charging"
	}
	return fmt.Sprintf("charger state %d", byte(p))
}

type Status struct {
	// Light is the night light's level, 0 when off, or Blinking.
	Light int
	Pad   Pad
}

type port interface {
	io.ReadWriteCloser
	// flush drops anything the base sent that nobody read.
	flush() error
	// until bounds the next reads.
	until(time.Time) error
}

type Base struct {
	mu   sync.Mutex
	port port
}

func Open(path string) (*Base, error) {
	p, err := openPort(path)
	if err != nil {
		return nil, fmt.Errorf("chargebase: opening %s: %w", path, err)
	}
	return &Base{port: p}, nil
}

func (b *Base) Close() error { return b.port.Close() }

// Light sets the night light, 0 (off) to MaxLevel, and reads it back.
func (b *Base) Light(level int) error {
	if level < 0 || level > MaxLevel {
		return fmt.Errorf("chargebase: night light level %d is outside 0 to %d", level, MaxLevel)
	}
	r, err := b.ask(frame(cmdSet, byte(level), keep))
	if err != nil {
		return fmt.Errorf("chargebase: setting the night light to %d: %w", level, err)
	}
	if len(r) < 7 || r[3] != 1 {
		return fmt.Errorf("chargebase: the base refused night light %d: % x", level, r)
	}

	st, err := b.Status()
	if err != nil {
		return err
	}
	if st.Light != level && st.Light != Blinking {
		return fmt.Errorf("chargebase: the night light is at %d after setting %d", st.Light, level)
	}
	return nil
}

// Status reads the night light and the charger's state. The request's
// arguments are the stock HAL's; the firmware answers only to those.
func (b *Base) Status() (Status, error) {
	r, err := b.ask(frame(cmdStatus, 1, 1, 1))
	if err != nil {
		return Status{}, fmt.Errorf("chargebase: reading the status: %w", err)
	}
	if len(r) < 8 || r[1]&^0x20 != cmdStatus {
		return Status{}, fmt.Errorf("chargebase: the base answered a status with % x", r)
	}
	return Status{Light: int(r[3]), Pad: Pad(r[5])}, nil
}

func (b *Base) ask(f []byte) ([]byte, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if err := b.port.flush(); err != nil {
		return nil, err
	}
	if _, err := b.port.Write(f); err != nil {
		return nil, err
	}
	return b.answer(time.Now().Add(answerWithin))
}

func (b *Base) answer(deadline time.Time) ([]byte, error) {
	if err := b.port.until(deadline); err != nil {
		return nil, err
	}

	var got []byte
	buf := make([]byte, 64)
	for time.Now().Before(deadline) {
		n, err := b.port.Read(buf)
		got = append(got, buf[:n]...)
		if r, ok := find(got); ok {
			return r, nil
		}
		switch {
		case err == nil, errors.Is(err, io.EOF):
			// A tty read that times out with nothing returns EOF.
		case errors.Is(err, os.ErrDeadlineExceeded):
			return nil, ErrNoAnswer
		default:
			return nil, err
		}
	}
	return nil, ErrNoAnswer
}

func frame(cmd byte, args ...byte) []byte {
	f := append([]byte{start, cmd, byte(len(args) + 5)}, args...)
	s := sum(f)
	return append(f, byte(s>>8), byte(s))
}

func sum(b []byte) uint16 {
	var s uint16
	for _, c := range b {
		s += uint16(c)
	}
	return s
}

// find returns the first well-formed frame in b, skipping anything before it.
// A frame whose sum is off by the unexplained 0x20 still counts.
func find(b []byte) ([]byte, bool) {
	for i := 0; i+3 <= len(b); i++ {
		n := int(b[i+2])
		if b[i] != start || n < 5 || n > maxFrame || i+n > len(b) {
			continue
		}
		f := b[i : i+n]
		got, want := uint16(f[n-2])<<8|uint16(f[n-1]), sum(f[:n-2])
		if got == want || got == want+0x20 {
			return f, true
		}
	}
	return nil, false
}
