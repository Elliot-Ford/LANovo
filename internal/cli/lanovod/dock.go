package lanovod

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/ygelfand/LANovo/internal/board"
	"github.com/ygelfand/LANovo/internal/hardware/chargebase"
)

func newDockCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "dock",
		Short: "Read or set the wireless charging base",
		Long: "Talks to the base directly, bypassing the saved settings. A running lanovod\n" +
			"puts its saved night light back within a few seconds.\n\n" +
			"  lanovod tools dock status\n" +
			"  lanovod tools dock light 9      night light, 0 (off) to 9",
	}
	c.AddCommand(
		dockCmd("status", "Show the night light and what is on the pad", cobra.NoArgs,
			func(b *chargebase.Base, _ []string) error {
				st, err := b.Status()
				if err != nil {
					return err
				}
				light := strconv.Itoa(st.Light)
				if st.Light == chargebase.Blinking {
					light = "blinking for a phone"
				}
				fmt.Printf("night light  %s\n", light)
				fmt.Printf("charging     %s (%s)\n", yesNo(st.Pad.Charging()), st.Pad)
				return nil
			}),
		dockCmd("light LEVEL", "Set the night light, 0 (off) to 9", cobra.ExactArgs(1),
			func(b *chargebase.Base, args []string) error {
				level, err := strconv.Atoi(args[0])
				if err != nil {
					return fmt.Errorf("level %q is not a number", args[0])
				}
				return b.Light(level)
			}),
	)
	return c
}

func dockCmd(
	use, short string,
	args cobra.PositionalArgs,
	run func(*chargebase.Base, []string) error,
) *cobra.Command {
	return &cobra.Command{
		Use:          use,
		Short:        short,
		Args:         args,
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, a []string) error {
			path := board.Current().ChargeBase
			if path == "" {
				return errors.New("this board has no charging base")
			}
			b, err := chargebase.Open(path)
			if err != nil {
				return err
			}
			defer b.Close()
			return run(b, a)
		},
	}
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}
