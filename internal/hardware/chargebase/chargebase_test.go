package chargebase

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"time"
)

// Frames the stock HAL and test_heliosbase sent to a SmartClock 2's base.
func TestFramesMatchWhatTheStockToolSends(t *testing.T) {
	tests := []struct {
		got  []byte
		want []byte
	}{
		{frame(cmdSet, 7, keep), []byte{0x80, 0x52, 0x07, 0x07, 0xff, 0x01, 0xdf}},
		{frame(cmdSet, 0, keep), []byte{0x80, 0x52, 0x07, 0x00, 0xff, 0x01, 0xd8}},
		{frame(cmdSet, keep, 1), []byte{0x80, 0x52, 0x07, 0xff, 0x01, 0x01, 0xd9}},
		{frame(0x20, 0, 0), []byte{0x80, 0x20, 0x07, 0x00, 0x00, 0x00, 0xa7}},
		{frame(cmdStatus, 1, 1, 1), []byte{0x80, 0x51, 0x08, 0x01, 0x01, 0x01, 0x00, 0xdc}},
	}
	for _, tt := range tests {
		if !bytes.Equal(tt.got, tt.want) {
			t.Errorf("frame = % x, want % x", tt.got, tt.want)
		}
	}
}

// Answers the base gave, and status answers built the same way.
var (
	lit        = []byte{0x80, 0x52, 0x07, 0x01, 0x00, 0x00, 0xda}
	litFlagged = []byte{0x80, 0x72, 0x07, 0x01, 0x00, 0x00, 0xfa}
	charging   = []byte{0x80, 0x51, 0x08, 0x00, 0x01, 0x03, 0x00, 0xdd}
	blinking   = []byte{0x80, 0x51, 0x08, 0x0b, 0x01, 0x02, 0x00, 0xe7}
	// The charger's version, flagged: the sum carries the 0x20, the command
	// byte (already 0x21) cannot show it.
	versionFlagged = []byte{0x80, 0x21, 0x09, 0x1c, 0x02, 0x00, 0x7f, 0x01, 0x67}
)

func status(light byte, pad Pad) []byte {
	f := []byte{0x80, 0x51, 0x08, light, 0x01, byte(pad)}
	s := sum(f)
	return append(f, byte(s>>8), byte(s))
}

type fakePort struct {
	sent    bytes.Buffer
	replies [][]byte
	flushed int
	closed  bool
}

// Each write is answered by the next reply.
func (p *fakePort) Write(b []byte) (int, error) { return p.sent.Write(b) }

func (p *fakePort) Read(b []byte) (int, error) {
	if len(p.replies) == 0 {
		return 0, io.EOF
	}
	n := copy(b, p.replies[0])
	p.replies = p.replies[1:]
	return n, nil
}

func (p *fakePort) Close() error          { p.closed = true; return nil }
func (p *fakePort) flush() error          { p.flushed++; return nil }
func (p *fakePort) until(time.Time) error { return nil }

func base(replies ...[]byte) (*Base, *fakePort) {
	p := &fakePort{replies: replies}
	return &Base{port: p}, p
}

func quick(t *testing.T) {
	was := answerWithin
	answerWithin = 20 * time.Millisecond
	t.Cleanup(func() { answerWithin = was })
}

func TestTheLightIsSetAndReadBack(t *testing.T) {
	b, p := base(lit, status(5, PadIdle))
	if err := b.Light(5); err != nil {
		t.Fatalf("Light(5): %v", err)
	}
	want := append(frame(cmdSet, 5, keep), frame(cmdStatus, 1, 1, 1)...)
	if !bytes.Equal(p.sent.Bytes(), want) {
		t.Errorf("sent % x, want % x", p.sent.Bytes(), want)
	}
	if p.flushed != 2 {
		t.Errorf("flushed %d times, want 2", p.flushed)
	}
}

func TestALightThatDidNotChangeIsAnError(t *testing.T) {
	b, _ := base(lit, status(0, PadIdle))
	if err := b.Light(5); err == nil {
		t.Error("Light(5) accepted a light still off")
	}
}

func TestABlinkForAPhoneIsNotAMismatch(t *testing.T) {
	b, _ := base(lit, blinking)
	if err := b.Light(5); err != nil {
		t.Errorf("Light(5) during a blink: %v", err)
	}
}

func TestAFlaggedAnswerIsStillAnAnswer(t *testing.T) {
	b, _ := base(litFlagged, status(4, PadIdle))
	if err := b.Light(4); err != nil {
		t.Errorf("Light(4): %v", err)
	}
	if f, ok := find(versionFlagged); !ok || len(f) != 9 {
		t.Errorf("find(% x) = % x, %v", versionFlagged, f, ok)
	}
}

func TestAnAnswerAfterStrayBytesIsFound(t *testing.T) {
	b, _ := base(append([]byte{0x00, 0x80, 0x13}, lit...), status(1, PadIdle))
	if err := b.Light(1); err != nil {
		t.Errorf("Light(1): %v", err)
	}
}

func TestARefusalIsAnError(t *testing.T) {
	refused := []byte{0x80, 0x52, 0x07, 0x00, 0x00, 0x00, 0xd9}
	b, _ := base(refused)
	if err := b.Light(3); err == nil {
		t.Error("Light(3) accepted a refusal")
	}
}

func TestSilenceIsNoAnswer(t *testing.T) {
	quick(t)
	b, _ := base()
	if err := b.Light(3); !errors.Is(err, ErrNoAnswer) {
		t.Errorf("Light(3) = %v, want ErrNoAnswer", err)
	}
}

func TestLevelsTheBaseIgnoresAreNotSent(t *testing.T) {
	for _, level := range []int{-1, MaxLevel + 1} {
		b, p := base(lit)
		if err := b.Light(level); err == nil {
			t.Errorf("Light(%d) succeeded", level)
		}
		if p.sent.Len() != 0 {
			t.Errorf("Light(%d) sent % x", level, p.sent.Bytes())
		}
	}
}

func TestTheStatusTellsTheLightAndThePad(t *testing.T) {
	tests := []struct {
		answer []byte
		want   Status
	}{
		{charging, Status{Light: 0, Pad: PadCharging}},
		{blinking, Status{Light: Blinking, Pad: PadDetected}},
		{status(7, PadIdle), Status{Light: 7, Pad: PadIdle}},
	}
	for _, tt := range tests {
		b, _ := base(tt.answer)
		got, err := b.Status()
		if err != nil {
			t.Fatalf("Status from % x: %v", tt.answer, err)
		}
		if got != tt.want {
			t.Errorf("Status from % x = %+v, want %+v", tt.answer, got, tt.want)
		}
	}
}

func TestChargingFollowsTheFirmware(t *testing.T) {
	for p, want := range map[Pad]bool{1: false, 2: true, 3: true, 6: true, 7: false} {
		if got := p.Charging(); got != want {
			t.Errorf("%v.Charging() = %v, want %v", p, got, want)
		}
	}
}

func TestAStatusAnsweredWithSomethingElseIsAnError(t *testing.T) {
	b, _ := base(lit)
	if _, err := b.Status(); err == nil {
		t.Error("Status accepted a set's answer")
	}
}
