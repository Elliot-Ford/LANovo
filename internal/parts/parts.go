package parts

import (
	"bytes"
	_ "embed"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"text/template"

	"github.com/ygelfand/LANovo/internal/android/prop"
	"github.com/ygelfand/LANovo/internal/board"
	"github.com/ygelfand/LANovo/internal/layout"
)

//go:embed lanovod.rc
var initRC string

var rcTemplate = template.Must(template.New("lanovod.rc").Option("missingkey=error").Parse(initRC))

//go:embed magisk/lanovod.sh
var magiskService []byte

//go:embed magisk/wificonf.awk
var wifiConf []byte

//go:embed magisk/install.sh
var magiskInstall []byte

// The installer a host runs on a Magisk board to put lanovod in place.
func MagiskInstaller() []byte { return magiskInstall }

// native (payload.go) is nil when the helpers are not built in.
func embedded(name string) []byte {
	if native == nil {
		return nil
	}
	b, err := fs.ReadFile(native, "payload/"+name)
	if err != nil {
		return nil
	}
	return b
}

// One lanovod carries every board's helpers, each built for the board's NativeAPI.
func helper(api int, name string) []byte { return embedded(fmt.Sprintf("api%d/%s", api, name)) }

type Part struct {
	Name     string
	Path     string
	Mode     os.FileMode
	Services []string
	SoC      board.SoC
	Boot     bool

	data []byte
}

func (p Part) Data() []byte { return p.data }

func Speech() []byte { return helper(board.Current().NativeAPI, "lanovo-asr") }

func Pryon() []byte { return helper(board.Current().NativeAPI, "lanovo-pryon") }

func AirPlay() []byte { return helper(board.Current().NativeAPI, "lanovo-airplay") }

func (p Part) For(soc board.SoC) bool { return p.SoC == "" || p.SoC == soc }

func (p Part) at(path string) Part {
	p.Path = path
	return p
}

func (p Part) with(data []byte) Part {
	p.data = data
	return p
}

var (
	Lanovod = Part{
		Name:     "lanovod",
		Mode:     0o755,
		Services: []string{layout.Service},
	}
	InitRC  = Part{Name: "service", Mode: 0o644, Boot: true}
	Surface = Part{Name: "helper", Mode: 0o755, Services: []string{layout.SurfaceService}}
	CamShim = Part{
		Name: "camera preload",
		Mode: 0o644,
		Services: []string{
			layout.CameraHALService,
			layout.CameraServerService,
			layout.CameraService,
		},
	}
	Camera = Part{Name: "camera helper", Mode: 0o755, Services: []string{layout.CameraService}}

	// On a Magisk board: the boot script that starts and keeps up lanovod and
	// the surface helper, the preload the API-28 helper needs on Android 10,
	// and the script that seeds Wi-Fi from the stock networks.
	MagiskService = Part{Name: "boot script", Mode: 0o755, Boot: true, data: magiskService}
	Shim          = Part{
		Name:     "surface preload",
		Mode:     0o644,
		Services: []string{layout.SurfaceService},
	}
	WifiConf = Part{Name: "wifi seed", Mode: 0o644, data: wifiConf}
)

func RC(p layout.Paths) ([]byte, error) {
	var b bytes.Buffer
	if err := rcTemplate.Execute(&b, p); err != nil {
		return nil, fmt.Errorf("parts: rendering %s: %w", rcTemplate.Name(), err)
	}
	return b.Bytes(), nil
}

// A board with its root elsewhere off /system carries the service definition
// in its boot image; a Magisk board carries its boot script here.
func Carried(b board.Board) ([]Part, error) {
	p := b.Paths()
	carried := []Part{Surface.at(p.Surface).with(helper(b.NativeAPI, "lanovo-surface"))}
	if b.CameraWidth > 0 {
		carried = append(carried,
			CamShim.at(p.CamShim).with(helper(b.NativeAPI, "liblanovo-camshim.so")),
			Camera.at(p.Camera).with(helper(b.NativeAPI, "lanovo-camera")))
	}
	switch {
	case p.Magisk():
		return append(carried,
			Shim.at(p.Shim).with(embedded("liblanovo-shim.so")),
			MagiskService.at(p.Service),
			WifiConf.at(p.WifiConf)), nil
	case !p.OnSystem():
		return carried, nil
	}
	rc, err := RC(p)
	if err != nil {
		return nil, err
	}
	rcPart := InitRC.at(p.InitRC)
	rcPart.data = rc
	return append([]Part{rcPart}, carried...), nil
}

func Ensure() (rebootPending bool) {
	b := board.Current()
	paths := b.Paths()
	carried, err := Carried(b)
	if err != nil {
		slog.Error("listing parts failed", "err", err)
		return false
	}

	var stale []Part
	for _, p := range carried {
		if len(p.data) == 0 || !p.For(b.SoC) {
			continue
		}
		if have, err := os.ReadFile(p.Path); err == nil && bytes.Equal(have, p.data) {
			continue
		}
		stale = append(stale, p)
	}
	if len(stale) == 0 {
		return false
	}

	restore, err := Writable(paths)
	if err != nil {
		slog.Error("remounting to update parts failed", "err", err)
		return false
	}
	defer func() {
		if err := restore(); err != nil {
			slog.Error("remounting read-only failed", "err", err)
		}
	}()

	for _, p := range stale {
		if err := Replace(p.Path, p.data, p.Mode, paths.Binary); err != nil {
			slog.Error("updating a part failed", "part", p.Name, "path", p.Path, "err", err)
			continue
		}
		slog.Info("part updated", "part", p.Name, "path", p.Path)
		if paths.Magisk() {
			// No init services here: the boot script restarts what exits.
			if len(p.Services) > 0 {
				stopRunning(paths.Surface)
			}
			rebootPending = rebootPending || p.Boot
			continue
		}
		for _, svc := range p.Services {
			if state, _ := prop.Local.Getprop("init.svc." + svc); state != "running" {
				continue
			}
			if err := prop.Restart(prop.Local, svc); err != nil {
				slog.Error("restarting a service failed", "service", svc, "err", err)
			}
		}
		rebootPending = rebootPending || p.Boot
	}
	return rebootPending
}

// stopRunning signals every process running the binary at path, including one
// whose file has just been replaced (its exe then reads "path (deleted)").
func stopRunning(path string) {
	dirs, err := os.ReadDir("/proc")
	if err != nil {
		return
	}
	for _, d := range dirs {
		exe, err := os.Readlink("/proc/" + d.Name() + "/exe")
		if err != nil || (exe != path && !strings.HasPrefix(exe, path+" ")) {
			continue
		}
		var pid int
		if _, err := fmt.Sscanf(d.Name(), "%d", &pid); err != nil {
			continue
		}
		if proc, err := os.FindProcess(pid); err == nil {
			if err := proc.Signal(syscall.SIGTERM); err != nil {
				slog.Warn("stopping a stale helper failed", "pid", pid, "err", err)
			}
		}
	}
}

func Replace(path string, data []byte, mode os.FileMode, labelFrom string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".new"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	if err := os.Chmod(tmp, mode); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := CopyLabel(labelFrom, tmp); err != nil {
		slog.Warn("labelling failed", "path", tmp, "err", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("parts: replacing %s: %w", path, err)
	}
	return nil
}
