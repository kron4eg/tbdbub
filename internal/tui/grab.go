package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/kron4eg/tbdbub/internal/board"
)

// Grab is the keyboard move mode: lift a task, choose a column and slot, drop.
type Grab struct {
	id           board.TaskID
	sourceColumn int
	sourceIndex  int
	column       int
	slot         int
}

func (m *Model) startGrab() {
	task, ok := m.focusedTask()
	if !ok {
		m.status = "No task selected"
		return
	}
	m.grab = &Grab{
		id:           task.ID,
		sourceColumn: m.column,
		sourceIndex:  m.task,
		column:       m.column,
		slot:         m.task,
	}
	m.mode = ModeGrab
	m.status = "Grab: choose a column and slot"
}

func (m Model) updateGrab(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	g := m.grab
	if g == nil {
		m.mode = ModeBoard
		return m, nil
	}

	switch key {
	case "esc", "q":
		m.grab = nil
		m.mode = ModeBoard
		m.status = "Move cancelled"
	case "h", "left":
		g.column = clamp(g.column-1, 0, len(m.Board.Columns)-1)
		m.clampGrabSlot()
	case "l", "right":
		g.column = clamp(g.column+1, 0, len(m.Board.Columns)-1)
		m.clampGrabSlot()
	case "j", "down":
		g.slot = clamp(g.slot+1, 0, m.grabSlotLimit())
	case "k", "up":
		g.slot = clamp(g.slot-1, 0, m.grabSlotLimit())
	case "enter":
		return m.dropGrab()
	default:
		if len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
			index := int(key[0]-'0') - 1
			if index < len(m.Board.Columns) {
				g.column = index
				m.clampGrabSlot()
			}
		}
	}
	return m, nil
}

func (m *Model) grabSlotLimit() int {
	if m.grab == nil || m.grab.column < 0 || m.grab.column >= len(m.Board.Columns) {
		return 0
	}
	limit := len(m.Board.Columns[m.grab.column].Tasks)
	if m.grab.column == m.grab.sourceColumn {
		limit--
	}
	if limit < 0 {
		limit = 0
	}
	return limit
}

func (m *Model) clampGrabSlot() {
	if m.grab == nil {
		return
	}
	m.grab.slot = clamp(m.grab.slot, 0, m.grabSlotLimit())
}

func (m Model) dropGrab() (tea.Model, tea.Cmd) {
	g := m.grab
	if g == nil {
		m.mode = ModeBoard
		return m, nil
	}
	if g.column == g.sourceColumn && g.slot == g.sourceIndex {
		m.grab = nil
		m.mode = ModeBoard
		m.status = "Move cancelled"
		return m, nil
	}

	next, err := m.Board.MoveTasks(board.Move{
		IDs:      []board.TaskID{g.id},
		ToColumn: g.column,
		ToIndex:  g.slot,
	})
	if err != nil {
		m.status = err.Error()
		m.grab = nil
		m.mode = ModeBoard
		return m, nil
	}
	name := next.Columns[g.column].Name
	m.commit(next)
	m.grab = nil
	m.mode = ModeBoard
	hidden := m.landOn(g.id)
	m.status = "Moved to " + name
	if hidden {
		m.status += " (hidden by filter)"
	}
	return m, m.scheduleSave()
}
