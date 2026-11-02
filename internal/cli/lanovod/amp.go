package lanovod

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ygelfand/libcountertop/pkg/system/i2c"

	"github.com/ygelfand/LANovo/internal/board"
)

// TAS5805M status registers, all on book 0 page 0.
var ampRegisters = []struct {
	reg  byte
	name string
}{
	{0x03, "DEVICE_CTRL2 (state: 0 deep sleep, 1 sleep, 2 Hi-Z, 3 play; bit 3 mute)"},
	{0x4c, "DIG_VOL (0x30 is 0 dB, 0.5 dB a step, 0xff mute)"},
	{0x33, "SAP_CTRL1"},
	{0x37, "FS_MON (detected sample rate)"},
	{0x38, "BCK_MON"},
	{0x39, "CLKDET_STATUS"},
	{0x68, "POWER_STATE (0 deep sleep, 1 sleep, 2 Hi-Z, 3 play)"},
	{0x70, "CHAN_FAULT"},
	{0x71, "GLOBAL_FAULT1"},
	{0x72, "GLOBAL_FAULT2"},
	{0x73, "WARNING"},
}

func newAmpCmd() *cobra.Command {
	var clear, play bool
	var writes []string
	var busFlag, addrFlag int
	c := &cobra.Command{
		Use:   "amp",
		Short: "Read the MediaTek board's TAS5805M amplifier state",
		Long: "Prints the amplifier's control and status registers.\n\n" +
			"  lanovod tools amp\n" +
			"  lanovod tools amp --clear    clear latched faults first\n" +
			"  lanovod tools amp --play     ask for play state first\n" +
			"  lanovod tools amp --write 0x02=0x04   write book 0 page 0 registers first",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(*cobra.Command, []string) error {
			n, addr := busFlag, uint16(addrFlag)
			if b := board.Current(); b.Amp != nil && addrFlag == 0 {
				n, addr = b.Amp.Bus, b.Amp.Addr
			}
			if addr == 0 {
				return errors.New(
					"this board has no amplifier driven from userspace: give --bus and --addr",
				)
			}
			bus, err := i2c.Open(n)
			if err != nil {
				return err
			}
			defer bus.Close()
			dev, err := i2c.At(bus, addr)
			if err != nil {
				return err
			}
			rows := [][2]byte{{0x00, 0x00}, {0x7f, 0x00}}
			if clear {
				rows = append(rows, [2]byte{0x78, 0x80})
			}
			for _, w := range writes {
				var reg, val uint8
				if _, err := fmt.Sscanf(w, "0x%x=0x%x", &reg, &val); err != nil {
					return fmt.Errorf("--write %q: want 0xRR=0xVV", w)
				}
				rows = append(rows, [2]byte{reg, val})
			}
			if play {
				rows = append(rows, [2]byte{0x03, 0x03})
			}
			for _, row := range rows {
				if err := dev.Write(row[0], row[1]); err != nil {
					return fmt.Errorf("writing %#02x=%#02x: %w", row[0], row[1], err)
				}
			}
			for _, r := range ampRegisters {
				v, err := dev.Byte(r.reg)
				if err != nil {
					fmt.Printf("%#04x  error: %v  %s\n", r.reg, err, r.name)
					continue
				}
				fmt.Printf("%#04x  %#04x  %s\n", r.reg, v, r.name)
			}
			return nil
		},
	}
	c.Flags().IntVar(&busFlag, "bus", 0, "I2C bus, when the board does not drive the amplifier")
	c.Flags().
		IntVar(&addrFlag, "addr", 0, "I2C address, when the board does not drive the amplifier (0x2d on the SmartClock 2)")
	c.Flags().BoolVar(&clear, "clear", false, "clear latched faults first")
	c.Flags().BoolVar(&play, "play", false, "ask for play state first")
	c.Flags().
		StringArrayVar(&writes, "write", nil, "register to write first, as 0xRR=0xVV; repeatable")
	return c
}
