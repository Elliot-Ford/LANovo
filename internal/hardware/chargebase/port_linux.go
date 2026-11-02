package chargebase

import (
	"fmt"
	"os"
	"syscall"
	"time"
	"unsafe"
)

// syscall has TCGETS and TCSETS but not TCFLSH; 0x540b on every Linux port
// the boards use. TCIFLUSH drops received bytes.
const (
	tcflsh   = 0x540b
	tciflush = 0
)

type tty struct{ *os.File }

func openPort(path string) (port, error) {
	f, err := os.OpenFile(path, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return nil, err
	}
	t := tty{f}
	if err := t.raw(); err != nil {
		f.Close()
		return nil, err
	}
	return t, nil
}

// raw sets 115200 8N1 with no echo or line editing. Over USB CDC the rate is
// only passed along to the CH554.
func (t tty) raw() error {
	tio := syscall.Termios{
		Cflag:  syscall.CREAD | syscall.CLOCAL | syscall.CS8 | syscall.B115200,
		Ispeed: syscall.B115200,
		Ospeed: syscall.B115200,
	}
	tio.Cc[syscall.VMIN] = 0
	tio.Cc[syscall.VTIME] = 1
	err := t.control(func(fd uintptr) syscall.Errno {
		_, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, syscall.TCSETS,
			uintptr(unsafe.Pointer(&tio)))
		return e
	})
	if err != nil {
		return fmt.Errorf("setting the line: %w", err)
	}
	return nil
}

func (t tty) flush() error {
	err := t.control(func(fd uintptr) syscall.Errno {
		_, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, tcflsh, tciflush)
		return e
	})
	if err != nil {
		return fmt.Errorf("chargebase: dropping unread input: %w", err)
	}
	return nil
}

// A tty is pollable, so the deadline holds; VTIME bounds each read if not.
func (t tty) until(deadline time.Time) error {
	if err := t.SetReadDeadline(deadline); err != nil && err != os.ErrNoDeadline {
		return err
	}
	return nil
}

func (t tty) control(call func(fd uintptr) syscall.Errno) error {
	sc, err := t.SyscallConn()
	if err != nil {
		return err
	}
	var errno syscall.Errno
	if err := sc.Control(func(fd uintptr) { errno = call(fd) }); err != nil {
		return err
	}
	if errno != 0 {
		return errno
	}
	return nil
}
