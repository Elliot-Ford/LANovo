package mic

import (
	"errors"
	"sync"

	"github.com/ygelfand/libcountertop/pkg/audio/alsa"

	"github.com/ygelfand/LANovo/internal/board"
	"github.com/ygelfand/LANovo/internal/hardware/mtkaudio"
)

// The TLV320ADC3101 sends 24-bit samples in 32-bit I2S words; PDM mics on the
// SoC's DMIC input (digital) come as 16-bit samples with nothing to load.
type mediatek struct {
	chip    *board.Chip
	digital bool

	mu     sync.Mutex
	loaded bool
}

func (*mediatek) device() int { return 1 }

func (a *mediatek) bits() int {
	if a.digital {
		return 16
	}
	return 32
}

func (a *mediatek) open(*alsa.Mixer) error {
	if a.digital {
		return nil
	}
	if a.chip == nil {
		return errors.New("mic: this board has no microphone ADC")
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.loaded {
		return nil
	}
	t, err := mtkaudio.Stock()
	if err != nil {
		return err
	}
	if err := mtkaudio.MicOn(*a.chip, t); err != nil {
		return err
	}
	a.loaded = true
	return nil
}

func (a *mediatek) gain(_ *alsa.Mixer, gain int) error {
	if a.chip == nil {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.loaded {
		return nil
	}
	return mtkaudio.MicGain(*a.chip, float64(gain-DefaultGain))
}
