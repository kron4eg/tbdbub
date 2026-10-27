# tbdbub — Implementation Plan

A keyboard-first kanban TUI built on Bubble Tea v2, where **the board is a Markdown file**.
One board = one file. The TUI is a structured editor over that file, not a viewer on top of
hidden state.

Last updated: 2026-10-27

**Status:** MVP complete (Phases 1–5, plus the MVP polish items from Phase 6: narrow
carousel and the optional CLI subcommands). Mouse drag, multi-select, extra themes, and
archive compaction remain post-MVP, as listed in §2 and §9.

---

## 1. Vision

- Markdown is the database. Boards are readable, diffable, greppable, and git-friendly.
- One board is one file; no sidecar state, no database.
- Arbitrary columns; the last column is Done.
- Tasks carry arbitrary tags and one milestone.
- Tags and milestones can filter the board.
- Done tasks can be archived without leaving the file.
- Movement between columns is a first-class, low-friction action.

### Core principles

1. **Canonical output.** `tbdbub` reads tolerantly and writes in one deterministic style,
   like `gofmt`. Unknown content is preserved where practical.
2. **One operation per concept.** All movement gestures call the same domain method, so
   keyboard, grab mode, and future mouse drag cannot diverge.
3. **No surprise data loss.** Saves are atomic; external edits are detected; archive is explicit.
4. **Keyboard first.** Every action is reachable without a mouse. Mouse drag is a later adapter.

---

## 2. Scope

### MVP

- Open or create a board file.
- Render an arbitrary number of columns and tasks.
- Create, edit, delete, reorder, and move tasks.
- Mark tasks done; auto-create a Done column on first completion.
- Tags and milestones on tasks.
- Filter by tag, milestone, and free text.
- Archive Done tasks into an in-file archive section; view and restore them.
- Undo/redo, debounced autosave, atomic writes.

### Non-goals for the MVP

- Multiple boards open in one process.
- Remote sync, collaboration, auth.
- Nested boards or first-class subtasks.
- WIP limits, due-date reminders, recurring tasks.
- Saved filters/views.
- Mouse drag (planned after the keyboard model is stable).
- Theming beyond one clean default.

---

## 3. Domain model

```go
type Board struct {
    Title         string
    Preamble      string        // free text before the first column, preserved verbatim
    Columns       []Column      // ordered; headings define them
    CaptureColumn string        // where new and un-done tasks go
    DoneColumn    string        // last column, by convention
    Milestones    []Milestone   // metadata only; usage also defines milestones
    Archive       []ArchivedTask
    RawFrontmatter map[string]any // unknown frontmatter keys, preserved
}

type Column struct {
    Name  string
    Tasks []Task
}

type Task struct {
    ID        string   // short, stable, survives reordering
    Title     string
    Notes     string   // verbatim indented Markdown block
    Tags      []string
    Milestone string   // empty or one milestone token
    Done      bool
}

type Milestone struct {
    Name        string
    Description string
    Due         string   // ISO date, optional
    Closed      bool     // optional, later
}

type ArchivedTask struct {
    Task
    ArchivedAt time.Time
}
```

### Invariants

- `Columns` is never empty. A new board starts with one column.
- If `DoneColumn` is set, it names the last column and that column exists.
- `CaptureColumn` names an existing non-Done column; fall back to the first column.
- A task is done if and only if it is in the Done column. `Task.Done` mirrors the checkbox
  and is kept in sync by the domain operations.
- Task IDs are unique within a board.
- Archived tasks are not part of any column.
- One milestone per task.

### Derived data (not stored)

- Tags in use (active tasks, optionally archive).
- Milestone progress: tasks with that milestone, done / total. Archived tasks count as done.
- Filtered visible slices per column.

---

## 4. File format

YAML frontmatter for board-level data, `##` headings for columns, checkbox lists for tasks,
inline metadata for tags/milestone/ID.

```markdown
---
tbdbub: 1
title: Home renovation
capture_column: Backlog
milestones:
  - name: Kitchen
    due: 2026-11-15
  - name: Bathroom
---

## Backlog

- [ ] Replace faucet #plumbing #urgent @Kitchen ^t7k2
  Cartridge is 35mm; bought already.
- [ ] Choose tiles #design @Bathroom ^t9x1
  - [ ] shortlist
  - [ ] order samples

## Doing

- [ ] Paint hallway #paint @Kitchen ^t3ab

## Done

- [x] Measure cabinets #design @Kitchen ^t1zz
```

After the first completion, the writer adds `done_column: Done` to the frontmatter and
appends the Done column.

Archive section, appended at the end of the file:

```markdown
<!-- tbdbub:archive -->
## Archived 2026-10

- [x] Old task #urgent ^t4qq
```

### Parsing rules

- Frontmatter is YAML between leading `---` fences.
- `## ` headings define columns, in file order.
- Only top-level list items count as tasks: `- [ ]` and `- [x]` (case-insensitive `x`).
  `*` bullets are accepted and canonicalized to `-`.
- Lines indented under a task belong to its notes and are preserved verbatim. Nested
  checkboxes are notes/subtasks, not board tasks.
- Inline tokens are extracted from the task title:
  - tag: `#[\p{L}\p{N}_-]+`
  - milestone: `@[\p{L}\p{N}_.-]+`
  - ID: trailing `^[a-z0-9]{4,8}`
  - `\#`, `\@`, and `\^` escape a literal leading character.
- The first milestone token wins; additional `@` tokens produce a validation warning rather
  than being silently dropped.
- An inline `@Name` auto-registers a milestone in memory. A frontmatter entry is written only
  when metadata (due, description) is set.
- Tasks without IDs get one generated on save.
- `<!-- tbdbub:archive -->` starts the archive. Everything after it is parsed as archived
  tasks grouped by `## Archived YYYY-MM` headings. It is never treated as a column.
- Preamble text before the first column is preserved verbatim.
- Unknown frontmatter keys are preserved. Unknown blocks in the archive are preserved.

### Canonical output

- LF line endings, one trailing newline, no trailing spaces.
- One blank line between frontmatter sections and column sections.
- Task line order: `- [ ] Title #tags @Milestone ^id`.
- Preserve the author's tag order; do not sort tags (avoids noisy diffs).
- Frontmatter key order: `tbdbub`, `title`, `capture_column`, `done_column`, `milestones`,
  then unknown keys.
- Empty columns are emitted with their heading and no tasks.
- Notes are re-emitted with their original indentation and content.

---

## 5. Semantics

### Done

- A new board starts with exactly one column (`Backlog`), `capture_column: Backlog`, and no
  `done_column`.
- First completion appends a `Done` column, writes `done_column: Done`, and moves the task
  there. This is one undoable edit.
- Completion always moves the task to the end of Done and sets `[x]`.
- Un-done (`space` on a Done task) moves it to the capture column and clears `[x]`.
- Direct movement is literal: moving a task out of Done by hand unchecks it and sends it to
  the chosen column. It does not force the capture column.
- New columns are inserted before Done to keep Done last. Renaming Done updates
  `done_column`. Deleting Done is allowed only when empty, with an offer to archive first.
- If frontmatter names a missing Done column after a hand-edit, treat the last column as Done;
  if there is only one column, recreate Done on the next completion.

### Movement

All gestures call one set-based domain operation:

```go
type Move struct {
    IDs      []TaskID
    ToColumn int
    ToIndex  int // insertion index in target; -1 = append
}

func (b Board) MoveTasks(m Move) (Board, error)
```

It validates the target, clamps the index, preserves order for multi-task moves, syncs the
done flag with the destination, and returns a new board. One undo snapshot per call.

Inputs:

| Gesture | Behavior |
| --- | --- |
| `H` / `L` | Move selected task one column left/right, append to the end of the target. Counts work (`2L`). |
| `m` | Grab mode: lift the task, choose column and slot, drop with `enter`. |
| Mouse drag | Later; same `MoveTasks` call via layout hit-testing. |

- Quick moves append. Grab mode chooses an exact slot.
- Cursor follows the moved task; the view auto-scrolls to keep it visible.
- Boundary moves stop with a brief status hint.
- Empty columns are valid targets.
- A no-op drop (same column and slot) creates no undo entry and no save.
- Moving into Done checks the box; moving out unchecks it.
- Moving right from a single-column board auto-creates Done.

**Grab mode:**

- `h` / `l` (arrows): target column.
- `j` / `k`: insertion slot.
- `1–9`: jump directly to a column; headers show numbers while grabbing.
- `enter`: drop and save; `esc` or `q`: cancel without writing.
- Source card dims with a handle; target header highlights; an insertion line marks the slot.
- Deleting, archiving, and editing are disabled while grabbing.
- No snapshot is taken until the drop.

**Filter interaction:**

- If an active filter hides the moved task, show `Moved to Done (hidden by filter)` and land
  the cursor on the nearest visible task in the target column, or the empty column itself.
- Never silently lose the selection.

### Archive

- `a` archives the selected task when it is in Done; otherwise show a status hint.
- `A` archives all Done tasks after confirmation.
- Archived tasks move to the reserved tail section, grouped by archival month.
- `V` opens the archive view: search, `u` restores to Done, `U` reopens in the capture column.
- Archiving is explicit; no auto-archive in the MVP.
- Archived tasks count as done for milestone progress.
- The original column is not recorded; restore puts the task in Done.

### Milestones

- One milestone per task, inline `@Kitchen`.
- Milestones are defined by usage plus optional frontmatter metadata.
- Progress = done tasks / all tasks carrying that milestone, including archived tasks.
- Renaming a milestone rewrites all inline references.
- Deleting a milestone detaches it from tasks after confirmation.
- A task with no milestone is valid and can be filtered as "no milestone".

### Tags

- Arbitrary free-form tokens, derived from usage; no registry.
- Multiple tags per task, any order.
- Tag filter supports any/all semantics.
- A tag has no metadata in the MVP.

### Filters

Ephemeral per session, shown as chips in the header, pure view-layer predicates:

```go
type Filter struct {
    Text       string
    Tags       []string
    TagMode    Any | All
    Milestone  string // empty = any; "\x00" = none
    ShowDone   bool
}
```

- Text search matches title, notes, tags, and milestone, case-insensitively.
- Filters compose: text AND tag AND milestone.
- Navigation skips hidden tasks; operations still act on real tasks.
- Archive is a separate view, not a filter.
- Filter state is not persisted.

---

## 6. TUI design

### Layout

```text
┌ tbdbub · Home renovation ────────────────────── board.md ✓ · @Kitchen · #plumbing ┐
│ Backlog (3)            │ Doing (1)            │ Done (2)                          │
│ ┌────────────────────┐ │ ┌──────────────────┐ │ ┌───────────────────────────────┐ │
│ │ Replace faucet     │ │ │ Paint hallway    │ │ │ Measure cabinets              │ │
│ │ #plumbing @Kitchen │ │ │ #paint @Kitchen  │ │ │ #design @Kitchen              │ │
│ └────────────────────┘ │ └──────────────────┘ │ └───────────────────────────────┘ │
│ ...                    │                      │                                   │
├───────────────────────────────────────────────────────────────────────────────────┤
│ NORMAL  n new  e edit  space done  m move  / filter  ? help                       │
└───────────────────────────────────────────────────────────────────────────────────┘
```

- Header: title, file path, save indicator (`●` dirty / `✓` saved), active filter chips,
  selected milestone progress.
- Columns: bordered, title with count; cards show title and a compact metadata line.
- Footer: mode-sensitive key hints and a one-line status/toast.
- Column width: equal share clamped to a readable range (~24–48 cells); horizontal scrolling
  when the total exceeds the terminal width.
- Render only visible tasks; each column keeps its own scroll offset.
- Narrow terminals (< ~90 columns): single-column carousel with a column tab bar.
- Selection: accent border on the focused card; focused column header highlighted.

### Modes

```go
type Mode int

const (
    ModeBoard Mode = iota
    ModeGrab
    ModeTaskEditor
    ModeColumnEditor
    ModeFilter
    ModeMilestones
    ModeArchive
    ModeHelp
    ModeConfirm
)
```

Only one mode/overlay at a time. `Update` is a switch over the current mode. Global keys that
remain active: `?` opens help, `ctrl+c` quits (with confirmation when dirty).

### Keymap (MVP)

**Board**

| Key | Action |
| --- | --- |
| `h` / `l`, arrows | Focus previous/next column |
| `j` / `k`, arrows | Focus next/previous task |
| `g` / `G` | First/last task |
| `H` / `L` | Move task left/right, append; counts supported |
| `J` / `K` | Reorder task down/up within its column |
| `m` | Grab mode |
| `space` | Toggle done |
| `n` | New task |
| `N` | New column |
| `r` | Rename focused column |
| `D` | Delete focused column (Done offers to archive first) |
| `e`, `enter` | Edit task |
| `t` | Edit tags |
| `M` | Milestone picker |
| `d` | Delete task (confirm) |
| `a` | Archive selected task (Done only) |
| `A` | Archive all Done tasks (confirm) |
| `V` | Archive view |
| `f` | Filter panel |
| `/` | Quick text search |
| `esc` | Clear filters |
| `u` / `ctrl+r` | Undo / redo |
| `s` | Force save |
| `?` | Help |
| `q` | Quit |

**Grab mode**

| Key | Action |
| --- | --- |
| `h` / `l`, arrows | Target column |
| `j` / `k` | Insertion slot |
| `1–9` | Jump to column |
| `enter` | Drop |
| `esc`, `q` | Cancel |
| `?` | Help |

**Milestones panel**

| Key | Action |
| --- | --- |
| `j` / `k` | Select milestone |
| `enter` | Assign to the focused task |
| `c` | Clear the focused task's milestone |
| `n` | New milestone |
| `e` | Rename milestone |
| `u` | Edit due date |
| `i` | Edit description |
| `d` | Delete milestone and detach it |
| `esc` | Close |

**Task editor**

- Fields: title, notes, tags, milestone.
- `tab` / `shift+tab`: next/previous field.
- `ctrl+s`: save; `esc`: cancel.
- Multiline notes use a textarea; plain `enter` inserts a newline.
- Invalid/extra milestone tokens are surfaced as a warning in the editor footer.

**Filter panel**

- Text field, tag toggles, milestone selector, any/all toggle, show-done toggle.
- `esc`: close (filters stay applied); `ctrl+r`/clear action: reset all.

**Archive view**

- `j`/`k`: navigate; `/`: search; `u`: restore to Done; `U`: reopen in capture; `esc`: close.

### Feedback

- Toast in the footer for moves, saves, archive/restore, and validation warnings.
- Dirty/saved indicator in the header.
- Grab mode: dimmed source card, highlighted target header, insertion marker.
- No animations in the MVP; a brief style flash on drop is enough.
- Errors (parse, save conflict) show a dismissible status/confirm overlay, not a crash.

---

## 7. Architecture

```text
main.go                     // CLI (open/create, add, ls), wiring, program startup
internal/board/
  board.go                  // Board, Column, Task, Milestone, ArchivedTask
  ids.go                    // ID generation and collision handling
  markdown.go               // parse + canonical serialize (frontmatter + archive)
  ops.go                    // AddTask, UpdateTask, DeleteTask, MoveTasks, ToggleDone,
                            // columns, archive/restore, milestones
  filter.go                 // pure filter predicates and visible slices
  testdata/                 // golden board fixtures
  *_test.go
internal/tui/
  model.go                  // root Model, modes, navigation, mutations, undo/redo
  view.go                   // board, carousel, header/footer, overlays, help
  overlays.go               // task editor, prompts, filter, milestones, archive, confirm
  grab.go                   // grab-mode state, drop, markers
  save.go                   // load, autosave scheduling, atomic write, conflict handling
  styles.go                 // lipgloss styles/theme
  demo.go                   // hard-coded board for tests
  *_test.go
```

### Dependencies

- `charm.land/bubbletea/v2` (already pinned at v2.1.0)
- `charm.land/bubbles/v2` v2.1.0 — textinput and textarea
- `charm.land/lipgloss/v2` v2.0.6 — layout and styles
- `gopkg.in/yaml.v3` v3.0.1 — frontmatter

`internal/board` imports no Bubble Tea. The TUI holds `board.Board` as a value.

### Root model

```go
type Model struct {
    board   board.Board
    path    string
    width   int
    height  int

    mode    Mode
    focus   Focus      // column, task, scroll offsets
    grab    *Grab
    overlay Overlay
    filter  board.Filter

    undo    []board.Board
    redo    []board.Board

    dirty          bool
    version        uint64 // increments on every mutation
    savedVersion   uint64
    fileHash       string // hash at load/last save
    savePending    bool
    status         string
    err            error
}
```

### Domain operations

- `NewBoard(title string) Board`
- `Parse(r io.Reader) (Board, []Warning, error)`
- `Write(w io.Writer, b Board) error`
- `AddTask(col int, title string) (Board, error)`
- `UpdateTask(id TaskID, patch TaskPatch) (Board, error)`
- `DeleteTask(id TaskID) (Board, error)`
- `MoveTasks(m Move) (Board, error)`
- `ToggleDone(id TaskID) (Board, error)`
- `AddColumn(name string)`, `RenameColumn`, `DeleteColumn`
- `ArchiveTasks(ids []TaskID, at time.Time)`, `RestoreTask`, `ReopenTask`
- `AddMilestone`, `UpdateMilestone`, `DeleteMilestone`
- `Filter.Apply(b Board) VisibleBoard`

Every mutation returns a new board. The TUI pushes the previous board onto the undo stack,
clears redo, and bumps `version`.

### Save pipeline

- Debounced autosave: ~750 ms after the last mutation.
- Forced save with `s`; save on quit when dirty.
- Write a temp file in the same directory, `fsync`, then `rename`; preserve file mode.
- Always write LF.
- Save is a `tea.Cmd` carrying `(version, snapshot)`; it returns `savedMsg{version, err}`.
- If `version` advanced while the write was in flight, keep dirty and schedule another save.
- Conflict detection: compare the current file hash with the hash at load/last save. On
  mismatch, open a confirm overlay: reload / overwrite / cancel. Do not silently clobber.
- A failed save keeps the dirty state and shows the error in the footer.

### Undo/redo

- Snapshot the whole board (cheap for personal boards); cap at ~100 entries.
- Push before every domain mutation, including auto-created Done columns, moves, archives,
  and milestone renames.
- `u` restores the previous snapshot; `ctrl+r` reapplies.
- A successful save does not clear the undo stack.

---

## 8. Testing strategy

### Domain

- Golden fixtures: parse → write → compare for canonical boards.
- Round-trip property: `parse(write(board)) == board` for generated boards.
- Fuzz the parser: no panics; unknown content is preserved or warned about, never silently lost.
- Table tests for each operation: add/edit/delete, move (append, same column, empty target,
  clamp, no-op), toggle done (auto-create, literal move out), archive/restore, milestone
  rename/detach, filters.
- ID collision handling.
- Notes and IDs survive moves and archive round-trips.

### TUI

- Table-driven `Update` tests using `tea.KeyPressMsg`.
- Movement: `L`, `2L`, boundary `L`, grab → column/slot → drop, grab → cancel leaves the board
  byte-identical, filtered destination fallback.
- Mode exclusivity: keys are consumed by the active mode.
- Save scheduling: debounce, coalescing, conflict handling.
- Render smoke tests at 80×24, 120×40, and a narrow layout; no panics on empty boards.

### Commands

- `go test ./...`
- `go test -race ./...`
- `go vet ./...`
- `gofmt -w .`
- Build to `/tmp` to avoid a repo-root binary: `go build -o /tmp/tbdbub .`

---

## 9. Implementation phases

### Phase 0 — Layout spike — done

Hard-coded in-memory board; three-column layout, selection, navigation, resize, help overlay.
No file I/O.

**Done when:** the board feels right at 80×24 and 200×50, and navigation never loses the cursor.

**Delivered (2026-10-27):**

- `internal/board/board.go` — initial `Board`/`Column`/`Task`/`Milestone` types plus
  `NewBoard`, `ColumnIndex`, `TaskByID`, `IsDoneColumn`, `TotalTasks`.
- `internal/tui` — root `Model`, mode-less board view, header/footer, column and card
  rendering, vertical clipping, horizontal column windowing, help overlay, resize handling.
- `internal/tui/demo.go` — hard-coded "Home renovation" board used by the spike.
- `main.go` — starts Bubble Tea with the demo board in the alternate screen.
- Tests cover navigation/clamping, help toggle, empty boards, selected-task visibility, and
  exact frame size at 80×24, 120×40, 200×50, and 40×12.
- Added `charm.land/lipgloss/v2` v2.0.6.

**Follow-up completed in Phase 2:** the demo board was replaced by file loading, the focused
column scrolls to keep the selection visible, and layout constants live with the root model.

### Phase 1 — Domain and Markdown — done

Model, parser, canonical writer, ID generation, board creation, golden/fuzz tests.

**Done when:** canonical fixtures round-trip, malformed input produces warnings rather than
panics, and a missing file can be initialized with one column.

**Delivered:**

- `internal/board/markdown.go` — YAML frontmatter, `##` columns, checkbox tasks, inline
  tags/milestones/IDs, escaped tokens, notes, preamble/column preambles, the archive section,
  warnings, and canonical output.
- `internal/board/ids.go` — short random IDs with collision handling.
- `internal/board/ops.go`, `filter.go` — all domain operations and pure filter predicates.
- Golden fixture (`testdata/home.md`), round-trip/stability tests, tolerant-input tests, and
  a parser fuzz target (run single-worker in this environment:
  `go test ./internal/board -run=^$ -fuzz=FuzzParse -fuzztime=10s -parallel=1`).

### Phase 2 — Read-only TUI — done

Load a board, render columns/cards, navigate, scroll, resize, show metadata and status,
help overlay.

**Done when:** a real board file can be browsed comfortably without mutations.

**Delivered:** `main.go` loads/creates a board; the TUI renders arbitrary columns and tasks,
skips hidden tasks, scrolls the focused column, shows header save/filter/milestone state,
and supports the full help overlay.

### Phase 3 — Mutations and movement — done

New/edit/delete task, `H`/`L`, grab mode, reorder, toggle done with auto-created Done,
undo/redo, debounced atomic autosave, conflict detection.

**Done when:** all movement tests pass; a move survives quit/reopen; external edits are
detected; `esc` during grab leaves the file unchanged.

**Delivered:** task editor, delete confirmation, `H`/`L` with counts, `J`/`K` reorder, grab
mode with insertion marker and no-op drop, `space` completion with the Done column created on
first use, column add/rename/delete, undo/redo (100 snapshots), 750 ms autosave, atomic
temp-file + fsync + rename writes with mode preservation, and reload/overwrite/cancel on
external edits.

### Phase 4 — Tags and milestones — done

Tag editor, milestone picker/manager, inline token validation, milestone progress in the
header, rename/detach flows.

**Done when:** tags and milestones can be fully managed without hand-editing the file;
progress counts archived tasks correctly.

**Delivered:** tags in the task editor plus `t`, inline-token extraction/warnings, milestone
picker/manager (assign, clear, add, rename, due, description, delete/detach), and progress in
the header and manager including archived tasks.

### Phase 5 — Filters and archive — done

Filter panel, quick search, chips, archive section writer/parser, archive view, restore/reopen.

**Done when:** filter + move + archive combinations keep the cursor coherent, and the archive
section round-trips.

**Delivered:** filter panel (text, tags any/all, milestone/no-milestone, show-done), `/` quick
search, header chips, navigation that skips hidden tasks, move/complete fallback to the
nearest visible task, archive view with search, `u` restore to Done, `U` reopen to capture,
`a` archive one, `A` archive all, and archive month grouping.

### Phase 6 — Polish — MVP items done

Narrow-terminal carousel, mouse drag-and-drop via `MoveTasks`, multi-select, themes, optional
CLI subcommands (`tbdbub add`, `tbdbub ls --json`), archive compaction.

**Delivered:** the narrow-terminal (<90 columns) single-column carousel with a tab bar, and
the optional `add` / `ls [--json]` CLI subcommands.

**Remaining post-MVP (explicit non-goals in §2):** mouse drag-and-drop, multi-select, themes
beyond the single default, and archive compaction. The layout already knows how to call
`MoveTasks`, so mouse drag can be added without changing domain semantics.

---

## 10. Defaults and open details

### Defaults

- Invocation: `tbdbub [board.md]` → `$TBDBUB_BOARD` → `./board.md`; create if missing.
- New board title from the filename stem; one `Backlog` column.
- Archive grouped by month (`## Archived YYYY-MM`).
- Undo cap: 100 snapshots.
- No mouse in the MVP.
- No saved views or persistent filters.
- Tag order preserved as authored.

### Settled in Phase 1

- **Multi-word milestone names.** Tokens remain `@[\p{L}\p{N}_.-]+`; spaces are not allowed.
  The bracket syntax is not implemented.
- **Column names** are taken from the `##` heading with surrounding whitespace trimmed; `##`
  is not allowed inside a name.
- **Raw-content preservation policy.** Indented text after a task is that task's notes. Other
  free text inside a column is preserved as the column preamble and emitted before its tasks.
  Unknown frontmatter keys and unrecognized archive blocks are preserved.
- **Initial column name**: `Backlog`; the capture column is configurable through
  `capture_column`.

---

## 11. Risks and mitigations

| Risk | Mitigation |
| --- | --- |
| Parser/writer loses hand-edited content | Preserve unknown frontmatter, preamble, notes, and archive blocks; golden + fuzz tests; canonical output documented. |
| External file changes while the TUI is open | Hash check before save; reload/overwrite/cancel prompt. File watching can come later. |
| Done column invariant breaks after manual column edits | Designated `done_column`, last-column fallback, auto-create on completion, delete only when empty. |
| Filtered tasks disappear after a move | Toast + cursor fallback; never silently lose the selection. |
| Terminal key conflicts (`ctrl+s`, shifted letters) | Raw mode; `s` also forces save; keymap centralized for easy remapping. |
| Wide boards on small terminals | Column width clamps, horizontal scrolling, narrow single-column mode. |
| Long boards hurting render performance | Render only visible rows; per-column scroll offsets; lipgloss width caching. |
| Multi-word names complicating token parsing | Decide bracket syntax vs slug convention before the parser is written. |

---

## 12. Definition of done for the MVP

- A user can create a Markdown board, add arbitrary columns and tasks, tag them, assign one
  milestone, and move tasks with `H`/`L` or grab mode.
- Completing a task moves it to Done; the first completion auto-creates Done.
- Done tasks can be archived to the in-file archive and restored.
- Tags, milestones, and text can filter the board.
- Every change is undoable and saved atomically; external edits are detected.
- The board file remains readable, diff-friendly, and valid Markdown.
- `go test ./...`, `go vet ./...`, and `gofmt` are clean.

---

## 13. Implementation log

### 2026-10-27 — Phase 0

- Stood up the board value types before the throwaway spike, so Phase 1 fills in parsing and
  operations instead of replacing the model.
- Built the layout around fixed-size column/card blocks: `Width`/`Height` include borders in
  Lip Gloss v2, which makes frame math exact.
- Added a one-cell gutter between columns by reserving `visibleColumns - 1` cells.
- Selected task visibility is handled with a simple computed scroll offset; long columns will
  get per-column scroll state in Phase 2.
- Verified the frame is exactly terminal width × height at 80×24, 120×40, 200×50, and 40×12.
- `go test ./...`, `go vet ./...`, and `go build -o /tmp/tbdbub .` pass.

### 2026-10-27 — MVP (Phases 1–5 and Phase 6 MVP items)

- Implemented the tolerant Markdown parser and canonical writer, including YAML frontmatter,
  inline tags/milestones/IDs, escapes, notes, preambles, month-grouped archive, unknown-key
  preservation, and stable IDs.
- Implemented all domain operations: tasks, movement, completion, columns, archive/restore,
  milestones, and filters. Every mutation returns a new board value.
- Replaced the demo-only root model with modes for board, grab, task editor, column prompt,
  filter, milestones, archive, help, and confirmation. Added counts, hidden-task navigation,
  cursor fallback after filtered moves, undo/redo, autosave, atomic writes, and conflict
  handling. Each column remembers its own cursor/scroll position, and `?` opens help from any
  mode.
- Milestones discovered only from inline usage stay implicit and are not written to
  frontmatter until they gain metadata; explicit frontmatter milestones are preserved.
- Added the narrow-terminal carousel and the optional `tbdbub add` / `tbdbub ls [--json]`.
- Tests cover domain round-trips and operations, movement/counts/grab, editors, filters,
  archive, milestones, undo/redo, save/coalescing/conflict, CLI-facing file creation, empty
  boards, and exact frame size for every mode at 80×24, 120×40, and 40×12.
- `go test ./...`, `go test -race ./...`, `go vet ./...`, `gofmt`, `go build`, and a
  single-worker fuzz run pass.

### 2026-10-27 — Movement keys (`H` / `L`)

- Added `internal/board/ops.go` with `Move` and `MoveTasks`, the single set-based operation
  that all movement gestures will use. It returns a new board, preserves source order, clamps
  the insertion index (`-1` appends), syncs `Task.Done` with the destination, and reports
  `ErrTaskNotFound` / `ErrColumnNotFound`.
- The TUI maps `H` / `L` to move the focused task one column left/right, appending it to the
  target. The cursor follows the task and the footer shows a move status or a boundary hint.
- Added tests for append/insert/clamp, Done sync, multi-task order, error paths, cursor
  following, empty targets, boundaries, and exact frame size after a move.
- This groundwork was completed into the full Phase 3 feature set above.
