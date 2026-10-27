# tbdbub

A keyboard-first kanban TUI where **the board is a plantext Markdown file**.

`tbdbub` is a structured editor over that file, not a viewer on top of hidden state. Boards
stay readable, diffable, greppable, and git-friendly: no database, no sidecar files, no sync
service.

```markdown
---
tbdbub: 1
title: Home renovation
capture_column: Backlog
done_column: Done
---

## Backlog

- [ ] Replace faucet #plumbing #urgent @Kitchen ^t7k2
  Cartridge is 35mm; bought already.
- [ ] Choose tiles #design @Bathroom ^t9x1

## Doing

- [ ] Paint hallway #paint @Kitchen ^t3ab

## Done

- [x] Measure cabinets #design @Kitchen ^t1zz
```

## Features

- Arbitrary columns, defined by `##` headings; the last column is Done.
- Tasks with tags, one milestone, and free-form notes.
- `H`/`L` quick moves plus grab mode for exact column and slot placement.
- Filtering by text, tag (any/all), milestone (or no milestone), and Done visibility.
- In-file archive grouped by month, with restore and reopen.
- Undo/redo, debounced autosave, atomic writes, and external-edit conflict detection.
- Single-column carousel on narrow terminals.
- Optional CLI: `tbdbub add` and `tbdbub ls [--json]`.

## Install

```sh
go install github.com/kron4eg/tbdbub@latest
```

Or build from a checkout:

```sh
git clone https://github.com/kron4eg/tbdbub
cd tbdbub
go build -o /tmp/tbdbub .
```

## Usage

```sh
tbdbub [board.md]        # open or create a board in the TUI
tbdbub add "Task title"  # add a task to the capture column and exit
tbdbub ls [--json]       # list tasks and exit
```

The board path is resolved in this order: the first argument, `$TBDBUB_BOARD`, then
`./board.md`. A missing file is created when the TUI or `add` opens it, with one `Backlog`
column and the title taken from the filename. `add` and `ls` accept `-b`/`--board <path>`;
`ls` also accepts `--json` and fails if the file does not exist.

### Keys

Board:

| Key | Action |
| --- | --- |
| `h` / `l`, arrows | Focus previous/next column |
| `j` / `k`, arrows | Focus next/previous task |
| `g` / `G` | First/last task |
| `H` / `L` | Move task left/right, appending to the target column (`2L` works) |
| `J` / `K` | Reorder task down/up within its column |
| `m` | Grab mode |
| `space` | Toggle done |
| `n` / `e` / `enter` | New / edit task |
| `t` | Edit tags |
| `M` | Milestone picker |
| `d` | Delete task (confirm) |
| `N` / `r` / `D` | New / rename / delete column |
| `a` / `A` | Archive selected Done task / archive all Done tasks |
| `V` | Archive view |
| `f` or `/` | Filter panel / quick search |
| `esc` | Clear filters |
| `u` / `ctrl+r` | Undo / redo |
| `s` | Force save |
| `?` | Help |
| `q` | Quit |

Grab mode: `h`/`l` choose the column, `j`/`k` the insertion slot, `1`–`9` jump to a column,
`enter` drops, `esc` or `q` cancels without writing.

Task editor: `tab`/`shift+tab` move between title, notes, tags, and milestone; `ctrl+s` saves,
`esc` cancels.

Milestones: `enter` assigns, `c` clears, `n`/`e`/`d` create/rename/delete, `u` edits the due
date, `i` the description.

Archive view: `j`/`k` navigate, `/` search, `u` restore to Done, `U` reopen in the capture
column, `esc` closes.

## How it behaves

- **Canonical output.** The file is parsed tolerantly and written in one deterministic style,
  like `gofmt`. Tag order is preserved to keep diffs quiet.
- **Done.** Completing a task moves it to Done; the first completion appends a `Done` column
  and writes `done_column`. Un-done moves the task back to the capture column.
- **Movement.** Every gesture goes through the same domain operation, so keyboard and grab
  mode cannot diverge. A no-op move creates neither an undo entry nor a write.
- **Saving.** Autosave runs about 750 ms after the last change. Writes go to a temp file in the
  same directory, `fsync`, then `rename`, preserving the file mode. External edits are detected
  by hash and offered as reload / overwrite / cancel — never silently clobbered.
- **Preservation.** Unknown frontmatter keys, preambles, notes, and unrecognized archive
  content are kept.

The full design, format rules, and phase history live in [`docs/PLAN.md`](docs/PLAN.md).

## Development

```sh
just check        # gofmt check + go vet + tests
just test         # go test ./...
just test-race    # go test -race ./...
just build        # build to $BUILD_DIR/tbdbub (default /tmp)
just cover        # coverage summary
```

Without `just`:

```sh
go test ./...
go vet ./...
gofmt -w .
go build -o /tmp/tbdbub .
```

Fuzzing the parser (single worker in this environment):

```sh
go test ./internal/board -run=^$ -fuzz=FuzzParse -fuzztime=10s -parallel=1
```

### Layout

```text
main.go                 CLI entry point: path resolution, TUI startup, add/ls
internal/board/         pure value model: parse, canonical write, IDs, operations, filters
internal/tui/           root model, modes, board view, overlays, grab, save pipeline
docs/PLAN.md            product behavior, format rules, keymap, implementation log
```

`internal/board` imports no Bubble Tea or Lip Gloss; the TUI holds a `board.Board` value and
every mutation returns a new one.
