package takeover

import (
	"fmt"

	"github.com/ygelfand/libcountertop/pkg/host/adb"

	"github.com/ygelfand/LANovo/internal/board"
)

func DetectBoard(d *adb.Device, name string) (board.Board, error) {
	if name != "" {
		b, ok := board.ByName(name)
		if !ok {
			return board.Board{}, fmt.Errorf("no board %q", name)
		}
		return b, nil
	}
	f, err := board.Read(d)
	if err != nil {
		// A Magisk board's adb shell is unprivileged and cannot read the panel mode.
		if rooted, suErr := board.Read(suReader{d}); suErr == nil {
			f, err = rooted, nil
		}
	}
	if err != nil {
		return board.Board{}, err
	}
	return board.Detect(f)
}

type suReader struct{ d *adb.Device }

func (r suReader) ReadFile(path string) ([]byte, error) {
	out, err := r.d.Shell(suCommand("cat " + path))
	return []byte(out), err
}
