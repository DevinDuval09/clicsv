//go:build linux

package main

import "golang.org/x/sys/unix"

// enableRawMode puts the terminal on fd into raw mode and returns the
// original termios so it can be restored later.
func enableRawMode(fd int) (*unix.Termios, error) {
	// TCGETS reads the terminal's current settings so they can be restored later.
	orig, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		return nil, err
	}

	raw := *orig
	// Input flags: disable software flow control (IXON), CR-to-NL translation
	// (ICRNL), break-to-interrupt (BRKINT), parity checking (INPCK), and
	// stripping the 8th bit off input bytes (ISTRIP).
	raw.Iflag &^= unix.IXON | unix.ICRNL | unix.BRKINT | unix.INPCK | unix.ISTRIP
	// Output flags: disable output post-processing (e.g. NL-to-CRNL) so bytes
	// are written to the terminal exactly as given.
	raw.Oflag &^= unix.OPOST
	// Local flags: disable echoing of input (ECHO), canonical/line-buffered
	// input (ICANON), generation of signals like SIGINT/SIGTSTP from control
	// characters (ISIG), and extended/implementation-defined input processing
	// (IEXTEN).
	raw.Lflag &^= unix.ECHO | unix.ICANON | unix.ISIG | unix.IEXTEN
	// VMIN/VTIME control read() blocking in non-canonical mode: VMIN=1 means
	// read() returns after at least 1 byte is available; VTIME=0 disables the
	// inter-byte timeout.
	raw.Cc[unix.VMIN] = 1
	raw.Cc[unix.VTIME] = 0

	// TCSETS applies the new termios settings immediately.
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
	// TIOCGWINSZ queries the kernel for the terminal's current row/column count.
	ws, err := unix.IoctlGetWinsize(fd, unix.TIOCGWINSZ)
	if err != nil {
		return 0, 0, err
	}
	return int(ws.Row), int(ws.Col), nil
}
