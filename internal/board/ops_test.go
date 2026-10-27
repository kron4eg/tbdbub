package board

import (
	"errors"
	"testing"
)

func moveTestBoard() Board {
	return Board{
		Title:         "test",
		CaptureColumn: "Backlog",
		DoneColumn:    "Done",
		Columns: []Column{
			{Name: "Backlog", Tasks: []Task{
				{ID: "t1", Title: "one"},
				{ID: "t2", Title: "two"},
				{ID: "t3", Title: "three"},
			}},
			{Name: "Doing", Tasks: []Task{
				{ID: "t4", Title: "four"},
			}},
			{Name: "Done", Tasks: []Task{
				{ID: "t5", Title: "five", Done: true},
			}},
		},
	}
}

func TestMoveTasksAppends(t *testing.T) {
	b := moveTestBoard()

	next, err := b.MoveTasks(Move{IDs: []TaskID{"t1"}, ToColumn: 1, ToIndex: -1})
	if err != nil {
		t.Fatalf("MoveTasks: %v", err)
	}

	if got := columnTitles(next.Columns[0]); got != "two,three" {
		t.Fatalf("source = %q", got)
	}
	if got := columnTitles(next.Columns[1]); got != "four,one" {
		t.Fatalf("target = %q", got)
	}

	if len(b.Columns[0].Tasks) != 3 || b.Columns[1].Tasks[0].ID != "t4" {
		t.Fatal("MoveTasks mutated the original board")
	}
}

func TestMoveTasksInsertIndex(t *testing.T) {
	b := moveTestBoard()

	next, err := b.MoveTasks(Move{IDs: []TaskID{"t3"}, ToColumn: 1, ToIndex: 0})
	if err != nil {
		t.Fatalf("MoveTasks: %v", err)
	}
	if got := columnTitles(next.Columns[1]); got != "three,four" {
		t.Fatalf("target = %q", got)
	}

	next, err = next.MoveTasks(Move{IDs: []TaskID{"t3"}, ToColumn: 1, ToIndex: 99})
	if err != nil {
		t.Fatalf("MoveTasks: %v", err)
	}
	if got := columnTitles(next.Columns[1]); got != "four,three" {
		t.Fatalf("clamped target = %q", got)
	}
}

func TestMoveTasksSyncsDone(t *testing.T) {
	b := moveTestBoard()

	next, err := b.MoveTasks(Move{IDs: []TaskID{"t1"}, ToColumn: 2, ToIndex: -1})
	if err != nil {
		t.Fatalf("MoveTasks: %v", err)
	}
	_, index, ok := next.TaskByID("t1")
	if !ok || !next.Columns[2].Tasks[index].Done {
		t.Fatal("moving into Done did not check the task")
	}

	next, err = next.MoveTasks(Move{IDs: []TaskID{"t1"}, ToColumn: 0, ToIndex: -1})
	if err != nil {
		t.Fatalf("MoveTasks: %v", err)
	}
	_, index, ok = next.TaskByID("t1")
	if !ok || next.Columns[0].Tasks[index].Done {
		t.Fatal("moving out of Done did not uncheck the task")
	}
}

func TestMoveTasksPreservesOrder(t *testing.T) {
	b := moveTestBoard()

	next, err := b.MoveTasks(Move{
		IDs:      []TaskID{"t3", "t1"},
		ToColumn: 1,
		ToIndex:  -1,
	})
	if err != nil {
		t.Fatalf("MoveTasks: %v", err)
	}
	if got := columnTitles(next.Columns[1]); got != "four,one,three" {
		t.Fatalf("target = %q", got)
	}
}

func TestMoveTasksErrors(t *testing.T) {
	b := moveTestBoard()

	if _, err := b.MoveTasks(Move{IDs: []TaskID{"t1"}, ToColumn: 7}); !errors.Is(err, ErrColumnNotFound) {
		t.Fatalf("bad column error = %v", err)
	}
	next, err := b.MoveTasks(Move{IDs: []TaskID{"missing"}, ToColumn: 1})
	if !errors.Is(err, ErrTaskNotFound) {
		t.Fatalf("bad task error = %v", err)
	}
	if got := next.TotalTasks(); got != b.TotalTasks() {
		t.Fatalf("failed move changed the board: %d tasks, want %d", got, b.TotalTasks())
	}
}

func TestMoveTasksNoIDs(t *testing.T) {
	b := moveTestBoard()
	next, err := b.MoveTasks(Move{ToColumn: 1})
	if err != nil {
		t.Fatalf("MoveTasks: %v", err)
	}
	if got := columnTitles(next.Columns[0]); got != "one,two,three" {
		t.Fatalf("board changed: %q", got)
	}
}

func columnTitles(col Column) string {
	out := ""
	for i, task := range col.Tasks {
		if i > 0 {
			out += ","
		}
		out += task.Title
	}
	return out
}
