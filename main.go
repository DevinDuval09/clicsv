package main

import (
	"bufio"
	"fmt"
	"os"
)

const (
	ctrlC      byte = 0x03
	enterCR    byte = '\r'
	enterLF    byte = '\n'
	backspace  byte = 0x7f
	backspace2 byte = 0x08
	escape     byte = 0x1b
)

func main() {
	fd := int(os.Stdin.Fd())

	orig, err := enableRawMode(fd)
	if err != nil {
		fmt.Fprintln(os.Stderr, "enable raw mode:", err)
		os.Exit(1)
	}
	defer func() {
		restoreMode(fd, orig)
		if r := recover(); r != nil {
			fmt.Fprintln(os.Stderr, "panic:", r)
			os.Exit(1)
		}
	}()

	rows, cols, err := getWindowSize(fd)
	if err != nil {
		rows, cols = 24, 80
	}

	out := bufio.NewWriter(os.Stdout)
	in := bufio.NewReader(os.Stdin)

	var echoText, inputText string
	draw(out, rows, cols, echoText, inputText)

	for {
		b, err := in.ReadByte()
		if err != nil {
			break
		}

		switch {
		case b == ctrlC:
			quit(out)
			return
		case b == enterCR || b == enterLF:
			if inputText == "quit" {
				quit(out)
				return
			}
			echoText = inputText
			inputText = ""
		case b == backspace || b == backspace2:
			if len(inputText) > 0 {
				inputText = inputText[:len(inputText)-1]
			}
		case b == escape:
			// Not handled in this proof of concept; ignored.
		case b >= 0x20 && b < 0x7f:
			inputText += string(rune(b))
		}

		draw(out, rows, cols, echoText, inputText)
	}
}

// draw renders the "echo" line in the middle of the screen and the "input"
// line at the bottom in a single write to minimize flicker.
func draw(w *bufio.Writer, rows, cols int, echoText, inputText string) {
	echoLine := truncate(fmt.Sprintf("echo: %s", echoText), cols)
	inputLine := truncate(fmt.Sprintf("input: %s", inputText), cols)
	mid := rows / 2

	w.WriteString("\x1b[?25l") // hide cursor
	w.WriteString("\x1b[2J")   // clear screen
	fmt.Fprintf(w, "\x1b[%d;1H%s", mid, echoLine)
	fmt.Fprintf(w, "\x1b[%d;1H%s", rows, inputLine)
	fmt.Fprintf(w, "\x1b[%d;%dH", rows, len(inputLine)+1) // cursor at end of input
	w.WriteString("\x1b[?25h")                            // show cursor
	w.Flush()
}

// quit clears the screen, restores the cursor, and flushes before exit.
func quit(w *bufio.Writer) {
	w.WriteString("\x1b[2J\x1b[H\x1b[?25h")
	w.Flush()
}

func truncate(s string, width int) string {
	if width <= 0 || len(s) <= width {
		return s
	}
	return s[:width]
}
