package tui

import (
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kron4eg/tbdbub/internal/board"
)

const (
	defaultWidth  = 80
	defaultHeight = 24

	minColumnWidth = 24
	maxColumnWidth = 48
	cardHeight     = 4

	narrowWidth   = 90
	undoLimit     = 100
	autosaveDelay = 750 * time.Millisecond
)

// Mode selects which overlay or input owns the keyboard.
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

// Model is the root Bubble Tea model.
type Model struct {
	Board board.Board
	path  string

	width  int
	height int

	mode       Mode
	column     int
	task       int
	columnTask map[string]int
	filter     board.Filter
	status     string
	warn       []board.Warning
	count      int

	undo []board.Board
	redo []board.Board

	dirty        bool
	version      uint64
	savedVersion uint64
	fileHash     string

	quitAfterSave bool
	helpBack      Mode

	grab       *Grab
	editor     TaskEditor
	filterPane FilterPanel
	miles      MilestonePanel
	archive    ArchiveView
	confirm    Confirm
	prompt     Prompt
}

// New creates an in-memory board model (used by tests and the demo).
func New(b board.Board) Model {
	m := Model{
		Board:      b,
		width:      defaultWidth,
		height:     defaultHeight,
		filter:     board.NewFilter(),
		columnTask: map[string]int{},
	}
	m.clampFocus()
	return m
}

// NewFile creates a model backed by a Markdown file.
func NewFile(b board.Board, path, hash string, warnings []board.Warning) Model {
	m := New(b)
	m.path = path
	m.fileHash = hash
	m.warn = warnings
	if len(warnings) > 0 {
		m.status = warnings[0].String()
	}
	return m
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = max(1, msg.Width)
		m.height = max(1, msg.Height)
		m.resizeOverlays()
		m.clampFocus()
		return m, nil
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case saveTickMsg:
		return m.handleSaveTick(msg)
	case savedMsg:
		return m.handleSaved(msg)
	}
	return m.updateOverlayMsg(msg)
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "ctrl+c" {
		return m.requestQuit()
	}
	if key == "?" && m.mode != ModeHelp {
		m.helpBack = m.mode
		m.mode = ModeHelp
		return m, nil
	}
	switch m.mode {
	case ModeHelp:
		switch key {
		case "esc", "?", "q":
			m.mode = m.helpBack
		}
		return m, nil
	case ModeConfirm:
		return m.updateConfirm(key)
	case ModeTaskEditor:
		return m.updateTaskEditor(msg)
	case ModeColumnEditor:
		return m.updatePrompt(msg)
	case ModeFilter:
		return m.updateFilterPanel(msg)
	case ModeMilestones:
		return m.updateMilestones(msg)
	case ModeArchive:
		return m.updateArchive(msg)
	case ModeGrab:
		return m.updateGrab(msg)
	default:
		return m.updateBoard(msg)
	}
}

func (m Model) updateBoard(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	before := m.version

	if len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
		m.count = m.count*10 + int(key[0]-'0')
		if m.count > 99 {
			m.count = 99
		}
		return m, nil
	}
	if key != "H" && key != "shift+h" && key != "L" && key != "shift+l" {
		m.count = 0
	}

	switch key {
	case "q":
		return m.requestQuit()
	case "h", "left":
		m.moveColumn(-1)
	case "l", "right":
		m.moveColumn(1)
	case "j", "down":
		m.moveTask(1)
	case "k", "up":
		m.moveTask(-1)
	case "g":
		m.firstTask()
	case "G", "shift+g":
		m.lastTask()
	case "H", "shift+h":
		m.moveSelected(-1)
	case "L", "shift+l":
		m.moveSelected(1)
	case "J", "shift+j":
		m.reorder(1)
	case "K", "shift+k":
		m.reorder(-1)
	case "m":
		m.startGrab()
	case " ", "space":
		m.toggleSelectedDone()
	case "n":
		m.openTaskEditor(true, 0)
	case "N":
		m.openPrompt(promptColumn, "New column", "", ModeBoard)
	case "r":
		m.openColumnRename()
	case "D":
		m.confirmDeleteColumn()
	case "e", "enter":
		m.openTaskEditor(false, 0)
	case "t":
		m.openTaskEditor(false, 2)
	case "M":
		m.openMilestones()
	case "d":
		m.confirmDeleteTask()
	case "a":
		m.archiveSelected()
	case "A":
		m.confirmArchiveAll()
	case "V":
		m.openArchive()
	case "f":
		m.openFilter(0)
	case "/":
		m.openFilter(0)
	case "esc":
		m.filter = board.NewFilter()
		m.count = 0
		m.status = ""
		m.clampFocus()
	case "u":
		m.undoBoard()
	case "ctrl+r":
		m.redoBoard()
	case "s":
		return m.saveNow(false, true)
	}

	var cmd tea.Cmd
	if m.version != before {
		cmd = m.scheduleSave()
	}
	return m, cmd
}

func (m *Model) clampFocus() {
	m.count = 0
	if len(m.Board.Columns) == 0 {
		m.column, m.task = 0, -1
		return
	}
	m.column = clamp(m.column, 0, len(m.Board.Columns)-1)
	vis := m.visibleIndices(m.column)
	if len(vis) == 0 {
		m.task = -1
	} else if m.task < 0 {
		m.task = vis[0]
	} else {
		m.task = nearestVisible(vis, m.task)
	}
	m.rememberTask()
}

// rememberTask stores the cursor per column so focus changes keep each
// column's scroll position.
func (m *Model) rememberTask() {
	if m.columnTask == nil {
		m.columnTask = map[string]int{}
	}
	if m.column >= 0 && m.column < len(m.Board.Columns) {
		m.columnTask[m.Board.Columns[m.column].Name] = m.task
	}
}

func (m Model) visibleIndices(col int) []int {
	if col < 0 || col >= len(m.Board.Columns) {
		return nil
	}
	if m.grab != nil {
		all := make([]int, len(m.Board.Columns[col].Tasks))
		for i := range all {
			all[i] = i
		}
		return all
	}
	return m.filter.VisibleIndices(m.Board.Columns[col])
}

func nearestVisible(vis []int, task int) int {
	if len(vis) == 0 {
		return -1
	}
	pos := sort.SearchInts(vis, task)
	if pos < len(vis) {
		return vis[pos]
	}
	return vis[len(vis)-1]
}

func (m *Model) moveColumn(delta int) {
	if len(m.Board.Columns) == 0 {
		return
	}
	m.rememberTask()
	previous := m.task
	m.column = clamp(m.column+delta, 0, len(m.Board.Columns)-1)
	vis := m.visibleIndices(m.column)
	if len(vis) == 0 {
		m.task = -1
		m.rememberTask()
		return
	}
	if stored, ok := m.columnTask[m.Board.Columns[m.column].Name]; ok {
		m.task = nearestVisible(vis, stored)
	} else if previous < 0 {
		m.task = vis[0]
	} else {
		m.task = nearestVisible(vis, previous)
	}
	m.rememberTask()
}

func (m *Model) moveTask(delta int) {
	vis := m.visibleIndices(m.column)
	if len(vis) == 0 {
		m.task = -1
		return
	}
	pos := 0
	for i, index := range vis {
		if index == m.task {
			pos = i
			break
		}
	}
	pos = clamp(pos+delta, 0, len(vis)-1)
	m.task = vis[pos]
}

func (m *Model) firstTask() {
	vis := m.visibleIndices(m.column)
	if len(vis) > 0 {
		m.task = vis[0]
	}
}

func (m *Model) lastTask() {
	vis := m.visibleIndices(m.column)
	if len(vis) > 0 {
		m.task = vis[len(vis)-1]
	}
}

func (m Model) focusedTask() (board.Task, bool) {
	if m.column < 0 || m.column >= len(m.Board.Columns) {
		return board.Task{}, false
	}
	if m.task < 0 || m.task >= len(m.Board.Columns[m.column].Tasks) {
		return board.Task{}, false
	}
	return m.Board.Columns[m.column].Tasks[m.task], true
}

// landOn puts the cursor on a task, clamping to the nearest visible task when a
// filter hides it. It reports whether the task was hidden.
func (m *Model) landOn(id board.TaskID) bool {
	ci, ti, ok := m.Board.TaskByID(id)
	if !ok {
		m.clampFocus()
		return false
	}
	m.column = ci
	vis := m.visibleIndices(ci)
	hidden := true
	for _, index := range vis {
		if index == ti {
			hidden = false
			break
		}
	}
	if hidden {
		m.task = nearestVisible(vis, ti)
	} else {
		m.task = ti
	}
	m.rememberTask()
	return hidden
}

func (m *Model) moveSelected(delta int) {
	task, ok := m.focusedTask()
	if !ok {
		m.status = "No task selected"
		return
	}
	steps := m.count
	if steps < 1 {
		steps = 1
	}
	m.count = 0

	before := m.Board
	next := m.Board
	col := m.column
	moved := 0
	edge := false
	for i := 0; i < steps; i++ {
		target := col + delta
		if target < 0 || target >= len(next.Columns) {
			if delta > 0 && len(next.Columns) == 1 {
				next = next.EnsureDoneColumn()
				if col+1 >= len(next.Columns) {
					edge = true
					break
				}
				target = col + 1
			} else {
				edge = true
				break
			}
		}
		var err error
		next, err = next.MoveTasks(board.Move{
			IDs:      []board.TaskID{task.ID},
			ToColumn: target,
			ToIndex:  -1,
		})
		if err != nil {
			m.status = err.Error()
			break
		}
		col = target
		moved++
	}
	if moved == 0 {
		if edge {
			m.status = boundaryStatus(delta)
		}
		return
	}

	m.commitFrom(before, next)
	hidden := m.landOn(task.ID)
	m.status = "Moved to " + next.Columns[col].Name
	if hidden {
		m.status += " (hidden by filter)"
	}
	if edge {
		m.status += " · reached edge"
	}
}

func boundaryStatus(delta int) string {
	if delta < 0 {
		return "No column to the left"
	}
	return "No column to the right"
}

func (m *Model) reorder(delta int) {
	task, ok := m.focusedTask()
	if !ok {
		m.status = "No task selected"
		return
	}
	target := m.task + delta
	if target < 0 || target >= len(m.Board.Columns[m.column].Tasks) {
		m.status = "Cannot reorder further"
		return
	}
	next, err := m.Board.MoveTasks(board.Move{
		IDs:      []board.TaskID{task.ID},
		ToColumn: m.column,
		ToIndex:  target,
	})
	if err != nil {
		m.status = err.Error()
		return
	}
	m.commit(next)
	m.landOn(task.ID)
	m.status = "Reordered"
}

func (m *Model) toggleSelectedDone() {
	task, ok := m.focusedTask()
	if !ok {
		m.status = "No task selected"
		return
	}
	wasDone := m.Board.IsDoneColumn(m.column) || task.Done
	next, err := m.Board.ToggleDone(task.ID)
	if err != nil {
		m.status = err.Error()
		return
	}
	m.commit(next)
	hidden := m.landOn(task.ID)
	if wasDone {
		m.status = "Reopened " + task.Title
	} else {
		m.status = "Completed " + task.Title
	}
	if hidden {
		m.status += " (hidden by filter)"
	}
}

func (m *Model) commit(next board.Board) {
	m.commitFrom(m.Board, next)
}

func (m *Model) commitFrom(before, next board.Board) {
	m.undo = append(m.undo, before)
	if len(m.undo) > undoLimit {
		m.undo = m.undo[len(m.undo)-undoLimit:]
	}
	m.redo = nil
	m.Board = next
	m.bump()
}

func (m *Model) bump() {
	m.version++
	m.dirty = m.version != m.savedVersion
}

func (m *Model) undoBoard() {
	if len(m.undo) == 0 {
		m.status = "Nothing to undo"
		return
	}
	m.redo = append(m.redo, m.Board)
	if len(m.redo) > undoLimit {
		m.redo = m.redo[len(m.redo)-undoLimit:]
	}
	m.Board = m.undo[len(m.undo)-1]
	m.undo = m.undo[:len(m.undo)-1]
	m.bump()
	m.clampFocus()
	m.status = "Undone"
}

func (m *Model) redoBoard() {
	if len(m.redo) == 0 {
		m.status = "Nothing to redo"
		return
	}
	m.undo = append(m.undo, m.Board)
	if len(m.undo) > undoLimit {
		m.undo = m.undo[len(m.undo)-undoLimit:]
	}
	m.Board = m.redo[len(m.redo)-1]
	m.redo = m.redo[:len(m.redo)-1]
	m.bump()
	m.clampFocus()
	m.status = "Redone"
}

func (m Model) requestQuit() (tea.Model, tea.Cmd) {
	if !m.dirty {
		return m, tea.Quit
	}
	m.confirm = Confirm{
		kind:    confirmQuit,
		title:   "Unsaved changes",
		message: "Save before quitting? (s save · d discard · esc cancel)",
		back:    m.mode,
	}
	m.mode = ModeConfirm
	return m, nil
}

func (m *Model) confirmDeleteTask() {
	task, ok := m.focusedTask()
	if !ok {
		m.status = "No task selected"
		return
	}
	m.confirm = Confirm{
		kind:    confirmDeleteTask,
		title:   "Delete task",
		message: "Delete \"" + task.Title + "\"?",
		taskID:  task.ID,
	}
	m.mode = ModeConfirm
}

func (m *Model) archiveSelected() {
	task, ok := m.focusedTask()
	if !ok {
		m.status = "No task selected"
		return
	}
	if !m.Board.IsDoneColumn(m.column) {
		m.status = "Archive is only available for Done tasks"
		return
	}
	next, err := m.Board.ArchiveTasks([]board.TaskID{task.ID}, time.Now().UTC())
	if err != nil {
		m.status = err.Error()
		return
	}
	m.commit(next)
	m.clampFocus()
	m.status = "Archived " + task.Title
}

func (m *Model) confirmArchiveAll() {
	done, ok := m.Board.DoneIndex()
	if !ok || len(m.Board.Columns[done].Tasks) == 0 {
		m.status = "Nothing to archive"
		return
	}
	m.confirm = Confirm{
		kind:    confirmArchiveAll,
		title:   "Archive Done",
		message: "Archive all tasks in " + m.Board.Columns[done].Name + "?",
	}
	m.mode = ModeConfirm
}

func (m *Model) openTaskEditor(newTask bool, defaultFocus int) {
	task, ok := m.focusedTask()
	if newTask {
		task, ok = board.Task{}, false
	}
	m.editor = newTaskEditor(*m, task, ok, m.column)
	m.editor.setFocus(defaultFocus)
	m.editor.resize(m.width)
	m.mode = ModeTaskEditor
}

func (m *Model) openMilestones() {
	m.miles = MilestonePanel{}
	m.mode = ModeMilestones
	m.clampMilestonePanel()
}

func (m *Model) openArchive() {
	m.archive = newArchiveView(m.width)
	m.mode = ModeArchive
	m.clampArchive()
}

func (m *Model) openFilter(focus int) {
	m.filterPane = newFilterPanel(*m)
	m.filterPane.focus = focus
	m.filterPane.resize(m.width)
	m.mode = ModeFilter
}

func (m *Model) openColumnRename() {
	if len(m.Board.Columns) == 0 {
		return
	}
	name := m.Board.Columns[m.column].Name
	m.openPrompt(promptColumnRename, "Rename column", name, ModeBoard)
	m.prompt.target = name
}

func (m *Model) confirmDeleteColumn() {
	if len(m.Board.Columns) == 0 {
		return
	}
	if len(m.Board.Columns) == 1 {
		m.status = "Cannot delete the last column"
		return
	}
	col := m.Board.Columns[m.column]
	if m.Board.IsDoneColumn(m.column) && len(col.Tasks) > 0 {
		m.confirm = Confirm{
			kind:    confirmArchiveDeleteDone,
			title:   "Delete Done column",
			message: "Archive all tasks in \"" + col.Name + "\" and delete the column?",
			column:  m.column,
			back:    ModeBoard,
		}
		m.mode = ModeConfirm
		return
	}
	if len(col.Tasks) > 0 {
		m.status = "Column is not empty"
		return
	}
	m.confirm = Confirm{
		kind:    confirmDeleteColumn,
		title:   "Delete column",
		message: "Delete empty column \"" + col.Name + "\"?",
		column:  m.column,
		back:    ModeBoard,
	}
	m.mode = ModeConfirm
}

func (m *Model) openPrompt(kind PromptKind, title, value string, back Mode) {
	m.prompt = newPrompt(kind, title, value, back, m.width)
	m.mode = ModeColumnEditor
}

func (m Model) updateOverlayMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.mode {
	case ModeTaskEditor:
		switch m.editor.focus {
		case 0:
			m.editor.title, cmd = m.editor.title.Update(msg)
		case 1:
			m.editor.notes, cmd = m.editor.notes.Update(msg)
		case 2:
			m.editor.tags, cmd = m.editor.tags.Update(msg)
		case 3:
			m.editor.milestone, cmd = m.editor.milestone.Update(msg)
		}
	case ModeFilter:
		if m.filterPane.focus == 0 {
			m.filterPane.text, cmd = m.filterPane.text.Update(msg)
			m.applyFilterPanel()
		}
	case ModeArchive:
		if m.archive.searching {
			m.archive.search, cmd = m.archive.search.Update(msg)
			m.clampArchive()
		}
	}
	return m, cmd
}

func (m *Model) resizeOverlays() {
	switch m.mode {
	case ModeTaskEditor:
		m.editor.resize(m.width)
	case ModeColumnEditor:
		m.prompt.resize(m.width)
	case ModeFilter:
		m.filterPane.resize(m.width)
	case ModeArchive:
		m.archive.resize(m.width)
	}
}

func (m Model) isNarrow() bool { return m.width < narrowWidth }

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// spread places left and right at the edges of a width-cell line.
func spread(left, right string, width int) string {
	if width <= 0 {
		return ""
	}
	left = truncate(left, width)
	lw := lipglossWidth(left)
	rw := lipglossWidth(right)
	if lw+rw >= width {
		if rw >= width {
			return truncate(right, width)
		}
		left = truncate(left, width-rw)
		lw = lipglossWidth(left)
	}
	gap := width - lw - rw
	if gap < 0 {
		gap = 0
	}
	return left + strings.Repeat(" ", gap) + right
}

func truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipglossWidth(s) <= width {
		return s
	}
	if width == 1 {
		return "…"
	}
	runes := []rune(s)
	for len(runes) > 0 {
		runes = runes[:len(runes)-1]
		if lipglossWidth(string(runes)) <= width-1 {
			return string(runes) + "…"
		}
	}
	return "…"
}

// splitTags accepts comma- or space-separated tags with optional leading '#'.
func splitTags(value string) []string {
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n'
	})
	tags := make([]string, 0, len(fields))
	seen := map[string]bool{}
	for _, field := range fields {
		field = strings.TrimPrefix(strings.TrimSpace(field), "#")
		if field == "" || seen[field] {
			continue
		}
		seen[field] = true
		tags = append(tags, field)
	}
	return tags
}
