package lanovo

import (
	"context"
	"fmt"
	"io"

	"github.com/ygelfand/libcountertop/pkg/host/adb"

	"github.com/ygelfand/LANovo/internal/board"
	"github.com/ygelfand/LANovo/internal/host/takeover"
)

// A Magisk board (the SmartClock 2) keeps its boot chain: there is no boot
// patch, dm-verity or adb root, and lanovod runs from a Magisk boot script
// rather than an init service. Wi-Fi carries over from the stock setup.
func installMagisk(
	ctx context.Context,
	out io.Writer,
	d *adb.Device,
	b board.Board,
	binary, name string,
) error {
	fmt.Fprintf(out, "  %-18s %s\n", "board", styleDone.Render(b.Name))
	if err := takeover.RequireSu(d); err != nil {
		return err
	}
	fmt.Fprintf(out, "  %-18s %s\n", "root", styleDone.Render("Magisk su"))

	chosen, err := resolveName(ctx, out, d, name)
	if err != nil {
		return err
	}

	payload, from, err := lanovod(binary)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "  %-18s %s\n", "lanovod", styleDetail.Render(from))

	paths := b.Paths()
	room, err := takeover.Room(d, paths, int64(len(payload)))
	if err != nil {
		return err
	}
	if !room.OK {
		fmt.Fprintf(out, "  %-18s %s\n", room.Name, styleFail.Render(room.Got))
		return fmt.Errorf("installing needs %s, have %s — %s", room.Want, room.Got, room.Fix)
	}
	fmt.Fprintf(out, "  %-18s %s\n", room.Name, styleDetail.Render(room.Got))

	res, err := takeover.InstallMagisk(d, payload, b, chosen)
	for _, s := range res.Steps {
		fmt.Fprintf(out, "  %-18s %s\n", s.What, styleDetail.Render(s.Note))
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "  %-18s %s\n", "name", styleDone.Render(chosen))

	fmt.Fprintf(out, "  %-18s %s\n", "starting",
		styleDetail.Render("over Wi-Fi the device drops off for a moment"))
	if err := takeover.WaitMagisk(ctx, d, payload, b); err != nil {
		return err
	}
	fmt.Fprintf(out, "  %-18s %s\n", "started", styleDone.Render("lanovod running"))
	fmt.Fprintf(out, "  %-18s %s\n", "wifi",
		styleDetail.Render("carried over from the stock setup"))

	reportKey(ctx, out, d, false)
	return nil
}
