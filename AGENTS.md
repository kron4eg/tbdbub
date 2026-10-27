# AGENTS.md

## Read first

- `docs/PLAN.md` is the source of truth for product behavior, the Markdown board format,
  TUI interaction design, and phase status. Read it before changing semantics; update its
  status/implementation log when completing work.
- No README, CI, lint config, `.gitignore`, or `opencode.json`. `justfile` has the common
  tasks below; run `just --list` for the full set.

## Current state

- MVP complete: Phases 1–5 plus the MVP polish items from Phase 6 (narrow carousel and the
  optional `add` / `ls [--json]` CLI subcommands).
- Markdown is the only storage: load/create, parse, canonical write, debounced atomic save,
  and external-edit conflict handling are implemented.
- `internal/board` is a pure value model with parsing, canonical serialization, IDs, domain
  operations, archive/milestone logic, and filters. It imports no Bubble Tea or Lip Gloss.
- `internal/tui` has modes for the board, grab, task editor, name prompts, filter, milestones,
  archive, help, and confirmation. It owns focus, visibility, undo/redo, and save scheduling.
- Post-MVP (documented non-goals in the plan): mouse drag-and-drop, multi-select, themes beyond
  the single default, and archive compaction.

## Commands

- Build: `go build -o /tmp/tbdbub .` — there is no `.gitignore`; avoid writing a binary into
  the repo root.
- Run: `go run . [board.md]` (falls back to `$TBDBUB_BOARD`, then `./board.md`; creates it).
- CLI: `go run . add "Task title"`, `go run . ls [--json]` (use `-b/--board` for a path with
  `add`/`ls`, or set `TBDBUB_BOARD`).
- Test all: `go test ./...`
- One test: `go test ./internal/tui -run TestNavigation`
- Race: `go test -race ./...`
- Vet: `go vet ./...`
- Format: `gofmt -w .`
- Fuzz (single-worker works in this environment): `go test ./internal/board -run=^$ -fuzz=FuzzParse -fuzztime=10s -parallel=1`
- Just: `just check` (fmt-check + vet + tests), `just build`, `just run`, `just cover`;
  `just test -run TestNavigation ./internal/tui` narrows tests.

`gofumpt` and `golangci-lint` are installed, but the repo has no config for them. Don't
reformat with them or add config unless asked.

## Toolchain quirks

- Go 1.27.3 is pinned as a patch version in `go.mod`; older toolchains refuse to build.
- Use `charm.land/bubbletea/v2`, `charm.land/lipgloss/v2`, and `charm.land/bubbles/v2`, **not**
  the `github.com/charmbracelet/*` v1 import paths. Most online examples are v1 and will not
  compile.
- Bubble Tea v2: `Model.View()` returns `tea.View` (`tea.NewView(...)`), with `AltScreen` and
  `MouseMode` set on the view. Keys arrive as `tea.KeyPressMsg`; match with `msg.String()`.
- Bubbles v2 textinput/textarea are used by the task editor, filter panel, archive search,
  and prompts. Focus state is set, but blink `tea.Cmd`s returned by `Focus()` are ignored;
  typing still works.
- Lip Gloss v2 `Width`/`Height` include borders. Frame tests assert exact terminal dimensions;
  keep `View()` output exactly width × height (see `fitBlock`, `overlayBox`, and the carousel).
- `gopkg.in/yaml.v3` handles frontmatter. Writer key order is explicit; unknown keys are
  emitted sorted.

## Architecture

- `main.go` resolves the board path, creates a missing board, loads/parses, starts the TUI,
  and implements `add` / `ls [--json]`.
- `internal/board/`
  - `board.go` — value types, `NewBoard`, column/task helpers.
  - `ids.go` — short random IDs, uniqueness, collision handling.
  - `markdown.go` — tolerant parser, warnings, inline tokens, canonical writer, archive.
  - `ops.go` — all mutations (tasks, movement, Done, columns, archive, milestones).
  - `filter.go` — pure filter predicates and visible indexes.
  - `testdata/home.md` — canonical golden fixture.
- `internal/tui/`
  - `model.go` — root `Model`, modes, navigation, filtering visibility, mutations, undo/redo.
  - `view.go` — header/footer, board, narrow carousel, grab decoration, overlays, help.
  - `overlays.go` — task editor, prompts, filter panel, milestone manager, archive view,
    confirmations.
  - `grab.go` — grab state and drop.
  - `save.go` — load, hash, autosave tick, atomic write, conflict/reload.
  - `styles.go`, `demo.go`.
- Domain operations return new boards. The TUI pushes the previous board onto undo before
  committing, clears redo, bumps `version`, marks dirty, and schedules autosave.

## Conventions and gotchas

- Locked decisions in `docs/PLAN.md` (auto-created Done column, in-file archive, one milestone
  per task, `H`/`L` + grab movement) shouldn't change silently; update the plan if they do.
- Movement always goes through `Board.MoveTasks`, including grab drops and mouse-ready layout
  paths. Keep it that way.
- TUI tests cover navigation/clamping, empty boards, help, selected-task visibility, exact
  frame size at 80×24, 120×40, 200×50, and 40×12 (and every mode at 80×24/120×40/40×12),
  movement/counts, grab cancel/no-op, editors, filters, archive, milestones, undo/redo, save
  coalescing, and conflict handling. Keep those green when touching layout or keys.
- Tests live in-package and inspect unexported `Model` fields; don't export internals just
  for tests.
- Filtered navigation must skip hidden tasks while operations act on real board indexes.
  After a move, use `landOn` so the cursor never disappears; report `(hidden by filter)`.
- No-op moves must not create undo entries or writes. Grab cancel must leave the board
  byte-identical.
- Saves write a temp file in the same directory, `fsync`, preserve mode, then rename. Never
  write directly to the board path.
- There is no CI or documented branch/PR convention. Run the commands above locally.
