package tui

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/kron4eg/tbdbub/internal/board"
)

func startedBoard(t *testing.T) Model {
	t.Helper()
	b := board.NewBoard("test")
	b, err := b.AddTask(0, "one #ui @v1")
	if err != nil {
		t.Fatal(err)
	}
	b, err = b.AddTask(0, "two #ui")
	if err != nil {
		t.Fatal(err)
	}
	return New(b)
}

func TestCountMove(t *testing.T) {
	m := New(DemoBoard())
	m = update(m, press("2"))
	m = update(m, press("L"))

	if m.column != 2 {
		t.Fatalf("column = %d, want Done", m.column)
	}
	ci, ti, ok := m.Board.TaskByID("t7k2")
	if !ok || ci != 2 {
		t.Fatalf("task in column %d, %v", ci, ok)
	}
	if !m.Board.Columns[ci].Tasks[ti].Done {
		t.Fatal("task moved into Done is not checked")
	}
	if m.status != "Moved to Done" {
		t.Fatalf("status = %q", m.status)
	}

	m = update(m, press("u"))
	if ci, _, _ := m.Board.TaskByID("t7k2"); ci != 0 {
		t.Fatalf("after undo task in column %d", ci)
	}
	m = update(m, press("ctrl+r"))
	if ci, _, _ := m.Board.TaskByID("t7k2"); ci != 2 {
		t.Fatalf("after redo task in column %d", ci)
	}
}

func TestMoveRightCreatesDone(t *testing.T) {
	m := startedBoard(t)
	m = update(m, press("L"))
	if len(m.Board.Columns) != 2 || m.Board.DoneColumn != "Done" {
		t.Fatalf("columns = %+v done=%q", m.Board.Columns, m.Board.DoneColumn)
	}
	if !m.Board.Columns[1].Tasks[0].Done {
		t.Fatal("task is not done")
	}
	if m.column != 1 || m.task != 0 {
		t.Fatalf("focus = (%d, %d)", m.column, m.task)
	}
}

func TestToggleDoneAndReopen(t *testing.T) {
	m := New(DemoBoard())
	task := m.Board.Columns[0].Tasks[0]

	m = update(m, press(" "))
	if ci, _, _ := m.Board.TaskByID(task.ID); ci != 2 {
		t.Fatalf("completed task in column %d", ci)
	}
	if m.column != 2 {
		t.Fatalf("cursor column = %d, want Done", m.column)
	}
	if !m.Board.Columns[2].Tasks[len(m.Board.Columns[2].Tasks)-1].Done {
		t.Fatal("task not checked")
	}

	m = update(m, press(" "))
	ci, _, _ := m.Board.TaskByID(task.ID)
	if ci != 0 {
		t.Fatalf("reopened task in column %d", ci)
	}
	if m.Board.Columns[0].Tasks[len(m.Board.Columns[0].Tasks)-1].Done {
		t.Fatal("reopened task still checked")
	}
}

func TestReorder(t *testing.T) {
	m := startedBoard(t)
	m = update(m, press("J"))
	if got := m.Board.Columns[0].Tasks[1].Title; got != "one" {
		t.Fatalf("after J tasks = %+v", m.Board.Columns[0].Tasks)
	}
	m = update(m, press("K"))
	if got := m.Board.Columns[0].Tasks[0].Title; got != "one" {
		t.Fatalf("after K tasks = %+v", m.Board.Columns[0].Tasks)
	}
}

func TestGrabMoveAndCancel(t *testing.T) {
	m := New(DemoBoard())
	m = update(m, press("m"))
	if m.mode != ModeGrab {
		t.Fatal("grab mode did not open")
	}
	m = update(m, press("l"))
	m = update(m, press("j"))
	m = update(m, press("enter"))
	if m.mode != ModeBoard {
		t.Fatalf("mode = %v", m.mode)
	}
	ci, ti, ok := m.Board.TaskByID("t7k2")
	if !ok || ci != 1 || ti != 1 {
		t.Fatalf("dropped at (%d, %d), %v", ci, ti, ok)
	}

	before := serialize(t, m.Board)
	version := m.version
	undo := len(m.undo)
	m = update(m, press("m"))
	m = update(m, press("l"))
	m = update(m, press("j"))
	m = update(m, pressEsc())
	if m.mode != ModeBoard {
		t.Fatalf("mode after cancel = %v", m.mode)
	}
	if after := serialize(t, m.Board); after != before {
		t.Fatal("cancelled grab changed the board")
	}
	if m.version != version || len(m.undo) != undo {
		t.Fatal("cancelled grab created history")
	}
}

func TestGrabNoOpDrop(t *testing.T) {
	m := New(DemoBoard())
	before := serialize(t, m.Board)
	m = update(m, press("m"))
	m = update(m, press("enter"))
	if m.mode != ModeBoard {
		t.Fatal("grab did not close")
	}
	if after := serialize(t, m.Board); after != before {
		t.Fatal("no-op drop changed the board")
	}
	if m.version != 0 || len(m.undo) != 0 {
		t.Fatal("no-op drop created history")
	}
}

func TestTaskEditorAdd(t *testing.T) {
	m := startedBoard(t)
	m = update(m, press("n"))
	if m.mode != ModeTaskEditor {
		t.Fatal("editor did not open")
	}
	m.editor.title.SetValue("three #docs @v2")
	m.editor.notes.SetValue("details")
	m = update(m, press("ctrl+s"))
	if m.mode != ModeBoard {
		t.Fatalf("mode = %v", m.mode)
	}
	tasks := m.Board.Columns[0].Tasks
	last := tasks[len(tasks)-1]
	if last.Title != "three" || last.Notes != "  details" || last.Milestone != "v2" {
		t.Fatalf("task = %+v", last)
	}
	if len(last.Tags) != 1 || last.Tags[0] != "docs" {
		t.Fatalf("tags = %v", last.Tags)
	}
	if m.status != "Task added" {
		t.Fatalf("status = %q", m.status)
	}
}

func TestTaskEditorEditAndValidation(t *testing.T) {
	m := startedBoard(t)
	m = update(m, press("e"))
	m.editor.title.SetValue("")
	m = update(m, press("ctrl+s"))
	if m.mode != ModeTaskEditor || m.editor.err == "" {
		t.Fatal("empty title was not rejected")
	}
	m.editor.title.SetValue("renamed")
	m.editor.tags.SetValue("a,b")
	m.editor.milestone.SetValue("@v3")
	m = update(m, press("ctrl+s"))
	if m.mode != ModeBoard {
		t.Fatalf("mode = %v", m.mode)
	}
	task := m.Board.Columns[0].Tasks[0]
	if task.Title != "renamed" || strings.Join(task.Tags, ",") != "a,b" || task.Milestone != "v3" {
		t.Fatalf("task = %+v", task)
	}
}

func TestDeleteConfirm(t *testing.T) {
	m := startedBoard(t)
	m = update(m, press("d"))
	if m.mode != ModeConfirm {
		t.Fatal("confirm did not open")
	}
	m = update(m, press("n"))
	if m.mode != ModeBoard || len(m.Board.Columns[0].Tasks) != 2 {
		t.Fatal("cancel deleted the task")
	}
	m = update(m, press("d"))
	m = update(m, press("y"))
	if len(m.Board.Columns[0].Tasks) != 1 {
		t.Fatal("task was not deleted")
	}
}

func TestNewColumn(t *testing.T) {
	m := startedBoard(t)
	m = update(m, press("N"))
	if m.mode != ModeColumnEditor {
		t.Fatal("prompt did not open")
	}
	m.prompt.input.SetValue("Doing")
	m = update(m, press("enter"))
	if m.mode != ModeBoard {
		t.Fatalf("mode = %v", m.mode)
	}
	if got := len(m.Board.Columns); got != 2 {
		t.Fatalf("columns = %d", got)
	}
	if m.Board.Columns[1].Name != "Doing" {
		t.Fatalf("new column = %q", m.Board.Columns[1].Name)
	}
	if m.column != 1 {
		t.Fatalf("focus column = %d", m.column)
	}
}

func TestFilterPanelAndChips(t *testing.T) {
	m := New(DemoBoard())
	m = update(m, press("f"))
	if m.mode != ModeFilter {
		t.Fatal("filter did not open")
	}
	m.filterPane.text.SetValue("tiles")
	m = update(m, press("tab")) // tags
	// Toggle the "design" tag.
	for i, tag := range m.filterPane.tags {
		if tag == "design" {
			m.filterPane.tagIdx = i
		}
	}
	m = update(m, press(" "))
	m = update(m, pressEsc())

	if !m.filter.Active() || m.filter.Text != "tiles" {
		t.Fatalf("filter = %+v", m.filter)
	}
	if len(m.filter.Tags) != 1 || m.filter.Tags[0] != "design" {
		t.Fatalf("tags = %v", m.filter.Tags)
	}
	content := m.View().Content
	if !strings.Contains(content, "#design") || !strings.Contains(content, "/tiles") {
		t.Fatal("filter chips are not rendered")
	}

	m = update(m, pressEsc())
	if m.filter.Active() {
		t.Fatal("esc did not clear the filter")
	}
}

func TestFilterSkipsHiddenTasks(t *testing.T) {
	m := New(DemoBoard())
	m.filter = board.Filter{Text: "permits", ShowDone: true}
	m.clampFocus()
	if got := m.Board.Columns[0].Tasks[m.task].Title; got != "Ask about permits" {
		t.Fatalf("focused = %q", got)
	}
	m = update(m, press("j"))
	if got := m.Board.Columns[0].Tasks[m.task].Title; got != "Ask about permits" {
		t.Fatalf("j moved to hidden task %q", got)
	}
	m = update(m, press("l"))
	if m.column != 1 {
		t.Fatalf("column = %d", m.column)
	}
}

func TestFilterHidesMovedTask(t *testing.T) {
	m := New(DemoBoard())
	m.filter = board.Filter{ShowDone: false}
	m.clampFocus()
	m = update(m, press("L"))
	m = update(m, press("L"))
	if !strings.Contains(m.status, "hidden by filter") {
		t.Fatalf("status = %q", m.status)
	}
	if m.column != 2 {
		t.Fatalf("column = %d", m.column)
	}
}

func TestArchiveViewRestoreAndReopen(t *testing.T) {
	m := New(DemoBoard())
	m = update(m, press("l"))
	m = update(m, press("l"))
	m = update(m, press("a"))
	if len(m.Board.Archive) != 1 {
		t.Fatalf("archive = %+v", m.Board.Archive)
	}
	m = update(m, press("V"))
	if m.mode != ModeArchive {
		t.Fatal("archive view did not open")
	}
	m = update(m, press("u"))
	if len(m.Board.Archive) != 0 {
		t.Fatal("restore did not remove the archived task")
	}
	if len(m.Board.Columns[2].Tasks) != 1 || !m.Board.Columns[2].Tasks[0].Done {
		t.Fatal("restore did not put the task back in Done")
	}

	m = update(m, press("a"))
	m = update(m, press("U"))
	if len(m.Board.Archive) != 0 {
		t.Fatal("reopen did not remove the archived task")
	}
	if got := m.Board.Columns[m.Board.CaptureIndex()].Tasks; len(got) == 0 || got[len(got)-1].Done {
		t.Fatal("reopen did not put the task in the capture column")
	}
	m = update(m, pressEsc())
	if m.mode != ModeBoard {
		t.Fatalf("mode = %v", m.mode)
	}
}

func TestArchiveAllConfirm(t *testing.T) {
	m := New(DemoBoard())
	m = update(m, press("A"))
	if m.mode != ModeConfirm {
		t.Fatal("archive all did not ask for confirmation")
	}
	m = update(m, press("y"))
	if len(m.Board.Archive) != 1 {
		t.Fatalf("archive = %+v", m.Board.Archive)
	}
	if len(m.Board.Columns[2].Tasks) != 0 {
		t.Fatal("Done column not emptied")
	}
}

func TestMilestonesAssignAddRenameDelete(t *testing.T) {
	m := startedBoard(t)
	m = update(m, press("M"))
	if m.mode != ModeMilestones {
		t.Fatal("milestones did not open")
	}
	m.miles.index = 0
	m = update(m, press("enter"))
	if m.mode != ModeBoard {
		t.Fatalf("mode = %v", m.mode)
	}
	if got := m.Board.Columns[0].Tasks[0].Milestone; got != "v1" {
		t.Fatalf("milestone = %q", got)
	}

	m = update(m, press("M"))
	m = update(m, press("n"))
	if m.mode != ModeColumnEditor {
		t.Fatal("milestone prompt did not open")
	}
	m.prompt.input.SetValue("v9")
	m = update(m, press("enter"))
	if m.mode != ModeMilestones {
		t.Fatalf("mode = %v", m.mode)
	}
	if _, ok := m.Board.MilestoneByName("v9"); !ok {
		t.Fatal("milestone was not added")
	}

	m.miles.index = len(m.Board.Milestones) - 1
	m = update(m, press("e"))
	m.prompt.input.SetValue("v10")
	m = update(m, press("enter"))
	if _, ok := m.Board.MilestoneByName("v10"); !ok {
		t.Fatal("milestone was not renamed")
	}

	m.miles.index = len(m.Board.Milestones) - 1
	m = update(m, press("d"))
	m = update(m, press("y"))
	if _, ok := m.Board.MilestoneByName("v10"); ok {
		t.Fatal("milestone was not deleted")
	}
	m = update(m, pressEsc())
}

func TestModeExclusivity(t *testing.T) {
	m := startedBoard(t)
	before := serialize(t, m.Board)
	m = update(m, press("n"))
	m = update(m, press("H"))
	if m.mode != ModeTaskEditor {
		t.Fatal("H closed the editor")
	}
	if serialize(t, m.Board) != before {
		t.Fatal("H moved a task while the editor was open")
	}

	m = update(m, pressEsc())
	m = update(m, press("f"))
	m = update(m, press("H"))
	if m.filterPane.text.Value() != "H" {
		t.Fatalf("filter text = %q", m.filterPane.text.Value())
	}
	if serialize(t, m.Board) != before {
		t.Fatal("H moved a task while the filter was open")
	}
}

func TestSavePipeline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "board.md")
	b := board.NewBoard("saved")
	b, _ = b.AddTask(0, "one")
	b = b.EnsureDoneColumn()
	b, _ = b.AddTask(0, "two")

	var initial bytes.Buffer
	if err := board.Write(&initial, b); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, initial.Bytes(), 0o640); err != nil {
		t.Fatal(err)
	}
	m := NewFile(b, path, hashBytes(initial.Bytes()), nil)

	next, cmd := m.Update(press("L"))
	m = next.(Model)
	if cmd == nil {
		t.Fatal("mutation did not schedule a save")
	}
	if !m.dirty {
		t.Fatal("mutation did not mark the board dirty")
	}

	next, cmd = m.Update(cmd())
	m = next.(Model)
	if cmd == nil {
		t.Fatal("save tick did not start a write")
	}
	next, cmd = m.Update(cmd())
	m = next.(Model)
	if cmd != nil {
		t.Log("unexpected follow-up command")
	}
	if m.dirty {
		t.Fatal("save did not clear the dirty flag")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte("## Done")) {
		t.Fatalf("saved file:\n%s", data)
	}
	if !strings.Contains(string(data), "- [x] one") && !strings.Contains(string(data), "- [x] two") {
		t.Fatalf("saved file has no done task:\n%s", data)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}

	reloaded, _, _, err := loadFile(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	var moved string
	for _, col := range reloaded.Columns {
		if col.Name != "Done" {
			continue
		}
		if len(col.Tasks) > 0 {
			moved = col.Tasks[0].Title
		}
	}
	if moved != "one" {
		t.Fatalf("move did not survive reload: done tasks = %+v", reloaded.Columns)
	}
}

func TestAutosaveCoalescing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "board.md")
	b := board.NewBoard("saved")
	b, _ = b.AddTask(0, "one")
	b, _ = b.AddTask(0, "two")
	var initial bytes.Buffer
	if err := board.Write(&initial, b); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, initial.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	m := NewFile(b, path, hashBytes(initial.Bytes()), nil)

	next, first := m.Update(press("J"))
	m = next.(Model)
	next, second := m.Update(press("K"))
	m = next.(Model)
	if first == nil || second == nil {
		t.Fatal("mutations did not schedule saves")
	}

	next, cmd := m.Update(first())
	m = next.(Model)
	if cmd != nil {
		t.Fatal("stale save tick was not ignored")
	}
	next, cmd = m.Update(second())
	m = next.(Model)
	if cmd == nil {
		t.Fatal("current save tick did not write")
	}
}

func TestConflictDetection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "board.md")
	b := board.NewBoard("conflict")
	b, _ = b.AddTask(0, "one")
	var initial bytes.Buffer
	if err := board.Write(&initial, b); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, initial.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	m := NewFile(b, path, hashBytes(initial.Bytes()), nil)

	m = update(m, press("L"))
	if err := os.WriteFile(path, []byte("external edit"), 0o644); err != nil {
		t.Fatal(err)
	}
	next, cmd := m.saveNow(false, false)
	m = next.(Model)
	if m.mode != ModeConfirm || m.confirm.kind != confirmConflict || cmd != nil {
		t.Fatalf("mode=%v kind=%v cmd=%v", m.mode, m.confirm.kind, cmd)
	}

	next, cmd = m.updateConfirm("r")
	m = next.(Model)
	if m.mode != ModeBoard || m.dirty {
		t.Fatalf("reload mode=%v dirty=%v", m.mode, m.dirty)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "external edit" {
		t.Fatal("reload overwrote the external edit")
	}

	m = update(m, press("L"))
	next, _ = m.saveNow(false, false)
	m = next.(Model)
	if m.confirm.kind != confirmConflict {
		t.Fatal("second conflict not detected")
	}
	next, cmd = m.updateConfirm("o")
	m = next.(Model)
	if cmd == nil {
		t.Fatal("overwrite did not write")
	}
	next, _ = m.Update(cmd())
	m = next.(Model)
	if m.dirty {
		t.Fatal("overwrite left the board dirty")
	}
	data, _ = os.ReadFile(path)
	if string(data) == "external edit" {
		t.Fatal("overwrite did not replace the file")
	}
}

func TestQuitConfirm(t *testing.T) {
	m := startedBoard(t)
	m = update(m, press("L"))
	next, cmd := m.updateBoard(press("q"))
	m = next.(Model)
	if m.mode != ModeConfirm || cmd != nil {
		t.Fatalf("quit mode=%v cmd=%v", m.mode, cmd)
	}
	next, cmd = m.updateConfirm("d")
	if cmd == nil {
		t.Fatal("discard did not quit")
	}

	m = startedBoard(t)
	next, cmd = m.updateBoard(press("q"))
	if cmd == nil {
		t.Fatal("clean quit did not quit")
	}
	_ = next
}

func TestExactFrameAllModes(t *testing.T) {
	sizes := []struct{ w, h int }{
		{80, 24}, {120, 40}, {40, 12},
	}
	setups := []struct {
		name string
		open func(Model) Model
	}{
		{"board", func(m Model) Model { return m }},
		{"help", func(m Model) Model { return update(m, press("?")) }},
		{"grab", func(m Model) Model { return update(m, press("m")) }},
		{"editor", func(m Model) Model { return update(m, press("n")) }},
		{"filter", func(m Model) Model { return update(m, press("f")) }},
		{"milestones", func(m Model) Model { return update(m, press("M")) }},
		{"archive", func(m Model) Model { return update(m, press("V")) }},
		{"confirm", func(m Model) Model { return update(m, press("d")) }},
		{"prompt", func(m Model) Model { return update(m, press("N")) }},
	}
	for _, size := range sizes {
		for _, setup := range setups {
			t.Run(setup.name+"-"+itoa(size.w)+"x"+itoa(size.h), func(t *testing.T) {
				m := New(DemoBoard())
				m = update(m, tea.WindowSizeMsg{Width: size.w, Height: size.h})
				m = setup.open(m)
				content := m.View().Content
				lines := strings.Split(content, "\n")
				if len(lines) != size.h {
					t.Fatalf("height = %d, want %d", len(lines), size.h)
				}
				for i, line := range lines {
					if got := lipglossWidth(line); got != size.w {
						t.Fatalf("line %d width = %d, want %d: %q", i, got, size.w, line)
					}
				}
			})
		}
	}
}

func TestNarrowCarousel(t *testing.T) {
	m := New(DemoBoard())
	m = update(m, tea.WindowSizeMsg{Width: 60, Height: 20})
	content := m.View().Content
	if !strings.Contains(content, "Replace faucet") {
		t.Fatal("focused column is not rendered")
	}
	if strings.Contains(content, "Paint hallway") {
		t.Fatal("carousel rendered a non-focused column")
	}
	m = update(m, press("l"))
	content = m.View().Content
	if !strings.Contains(content, "Paint hallway") {
		t.Fatal("carousel did not switch columns")
	}
	if strings.Contains(content, "Replace faucet") {
		t.Fatal("carousel still renders the previous column")
	}
}

func TestWarningsShown(t *testing.T) {
	m := NewFile(DemoBoard(), "", "", []board.Warning{{Line: 3, Message: "extra milestone"}})
	content := m.View().Content
	if !strings.Contains(content, "warnings") {
		t.Fatal("warning count is not rendered")
	}
	if !strings.Contains(m.status, "line 3") {
		t.Fatalf("status = %q", m.status)
	}
}

func TestEmptyBoardAllModes(t *testing.T) {
	for _, b := range []board.Board{board.NewBoard("empty"), {}} {
		m := New(b)
		m = update(m, tea.WindowSizeMsg{Width: 80, Height: 24})
		for _, key := range []string{"j", "l", "G", "H", "L", "m", " ", "f", "V", "M"} {
			m = update(m, press(key))
			m = update(m, pressEsc())
		}
		if m.View().Content == "" {
			t.Fatal("empty board rendered nothing")
		}
	}
}

func serialize(t *testing.T, b board.Board) string {
	t.Helper()
	var buf bytes.Buffer
	if err := board.Write(&buf, b); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

func TestColumnRenameAndDelete(t *testing.T) {
	m := startedBoard(t)
	m = update(m, press("N"))
	m.prompt.input.SetValue("Doing")
	m = update(m, press("enter"))

	m = update(m, press("r"))
	if m.mode != ModeColumnEditor || m.prompt.input.Value() != "Doing" {
		t.Fatalf("rename prompt: mode=%v value=%q", m.mode, m.prompt.input.Value())
	}
	m.prompt.input.SetValue("Next")
	m = update(m, press("enter"))
	if m.mode != ModeBoard || m.Board.Columns[1].Name != "Next" {
		t.Fatalf("columns = %+v", m.Board.Columns)
	}

	m = update(m, press("D"))
	if m.mode != ModeConfirm {
		t.Fatal("delete column did not ask for confirmation")
	}
	m = update(m, press("y"))
	if len(m.Board.Columns) != 1 || m.Board.Columns[0].Name != "Backlog" {
		t.Fatalf("columns = %+v", m.Board.Columns)
	}

	m = update(m, press("D"))
	if m.mode != ModeBoard || !strings.Contains(m.status, "last column") {
		t.Fatalf("mode=%v status=%q", m.mode, m.status)
	}
}

func TestMilestoneMetadata(t *testing.T) {
	m := startedBoard(t)
	m = update(m, press("M"))
	m.miles.index = 0
	m = update(m, press("u"))
	if m.mode != ModeColumnEditor {
		t.Fatal("due prompt did not open")
	}
	m.prompt.input.SetValue("2026-12-31")
	m = update(m, press("enter"))
	ms, _ := m.Board.MilestoneByName("v1")
	if ms.Due != "2026-12-31" {
		t.Fatalf("due = %q", ms.Due)
	}

	m.miles.index = 0
	m = update(m, press("i"))
	m.prompt.input.SetValue("Release milestone")
	m = update(m, press("enter"))
	ms, _ = m.Board.MilestoneByName("v1")
	if ms.Description != "Release milestone" {
		t.Fatalf("description = %q", ms.Description)
	}
}

func TestDeleteDoneColumnArchivesFirst(t *testing.T) {
	m := New(DemoBoard())
	m = update(m, press("l"))
	m = update(m, press("l"))
	m = update(m, press("D"))
	if m.confirm.kind != confirmArchiveDeleteDone {
		t.Fatalf("kind = %v", m.confirm.kind)
	}
	m = update(m, press("y"))
	if len(m.Board.Columns) != 2 {
		t.Fatalf("columns = %+v", m.Board.Columns)
	}
	if len(m.Board.Archive) != 1 || m.Board.Archive[0].Title != "Measure cabinets" {
		t.Fatalf("archive = %+v", m.Board.Archive)
	}
	if m.Board.DoneColumn != "" {
		t.Fatalf("done column = %q", m.Board.DoneColumn)
	}
}

func TestHelpIsGlobal(t *testing.T) {
	m := startedBoard(t)
	m = update(m, press("n"))
	m = update(m, press("?"))
	if m.mode != ModeHelp {
		t.Fatalf("mode = %v, want help", m.mode)
	}
	m = update(m, pressEsc())
	if m.mode != ModeTaskEditor {
		t.Fatalf("mode = %v, want editor", m.mode)
	}
}

func TestColumnCursorIsRemembered(t *testing.T) {
	m := New(DemoBoard())
	m = update(m, press("G"))
	if m.task != 6 {
		t.Fatalf("task = %d", m.task)
	}
	m = update(m, press("l"))
	m = update(m, press("h"))
	if m.task != 6 {
		t.Fatalf("returned to task %d, want 6", m.task)
	}
}
