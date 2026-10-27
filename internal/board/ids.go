package board

import (
	"crypto/rand"
	"strings"
)

const (
	idAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	idLength   = 4
)

// newTaskID returns a random short ID. Uniqueness is enforced by withIDs.
func newTaskID() TaskID {
	buf := make([]byte, idLength)
	if _, err := rand.Read(buf); err != nil {
		return TaskID("t0000")
	}
	out := make([]byte, idLength)
	for i, b := range buf {
		out[i] = idAlphabet[int(b)%len(idAlphabet)]
	}
	return TaskID(out)
}

func uniqueID(used map[TaskID]bool) TaskID {
	for {
		id := newTaskID()
		if !used[id] {
			used[id] = true
			return id
		}
	}
}

func (b Board) usedIDs() map[TaskID]bool {
	used := make(map[TaskID]bool, b.TotalTasks()+len(b.Archive))
	for _, col := range b.Columns {
		for _, task := range col.Tasks {
			used[task.ID] = true
		}
	}
	for _, task := range b.Archive {
		used[task.ID] = true
	}
	return used
}

// withIDs returns a copy of the board where every task has a unique ID. It is
// used by the parser and writer so a hand-written task without an ID becomes
// stable after it is first loaded.
func (b Board) withIDs() (Board, []Warning) {
	used := make(map[TaskID]bool, b.TotalTasks()+len(b.Archive))
	var warnings []Warning

	assign := func(task *Task) {
		id := task.ID
		if validID(id) && !used[id] {
			used[id] = true
			return
		}
		switch {
		case id == "":
			// Expected for hand-written tasks.
		case !validID(id):
			warnings = append(warnings, Warning{Message: "invalid task ID " + string(id) + " regenerated"})
		default:
			warnings = append(warnings, Warning{Message: "duplicate task ID " + string(id) + " renumbered"})
		}
		task.ID = uniqueID(used)
	}

	next := b
	next.Columns = make([]Column, len(b.Columns))
	for ci, col := range b.Columns {
		next.Columns[ci] = col
		next.Columns[ci].Tasks = append([]Task(nil), col.Tasks...)
		for ti := range next.Columns[ci].Tasks {
			assign(&next.Columns[ci].Tasks[ti])
		}
	}
	if len(b.Archive) > 0 {
		next.Archive = append([]ArchivedTask(nil), b.Archive...)
		for i := range next.Archive {
			assign(&next.Archive[i].Task)
		}
	}
	return next, warnings
}

func validID(id TaskID) bool {
	if len(id) < 4 || len(id) > 8 {
		return false
	}
	for _, r := range string(id) {
		if !strings.ContainsRune("abcdefghijklmnopqrstuvwxyz0123456789", r) {
			return false
		}
	}
	return true
}
