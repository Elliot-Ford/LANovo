package config

// Dock is a board's charging base.
type Dock struct {
	Light NightLight `json:"light"`
}

// NightLight is the base's light. Level is kept while it is off, so turning it
// back on returns to the same brightness.
type NightLight struct {
	On    bool `json:"on"`
	Level int  `json:"level"`
}

const (
	NightLightLevelMin     = 1
	NightLightLevelMax     = 9
	DefaultNightLightLevel = 5
)

func defaultDock() Dock { return Dock{Light: NightLight{Level: DefaultNightLightLevel}} }

type DockWriter struct{ st *Store }

func (w DockWriter) Set(d Dock) error {
	return w.st.Update(func(c *Config) { c.Dock = d })
}
