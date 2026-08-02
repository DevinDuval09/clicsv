package main

import (
	"bufio"
	"container/list"
	"fmt"
	"log"
	"os"
)

const (
	ctrlC      byte = 0x03 // ETX, sent when the user presses Ctrl+C in raw mode
	enterCR    byte = '\r' // 0x0d, carriage return (most terminals send this for Enter)
	enterLF    byte = '\n' // 0x0a, line feed (sent for Enter by some terminals/pipes)
	backspace  byte = 0x7f // DEL, what most terminals send for the Backspace key
	backspace2 byte = 0x08 // BS, backspace byte sent by some terminals/emulators instead of DEL
	escape     byte = 0x1b // ESC, prefix byte for arrow keys and other escape sequences
	uparrow    byte = 0x41
	downarrow  byte = 0x42
	rightarrow byte = 0x43
	leftarrow  byte = 0x44
)

type Location struct {
	x int
	y int
}

/**type Dimensions struct {
	rows    int
	columns int
}**/

func absInt(val int) int {
	if val < 0 {
		return -val
	}
	return val
}

func main() {

	//args
	fpath := os.Args[1]    //position 0 is program
	data := readcsv(fpath) //load a list of rows
	location := Location{x: 1, y: 0}
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

	win_rows, win_cols, err := getWindowSize(fd)
	if err != nil {
		win_rows, win_cols = 24, 80
	}

	row_count := win_rows
	if data.Len() < row_count {
		row_count = data.Len()
	}

	col_count := win_cols
	var f_elem *list.Element
	f_elem = data.Front()

	// if there's a first element, use number of columns
	if f_elem != nil {
		f_row, ok := f_elem.Value.(*list.List)
		if ok {
			if f_row.Len() < col_count {
				col_count = f_row.Len()
			}
		}
	}

	out := bufio.NewWriter(os.Stdout)
	in := bufio.NewReader(os.Stdin)

	var inputText string
	draw(out, data, location, inputText, row_count, col_count)

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
			// Should be arrow key
			_, err := in.ReadByte() //throw away
			if err != nil {
				log.Fatalf("Error reading after escape byte: %s", err)
			}
			/**if next != 91 {
				log.Fatalf("Expected 91 to throw away on arrow key, got %d", next)
			}**/
			arrow, err := in.ReadByte() //should indicate arrow key
			if err != nil {
				log.Fatalf("Error reading after escape byte: %s", err)
			}
			switch arrow {
			case uparrow:
				location.x--
			case downarrow:
				location.x++
			case rightarrow:
				location.y++
			case leftarrow:
				location.y--
			}
		case b >= 0x20 && b < 0x7f: // printable ASCII range (space through '~')
			inputText += string(rune(b))
		}
		//make sure location is in screen
		//TODO: Fix this. Location x still hitting empty space
		location.x = (location.x + row_count + 1) % (row_count + 1)
		location.y = (location.y + col_count) % (col_count)

		draw(out, data, location, inputText, row_count, col_count)
	}
}

// draw renders the "echo" line in the middle of the screen and the "input"
// line at the bottom in a single write to minimize flicker.
func draw(w *bufio.Writer, data list.List, location Location, inputText string, row_count int, col_count int) {
	inputLine := truncate(fmt.Sprintf("input: %s", inputText), data.Len())

	w.WriteString("\x1b[?25l") // hide cursor
	w.WriteString("\x1b[2J")   // clear screen

	r := 1
	for row := data.Front(); row != nil; row = row.Next() {
		c := 0
		col_list := row.Value.(*list.List)
		c_loc := 0
		for column := col_list.Front(); column != nil; column = column.Next() {
			format := "%s"
			if r == location.x && c == location.y {
				format = "{%s}"
			}
			c_loc = c * 20
			fmt.Fprintf(w, "\x1b[%d;%dH", r, c_loc) // move cursor?
			fmt.Fprintf(w, format, column.Value)
			c += 1
		}
		r += 1
	}

	fmt.Fprintf(w, "\x1b[%d;1H%s", row_count+2, inputLine)       // move cursor to (rows, 1) and print input line
	fmt.Fprintf(w, "\x1b[%d;%dH", row_count+2, len(inputLine)+1) // move cursor end of input
	w.WriteString("\x1b[?25h")                                   // show cursor
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
