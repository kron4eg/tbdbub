package board

import "testing"

func TestNewBoard(t *testing.T) {
	b := NewBoard("Home renovation")
	if b.Title != "Home renovation" {
		t.Fatalf("Title = %q", b.Title)
	}
	if len(b.Columns) != 1 || b.Columns[0].Name != "Backlog" {
		t.Fatalf("Columns = %+v", b.Columns)
	}
	if b.CaptureColumn != "Backlog" {
		t.Fatalf("CaptureColumn = %q", b.CaptureColumn)
	}
	if b.DoneColumn != "" {
		t.Fatalf("DoneColumn = %q, want empty", b.DoneColumn)
	}
}

func TestIsDoneColumn(t *testing.T) {
	b := NewBoard("x")
	b.Columns = append(b.Columns, Column{Name: "Doing"}, Column{Name: "Done"})
	b.DoneColumn = "Done"

	if b.IsDoneColumn(0) || b.IsDoneColumn(1) {
		t.Fatal("only the last column should be done")
	}
	if !b.IsDoneColumn(2) {
		t.Fatal("Done column not detected")
	}
	if b.IsDoneColumn(-1) || b.IsDoneColumn(3) {
		t.Fatal("out of range column should not be done")
	}
}

func TestIsDoneColumnFallsBackToLast(t *testing.T) {
	b := NewBoard("x")
	b.Columns = append(b.Columns, Column{Name: "Done"})

	if b.IsDoneColumn(0) {
		t.Fatal("single-column board should not have an implicit Done column")
	}
	if !b.IsDoneColumn(1) {
		t.Fatal("last column should be treated as Done")
	}
}

func TestTaskByID(t *testing.T) {
	b := NewBoard("x")
	b.Columns[0].Tasks = []Task{{ID: "t1", Title: "one"}}
	b.Columns = append(b.Columns, Column{Name: "Done", Tasks: []Task{{ID: "t2", Title: "two"}}})

	col, idx, ok := b.TaskByID("t2")
	if !ok || col != 1 || idx != 0 {
		t.Fatalf("TaskByID(t2) = %d, %d, %v", col, idx, ok)
	}
	if _, _, ok := b.TaskByID("missing"); ok {
		t.Fatal("missing task found")
	}
}
