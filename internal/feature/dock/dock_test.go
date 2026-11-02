package dock

import (
	"errors"
	"path/filepath"
	"slices"
	"testing"

	esphome "github.com/ygelfand/go-esphome-device"

	"github.com/ygelfand/LANovo/internal/config"
	"github.com/ygelfand/LANovo/internal/hardware/chargebase"
)

type fakeBase struct {
	levels []int
	status chargebase.Status
	fail   bool
	closed bool
}

func (b *fakeBase) Light(level int) error {
	if b.fail {
		return errors.New("gone")
	}
	b.levels = append(b.levels, level)
	b.status.Light = level
	return nil
}

func (b *fakeBase) Status() (chargebase.Status, error) {
	if b.fail {
		return chargebase.Status{}, errors.New("gone")
	}
	return b.status, nil
}

func (b *fakeBase) Close() error { b.closed = true; return nil }

// rig hands out the bases in order, then fails to open any more.
func rig(t *testing.T, bases ...*fakeBase) (*Dock, *int) {
	t.Helper()
	config.Use(filepath.Join(t.TempDir(), "state.json"))

	opened := 0
	d := build("/dev/null", func(string) (base, error) {
		if opened >= len(bases) {
			return nil, errors.New("no base")
		}
		b := bases[opened]
		opened++
		return b, nil
	})
	return d, &opened
}

func TestEveryLevelSurvivesHomeAssistantsRounding(t *testing.T) {
	for level := config.NightLightLevelMin; level <= config.NightLightLevelMax; level++ {
		published := float32(level) / config.NightLightLevelMax
		rounded := float32(int(published*255+0.5)) / 255
		if got := levelFor(rounded); got != level {
			t.Errorf("level %d comes back from Home Assistant as %d", level, got)
		}
	}
	if got := levelFor(1.0 / 255); got != config.NightLightLevelMin {
		t.Errorf("the dimmest brightness is level %d, want %d", got, config.NightLightLevelMin)
	}
}

func TestStartingPutsTheSavedLightOnTheBase(t *testing.T) {
	b := &fakeBase{}
	d, _ := rig(t, b)

	if err := d.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(b.levels, []int{0}) {
		t.Errorf("sent %v, want [0]", b.levels)
	}
}

func TestTurningTheLightOnSetsTheLevelAndSavesIt(t *testing.T) {
	b := &fakeBase{}
	d, _ := rig(t, b)

	d.lightCommand(esphome.LightState{On: true, Brightness: 1})

	if !slices.Equal(b.levels, []int{9}) {
		t.Errorf("sent %v, want [9]", b.levels)
	}
	if got := config.Get().Dock.Light; got != (config.NightLight{On: true, Level: 9}) {
		t.Errorf("saved %+v", got)
	}
	if s := d.light.Get(); !s.On || s.Brightness != 1 {
		t.Errorf("published %+v", s)
	}
}

func TestTurningTheLightOffKeepsTheLevel(t *testing.T) {
	b := &fakeBase{}
	d, _ := rig(t, b)

	d.lightCommand(esphome.LightState{On: true, Brightness: 3.0 / 9})
	d.lightCommand(esphome.LightState{On: false, Brightness: 3.0 / 9})

	if !slices.Equal(b.levels, []int{3, 0}) {
		t.Errorf("sent %v, want [3 0]", b.levels)
	}
	if got := config.Get().Dock.Light; got != (config.NightLight{On: false, Level: 3}) {
		t.Errorf("saved %+v", got)
	}
}

func TestAFailedCommandLeavesTheSavedState(t *testing.T) {
	d, _ := rig(t)

	d.lightCommand(esphome.LightState{On: true, Brightness: 1})

	if got := config.Get().Dock.Light; got.On {
		t.Errorf("saved %+v after the base was unreachable", got)
	}
	if d.light.Get().On {
		t.Error("published a change the base never took")
	}
}

func TestADeadPortIsReopened(t *testing.T) {
	dead, fresh := &fakeBase{}, &fakeBase{}
	d, opened := rig(t, dead, fresh)

	d.lightCommand(esphome.LightState{On: true, Brightness: 1})
	dead.fail = true
	d.lightCommand(esphome.LightState{On: false, Brightness: 1})

	if *opened != 2 || !dead.closed {
		t.Errorf("opened %d times, dead closed %v", *opened, dead.closed)
	}
	if !slices.Equal(fresh.levels, []int{0}) {
		t.Errorf("the reopened base got %v, want [0]", fresh.levels)
	}
}

func TestPollingReportsAPhoneCharging(t *testing.T) {
	b := &fakeBase{status: chargebase.Status{Pad: chargebase.PadIdle}}
	d, _ := rig(t, b)
	if err := d.Start(t.Context()); err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		pad  chargebase.Pad
		want bool
	}{
		{chargebase.PadDetected, true},
		{chargebase.PadCharging, true},
		{chargebase.PadIdle, false},
	} {
		b.status.Pad = tt.pad
		d.poll()
		if got := d.charging.Get(); got != tt.want {
			t.Errorf("with the pad %v, charging = %v, want %v", tt.pad, got, tt.want)
		}
	}
}

func TestALightThatWentDarkIsSetAgain(t *testing.T) {
	gone, back := &fakeBase{}, &fakeBase{}
	d, _ := rig(t, gone, back)
	d.lightCommand(esphome.LightState{On: true, Brightness: 5.0 / 9})

	gone.fail = true
	d.poll()

	if !slices.Equal(back.levels, []int{5}) {
		t.Errorf("the replugged base got %v, want [5]", back.levels)
	}
}

func TestABaseThatIsGoneStopsShowingCharging(t *testing.T) {
	b := &fakeBase{status: chargebase.Status{Pad: chargebase.PadCharging}}
	d, _ := rig(t, b)
	if err := d.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	d.poll()

	b.fail = true
	d.poll()

	if d.reached.Load() || d.charging.Get() {
		t.Errorf("reached %v, charging %v after the base went away",
			d.reached.Load(), d.charging.Get())
	}
}

func TestABlinkingLightIsLeftAlone(t *testing.T) {
	b := &fakeBase{}
	d, _ := rig(t, b)
	d.lightCommand(esphome.LightState{On: true, Brightness: 5.0 / 9})

	b.status.Light = chargebase.Blinking
	d.poll()

	if !slices.Equal(b.levels, []int{5}) {
		t.Errorf("sent %v, want [5]", b.levels)
	}
}
