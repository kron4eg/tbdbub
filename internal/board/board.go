// Package board defines the tbdbub board model and the operations over it.
//
// The board is the single source of truth for the TUI. It is a plain value:
// operations return a new Board instead of mutating shared state, which makes
// undo, tests, and future CLI use straightforward.
package board

import (
	"strings"
	"time"
)

// TaskID is a short, stable identifier for a task. It survives reordering and
// movement between columns.
type TaskID string

// Task is a single card on the board.
type Task struct {
	ID        TaskID
	Title     string
	Notes     string // verbatim indented Markdown block
	Tags      []string
	Milestone string // empty or one milestone token
	Done      bool
}

// Column is an ordered list of tasks. Columns are defined by "##" headings in
// the Markdown file.
type Column struct {
	Name     string
	Tasks    []Task
	Preamble string // free text before the first task, preserved verbatim
}

// Milestone groups tasks across columns. Metadata is optional; a milestone is
// also defined by being referenced from a task.
type Milestone struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
	Due         string `yaml:"due,omitempty"` // ISO date, optional
	Closed      bool   `yaml:"closed,omitempty"`

	// Implicit milestones are discovered from inline task usage. They are not
	// written to frontmatter unless they gain metadata.
	Implicit bool `yaml:"-"`
}

// ArchivedTask is a task that has been moved out of the Done column and into
// the archive section at the end of the file.
type ArchivedTask struct {
	Task
	ArchivedAt time.Time
}

// Board is one Markdown file.
type Board struct {
	Title          string
	Preamble       string // free text before the first column, preserved verbatim
	Columns        []Column
	CaptureColumn  string // where new and un-done tasks go
	DoneColumn     string // last column, by convention
	Milestones     []Milestone
	Archive        []ArchivedTask
	ArchiveRaw     string         // unrecognized archive content, preserved verbatim
	RawFrontmatter map[string]any // unknown frontmatter keys, preserved
}

// NewBoard returns a board with a single capture column and no Done column.
// The Done column is created on first completion.
func NewBoard(title string) Board {
	return Board{
		Title:         title,
		CaptureColumn: "Backlog",
		Columns: []Column{
			{Name: "Backlog"},
		},
	}
}

// ColumnIndex returns the index of the column with the given name, or -1.
func (b Board) ColumnIndex(name string) int {
	for i := range b.Columns {
		if b.Columns[i].Name == name {
			return i
		}
	}
	return -1
}

// TaskByID finds a task by ID and returns its column and position.
func (b Board) TaskByID(id TaskID) (column, index int, ok bool) {
	for ci := range b.Columns {
		for ti := range b.Columns[ci].Tasks {
			if b.Columns[ci].Tasks[ti].ID == id {
				return ci, ti, true
			}
		}
	}
	return -1, -1, false
}

// IsDoneColumn reports whether the column at index is the Done column. A
// configured Done column wins; a stale name falls back to the last column. With
// no configured Done column, a column named "Done" is recognized.
func (b Board) IsDoneColumn(index int) bool {
	if index < 0 || index >= len(b.Columns) {
		return false
	}
	if b.DoneColumn != "" {
		if b.Columns[index].Name == b.DoneColumn {
			return true
		}
		return len(b.Columns) > 1 && index == len(b.Columns)-1 && b.ColumnIndex(b.DoneColumn) < 0
	}
	return strings.EqualFold(b.Columns[index].Name, "Done")
}

// TotalTasks returns the number of active tasks on the board.
func (b Board) TotalTasks() int {
	n := 0
	for i := range b.Columns {
		n += len(b.Columns[i].Tasks)
	}
	return n
}

// CaptureIndex returns the index of the capture column, falling back to the
// first non-Done column (or the first column).
func (b Board) CaptureIndex() int {
	if len(b.Columns) == 0 {
		return -1
	}
	if i := b.ColumnIndex(b.CaptureColumn); i >= 0 && !b.IsDoneColumn(i) {
		return i
	}
	for i := range b.Columns {
		if !b.IsDoneColumn(i) {
			return i
		}
	}
	return 0
}

// DoneIndex returns the index of the Done column, if there is one.
func (b Board) DoneIndex() (int, bool) {
	for i := range b.Columns {
		if b.IsDoneColumn(i) {
			return i, true
		}
	}
	return -1, false
}
