package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/kron4eg/tbdbub/internal/board"
)

// ---------- task editor ----------

type TaskEditor struct {
	taskID board.TaskID // empty for a new task
	column int

	title     textinput.Model
	notes     textarea.Model
	tags      textinput.Model
	milestone textinput.Model

	focus int
	width int
	err   string
	warn  string
}

func newTaskEditor(m Model, task board.Task, existing bool, column int) TaskEditor {
	e := TaskEditor{column: column}
	e.title = textinput.New()
	e.title.Prompt = ""
	e.title.Placeholder = "Task title"
	e.title.CharLimit = 300
	e.tags = textinput.New()
	e.tags.Prompt = ""
	e.tags.Placeholder = "tag1 tag2 (optional)"
	e.tags.CharLimit = 300
	e.milestone = textinput.New()
	e.milestone.Prompt = ""
	e.milestone.Placeholder = "milestone (optional)"
	e.milestone.CharLimit = 80
	e.notes = textarea.New()
	e.notes.Prompt = ""
	e.notes.Placeholder = "Notes (optional)"
	e.notes.ShowLineNumbers = false
	if existing {
		e.taskID = task.ID
		e.title.SetValue(task.Title)
		e.notes.SetValue(task.Notes)
		e.tags.SetValue(strings.Join(prefixedTags(task.Tags), " "))
		e.milestone.SetValue(task.Milestone)
	}
	e.setFocus(0)
	e.resize(m.width)
	return e
}

func mergeTags(primary, extra []string) []string {
	out := append([]string(nil), primary...)
	seen := map[string]bool{}
	for _, tag := range out {
		seen[tag] = true
	}
	for _, tag := range extra {
		if !seen[tag] {
			seen[tag] = true
			out = append(out, tag)
		}
	}
	return out
}

func prefixedTags(tags []string) []string {
	out := make([]string, len(tags))
	for i, tag := range tags {
		out[i] = "#" + tag
	}
	return out
}

func (e *TaskEditor) setFocus(focus int) {
	e.focus = clamp(focus, 0, 3)
	e.title.Blur()
	e.notes.Blur()
	e.tags.Blur()
	e.milestone.Blur()
	switch e.focus {
	case 0:
		e.title.Focus()
	case 1:
		e.notes.Focus()
	case 2:
		e.tags.Focus()
	case 3:
		e.milestone.Focus()
	}
}

func (e *TaskEditor) nextField(delta int) {
	e.setFocus((e.focus + delta + 4) % 4)
}

func (e *TaskEditor) resize(screenWidth int) {
	width := clamp(screenWidth-8, 28, 76)
	if screenWidth <= 0 {
		width = 40
	}
	e.width = width
	inner := max(10, width-4)
	e.title.SetWidth(inner)
	e.tags.SetWidth(inner)
	e.milestone.SetWidth(inner)
	e.notes.SetWidth(inner)
	e.notes.SetHeight(5)
}

func (m Model) updateTaskEditor(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	e := &m.editor
	switch key {
	case "esc":
		m.mode = ModeBoard
		return m, nil
	case "ctrl+s":
		return m.saveTaskEditor()
	case "tab":
		e.nextField(1)
		return m, nil
	case "shift+tab":
		e.nextField(-1)
		return m, nil
	case "enter":
		if e.focus != 1 {
			if e.focus == 3 {
				return m.saveTaskEditor()
			}
			e.nextField(1)
			return m, nil
		}
	}

	var cmd tea.Cmd
	switch e.focus {
	case 0:
		e.title, cmd = e.title.Update(msg)
		_, _, _, _, warnings := board.ParseTaskText(e.title.Value())
		if len(warnings) > 0 {
			e.warn = warnings[0]
		} else {
			e.warn = ""
		}
	case 1:
		e.notes, cmd = e.notes.Update(msg)
	case 2:
		e.tags, cmd = e.tags.Update(msg)
	case 3:
		e.milestone, cmd = e.milestone.Update(msg)
	}
	return m, cmd
}

func (m Model) saveTaskEditor() (tea.Model, tea.Cmd) {
	e := &m.editor
	title := strings.TrimSpace(e.title.Value())
	if title == "" {
		e.err = "Title is required"
		return m, nil
	}
	parsedTitle, parsedTags, parsedMilestone, _, warnings := board.ParseTaskText(title)
	tags := mergeTags(splitTags(e.tags.Value()), parsedTags)
	milestone := strings.TrimSpace(strings.TrimPrefix(e.milestone.Value(), "@"))
	if milestone == "" {
		milestone = parsedMilestone
	}
	notes := e.notes.Value()

	var (
		next board.Board
		err  error
		id   board.TaskID
		verb string
	)
	if e.taskID == "" {
		before := m.Board
		next, err = before.AddTask(e.column, title)
		if err != nil {
			e.err = err.Error()
			return m, nil
		}
		id = next.Columns[e.column].Tasks[len(next.Columns[e.column].Tasks)-1].ID
		next, err = next.UpdateTask(id, board.TaskPatch{
			Title:     &parsedTitle,
			Notes:     &notes,
			Tags:      &tags,
			Milestone: &milestone,
		})
		if err != nil {
			e.err = err.Error()
			return m, nil
		}
		m.commitFrom(before, next)
		verb = "Task added"
	} else {
		id = e.taskID
		next, err = m.Board.UpdateTask(id, board.TaskPatch{
			Title:     &parsedTitle,
			Notes:     &notes,
			Tags:      &tags,
			Milestone: &milestone,
		})
		if err != nil {
			e.err = err.Error()
			return m, nil
		}
		m.commit(next)
		verb = "Task saved"
	}

	m.mode = ModeBoard
	hidden := m.landOn(id)
	m.status = verb
	if hidden {
		m.status += " (hidden by filter)"
	}
	if len(warnings) > 0 {
		m.status += " · " + warnings[0]
	}
	return m, m.scheduleSave()
}

// ---------- text prompt ----------

type PromptKind int

const (
	promptColumn PromptKind = iota + 1
	promptColumnRename
	promptMilestoneAdd
	promptMilestoneRename
	promptMilestoneDue
	promptMilestoneDescription
)

type Prompt struct {
	kind   PromptKind
	title  string
	target string
	back   Mode
	input  textinput.Model
	err    string
}

func newPrompt(kind PromptKind, title, value string, back Mode, screenWidth int) Prompt {
	p := Prompt{kind: kind, title: title, back: back}
	p.input = textinput.New()
	p.input.Prompt = ""
	p.input.CharLimit = 80
	p.input.SetValue(value)
	p.input.Focus()
	p.resize(screenWidth)
	return p
}

func (p *Prompt) resize(screenWidth int) {
	width := clamp(screenWidth-12, 20, 60)
	if screenWidth <= 0 {
		width = 30
	}
	p.input.SetWidth(width)
}

func (m Model) updatePrompt(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc":
		m.mode = m.prompt.back
		return m, nil
	case "enter", "ctrl+s":
		return m.savePrompt()
	}
	var cmd tea.Cmd
	m.prompt.input, cmd = m.prompt.input.Update(msg)
	return m, cmd
}

func (m Model) savePrompt() (tea.Model, tea.Cmd) {
	p := &m.prompt
	value := strings.TrimSpace(p.input.Value())
	if value == "" && p.kind != promptMilestoneDue && p.kind != promptMilestoneDescription {
		p.err = "Name is required"
		return m, nil
	}

	var (
		next board.Board
		err  error
	)
	switch p.kind {
	case promptColumn:
		next, err = m.Board.AddColumn(value)
		if err != nil {
			p.err = err.Error()
			return m, nil
		}
		m.commit(next)
		m.mode = ModeBoard
		if index := m.Board.ColumnIndex(value); index >= 0 {
			m.column = index
			m.task = -1
			m.clampFocus()
		}
		m.status = "Column added: " + value
	case promptColumnRename:
		index := m.Board.ColumnIndex(p.target)
		if index < 0 {
			p.err = "Column no longer exists"
			return m, nil
		}
		next, err = m.Board.RenameColumn(index, value)
		if err != nil {
			p.err = err.Error()
			return m, nil
		}
		m.commit(next)
		m.mode = ModeBoard
		m.column = index
		m.clampFocus()
		m.status = "Column renamed: " + value
	case promptMilestoneAdd:
		next, err = m.Board.AddMilestone(board.Milestone{Name: value})
		if err != nil {
			p.err = err.Error()
			return m, nil
		}
		m.commit(next)
		m.mode = ModeMilestones
		m.miles.index = len(m.Board.Milestones) - 1
		m.status = "Milestone added: " + value
	case promptMilestoneDue, promptMilestoneDescription:
		var patch board.MilestonePatch
		if p.kind == promptMilestoneDue {
			patch.Due = &value
		} else {
			patch.Description = &value
		}
		next, err = m.Board.UpdateMilestone(p.target, patch)
		if err != nil {
			p.err = err.Error()
			return m, nil
		}
		m.commit(next)
		m.mode = ModeMilestones
		m.clampMilestonePanel()
		if p.kind == promptMilestoneDue {
			m.status = "Due date updated"
		} else {
			m.status = "Description updated"
		}
	case promptMilestoneRename:
		old := p.target
		next, err = m.Board.UpdateMilestone(old, board.MilestonePatch{Name: &value})
		if err != nil {
			p.err = err.Error()
			return m, nil
		}
		m.commit(next)
		if m.filter.Milestone == old {
			m.filter.Milestone = value
		}
		m.mode = ModeMilestones
		m.clampMilestonePanel()
		m.status = "Milestone renamed: " + value
	}
	return m, m.scheduleSave()
}

// ---------- filter panel ----------

type FilterPanel struct {
	text    textinput.Model
	focus   int
	tags    []string
	chosen  map[string]bool
	tagMode board.TagMode
	tagIdx  int

	milestones []string
	milIdx     int
	showDone   bool
	width      int
}

func newFilterPanel(m Model) FilterPanel {
	p := FilterPanel{
		tagMode:  m.filter.TagMode,
		showDone: m.filter.ShowDone,
		chosen:   map[string]bool{},
	}
	p.text = textinput.New()
	p.text.Prompt = ""
	p.text.Placeholder = "search text"
	p.text.SetValue(m.filter.Text)
	p.text.Focus()
	p.tags = m.Board.Tags(true)
	for _, tag := range m.filter.Tags {
		p.chosen[tag] = true
		for i, existing := range p.tags {
			if existing == tag {
				p.tagIdx = i
			}
		}
	}
	p.milestones = append([]string{"", board.NoMilestone}, milestoneNames(m.Board)...)
	p.milIdx = 0
	for i, name := range p.milestones {
		if name == m.filter.Milestone {
			p.milIdx = i
		}
	}
	p.resize(m.width)
	return p
}

func milestoneNames(b board.Board) []string {
	names := make([]string, 0, len(b.Milestones))
	for _, m := range b.Milestones {
		names = append(names, m.Name)
	}
	return names
}

func (p *FilterPanel) resize(screenWidth int) {
	width := clamp(screenWidth-12, 20, 60)
	if screenWidth <= 0 {
		width = 30
	}
	p.width = width
	p.text.SetWidth(width)
}

func (p FilterPanel) filter() board.Filter {
	f := board.Filter{
		Text:      p.text.Value(),
		TagMode:   p.tagMode,
		ShowDone:  p.showDone,
		Milestone: "",
	}
	if p.milIdx >= 0 && p.milIdx < len(p.milestones) {
		f.Milestone = p.milestones[p.milIdx]
	}
	for _, tag := range p.tags {
		if p.chosen[tag] {
			f.Tags = append(f.Tags, tag)
		}
	}
	return f
}

func (m *Model) applyFilterPanel() {
	m.filter = m.filterPane.filter()
	m.clampFocus()
}

func (m Model) updateFilterPanel(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	p := &m.filterPane

	switch key {
	case "esc":
		m.applyFilterPanel()
		m.mode = ModeBoard
		if m.filter.Active() {
			m.status = "Filter applied"
		} else {
			m.status = "Filter cleared"
		}
		return m, nil
	case "ctrl+r":
		p.text.SetValue("")
		p.chosen = map[string]bool{}
		p.tagMode = board.TagAny
		p.showDone = true
		p.milIdx = 0
		p.tagIdx = 0
		m.applyFilterPanel()
		return m, nil
	case "tab":
		p.focus = (p.focus + 1) % 4
		if p.focus == 0 {
			p.text.Focus()
		} else {
			p.text.Blur()
		}
		return m, nil
	case "shift+tab":
		p.focus = (p.focus + 3) % 4
		if p.focus == 0 {
			p.text.Focus()
		} else {
			p.text.Blur()
		}
		return m, nil
	}

	if p.focus == 0 {
		var cmd tea.Cmd
		p.text, cmd = p.text.Update(msg)
		m.applyFilterPanel()
		return m, cmd
	}

	switch p.focus {
	case 1: // tags
		switch key {
		case "j", "down":
			if len(p.tags) > 0 {
				p.tagIdx = clamp(p.tagIdx+1, 0, len(p.tags)-1)
			}
		case "k", "up":
			if len(p.tags) > 0 {
				p.tagIdx = clamp(p.tagIdx-1, 0, len(p.tags)-1)
			}
		case " ", "space", "enter":
			if len(p.tags) > 0 {
				tag := p.tags[p.tagIdx]
				p.chosen[tag] = !p.chosen[tag]
			}
		case "a":
			if p.tagMode == board.TagAny {
				p.tagMode = board.TagAll
			} else {
				p.tagMode = board.TagAny
			}
		case "c":
			p.chosen = map[string]bool{}
		}
	case 2: // milestone
		switch key {
		case "j", "down":
			p.milIdx = clamp(p.milIdx+1, 0, max(0, len(p.milestones)-1))
		case "k", "up":
			p.milIdx = clamp(p.milIdx-1, 0, max(0, len(p.milestones)-1))
		case " ", "space", "enter":
			// j/k already selects; space is a no-op affordance.
		}
	case 3: // options
		if key == " " || key == "space" || key == "enter" {
			p.showDone = !p.showDone
		}
	}
	m.applyFilterPanel()
	return m, nil
}

// ---------- milestones ----------

type MilestonePanel struct {
	index int
	err   string
}

func (m *Model) clampMilestonePanel() {
	if len(m.Board.Milestones) == 0 {
		m.miles.index = 0
		return
	}
	m.miles.index = clamp(m.miles.index, 0, len(m.Board.Milestones)-1)
}

func (m Model) updateMilestones(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc", "q":
		m.mode = ModeBoard
		return m, nil
	case "j", "down":
		if len(m.Board.Milestones) > 0 {
			m.miles.index = clamp(m.miles.index+1, 0, len(m.Board.Milestones)-1)
		}
	case "k", "up":
		if len(m.Board.Milestones) > 0 {
			m.miles.index = clamp(m.miles.index-1, 0, len(m.Board.Milestones)-1)
		}
	case "g":
		m.miles.index = 0
	case "G", "shift+g":
		if len(m.Board.Milestones) > 0 {
			m.miles.index = len(m.Board.Milestones) - 1
		}
	case "enter":
		if len(m.Board.Milestones) == 0 {
			m.status = "No milestones yet"
			return m, nil
		}
		return m.assignMilestone(m.Board.Milestones[m.miles.index].Name)
	case "c":
		return m.assignMilestone("")
	case "n":
		m.openPrompt(promptMilestoneAdd, "New milestone", "", ModeMilestones)
	case "e":
		if len(m.Board.Milestones) > 0 {
			m.milestonePrompt(promptMilestoneRename, "Rename milestone", m.Board.Milestones[m.miles.index].Name)
		}
	case "u":
		if len(m.Board.Milestones) > 0 {
			m.milestonePrompt(promptMilestoneDue, "Milestone due date", m.Board.Milestones[m.miles.index].Due)
		}
	case "i":
		if len(m.Board.Milestones) > 0 {
			m.milestonePrompt(promptMilestoneDescription, "Milestone description", m.Board.Milestones[m.miles.index].Description)
		}
	case "d":
		if len(m.Board.Milestones) == 0 {
			m.status = "No milestones yet"
			return m, nil
		}
		name := m.Board.Milestones[m.miles.index].Name
		m.confirm = Confirm{
			kind:      confirmDeleteMilestone,
			title:     "Delete milestone",
			message:   "Delete \"" + name + "\" and detach it from tasks?",
			milestone: name,
			back:      ModeMilestones,
		}
		m.mode = ModeConfirm
	}
	return m, nil
}

func (m *Model) milestonePrompt(kind PromptKind, title, value string) {
	name := m.Board.Milestones[m.miles.index].Name
	m.openPrompt(kind, title, value, ModeMilestones)
	m.prompt.target = name
}

func (m Model) assignMilestone(name string) (tea.Model, tea.Cmd) {
	task, ok := m.focusedTask()
	if !ok {
		m.status = "No task selected"
		m.mode = ModeBoard
		return m, nil
	}
	next, err := m.Board.UpdateTask(task.ID, board.TaskPatch{Milestone: &name})
	if err != nil {
		m.status = err.Error()
		return m, nil
	}
	m.commit(next)
	m.mode = ModeBoard
	m.landOn(task.ID)
	if name == "" {
		m.status = "Milestone cleared"
	} else {
		m.status = "Milestone " + name + " assigned"
	}
	return m, m.scheduleSave()
}

// ---------- archive view ----------

type ArchiveView struct {
	index     int
	searching bool
	search    textinput.Model
	width     int
}

func newArchiveView(screenWidth int) ArchiveView {
	a := ArchiveView{}
	a.search = textinput.New()
	a.search.Prompt = ""
	a.search.Placeholder = "search archive"
	a.resize(screenWidth)
	return a
}

func (a *ArchiveView) resize(screenWidth int) {
	width := clamp(screenWidth-12, 20, 60)
	if screenWidth <= 0 {
		width = 30
	}
	a.width = width
	a.search.SetWidth(width)
}

func (m Model) archiveVisible() []int {
	f := board.NewFilter()
	f.Text = m.archive.search.Value()
	indices := make([]int, 0, len(m.Board.Archive))
	for i, task := range m.Board.Archive {
		if f.MatchesArchive(task.Task) {
			indices = append(indices, i)
		}
	}
	return indices
}

func (m *Model) clampArchive() {
	visible := m.archiveVisible()
	if len(visible) == 0 {
		m.archive.index = 0
		return
	}
	m.archive.index = clamp(m.archive.index, 0, len(visible)-1)
}

func (m Model) updateArchive(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if m.archive.searching {
		switch key {
		case "esc", "enter":
			m.archive.searching = false
			m.archive.search.Blur()
			m.clampArchive()
			return m, nil
		}
		var cmd tea.Cmd
		m.archive.search, cmd = m.archive.search.Update(msg)
		m.clampArchive()
		return m, cmd
	}

	switch key {
	case "esc", "q":
		m.mode = ModeBoard
	case "/":
		m.archive.searching = true
		m.archive.search.Focus()
	case "j", "down":
		visible := m.archiveVisible()
		if len(visible) > 0 {
			m.archive.index = clamp(m.archive.index+1, 0, len(visible)-1)
		}
	case "k", "up":
		visible := m.archiveVisible()
		if len(visible) > 0 {
			m.archive.index = clamp(m.archive.index-1, 0, len(visible)-1)
		}
	case "g":
		m.archive.index = 0
	case "G", "shift+g":
		if visible := m.archiveVisible(); len(visible) > 0 {
			m.archive.index = len(visible) - 1
		}
	case "u", "U":
		visible := m.archiveVisible()
		if len(visible) == 0 {
			m.status = "Archive is empty"
			return m, nil
		}
		task := m.Board.Archive[visible[m.archive.index]]
		var (
			next board.Board
			err  error
		)
		if key == "u" {
			next, err = m.Board.RestoreTask(task.ID)
		} else {
			next, err = m.Board.ReopenTask(task.ID)
		}
		if err != nil {
			m.status = err.Error()
			return m, nil
		}
		m.commit(next)
		m.clampArchive()
		if key == "u" {
			m.status = "Restored " + task.Title + " to Done"
		} else {
			m.status = "Reopened " + task.Title
		}
		return m, m.scheduleSave()
	}
	return m, nil
}

// ---------- confirmation ----------

type ConfirmKind int

const (
	confirmQuit ConfirmKind = iota + 1
	confirmDeleteTask
	confirmArchiveAll
	confirmDeleteMilestone
	confirmDeleteColumn
	confirmArchiveDeleteDone
	confirmConflict
)

type Confirm struct {
	kind      ConfirmKind
	title     string
	message   string
	taskID    board.TaskID
	milestone string
	column    int
	back      Mode
}

func (m Model) updateConfirm(key string) (tea.Model, tea.Cmd) {
	c := m.confirm
	switch c.kind {
	case confirmConflict:
		switch key {
		case "r":
			m.mode = ModeBoard
			m.reloadFile()
			return m, nil
		case "o":
			m.mode = ModeBoard
			return m.saveNow(true, true)
		case "esc":
			m.mode = ModeBoard
			m.quitAfterSave = false
			m.status = "Save cancelled"
			return m, nil
		}
		return m, nil
	case confirmQuit:
		switch key {
		case "s", "y", "enter":
			m.mode = c.back
			if m.path == "" {
				m.dirty = false
				return m, tea.Quit
			}
			m.quitAfterSave = true
			return m.saveNow(false, true)
		case "d":
			return m, tea.Quit
		case "esc", "n":
			m.mode = c.back
			m.status = "Quit cancelled"
		}
		return m, nil
	}

	switch key {
	case "y", "enter":
		switch c.kind {
		case confirmDeleteTask:
			next, err := m.Board.DeleteTask(c.taskID)
			if err == nil {
				m.commit(next)
				m.clampFocus()
				m.status = "Task deleted"
			} else {
				m.status = err.Error()
			}
			m.mode = ModeBoard
		case confirmArchiveAll:
			next, n, err := m.Board.ArchiveAllDone(time.Now().UTC())
			if err != nil {
				m.status = err.Error()
			} else {
				m.commit(next)
				m.clampFocus()
				m.status = fmt.Sprintf("Archived %d tasks", n)
			}
			m.mode = ModeBoard
		case confirmDeleteColumn:
			next, err := m.Board.DeleteColumn(c.column)
			if err != nil {
				m.status = err.Error()
			} else {
				m.commit(next)
				m.clampFocus()
				m.status = "Column deleted"
			}
			m.mode = c.back
		case confirmArchiveDeleteDone:
			next, n, err := m.Board.ArchiveAllDone(time.Now().UTC())
			if err != nil {
				m.status = err.Error()
				break
			}
			next, err = next.DeleteColumn(c.column)
			if err != nil {
				m.status = err.Error()
				break
			}
			m.commit(next)
			m.clampFocus()
			m.status = fmt.Sprintf("Archived %d tasks and deleted the column", n)
			m.mode = c.back
		case confirmDeleteMilestone:
			next, err := m.Board.DeleteMilestone(c.milestone)
			if err != nil {
				m.status = err.Error()
			} else {
				m.commit(next)
				if m.filter.Milestone == c.milestone {
					m.filter.Milestone = ""
					m.clampFocus()
				}
				m.status = "Milestone deleted"
			}
			m.mode = c.back
			m.clampMilestonePanel()
		}
		cmd := m.scheduleSave()
		return m, cmd
	case "n", "esc":
		m.mode = c.back
		if m.mode == ModeMilestones {
			m.clampMilestonePanel()
		}
		return m, nil
	}
	return m, nil
}
