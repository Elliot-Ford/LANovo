package takeover

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/ygelfand/libcountertop/pkg/host/adb"

	"github.com/ygelfand/LANovo/internal/board"
	"github.com/ygelfand/LANovo/internal/layout"
	"github.com/ygelfand/LANovo/internal/parts"
)

// A Magisk board keeps its stock, locked boot chain: adbd stays unprivileged,
// and root comes from Magisk's su, which the shell user must be granted.
func Magisk(b board.Board) bool { return b.Paths().Magisk() }

const magiskTmp = "/data/local/tmp"

// suCommand runs cmd as root through Magisk's su. cmd must not contain a
// single quote.
func suCommand(cmd string) string { return "su -c '" + cmd + "'" }

var ErrNoSu = errors.New("su did not give the shell root")

func RequireSu(d *adb.Device) error {
	out, err := d.Shell(suCommand("id -u"))
	if err != nil || strings.TrimSpace(out) != "0" {
		return fmt.Errorf("%w: install Magisk, then grant Shell superuser access in it", ErrNoSu)
	}
	return nil
}

type pushed struct {
	what, name string
	data       []byte
	mode       os.FileMode
}

// InstallMagisk pushes lanovod, its boot script and the Wi-Fi seed to the
// board, and starts the installer detached as root (parts.MagiskInstaller):
// restarting lanovod drops an adb connection made over Wi-Fi, so the swap
// cannot depend on this session. Wait for the result with WaitMagisk.
func InstallMagisk(d *adb.Device, binary []byte, b board.Board, name string) (Result, error) {
	var res Result
	if len(binary) == 0 {
		return res, errors.New("no lanovod to install")
	}
	files := []pushed{
		{parts.Lanovod.Name, "lanovod", binary, 0o755},
		{parts.MagiskService.Name, "lanovod.sh", parts.MagiskService.Data(), 0o644},
		{parts.WifiConf.Name, "wificonf.awk", parts.WifiConf.Data(), 0o644},
		{"installer", "lanovo-install.sh", parts.MagiskInstaller(), 0o644},
	}
	if name != "" {
		if err := ValidName(name); err != nil {
			return res, err
		}
		files = append(files, pushed{"name", "lanovo-name", []byte(name + "\n"), 0o644})
	}
	for _, f := range files {
		to := magiskTmp + "/" + f.name
		if err := d.WriteFile(to, f.data, f.mode); err != nil {
			return res, fmt.Errorf("pushing %s: %w", to, err)
		}
	}

	start := "setsid nohup sh " + magiskTmp + "/lanovo-install.sh " + digest(binary) +
		" </dev/null >/dev/null 2>&1 &"
	if _, err := d.Shell(suCommand(start)); err != nil {
		return res, fmt.Errorf("starting the installer: %w", err)
	}

	paths := b.Paths()
	res.Steps = append(res.Steps,
		Step{What: parts.Lanovod.Name, Note: paths.Binary},
		Step{What: parts.MagiskService.Name, Note: paths.Service},
	)
	return res, nil
}

// WaitMagisk waits for the given lanovod to be in place and answering. Over
// Wi-Fi the board drops off the network while lanovod restarts, so a serial
// that is an address is reconnected as it goes.
func WaitMagisk(ctx context.Context, d *adb.Device, binary []byte, b board.Board) error {
	paths := b.Paths()
	want := digest(binary)

	deadline := time.Now().Add(3 * time.Minute)
	for time.Now().Before(deadline) {
		out, err := d.Shell(suCommand(
			"sha256sum " + paths.Binary + " && " + paths.Binary + " ctl device version"))
		if err == nil && strings.HasPrefix(strings.TrimSpace(out), want) {
			return nil
		}
		if err != nil && strings.Contains(d.Serial(), ":") {
			_ = exec.CommandContext(ctx, adb.Binary, "connect", d.Serial()).Run()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
	return fmt.Errorf("%s did not come back within three minutes; the installer logs "+
		"to logcat as %s", layout.Service, layout.LogTag)
}

func digest(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
