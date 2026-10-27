package board

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestAddTask(t *testing.T) {
	b := NewBoard("x")
	next, err := b.AddTask(0, "Write docs #docs @v1")
	if err != nil {
		t.Fatalf("AddTask: %v", err)
	}
	if len(next.Columns[0].Tasks) != 1 {
		t.Fatalf("tasks = %+v", next.Columns[0].Tasks)
	}
	task := next.Columns[0].Tasks[0]
	if task.Title != "Write docs" || len(task.Tags) != 1 || task.Milestone != "v1" {
		t.Fatalf("task = %+v", task)
	}
	if task.ID == "" || !validID(task.ID) {
		t.Fatalf("id = %q", task.ID)
	}
	if _, ok := next.MilestoneByName("v1"); !ok {
		t.Fatal("inline milestone was not registered")
	}
	if _, err := b.AddTask(0, "   "); !errors.Is(err, ErrEmptyTitle) {
		t.Fatalf("empty title error = %v", err)
	}
	if _, err := b.AddTask(5, "x"); !errors.Is(err, ErrColumnNotFound) {
		t.Fatalf("bad column error = %v", err)
	}
	if len(b.Columns[0].Tasks) != 0 {
		t.Fatal("AddTask mutated the original board")
	}
}

func TestUpdateTask(t *testing.T) {
	b := NewBoard("x")
	b, _ = b.AddTask(0, "one #old @v1")
	id := b.Columns[0].Tasks[0].ID

	title := "two #new @v2"
	notes := "line one\n  line two"
	tags := []string{"#explicit", "explicit", "other"}
	milestone := "@v3"
	next, err := b.UpdateTask(id, TaskPatch{
		Title:     &title,
		Notes:     &notes,
		Tags:      &tags,
		Milestone: &milestone,
	})
	if err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}
	task := next.Columns[0].Tasks[0]
	if task.Title != "two" || task.Notes != "  line one\n  line two" {
		t.Fatalf("task = %+v", task)
	}
	if strings.Join(task.Tags, ",") != "explicit,other" {
		t.Fatalf("tags = %v", task.Tags)
	}
	if task.Milestone != "v3" {
		t.Fatalf("milestone = %q", task.Milestone)
	}
	if _, ok := next.MilestoneByName("v3"); !ok {
		t.Fatal("milestone was not registered")
	}
	if _, err := b.UpdateTask("nope", TaskPatch{}); !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("missing task error = %v", err)
	}
}

func TestDeleteTask(t *testing.T) {
	b := NewBoard("x")
	b, _ = b.AddTask(0, "one")
	b, _ = b.AddTask(0, "two")
	id := b.Columns[0].Tasks[0].ID
	next, err := b.DeleteTask(id)
	if err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	if len(next.Columns[0].Tasks) != 1 || next.Columns[0].Tasks[0].Title != "two" {
		t.Fatalf("tasks = %+v", next.Columns[0].Tasks)
	}
	if len(b.Columns[0].Tasks) != 2 {
		t.Fatal("DeleteTask mutated the original board")
	}
}

func TestToggleDoneCreatesDone(t *testing.T) {
	b := NewBoard("x")
	b, _ = b.AddTask(0, "one")
	id := b.Columns[0].Tasks[0].ID

	next, err := b.ToggleDone(id)
	if err != nil {
		t.Fatalf("ToggleDone: %v", err)
	}
	if len(next.Columns) != 2 {
		t.Fatalf("columns = %d, want 2", len(next.Columns))
	}
	if next.DoneColumn != "Done" || next.Columns[1].Name != "Done" {
		t.Fatalf("done column = %q / %+v", next.DoneColumn, next.Columns)
	}
	if !next.Columns[1].Tasks[0].Done {
		t.Fatal("completed task is not checked")
	}
	if len(next.Columns[0].Tasks) != 0 {
		t.Fatal("completed task stayed in the source column")
	}

	back, err := next.ToggleDone(id)
	if err != nil {
		t.Fatalf("ToggleDone back: %v", err)
	}
	if len(back.Columns[1].Tasks) != 0 {
		t.Fatal("reopened task stayed in Done")
	}
	if back.Columns[back.CaptureIndex()].Tasks[0].Done {
		t.Fatal("reopened task is still checked")
	}
}

func TestToggleDoneNamedDoneWithoutFrontmatter(t *testing.T) {
	b := Board{
		Title:         "x",
		CaptureColumn: "Backlog",
		Columns: []Column{
			{Name: "Backlog", Tasks: []Task{{ID: "t1ab", Title: "one"}}},
			{Name: "Doing"},
			{Name: "Done"},
		},
	}
	next, err := b.ToggleDone("t1ab")
	if err != nil {
		t.Fatalf("ToggleDone: %v", err)
	}
	if next.DoneColumn != "Done" {
		t.Fatalf("done column = %q", next.DoneColumn)
	}
	if len(next.Columns[2].Tasks) != 1 {
		t.Fatal("task not moved to Done")
	}
}

func TestEnsureDoneColumn(t *testing.T) {
	b := NewBoard("x")
	next := b.EnsureDoneColumn()
	if len(next.Columns) != 2 || next.DoneColumn != "Done" {
		t.Fatalf("columns = %+v", next.Columns)
	}
	if again := next.EnsureDoneColumn(); len(again.Columns) != 2 {
		t.Fatal("EnsureDoneColumn added a second Done column")
	}
}

func TestColumns(t *testing.T) {
	b := NewBoard("x")
	b, _ = b.AddTask(0, "one")
	b = b.EnsureDoneColumn()

	next, err := b.AddColumn("Doing")
	if err != nil {
		t.Fatalf("AddColumn: %v", err)
	}
	if got := columnNames(next); got != "Backlog,Doing,Done" {
		t.Fatalf("columns = %s", got)
	}
	if _, err := next.AddColumn("doing"); !errors.Is(err, ErrDuplicateColumn) {
		t.Fatalf("duplicate error = %v", err)
	}
	if _, err := next.AddColumn(" "); !errors.Is(err, ErrEmptyName) {
		t.Fatalf("empty error = %v", err)
	}

	renamed, err := next.RenameColumn(2, "Finished")
	if err != nil {
		t.Fatalf("RenameColumn: %v", err)
	}
	if renamed.DoneColumn != "Finished" {
		t.Fatalf("done column = %q", renamed.DoneColumn)
	}

	del, err := renamed.DeleteColumn(1)
	if err != nil {
		t.Fatalf("DeleteColumn: %v", err)
	}
	if got := columnNames(del); got != "Backlog,Finished" {
		t.Fatalf("columns = %s", got)
	}

	if _, err := renamed.DeleteColumn(0); !errors.Is(err, ErrColumnNotEmpty) {
		t.Fatalf("non-empty error = %v", err)
	}
	if _, err := NewBoard("x").DeleteColumn(0); !errors.Is(err, ErrLastColumn) {
		t.Fatalf("last column error = %v", err)
	}
}

func TestArchiveAndRestore(t *testing.T) {
	at := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	b := NewBoard("x")
	b, _ = b.AddTask(0, "one #a")
	id := b.Columns[0].Tasks[0].ID
	b, _ = b.ToggleDone(id)

	archived, err := b.ArchiveTasks([]TaskID{id}, at)
	if err != nil {
		t.Fatalf("ArchiveTasks: %v", err)
	}
	if len(archived.Archive) != 1 || archived.Archive[0].ArchivedAt != at {
		t.Fatalf("archive = %+v", archived.Archive)
	}
	if len(archived.Columns[1].Tasks) != 0 {
		t.Fatal("archived task stayed in Done")
	}

	restored, err := archived.RestoreTask(id)
	if err != nil {
		t.Fatalf("RestoreTask: %v", err)
	}
	if len(restored.Archive) != 0 || len(restored.Columns[1].Tasks) != 1 || !restored.Columns[1].Tasks[0].Done {
		t.Fatalf("restored = %+v", restored)
	}

	reopened, err := archived.ReopenTask(id)
	if err != nil {
		t.Fatalf("ReopenTask: %v", err)
	}
	if reopened.Columns[reopened.CaptureIndex()].Tasks[0].Done {
		t.Fatal("reopened task is checked")
	}

	pending := NewBoard("x")
	pending, _ = pending.AddTask(0, "not done")
	if _, err := pending.ArchiveTasks([]TaskID{pending.Columns[0].Tasks[0].ID}, at); !errors.Is(err, ErrNotDone) {
		t.Fatalf("not done error = %v", err)
	}
}

func TestArchiveAllDone(t *testing.T) {
	b := NewBoard("x")
	b, _ = b.AddTask(0, "one")
	b, _ = b.AddTask(0, "two")
	for _, task := range append([]Task(nil), b.Columns[0].Tasks...) {
		b, _ = b.ToggleDone(task.ID)
	}
	next, n, err := b.ArchiveAllDone(time.Now())
	if err != nil {
		t.Fatalf("ArchiveAllDone: %v", err)
	}
	if n != 2 || len(next.Archive) != 2 || len(next.Columns[1].Tasks) != 0 {
		t.Fatalf("n=%d archive=%d done=%d", n, len(next.Archive), len(next.Columns[1].Tasks))
	}
}

func TestMilestones(t *testing.T) {
	b := NewBoard("x")
	b, _ = b.AddTask(0, "one @v1")
	b, _ = b.AddTask(0, "two @v1")

	renamed, err := b.UpdateMilestone("v1", MilestonePatch{Name: strptr("v2"), Due: strptr("2026-12-01")})
	if err != nil {
		t.Fatalf("UpdateMilestone: %v", err)
	}
	for _, task := range renamed.Columns[0].Tasks {
		if task.Milestone != "v2" {
			t.Fatalf("milestone = %q", task.Milestone)
		}
	}
	m, ok := renamed.MilestoneByName("v2")
	if !ok || m.Due != "2026-12-01" {
		t.Fatalf("milestone = %+v, %v", m, ok)
	}

	detached, err := renamed.DeleteMilestone("v2")
	if err != nil {
		t.Fatalf("DeleteMilestone: %v", err)
	}
	if len(detached.Milestones) != 0 || detached.Columns[0].Tasks[0].Milestone != "" {
		t.Fatalf("detached = %+v", detached)
	}

	if _, err := b.DeleteMilestone("missing"); !errors.Is(err, ErrMilestoneNotFound) {
		t.Fatalf("missing error = %v", err)
	}
	if _, err := b.AddMilestone(Milestone{Name: "v1"}); !errors.Is(err, ErrDuplicateMilestone) {
		t.Fatalf("duplicate error = %v", err)
	}
}

func TestMilestoneProgress(t *testing.T) {
	b := NewBoard("x")
	b, _ = b.AddTask(0, "one @v1")
	b, _ = b.AddTask(0, "two @v1")
	b, _ = b.ToggleDone(b.Columns[0].Tasks[1].ID)
	done, total := b.MilestoneProgress("v1")
	if done != 1 || total != 2 {
		t.Fatalf("progress = %d/%d", done, total)
	}
}

func TestFilter(t *testing.T) {
	b := NewBoard("x")
	b, _ = b.AddTask(0, "Fix login #ui #urgent @v1")
	b, _ = b.AddTask(0, "Write docs #docs")
	b, _ = b.AddTask(0, "Hidden task")
	b, _ = b.AddTask(0, "Done task @v1")
	b, _ = b.ToggleDone(b.Columns[0].Tasks[3].ID)

	col := b.Columns[0]
	f := NewFilter()
	if got := f.VisibleCount(col); got != 3 {
		t.Fatalf("default visible = %d", got)
	}
	f.Text = "LOGIN"
	if got := f.VisibleCount(col); got != 1 {
		t.Fatalf("text visible = %d", got)
	}
	f = NewFilter()
	f.Tags = []string{"ui"}
	if got := f.VisibleCount(col); got != 1 {
		t.Fatalf("tag visible = %d", got)
	}
	f.Tags = []string{"ui", "docs"}
	f.TagMode = TagAny
	if got := f.VisibleCount(col); got != 2 {
		t.Fatalf("any visible = %d", got)
	}
	f.TagMode = TagAll
	if got := f.VisibleCount(col); got != 0 {
		t.Fatalf("all visible = %d", got)
	}
	f = NewFilter()
	f.Milestone = "v1"
	if got := f.VisibleCount(col); got != 1 {
		t.Fatalf("milestone visible = %d", got)
	}
	f.ShowDone = false
	if got := f.VisibleCount(col); got != 1 {
		t.Fatalf("milestone visible without done = %d", got)
	}
	f = NewFilter()
	f.ShowDone = false
	if got := f.VisibleCount(b.Columns[1]); got != 0 {
		t.Fatalf("done column hidden = %d", got)
	}
	f = NewFilter()
	f.Milestone = NoMilestone
	if got := f.VisibleCount(col); got != 2 {
		t.Fatalf("no milestone visible = %d", got)
	}
}

func TestTags(t *testing.T) {
	b := NewBoard("x")
	b, _ = b.AddTask(0, "one #b #a")
	b, _ = b.AddTask(0, "two #a #c")
	if got := strings.Join(b.Tags(false), ","); got != "a,b,c" {
		t.Fatalf("tags = %s", got)
	}
}

func columnNames(b Board) string {
	names := make([]string, len(b.Columns))
	for i, col := range b.Columns {
		names[i] = col.Name
	}
	return strings.Join(names, ",")
}

func strptr(s string) *string { return &s }
