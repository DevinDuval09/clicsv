# TUI Implementation Plan

This document outlines plans for implementing the terminal UI on Linux and
on Windows using only the Go standard library. Both platforms share the
same OS-agnostic core (`editor.go`, `render.go`, `csvmodel.go`) and the same
mode/event-loop design described in `requirements.md`; only raw-mode/input
handling differ per platform, isolated behind Go build tags.

## Part 1: Linux Implementation

### 1. Raw terminal mode via termios

Linux terminals normally operate in "cooked" mode: input is line-buffered,
echoed to the screen, and special keys (Ctrl-C, backspace, etc.) are handled
by the kernel's line discipline before the program ever sees them. To read
individual keystrokes (including arrow keys) as they happen, the program
must put the terminal into raw mode.

#TODO: replace syscall package with x sys unix

- Use the `syscall` package to issue `ioctl` calls against the file
  descriptor for `/dev/tty` (or `os.Stdin.Fd()`), using `syscall.Syscall`
  with `SYS_IOCTL`.
- Read the current terminal settings with `TCGETS` into a `termios` struct
  (fields: `Iflag`, `Oflag`, `Cflag`, `Lflag`, `Cc [20]byte`, etc. — matches
  `unix.Termios` layout, but defined locally so no external package is
  needed).
- Modify flags to enter raw mode:
  - Clear `ECHO`, `ICANON`, `ISIG`, `IEXTEN` in `Lflag`.
  - Clear `IXON`, `ICRNL`, `BRKINT`, `INPCK`, `ISTRIP` in `Iflag`.
  - Clear `OPOST` in `Oflag`.
  - Set `Cc[VMIN] = 1`, `Cc[VTIME] = 0` so reads block until at least one
    byte is available.
- Apply the modified struct with `TCSETS`.
- Save the original `termios` value at startup and restore it with `TCSETS`
  on exit (including on panics — use `defer` plus a `recover` at the top of
  `main`) so the user's shell isn't left in a broken state.

Component: `terminal_linux.go`
- `enableRawMode() (*termios, error)` — returns the original state.
- `restoreMode(orig *termios) error`.
- `getWindowSize() (rows, cols int, err error)` — via `TIOCGWINSZ` ioctl, so
  the display can size itself to the terminal and react to resizes.

### 2. Reading input

With raw mode enabled, input arrives as an unbuffered byte stream on stdin.

- Read one byte at a time with `os.Stdin.Read`.
- Plain characters (printable ASCII) map directly to "typed character" edit
  events.
- Special keys arrive as multi-byte escape sequences starting with `0x1b`
  (ESC):
  - Arrow keys: `ESC [ A` (up), `ESC [ B` (down), `ESC [ C` (right),
    `ESC [ D` (left).
  - A lone `0x1b` byte not followed by `[` within a short read (or followed
    by an unrecognized byte) is treated as the Escape key itself (exits edit
    mode).
- Enter is `\r` (0x0D). Backspace is typically `0x7f` (DEL) on Linux
  terminals; Delete may arrive as `ESC [ 3 ~`. Both should be handled and
  treated as "remove character" in edit mode.
- Because ESC is also a prefix byte, disambiguating a standalone Escape key
  press from the start of an arrow-key sequence requires a short non-blocking
  read-ahead: after seeing `0x1b`, attempt to read the next byte with a small
  timeout (`Cc[VTIME]` can be temporarily set, or a read with a timeout via
  `syscall.Select`/`poll` on the fd). If no further byte arrives promptly,
  treat it as a standalone Escape.

Component: `input_linux.go`
- `readKey() (Key, error)` — returns a decoded `Key` enum
  (`KeyUp`, `KeyDown`, `KeyLeft`, `KeyRight`, `KeyEnter`, `KeyEscape`,
  `KeyBackspace`, `KeyDelete`, `KeyRune(r rune)`, or a command-line
  variant when not in a raw navigation context — see section 4).

### 3. Rendering

All rendering is done with ANSI/VT100 escape sequences written directly to
`os.Stdout`; no external rendering library is used.

- Clear screen: `ESC [ 2 J`.
- Move cursor to row/col: `ESC [ {row} ; {col} H`.
- Hide/show cursor during redraws to avoid flicker: `ESC [ ? 25 l` /
  `ESC [ ? 25 h`.
- Reverse video (cell highlight): `ESC [ 7 m` ... `ESC [ 0 m`.
- Each frame is built into an in-memory `bytes.Buffer` and written to stdout
  in a single `Write` call, rather than issuing many small writes, to
  minimize flicker.

Layout, top to bottom:
1. **Header row** (row 1, locked): rendered from the CSV header record.
   Column widths are computed from content (see section 5) and header text
   is truncated/padded to fit.
2. **Data rows** (rows 2..N-2, where N is terminal height): each row is
   prefixed with a right-aligned line number gutter (width based on total
   row count, e.g. 4-6 chars) followed by cell contents. The highlighted
   cell is drawn with reverse video; if the cell is in edit mode, an
   additional style (e.g. underline, `ESC [ 4 m`) or a visible cursor
   position is shown at the edit caret.
3. **Input/status line** (last row): either the raw text the user is typing
   as a command, or the most recent status/warning message (e.g. "unsaved
   changes" — see requirements.md).

Component: `render.go`
- `draw(state *EditorState)` — single entry point called after every state
  change; recomputes visible row/column window based on cursor position and
  terminal size (scrolling), then writes one full frame.

### 4. Modes and the event loop

Three modes, matching `requirements.md`:
- **Navigate mode**: arrow keys move the highlighted cell; Enter switches to
  Edit mode; any other printable key (specifically when the input line is
  "focused" — see below) begins Command mode.
- **Edit mode**: printable runes append to the highlighted cell's buffer;
  Backspace/Delete remove from it; Escape returns to Navigate mode.
- **Command mode**: the bottom input line accepts a line of text; Enter
  submits it as a command (`write`, `revert`, `quit`); unrecognized
  commands produce a status message rather than an error/crash.

Since requirements.md specifies navigation via arrow keys and commands via
the input line while "not in edit mode," Command mode is entered whenever
the user starts typing on the input line during Navigate mode (e.g. by
pressing a designated key or simply typing non-arrow printable characters
while no cell is being edited). This should be confirmed against
requirements.md and made explicit before implementation — see Open
Questions.

Component: `editor.go`
- `type Mode int` with `ModeNavigate`, `ModeEdit`, `ModeCommand`.
- `type EditorState struct` holding: parsed CSV records, cursor
  row/col, current mode, in-progress edit buffer, in-progress command
  buffer, last-edited-cell snapshot (for `revert`), dirty flag (for the
  unsaved-changes warning), and status message.
- `run(state *EditorState)` — main loop: `readKey()`, dispatch to the
  handler for the current mode, mutate state, call `draw(state)`, repeat
  until a `quit` command exits the loop.

### 5. CSV data model

- Parse the file at startup with `encoding/csv` into `[][]string`
  (`records[0]` is the header).
- Column widths for rendering are computed once at load (max rune-width of
  header vs. all data cells in that column, capped at a max width with
  truncation for very long values) and recomputed after edits that change a
  cell's length materially, or simply recomputed every frame if performance
  allows (dataset sizes are assumed small enough for this to be cheap).
- Edits mutate `records` in place; `revert` restores the previous string
  value from the last-edit snapshot; `write` uses `encoding/csv.Writer` to
  overwrite the original file path (write to a temp file in the same
  directory, then `os.Rename` over the original, so a crash mid-write can't
  corrupt the file).

Component: `csvmodel.go`
- `loadCSV(path string) (*CSVData, error)`
- `(*CSVData) columnWidths() []int`
- `(*CSVData) writeTo(path string) error` (temp file + rename)

### 6. Program structure summary (Linux)

```
clicsv/
  main.go            entry point: parse args, load CSV, enable raw mode,
                      defer restore, run(), handle panics
  editor.go           EditorState, Mode, run() event loop
  terminal_linux.go   raw mode enable/restore, window size (build-tagged
                      for eventual multi-OS support)
  input_linux.go      readKey() and escape-sequence decoding
  render.go           draw(), layout/scrolling logic (OS-agnostic)
  csvmodel.go         CSV load/write, column width computation
```

`terminal_linux.go` and `input_linux.go` are named/tagged for Linux
specifically (`//go:build linux`) so that the Windows implementation (Part 2)
can be added alongside without touching the OS-agnostic files (`editor.go`,
`render.go`, `csvmodel.go`).

## Part 2: Windows Implementation

### 1. Raw terminal mode via the Windows Console API

Windows terminals (conhost and Windows Terminal) default to "cooked" console
mode: input is line-buffered and echoed, similar in effect to Linux's
cooked termios mode. There is no `termios`/`ioctl` on Windows; equivalent
control is exposed through the Win32 Console API, callable from the
standard library's `syscall` package (no `golang.org/x/sys` dependency)
via `syscall.NewLazyDLL("kernel32.dll")` and `NewProc(...)`.

- Obtain handles with `GetStdHandle(STD_INPUT_HANDLE)` and
  `GetStdHandle(STD_OUTPUT_HANDLE)`.
- Read current modes with `GetConsoleMode` for both handles; save them for
  restoration on exit.
- On the input handle, clear `ENABLE_LINE_INPUT` and `ENABLE_ECHO_INPUT`
  (so keystrokes arrive immediately, unbuffered and unechoed), and clear
  `ENABLE_PROCESSED_INPUT` (so Ctrl-C etc. arrive as regular input rather
  than being intercepted). Set `ENABLE_VIRTUAL_TERMINAL_INPUT` so that
  arrow keys and other special keys are delivered on stdin as the same
  ANSI escape sequences used on Linux (`ESC [ A`, etc.), rather than as
  Win32 `KEY_EVENT_RECORD` structures. This is what allows `input_linux.go`'s
  escape-sequence decoding logic to be reused almost as-is.
- On the output handle, set `ENABLE_VIRTUAL_TERMINAL_PROCESSING` so the
  console interprets the same ANSI/VT100 escape sequences `render.go`
  already emits (cursor positioning, clear screen, reverse video, etc.).
  Both flags are supported on Windows 10 (build 1511, Nov. 2015) and later,
  which is assumed to be the minimum supported target.
- Apply the modified modes with `SetConsoleMode` on each handle.
- Restore the original modes with `SetConsoleMode` on exit (`defer` plus a
  top-level `recover`, mirroring the Linux plan) so the user's shell/console
  isn't left in a broken state.

Component: `terminal_windows.go`
- `enableRawMode() (origIn, origOut uint32, err error)` — returns the
  original console modes for both handles.
- `restoreMode(origIn, origOut uint32) error`.
- `getWindowSize() (rows, cols int, err error)` — via
  `GetConsoleScreenBufferInfo`, reading the `srWindow` rectangle from the
  returned `CONSOLE_SCREEN_BUFFER_INFO` struct (Windows has no `SIGWINCH`;
  see Open Questions for how resize is detected).

### 2. Reading input

With `ENABLE_VIRTUAL_TERMINAL_INPUT` set, stdin behaves like the Linux case:
an unbuffered byte stream where arrow keys, Enter, Escape, Backspace, and
Delete arrive as the same bytes/escape sequences documented in Part 1,
Section 2.

- Read one byte at a time with `os.Stdin.Read`, exactly as on Linux.
- The same escape-sequence decoding (`ESC [ A/B/C/D` for arrows, lone `ESC`
  for Escape with a short read-ahead to disambiguate, `ESC [ 3 ~` for
  Delete) applies unchanged.
- One platform difference: Backspace on Windows typically arrives as
  `0x08` (BS) rather than Linux's `0x7f` (DEL). The key decoder should
  accept both byte values as "Backspace" across platforms.
- Enter may arrive as `\r`, `\n`, or `\r\n` depending on console/terminal;
  treat any of these as the Enter key.

Component: `input_windows.go`
- `readKey() (Key, error)` — same `Key` enum as `input_linux.go`
  (`KeyUp`, `KeyDown`, `KeyLeft`, `KeyRight`, `KeyEnter`, `KeyEscape`,
  `KeyBackspace`, `KeyDelete`, `KeyRune(r rune)`), so `editor.go` does not
  need to know which platform it's running on.
- Given the VT-input-mode approach above, the byte-level parsing logic in
  this file is expected to be nearly identical to `input_linux.go`; a
  shared helper (e.g. `decodeEscapeSequence` in an OS-agnostic file) could
  be factored out once both implementations exist, to avoid duplicating
  the parser.

### 3. Rendering

Unchanged from Part 1: with `ENABLE_VIRTUAL_TERMINAL_PROCESSING` enabled on
the output handle, the same ANSI escape sequences (`ESC [ 2 J`,
`ESC [ {row} ; {col} H`, `ESC [ ? 25 l/h`, `ESC [ 7 m`, etc.) written to
`os.Stdout` by `render.go` work identically on Windows. No Windows-specific
rendering code is required; `render.go` is fully shared between platforms.

### 4. Modes and the event loop

Unchanged from Part 1 — `editor.go` is OS-agnostic and depends only on the
shared `Key` enum produced by `readKey()`, so Navigate/Edit/Command mode
handling, `EditorState`, and `run()` are identical on Windows.

### 5. CSV data model

Unchanged from Part 1 — `csvmodel.go` uses only `encoding/csv`, `os`, and
`os.Rename` for the write-temp-then-rename strategy, all of which behave
the same on Windows (the temp file should still be created in the same
directory as the target file so the rename is atomic on the same volume).

### 6. Program structure summary (Windows)

```
clicsv/
  main.go              entry point (shared, may need small OS-specific
                       branching only if handle setup differs)
  editor.go             EditorState, Mode, run() event loop (shared)
  terminal_windows.go   raw mode enable/restore via Win32 Console API,
                       window size, build-tagged `//go:build windows`
  input_windows.go      readKey(), same escape-sequence decoding as Linux
  render.go             draw(), layout/scrolling logic (shared, OS-agnostic)
  csvmodel.go            CSV load/write, column width computation (shared)
```

`terminal_windows.go` and `input_windows.go` sit alongside their Linux
counterparts with matching function signatures, so the Go compiler
build-tag mechanism (`//go:build windows` / `//go:build linux`) selects the
correct file per platform without any conditional logic elsewhere in the
codebase.

## Open questions / follow-ups

- Exact key used to enter Command mode from Navigate mode (a dedicated key
  like `:` vs. any non-arrow keystroke) should be pinned down and reflected
  back into `requirements.md`.
- Terminal/console resize handling differs by platform: Linux can catch
  `SIGWINCH` via `os.Signal`; Windows has no equivalent signal, so window
  size would need to be re-checked via `GetConsoleScreenBufferInfo` on
  every input read (or on a timer) rather than via an interrupt.
- Whether `ENABLE_VIRTUAL_TERMINAL_INPUT`/`ENABLE_VIRTUAL_TERMINAL_PROCESSING`
  can be assumed present, or whether a fallback to native Win32 console
  APIs (`ReadConsoleInputW`, `WriteConsoleOutputW`) is needed for older
  Windows versions or unusual terminal hosts, should be decided before
  implementation.
- Behavior for CSV rows with a differing number of columns than the header
  (malformed input) is not yet specified.
