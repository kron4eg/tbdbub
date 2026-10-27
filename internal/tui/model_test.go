package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/kron4eg/tbdbub/internal/board"
)

func press(s string) tea.KeyPressMsg {
	runes := []rune(s)
	return tea.KeyPressMsg{Code: runes[0], Text: s}
}

func pressEsc() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeyEscape}
}

func update(m Model, msg tea.Msg) Model {
	next, _ := m.Update(msg)
	return next.(Model)
}

func TestNavigation(t *testing.T) {
	m := New(DemoBoard())

	if m.column != 0 || m.task != 0 {
		t.Fatalf("initial focus = (%d, %d)", m.column, m.task)
	}

	m = update(m, press("l"))
	if m.column != 1 || m.task != 0 {
		t.Fatalf("after l focus = (%d, %d)", m.column, m.task)
	}

	m = update(m, press("j"))
	if m.task != 1 {
		t.Fatalf("after j task = %d", m.task)
	}

	m = update(m, press("l"))
	if m.column != 2 || m.task != 0 {
		t.Fatalf("after second l focus = (%d, %d)", m.column, m.task)
	}

	m = update(m, press("h"))
	if m.column != 1 {
		t.Fatalf("after h column = %d", m.column)
	}

	m = update(m, press("g"))
	if m.task != 0 {
		t.Fatalf("after g task = %d", m.task)
	}

	m = update(m, press("G"))
	if m.task != 1 {
		t.Fatalf("after G task = %d", m.task)
	}
}

func TestNavigationClampsAtEdges(t *testing.T) {
	m := New(DemoBoard())

	m = update(m, press("h"))
	m = update(m, press("k"))
	if m.column != 0 || m.task != 0 {
		t.Fatalf("edge focus = (%d, %d)", m.column, m.task)
	}

	for range 10 {
		m = update(m, press("l"))
		m = update(m, press("j"))
	}
	if m.column != len(m.Board.Columns)-1 {
		t.Fatalf("column = %d, want %d", m.column, len(m.Board.Columns)-1)
	}
	if m.task != len(m.Board.Columns[m.column].Tasks)-1 {
		t.Fatalf("task = %d, want last", m.task)
	}
}

func TestHelpToggle(t *testing.T) {
	m := New(DemoBoard())

	m = update(m, press("?"))
	if m.mode != ModeHelp {
		t.Fatal("help did not open")
	}
	m = update(m, pressEsc())
	if m.mode != ModeBoard {
		t.Fatal("help did not close")
	}
}

func TestEmptyBoard(t *testing.T) {
	for _, b := range []board.Board{board.NewBoard("empty"), {}} {
		m := New(b)
		m = update(m, tea.WindowSizeMsg{Width: 80, Height: 24})
		m = update(m, press("j"))
		m = update(m, press("l"))
		m = update(m, press("G"))
		if got := m.View().Content; got == "" {
			t.Fatal("empty board rendered nothing")
		}
	}
}

func TestViewFitsTerminal(t *testing.T) {
	for _, tc := range []struct {
		name string
		w, h int
	}{
		{"80x24", 80, 24},
		{"120x40", 120, 40},
		{"200x50", 200, 50},
		{"40x12", 40, 12},
	} {
		for _, help := range []bool{false, true} {
			name := tc.name
			if help {
				name += "/help"
			}
			t.Run(name, func(t *testing.T) {
				m := New(DemoBoard())
				m = update(m, tea.WindowSizeMsg{Width: tc.w, Height: tc.h})
				if help {
					m = update(m, press("?"))
				}
				content := m.View().Content
				lines := strings.Split(content, "\n")
				if len(lines) != tc.h {
					t.Fatalf("height = %d, want %d", len(lines), tc.h)
				}
				for i, line := range lines {
					if got := lipgloss.Width(line); got != tc.w {
						t.Fatalf("line %d width = %d, want %d: %q", i, got, tc.w, line)
					}
				}
			})
		}
	}
}

func TestSelectedCardStaysVisible(t *testing.T) {
	m := New(DemoBoard())
	m = update(m, tea.WindowSizeMsg{Width: 120, Height: 40})

	for range 6 {
		m = update(m, press("j"))
	}
	last := len(m.Board.Columns[0].Tasks) - 1
	if m.task != last {
		t.Fatalf("task = %d, want %d", m.task, last)
	}

	content := m.View().Content
	if !strings.Contains(content, "Clear the spare room") {
		t.Fatal("selected task is not rendered")
	}
}

func TestMoveSelectedWithHL(t *testing.T) {
	m := New(DemoBoard())
	m = update(m, press("j")) // focus "Choose tiles"

	m = update(m, press("L"))
	if m.column != 1 {
		t.Fatalf("after L column = %d, want 1", m.column)
	}
	if m.task != len(m.Board.Columns[1].Tasks)-1 {
		t.Fatalf("after L task = %d, want end of target", m.task)
	}
	if got := m.Board.Columns[1].Tasks[m.task].Title; got != "Choose tiles" {
		t.Fatalf("moved task = %q", got)
	}
	if m.status != "Moved to Doing" {
		t.Fatalf("status = %q", m.status)
	}

	m = update(m, press("H"))
	if m.column != 0 {
		t.Fatalf("after H column = %d, want 0", m.column)
	}
	if got := m.Board.Columns[0].Tasks[m.task].Title; got != "Choose tiles" {
		t.Fatalf("moved back task = %q", got)
	}
	if m.status != "Moved to Backlog" {
		t.Fatalf("status = %q", m.status)
	}
}

func TestMoveSelectedAtEdges(t *testing.T) {
	m := New(DemoBoard())

	m = update(m, press("H"))
	if m.column != 0 || m.status != "No column to the left" {
		t.Fatalf("left edge: column = %d, status = %q", m.column, m.status)
	}
	if got := m.Board.Columns[0].Tasks[0].Title; got != "Replace faucet" {
		t.Fatalf("left edge changed the board: %q", got)
	}

	for range 5 {
		m = update(m, press("L"))
	}
	if m.column != len(m.Board.Columns)-1 {
		t.Fatalf("column = %d, want last", m.column)
	}
	tasks := len(m.Board.Columns[m.column].Tasks)
	m = update(m, press("L"))
	if m.column != len(m.Board.Columns)-1 || len(m.Board.Columns[m.column].Tasks) != tasks {
		t.Fatal("right edge changed the board")
	}
	if m.status != "No column to the right" {
		t.Fatalf("right edge status = %q", m.status)
	}
}

func TestMoveSelectedSyncsDone(t *testing.T) {
	m := New(DemoBoard())
	m = update(m, press("l"))
	m = update(m, press("l")) // Done

	m = update(m, press("H"))
	_, index, ok := m.Board.TaskByID("t1zz")
	if !ok || m.Board.Columns[1].Tasks[index].Done {
		t.Fatal("moving out of Done left the task checked")
	}

	m = update(m, press("L"))
	_, index, ok = m.Board.TaskByID("t1zz")
	if !ok || !m.Board.Columns[2].Tasks[index].Done {
		t.Fatal("moving into Done did not check the task")
	}
}

func TestMoveSelectedIntoEmptyColumn(t *testing.T) {
	b := DemoBoard()
	b.Columns[1].Tasks = nil
	b.Columns = b.Columns[:2]
	m := New(b)

	m = update(m, press("L"))
	if m.column != 1 || m.task != 0 {
		t.Fatalf("focus = (%d, %d), want (1, 0)", m.column, m.task)
	}
	if len(m.Board.Columns[1].Tasks) != 1 {
		t.Fatalf("target tasks = %d, want 1", len(m.Board.Columns[1].Tasks))
	}
	if m.status != "Moved to Doing" {
		t.Fatalf("status = %q", m.status)
	}
}

func TestMoveSelectedWithoutTask(t *testing.T) {
	m := New(board.Board{Columns: []board.Column{{Name: "Backlog"}}})

	m = update(m, press("L"))
	if m.column != 0 || m.status != "No task selected" {
		t.Fatalf("column = %d, status = %q", m.column, m.status)
	}
}

func TestViewFitsTerminalAfterMove(t *testing.T) {
	m := New(DemoBoard())
	m = update(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = update(m, press("L"))

	lines := strings.Split(m.View().Content, "\n")
	if len(lines) != 24 {
		t.Fatalf("height = %d, want 24", len(lines))
	}
	for i, line := range lines {
		if got := lipgloss.Width(line); got != 80 {
			t.Fatalf("line %d width = %d, want 80: %q", i, got, line)
		}
	}
}
