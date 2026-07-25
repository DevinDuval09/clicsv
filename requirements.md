# clicsv Requirements

## Overview
An interactive command-line tool for reading and editing CSV files. The
program runs as a terminal UI (TUI) that renders the CSV as a grid, lets the
user navigate and edit cells, and lets the user write changes back to disk.

## Constraints
- Implemented using only packages from the Go standard library. No
  third-party dependencies (including terminal/TUI libraries) are permitted.

## Display
- The CSV header row (first row of the file) is locked at the top of the
  display and remains visible at all times, even when the user scrolls
  through the rest of the data.
- Line numbers are displayed along the left side of the screen, one per data
  row.
- A single input line is shown at the bottom of the screen. It is used both
  for entering commands and for displaying status/warning messages to the
  user.

## Navigation (normal mode)
- The arrow keys move a highlighted cursor between cells:
  - Up/Down moves between rows.
  - Left/Right moves between columns.
- Exactly one cell is highlighted at a time.

## Editing a cell
- Pressing Enter while a cell is highlighted enters edit mode for that cell.
- While in edit mode:
  - Typing a character appends it to the end of the cell's current contents.
  - Backspace or Delete removes characters from the cell's contents.
  - Escape exits edit mode and returns to normal (navigation) mode.

## Commands (normal mode)
While not in edit mode, the bottom input line accepts the following typed
commands:
- `write` — overwrites the existing CSV file on disk with the current state
  of the data.
- `revert` — reverts the most recently edited cell back to its
  previous (pre-edit) value.
- `quit` — exits the program.

## Warnings
- If there are unsaved edits (i.e. edits have been made since the last
  `write`), a warning message is displayed in the input line to alert the
  user that changes have not been saved.
