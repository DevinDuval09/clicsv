package main

import (
	"bufio"
	"container/list"
	"fmt"
	"os"
)

const (
	ctrlC      byte = 0x03 // ETX, sent when the user presses Ctrl+C in raw mode
	enterCR    byte = '\r' // 0x0d, carriage return (most terminals send this for Enter)
	enterLF    byte = '\n' // 0x0a, line feed (sent for Enter by some terminals/pipes)
	backspace  byte = 0x7f // DEL, what most terminals send for the Backspace key
	backspace2 byte = 0x08 // BS, backspace byte sent by some terminals/emulators instead of DEL
	escape     byte = 0x1b // ESC, prefix byte for arrow keys and other escape sequences
)

type Location struct {
	x int
	y int
}

/**type Dimensions struct {
	rows    int
	columns int
}**/

func main() {

	//args
	fpath := os.Args[1]    //position 0 is program
	data := readcsv(fpath) //load a list of rows
	location := Location{x: 0, y: 0}
	//dims := Dimensions{rows: rows.Len(), columns: rows[0].Len()}

	fd := int(os.Stdin.Fd()) //capture stdin

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

	/**rows, cols, err := getWindowSize(fd)
	if err != nil {
		rows, cols = 24, 80
	}**/

	out := bufio.NewWriter(os.Stdout)
	in := bufio.NewReader(os.Stdin)

	var inputText string
	draw(out, data, location, inputText)

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
			inputText = ""
		case b == backspace || b == backspace2:
			if len(inputText) > 0 {
				inputText = inputText[:len(inputText)-1]
			}
		case b == escape:
			// Not handled in this proof of concept; ignored.
		case b >= 0x20 && b < 0x7f: // printable ASCII range (space through '~')
			inputText += string(rune(b))
		}

		draw(out, data, location, inputText)
	}
}

// draw renders the "echo" line in the middle of the screen and the "input"
// line at the bottom in a single write to minimize flicker.
func draw(w *bufio.Writer, data list.List, location Location, inputText string) {
	inputLine := truncate(fmt.Sprintf("input: %s", inputText), data.Len())

	w.WriteString("\x1b[?25l") // hide cursor
	w.WriteString("\x1b[2J")   // clear screen

	r := 0
	for row := data.Front(); row != nil; row = row.Next() {
		location.x = r
		r += 1
		c := 0
		col_list := row.Value.(*list.List)
		for column := col_list.Front(); column != nil; column = column.Next() {
			location.y = c * 20
			fmt.Fprintf(w, "\x1b[%d;%dH", r, location.y) // move cursor?
			fmt.Fprintf(w, "%s", column.Value)
			c += 1
		}
	}

	fmt.Fprintf(w, "\x1b[%d;1H%s", data.Len()+1, inputLine)       // move cursor to (rows, 1) and print input line
	fmt.Fprintf(w, "\x1b[%d;%dH", data.Len()+1, len(inputLine)+1) // cursor at end of input
	w.WriteString("\x1b[?25h")                                    // show cursor
	w.Flush()
}

// quit clears the screen, restores the cursor, and flushes before exit.
func quit(w *bufio.Writer) {
	w.WriteString("\x1b[2J\x1b[H\x1b[?25h") // clear screen, move cursor to (1,1), show cursor
	w.Flush()
}

func truncate(s string, width int) string {
	if width <= 0 || len(s) <= width {
		return s
	}
	return s[:width]
}
