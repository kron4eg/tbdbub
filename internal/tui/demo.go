package tui

import "github.com/kron4eg/tbdbub/internal/board"

// DemoBoard is the hard-coded board used by tests and `go run .` without a file.
func DemoBoard() board.Board {
	return board.Board{
		Title:         "Home renovation",
		CaptureColumn: "Backlog",
		DoneColumn:    "Done",
		Columns: []board.Column{
			{
				Name: "Backlog",
				Tasks: []board.Task{
					{ID: "t7k2", Title: "Replace faucet", Tags: []string{"plumbing", "urgent"}, Milestone: "Kitchen"},
					{ID: "t9x1", Title: "Choose tiles", Tags: []string{"design"}, Milestone: "Bathroom"},
					{ID: "t2m4", Title: "Order samples", Tags: []string{"design"}, Milestone: "Bathroom"},
					{ID: "t5ra", Title: "Measure the hallway", Tags: []string{"paint"}, Milestone: "Kitchen"},
					{ID: "t8bc", Title: "Ask about permits", Tags: []string{"admin"}, Milestone: "Kitchen"},
					{ID: "t6de", Title: "Find a plumber", Tags: []string{"plumbing"}, Milestone: "Kitchen"},
					{ID: "t0fg", Title: "Clear the spare room", Tags: []string{"admin"}},
				},
			},
			{
				Name: "Doing",
				Tasks: []board.Task{
					{ID: "t3ab", Title: "Paint hallway", Tags: []string{"paint"}, Milestone: "Kitchen"},
					{ID: "t4cd", Title: "Shortlist tiles", Tags: []string{"design"}, Milestone: "Bathroom"},
				},
			},
			{
				Name: "Done",
				Tasks: []board.Task{
					{ID: "t1zz", Title: "Measure cabinets", Tags: []string{"design"}, Milestone: "Kitchen", Done: true},
				},
			},
		},
		Milestones: []board.Milestone{
			{Name: "Kitchen", Due: "2026-11-15"},
			{Name: "Bathroom"},
		},
	}
}
