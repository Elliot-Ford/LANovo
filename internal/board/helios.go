package board

// The Lenovo SmartClock 2 (LenovoCD-24502F) is a horizontal 4" bedside display
// on the MT8167 with a portrait 480x800 framebuffer. Android 10's
// SurfaceFlinger rotates that itself (ro.surface_flinger.
// primary_display_orientation=1), so LANovo sees, and the fts_ts touchscreen
// reports, an 800x480 landscape panel mounted upright. There is no camera; the
// microphones are PDM mics on the SoC (the AD82128 at i2c-2 0x31 is not used).
//
// LANovo lives under a Magisk root here, run from /data/adb by a service.d
// script instead of /system, so no init rc is carried (see Carried).
var Helios = Board{
	NativeAPI:   28,
	Name:        "helios",
	Model:       "Lenovo SmartClock 2",
	SoC:         MediaTek,
	PanelWidth:  800,
	PanelHeight: 480,
	FBWidth:     480,
	FBHeight:    800,
	Diagonal:    4,
	UISize:      "compact",
	Mounted:     0,
	Touch:       []string{"fts_ts", "mtk-tpd"},
	Thermal:     Thermal{CPU: []string{"mtktscpu"}, PerDegree: 1000},
	// The kernel has no sysfs gpio export; odm:gpio-keys reports the buttons
	// (KEY_MICMUTE holds while the mic switch is on).
	Keys: map[uint16]string{
		115: "volume up",
		114: "volume down",
		248: "mic mute",
	},
	// No Amp: the TAS5805M at i2c-2 0x2d belongs to the kernel's clock2 codec
	// (2-001d), which sets it up for this speaker itself. The Smart Display
	// tables loaded over it trip its overcurrent fault and leave it hissing.
	// The stock HAL's speaker path (mixer_paths.xml): DL1 out over I2S.
	SpeakerSwitches: []string{"I2S O03_O04 Switch"},
	// Provisional: the speaker is mono; confirm in a listening test.
	Stereo: false,
	// Two PDM mics on the MT8167's DMIC (AIF TX Mux = Digital MIC), read on
	// MultiMedia1_Capture; a clap peaks near -12 dBFS over a -64 dBFS floor.
	DigitalMics: true,
	// Provisional, borrowed from ivy; calibrate against a reference.
	MicSensitivity: Ivy.MicSensitivity,
	// The GE8100 decodes H.264/HEVC at 30 fps; there is no video pipe.
	MaxFPS: 30,
	// The wireless charging base's night light, run by its CH554 over USB CDC.
	ChargeBase: "/dev/ttyACM0",
	Root:       "/data/adb/lanovod",
}

func init() { register(Helios) }
