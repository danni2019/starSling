package main

import (
	"context"
	"os"
	"time"

	"github.com/charmbracelet/x/term"
	"golang.org/x/sys/unix"
)

type keyEvent int

const (
	keyEsc keyEvent = iota + 1
	keyCtrlC
)

func watchKeys(ctx context.Context) <-chan keyEvent {
	ch := make(chan keyEvent, 1)
	go func() {
		defer close(ch)
		fd := int(os.Stdin.Fd())
		if !term.IsTerminal(uintptr(fd)) {
			return
		}
		oldState, err := term.MakeRaw(uintptr(fd))
		if err != nil {
			return
		}
		defer term.Restore(uintptr(fd), oldState)

		oldFlags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFL, 0)
		if err != nil {
			return
		}
		if _, err := unix.FcntlInt(uintptr(fd), unix.F_SETFL, oldFlags|unix.O_NONBLOCK); err != nil {
			return
		}
		defer func() {
			_, _ = unix.FcntlInt(uintptr(fd), unix.F_SETFL, oldFlags)
		}()

		buf := []byte{0}
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			n, err := unix.Read(fd, buf)
			if err != nil {
				if err == unix.EAGAIN || err == unix.EWOULDBLOCK {
					time.Sleep(50 * time.Millisecond)
					continue
				}
				return
			}
			if n == 0 {
				time.Sleep(50 * time.Millisecond)
				continue
			}

			switch buf[0] {
			case 0x1b:
				next, err := readEscapeFollow(fd)
				if err != nil {
					ch <- keyEsc
					return
				}
				if next == 0 {
					ch <- keyEsc
					return
				}
				continue
			case 0x03:
				ch <- keyCtrlC
				return
			}
		}
	}()
	return ch
}

func readEscapeFollow(fd int) (byte, error) {
	deadline := time.Now().Add(80 * time.Millisecond)
	buf := []byte{0}
	for time.Now().Before(deadline) {
		n, err := unix.Read(fd, buf)
		if err != nil {
			if err == unix.EAGAIN || err == unix.EWOULDBLOCK {
				time.Sleep(5 * time.Millisecond)
				continue
			}
			return 0, err
		}
		if n == 0 {
			time.Sleep(5 * time.Millisecond)
			continue
		}
		return buf[0], nil
	}
	return 0, nil
}
