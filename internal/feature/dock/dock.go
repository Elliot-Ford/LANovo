// Package dock offers a board's wireless charging base to Home Assistant: its
// night light, and whether something is charging on it.
package dock

import (
	"context"
	"log/slog"
	"math"
	"sync"
	"sync/atomic"
	"time"

	esphome "github.com/ygelfand/go-esphome-device"

	sharedcomponent "github.com/ygelfand/libcountertop/pkg/runtime/component"

	"github.com/ygelfand/LANovo/internal/board"
	"github.com/ygelfand/LANovo/internal/component"
	"github.com/ygelfand/LANovo/internal/config"
	"github.com/ygelfand/LANovo/internal/hardware/chargebase"
)

func init() {
	component.Register(sharedcomponent.Device, Get, sharedcomponent.Order(16),
		component.Needs(board.Dock))
}

// A phone takes 7 to 10 seconds from being set down to charging.
const pollEvery = 3 * time.Second

type base interface {
	Light(level int) error
	Status() (chargebase.Status, error)
	Close() error
}

type Dock struct {
	light    *esphome.Light
	charging *esphome.BinarySensor

	path string
	open func(string) (base, error)

	mu   sync.Mutex
	base base

	// reached is whether the base last answered: the clock can be lifted
	// off it.
	reached atomic.Bool
	pad     chargebase.Pad
}

var Get = sync.OnceValue(func() *Dock {
	return build(board.Current().ChargeBase, func(path string) (base, error) {
		return chargebase.Open(path)
	})
})

func build(path string, open func(string) (base, error)) *Dock {
	d := &Dock{path: path, open: open}
	d.light = &esphome.Light{
		Base: esphome.Base{
			ObjectID: "night_light",
			Name:     "Night light",
			Icon:     "mdi:lightbulb-night",
		},
		SupportedColorModes: []esphome.ColorMode{esphome.ColorModeBrightness},
		OnCommand:           d.lightCommand,
	}
	d.charging = &esphome.BinarySensor{
		Base: esphome.Base{
			ObjectID: "charging",
			Name:     "Charging",
		},
		DeviceClass: "battery_charging",
	}
	return d
}

func (d *Dock) Name() string { return "dock" }

func (d *Dock) Entities() []esphome.Entity { return []esphome.Entity{d.light, d.charging} }

func (d *Dock) Restore(cfg config.Config) { d.publishLight(cfg.Dock.Light) }

// A missing base is not a failure: the clock may be off it.
func (d *Dock) Start(context.Context) error {
	if err := d.setLight(lightLevel(config.Get().Dock.Light)); err != nil {
		slog.Warn("the charging base is unreachable", "err", err)
		return nil
	}
	d.reached.Store(true)
	return nil
}

func (d *Dock) Run(ctx context.Context) error {
	tick := time.NewTicker(pollEvery)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
		}
		d.poll()
	}
}

func (d *Dock) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.base == nil {
		return nil
	}
	err := d.base.Close()
	d.base = nil
	return err
}

func (d *Dock) poll() {
	var st chargebase.Status
	err := d.with(func(b base) (err error) {
		st, err = b.Status()
		return err
	})
	if err != nil {
		if d.reached.Swap(false) {
			slog.Info("the charging base is gone", "err", err)
			d.charging.Set(false)
		}
		return
	}
	if !d.reached.Swap(true) {
		slog.Info("the charging base is back")
	}

	// A base replugged, or reset, comes back dark.
	want := lightLevel(config.Get().Dock.Light)
	if st.Light != want && st.Light != chargebase.Blinking {
		slog.Info("the night light is not as set, setting it again", "at", st.Light, "want", want)
		if err := d.setLight(want); err != nil {
			slog.Warn("restoring the night light failed", "err", err)
		}
	}

	if st.Pad != d.pad {
		slog.Info("charging pad", "state", st.Pad)
		d.pad = st.Pad
	}
	d.charging.Set(st.Pad.Charging())
}

func (d *Dock) lightCommand(s esphome.LightState) {
	saved := config.Get().Dock
	next := saved
	next.Light.On = s.On && s.Brightness > 0
	if next.Light.On {
		next.Light.Level = levelFor(s.Brightness)
	}

	if err := d.setLight(lightLevel(next.Light)); err != nil {
		slog.Warn("setting the night light failed", "err", err)
		d.publishLight(saved.Light)
		return
	}
	if err := config.Set().Dock().Set(next); err != nil {
		slog.Warn("saving the night light failed", "err", err)
	}
	d.publishLight(next.Light)
}

func (d *Dock) setLight(level int) error {
	if err := d.with(func(b base) error { return b.Light(level) }); err != nil {
		return err
	}
	slog.Info("night light", "level", level)
	return nil
}

// with runs f on the open base. A base that was unplugged and replugged
// leaves the old port dead, so a failure reopens it once.
func (d *Dock) with(f func(base) error) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.base != nil {
		err := f(d.base)
		if err == nil {
			return nil
		}
		slog.Debug("the charging base did not answer, reopening it", "err", err)
		_ = d.base.Close()
		d.base = nil
	}

	b, err := d.open(d.path)
	if err != nil {
		return err
	}
	d.base = b
	return f(b)
}

func (d *Dock) publishLight(s config.NightLight) {
	d.light.Set(esphome.LightState{
		On:         s.On,
		Brightness: float32(clampLevel(s.Level)) / config.NightLightLevelMax,
		ColorMode:  esphome.ColorModeBrightness,
	})
}

func lightLevel(s config.NightLight) int {
	if !s.On {
		return 0
	}
	return clampLevel(s.Level)
}

// Home Assistant's brightness comes back rounded to 1/255, so a level read
// back from what was published must land on itself.
func levelFor(brightness float32) int {
	return clampLevel(int(math.Round(float64(brightness) * config.NightLightLevelMax)))
}

func clampLevel(l int) int {
	return max(config.NightLightLevelMin, min(l, config.NightLightLevelMax))
}
