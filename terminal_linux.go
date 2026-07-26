//go:build linux

package main

import "golang.org/x/sys/unix"

// enableRawMode puts the terminal on fd into raw mode and returns the
// original termios so it can be restored later.
func enableRawMode(fd int) (*unix.Termios, error) {
	orig, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		return nil, err
	}

	raw := *orig
	raw.Iflag &^= unix.IXON | unix.ICRNL | unix.BRKINT | unix.INPCK | unix.ISTRIP
	raw.Oflag &^= unix.OPOST
	raw.Lflag &^= unix.ECHO | unix.ICANON | unix.ISIG | unix.IEXTEN
	raw.Cc[unix.VMIN] = 1
	raw.Cc[unix.VTIME] = 0

	if err := unix.IoctlSetTermios(fd, unix.TCSETS, &raw); err != nil {
		return nil, err
	}
	return orig, nil
}

// restoreMode restores a termios previously returned by enableRawMode.
func restoreMode(fd int, orig *unix.Termios) error {
	return unix.IoctlSetTermios(fd, unix.TCSETS, orig)
}

// getWindowSize returns the terminal's current size in rows and columns.
func getWindowSize(fd int) (rows, cols int, err error) {
	ws, err := unix.IoctlGetWinsize(fd, unix.TIOCGWINSZ)
	if err != nil {
		return 0, 0, err
	}
	return int(ws.Row), int(ws.Col), nil
}
