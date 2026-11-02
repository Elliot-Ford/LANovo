package parts

import (
	"strings"
	"testing"

	"github.com/ygelfand/LANovo/internal/board"
	"github.com/ygelfand/LANovo/internal/layout"
)

func TestRC(t *testing.T) {
	tests := []struct {
		name   string
		paths  layout.Paths
		want   []string
		absent []string
	}{
		{
			name:  "system",
			paths: board.Blueberry.Paths(),
			want: []string{
				"service lanovod /system/bin/lanovod run",
				"service lanovo_surface /system/bin/lanovo-surface",
				"setenv LD_PRELOAD /system/lib/liblanovo-camshim.so",
				"service lanovo_camera /system/bin/lanovo-camera",
				"group root system graphics input audio inet net_raw net_admin\n    seclabel u:r:su:s0\n    disabled",
			},
			absent: []string{"ro.logd.auditd", "lanovo_widevine"},
		},
		{
			name:  "data",
			paths: board.Penghu.Paths(),
			want: []string{
				"service lanovod /data/misc/lanovo/bin/lanovod run",
				"service lanovo_surface /data/misc/lanovo/bin/lanovo-surface",
				"setenv LD_PRELOAD /data/misc/lanovo/lib/liblanovo-camshim.so",
				"service lanovo_camera /data/misc/lanovo/bin/lanovo-camera",
				"group root system graphics input audio inet net_raw net_admin\n    seclabel u:r:init:s0\n    disabled",
				"on early-init\n    setprop ro.logd.auditd false",
				"service lanovo_widevine /data/misc/lanovo/bin/android.hardware.drm@1.0-service.widevine",
				"setenv LD_LIBRARY_PATH /data/misc/lanovo/lib:/vendor/lib:/system/lib",
				"on post-fs-data\n    start lanovo_widevine",
			},
			absent: []string{"/system/bin/lanovo", "/system/lib/liblanovo"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rc, err := RC(tt.paths)
			if err != nil {
				t.Fatalf("RC(%+v) = %v", tt.paths, err)
			}
			got := string(rc)
			if strings.Contains(got, "{{") {
				t.Errorf("RC(%s) left a template action unrendered", tt.name)
			}
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("RC(%s) lacks %q", tt.name, w)
				}
			}
			for _, a := range tt.absent {
				if strings.Contains(got, a) {
					t.Errorf("RC(%s) still names %q", tt.name, a)
				}
			}
		})
	}
}

func TestCarriedLeavesTheBootImageRC(t *testing.T) {
	for _, tt := range []struct {
		name   string
		board  board.Board
		wantRC bool
	}{
		{name: "system", board: board.Blueberry, wantRC: true},
		{name: "data", board: board.Penghu, wantRC: false},
		{name: "magisk", board: board.Helios, wantRC: false},
	} {
		paths := tt.board.Paths()
		carried, err := Carried(tt.board)
		if err != nil {
			t.Fatalf("Carried(%s) = %v", tt.name, err)
		}
		hasRC := false
		for _, p := range carried {
			if p.Name == InitRC.Name {
				hasRC = true
				if p.Path != paths.InitRC {
					t.Errorf("Carried(%s) rc at %q, want %q", tt.name, p.Path, paths.InitRC)
				}
			}
		}
		if hasRC != tt.wantRC {
			t.Errorf("Carried(%s) carries the rc = %v, want %v", tt.name, hasRC, tt.wantRC)
		}
	}
}

func TestAMagiskBoardCarriesItsBootScript(t *testing.T) {
	paths := board.Helios.Paths()
	if !paths.Magisk() || paths.Service != layout.MagiskService {
		t.Fatalf("helios paths %+v, want a Magisk service at %s", paths, layout.MagiskService)
	}
	carried, err := Carried(board.Helios)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		Surface.Name:       paths.Surface,
		Shim.Name:          paths.Shim,
		MagiskService.Name: paths.Service,
		WifiConf.Name:      paths.WifiConf,
	}
	for _, p := range carried {
		path, ok := want[p.Name]
		if !ok {
			t.Errorf("helios carries %s at %s, which a board with no camera does not need",
				p.Name, p.Path)
			continue
		}
		if p.Path != path {
			t.Errorf("%s at %s, want %s", p.Name, p.Path, path)
		}
		delete(want, p.Name)
	}
	for name := range want {
		t.Errorf("helios does not carry its %s", name)
	}
	if len(MagiskService.Data()) == 0 || len(WifiConf.Data()) == 0 || len(MagiskInstaller()) == 0 {
		t.Error("the Magisk scripts are not built in")
	}
}

func TestEveryBoardWithACameraCarriesItsHelpers(t *testing.T) {
	for _, b := range board.All() {
		carried, err := Carried(b)
		if err != nil {
			t.Fatal(err)
		}
		has := map[string]bool{}
		for _, p := range carried {
			has[p.Name] = true
		}
		if has[Camera.Name] != (b.CameraWidth > 0) || has[CamShim.Name] != (b.CameraWidth > 0) {
			t.Errorf("%s: camera parts %v/%v with a %dpx camera",
				b.Name, has[Camera.Name], has[CamShim.Name], b.CameraWidth)
		}
	}
}
