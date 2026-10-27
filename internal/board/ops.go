package board

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	// ErrColumnNotFound is returned when a move targets a column that does not
	// exist.
	ErrColumnNotFound = errors.New("column not found")
	// ErrTaskNotFound is returned when a move references an unknown task ID.
	ErrTaskNotFound = errors.New("task not found")
	// ErrEmptyTitle is returned when a task title is empty.
	ErrEmptyTitle = errors.New("task title is required")
	// ErrEmptyName is returned when a column or milestone name is empty.
	ErrEmptyName = errors.New("name is required")
	// ErrDuplicateColumn is returned when a column name is already in use.
	ErrDuplicateColumn = errors.New("column already exists")
	// ErrDuplicateMilestone is returned when a milestone name is already in use.
	ErrDuplicateMilestone = errors.New("milestone already exists")
	// ErrMilestoneNotFound is returned when a milestone does not exist.
	ErrMilestoneNotFound = errors.New("milestone not found")
	// ErrColumnNotEmpty is returned when deleting a column that still has tasks.
	ErrColumnNotEmpty = errors.New("column is not empty")
	// ErrLastColumn is returned when deleting the only column.
	ErrLastColumn = errors.New("cannot delete the last column")
	// ErrNotDone is returned when archiving a task that is not in Done.
	ErrNotDone = errors.New("task is not in the Done column")
)

// Move describes a set-based task move. All movement gestures (keys, grab mode,
// and future mouse drag) go through Board.MoveTasks so they cannot diverge.
type Move struct {
	IDs      []TaskID
	ToColumn int
	ToIndex  int // insertion index in the target; -1 appends
}

// MoveTasks returns a new board with the given tasks moved to ToColumn at
// ToIndex. The order of the moved tasks follows their order on the board. The
// index is clamped to the target column. Moving into or out of the Done column
// syncs Task.Done with the destination.
func (b Board) MoveTasks(m Move) (Board, error) {
	if m.ToColumn < 0 || m.ToColumn >= len(b.Columns) {
		return b, fmt.Errorf("%w: %d", ErrColumnNotFound, m.ToColumn)
	}
	if len(m.IDs) == 0 {
		return b, nil
	}

	next := clone(b)

	wanted := make(map[TaskID]struct{}, len(m.IDs))
	for _, id := range m.IDs {
		wanted[id] = struct{}{}
	}

	moved := make([]Task, 0, len(m.IDs))
	for ci := range next.Columns {
		tasks := next.Columns[ci].Tasks
		kept := make([]Task, 0, len(tasks))
		for _, task := range tasks {
			if _, ok := wanted[task.ID]; ok {
				moved = append(moved, task)
				delete(wanted, task.ID)
				continue
			}
			kept = append(kept, task)
		}
		next.Columns[ci].Tasks = kept
	}
	for _, id := range m.IDs {
		if _, ok := wanted[id]; ok {
			return b, fmt.Errorf("%w: %s", ErrTaskNotFound, id)
		}
	}

	done := next.IsDoneColumn(m.ToColumn)
	for i := range moved {
		moved[i].Done = done
	}

	target := next.Columns[m.ToColumn].Tasks
	index := m.ToIndex
	if index < 0 || index > len(target) {
		index = len(target)
	}
	merged := make([]Task, 0, len(target)+len(moved))
	merged = append(merged, target[:index]...)
	merged = append(merged, moved...)
	merged = append(merged, target[index:]...)
	next.Columns[m.ToColumn].Tasks = merged

	return next, nil
}

// AddTask appends a task to a column. Inline tags, a milestone, and an ID in the
// title are extracted exactly as they are when parsing a file.
func (b Board) AddTask(col int, title string) (Board, error) {
	if col < 0 || col >= len(b.Columns) {
		return b, fmt.Errorf("%w: %d", ErrColumnNotFound, col)
	}
	task, _ := parseTaskText(title, 0)
	if task.Title == "" {
		return b, ErrEmptyTitle
	}
	task.Tags = dedupeTags(task.Tags)
	task.Done = b.IsDoneColumn(col)

	next := clone(b)
	used := next.usedIDs()
	if task.ID == "" || used[task.ID] {
		task.ID = uniqueID(used)
	} else {
		used[task.ID] = true
	}
	next.Columns[col].Tasks = append(next.Columns[col].Tasks, task)
	next.registerMilestones()
	return next, nil
}

// TaskPatch describes a partial update. Nil fields are left unchanged.
type TaskPatch struct {
	Title     *string
	Notes     *string
	Tags      *[]string
	Milestone *string
}

// UpdateTask applies a patch to one task. Inline tokens in a new title are
// extracted; an explicit Tags or Milestone patch takes precedence.
func (b Board) UpdateTask(id TaskID, patch TaskPatch) (Board, error) {
	next := clone(b)
	ci, ti, ok := next.TaskByID(id)
	if !ok {
		return b, fmt.Errorf("%w: %s", ErrTaskNotFound, id)
	}
	task := &next.Columns[ci].Tasks[ti]

	if patch.Title != nil {
		parsed, _ := parseTaskText(*patch.Title, 0)
		if parsed.Title == "" {
			return b, ErrEmptyTitle
		}
		task.Title = parsed.Title
		task.Tags = dedupeTags(parsed.Tags)
		task.Milestone = parsed.Milestone
	}
	if patch.Notes != nil {
		task.Notes = normalizeNotes(*patch.Notes)
	}
	if patch.Tags != nil {
		task.Tags = dedupeTags(*patch.Tags)
	}
	if patch.Milestone != nil {
		task.Milestone = strings.TrimSpace(strings.TrimPrefix(*patch.Milestone, "@"))
	}
	next.registerMilestones()
	return next, nil
}

// DeleteTask removes a task from the board.
func (b Board) DeleteTask(id TaskID) (Board, error) {
	next := clone(b)
	ci, ti, ok := next.TaskByID(id)
	if !ok {
		return b, fmt.Errorf("%w: %s", ErrTaskNotFound, id)
	}
	tasks := next.Columns[ci].Tasks
	next.Columns[ci].Tasks = append(tasks[:ti], tasks[ti+1:]...)
	return next, nil
}

// ToggleDone completes or reopens a task. Completing appends the Done column
// when it does not exist yet and moves the task to its end. Reopening moves the
// task to the capture column.
func (b Board) ToggleDone(id TaskID) (Board, error) {
	next := clone(b)
	ci, ti, ok := next.TaskByID(id)
	if !ok {
		return b, fmt.Errorf("%w: %s", ErrTaskNotFound, id)
	}
	task := next.Columns[ci].Tasks[ti]

	if next.IsDoneColumn(ci) || task.Done {
		task.Done = false
		tasks := next.Columns[ci].Tasks
		next.Columns[ci].Tasks = append(tasks[:ti], tasks[ti+1:]...)
		capture := next.CaptureIndex()
		next.Columns[capture].Tasks = append(next.Columns[capture].Tasks, task)
		return next, nil
	}

	done := ensureDone(&next)
	task.Done = true
	tasks := next.Columns[ci].Tasks
	next.Columns[ci].Tasks = append(tasks[:ti], tasks[ti+1:]...)
	next.Columns[done].Tasks = append(next.Columns[done].Tasks, task)
	return next, nil
}

// EnsureDoneColumn appends the Done column if the board has none. It is used
// when moving right from a single-column board.
func (b Board) EnsureDoneColumn() Board {
	if _, ok := b.DoneIndex(); ok {
		return b
	}
	next := clone(b)
	ensureDone(&next)
	return next
}

// AddColumn inserts a column before Done. A new Done column becomes the Done
// column when the board does not have one.
func (b Board) AddColumn(name string) (Board, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return b, ErrEmptyName
	}
	for _, col := range b.Columns {
		if strings.EqualFold(col.Name, name) {
			return b, fmt.Errorf("%w: %s", ErrDuplicateColumn, name)
		}
	}

	next := clone(b)
	col := Column{Name: name}
	if done, ok := next.DoneIndex(); ok && !strings.EqualFold(name, "Done") {
		next.Columns = append(next.Columns, Column{})
		copy(next.Columns[done+1:], next.Columns[done:])
		next.Columns[done] = col
	} else {
		next.Columns = append(next.Columns, col)
		if strings.EqualFold(name, "Done") {
			next.DoneColumn = name
		}
	}
	if next.CaptureColumn == "" {
		next.CaptureColumn = next.Columns[next.CaptureIndex()].Name
	}
	return next, nil
}

// RenameColumn renames a column and keeps capture/done references in sync.
func (b Board) RenameColumn(index int, name string) (Board, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return b, ErrEmptyName
	}
	if index < 0 || index >= len(b.Columns) {
		return b, fmt.Errorf("%w: %d", ErrColumnNotFound, index)
	}
	for i, col := range b.Columns {
		if i != index && strings.EqualFold(col.Name, name) {
			return b, fmt.Errorf("%w: %s", ErrDuplicateColumn, name)
		}
	}

	next := clone(b)
	old := next.Columns[index].Name
	next.Columns[index].Name = name
	if next.CaptureColumn == old {
		next.CaptureColumn = name
	}
	if next.DoneColumn == old {
		next.DoneColumn = name
	}
	return next, nil
}

// DeleteColumn removes an empty column. The last column and non-empty columns
// cannot be deleted.
func (b Board) DeleteColumn(index int) (Board, error) {
	if index < 0 || index >= len(b.Columns) {
		return b, fmt.Errorf("%w: %d", ErrColumnNotFound, index)
	}
	if len(b.Columns) == 1 {
		return b, ErrLastColumn
	}
	if len(b.Columns[index].Tasks) > 0 {
		return b, ErrColumnNotEmpty
	}

	next := clone(b)
	old := next.Columns[index].Name
	next.Columns = append(next.Columns[:index], next.Columns[index+1:]...)
	if next.DoneColumn == old {
		next.DoneColumn = ""
	}
	if next.CaptureColumn == old {
		next.CaptureColumn = next.Columns[next.CaptureIndex()].Name
	}
	return next, nil
}

// ArchiveTasks moves Done tasks into the archive.
func (b Board) ArchiveTasks(ids []TaskID, at time.Time) (Board, error) {
	if len(ids) == 0 {
		return b, nil
	}
	next := clone(b)
	for _, id := range ids {
		ci, ti, ok := next.TaskByID(id)
		if !ok {
			return b, fmt.Errorf("%w: %s", ErrTaskNotFound, id)
		}
		if !next.IsDoneColumn(ci) {
			return b, fmt.Errorf("%w: %s", ErrNotDone, id)
		}
		task := next.Columns[ci].Tasks[ti]
		task.Done = true
		next.Columns[ci].Tasks = append(next.Columns[ci].Tasks[:ti], next.Columns[ci].Tasks[ti+1:]...)
		next.Archive = append(next.Archive, ArchivedTask{Task: task, ArchivedAt: at})
	}
	return next, nil
}

// ArchiveAllDone archives every task in the Done column.
func (b Board) ArchiveAllDone(at time.Time) (Board, int, error) {
	done, ok := b.DoneIndex()
	if !ok {
		return b, 0, nil
	}
	ids := make([]TaskID, 0, len(b.Columns[done].Tasks))
	for _, task := range b.Columns[done].Tasks {
		ids = append(ids, task.ID)
	}
	next, err := b.ArchiveTasks(ids, at)
	return next, len(ids), err
}

// RestoreTask moves an archived task back to the end of Done.
func (b Board) RestoreTask(id TaskID) (Board, error) {
	return b.restoreArchived(id, true)
}

// ReopenTask moves an archived task to the capture column and clears Done.
func (b Board) ReopenTask(id TaskID) (Board, error) {
	return b.restoreArchived(id, false)
}

func (b Board) restoreArchived(id TaskID, done bool) (Board, error) {
	next := clone(b)
	index := -1
	for i := range next.Archive {
		if next.Archive[i].ID == id {
			index = i
			break
		}
	}
	if index < 0 {
		return b, fmt.Errorf("%w: %s", ErrTaskNotFound, id)
	}
	task := next.Archive[index].Task
	next.Archive = append(next.Archive[:index], next.Archive[index+1:]...)

	if done {
		col := ensureDone(&next)
		task.Done = true
		next.Columns[col].Tasks = append(next.Columns[col].Tasks, task)
	} else {
		task.Done = false
		capture := next.CaptureIndex()
		next.Columns[capture].Tasks = append(next.Columns[capture].Tasks, task)
	}
	next.registerMilestones()
	return next, nil
}

// AddMilestone registers milestone metadata.
func (b Board) AddMilestone(m Milestone) (Board, error) {
	m.Name = strings.TrimSpace(m.Name)
	if m.Name == "" {
		return b, ErrEmptyName
	}
	if _, ok := b.MilestoneByName(m.Name); ok {
		return b, fmt.Errorf("%w: %s", ErrDuplicateMilestone, m.Name)
	}
	next := clone(b)
	next.Milestones = append(next.Milestones, m)
	return next, nil
}

// MilestonePatch describes a partial milestone update.
type MilestonePatch struct {
	Name        *string
	Description *string
	Due         *string
	Closed      *bool
}

// UpdateMilestone renames or changes milestone metadata. Renaming rewrites all
// inline references on active and archived tasks.
func (b Board) UpdateMilestone(name string, patch MilestonePatch) (Board, error) {
	index := -1
	for i := range b.Milestones {
		if b.Milestones[i].Name == name {
			index = i
			break
		}
	}
	if index < 0 {
		return b, fmt.Errorf("%w: %s", ErrMilestoneNotFound, name)
	}

	next := clone(b)
	if patch.Name != nil {
		newName := strings.TrimSpace(strings.TrimPrefix(*patch.Name, "@"))
		if newName == "" {
			return b, ErrEmptyName
		}
		if newName != name {
			if _, ok := next.MilestoneByName(newName); ok {
				return b, fmt.Errorf("%w: %s", ErrDuplicateMilestone, newName)
			}
			rewriteMilestone(&next, name, newName)
		}
	}
	m := &next.Milestones[index]
	if patch.Description != nil {
		m.Description = *patch.Description
		m.Implicit = false
	}
	if patch.Due != nil {
		m.Due = *patch.Due
		m.Implicit = false
	}
	if patch.Closed != nil {
		m.Closed = *patch.Closed
		m.Implicit = false
	}
	return next, nil
}

// DeleteMilestone removes metadata and detaches it from all tasks.
func (b Board) DeleteMilestone(name string) (Board, error) {
	index := -1
	for i := range b.Milestones {
		if b.Milestones[i].Name == name {
			index = i
			break
		}
	}
	if index < 0 {
		return b, fmt.Errorf("%w: %s", ErrMilestoneNotFound, name)
	}
	next := clone(b)
	next.Milestones = append(next.Milestones[:index], next.Milestones[index+1:]...)
	detachMilestone(&next, name)
	return next, nil
}

// MilestoneByName finds milestone metadata.
func (b Board) MilestoneByName(name string) (Milestone, bool) {
	for _, m := range b.Milestones {
		if m.Name == name {
			return m, true
		}
	}
	return Milestone{}, false
}

// MilestoneProgress returns done and total counts, including archived tasks.
func (b Board) MilestoneProgress(name string) (done, total int) {
	for _, col := range b.Columns {
		for _, task := range col.Tasks {
			if task.Milestone == name {
				total++
				if task.Done || b.IsDoneColumn(b.ColumnIndex(col.Name)) {
					done++
				}
			}
		}
	}
	for _, task := range b.Archive {
		if task.Milestone == name {
			total++
			done++
		}
	}
	return done, total
}

// Tags returns the unique tag names in use, sorted alphabetically.
func (b Board) Tags(includeArchive bool) []string {
	seen := map[string]bool{}
	var tags []string
	add := func(task Task) {
		for _, tag := range task.Tags {
			if !seen[tag] {
				seen[tag] = true
				tags = append(tags, tag)
			}
		}
	}
	for _, col := range b.Columns {
		for _, task := range col.Tasks {
			add(task)
		}
	}
	if includeArchive {
		for _, task := range b.Archive {
			add(task.Task)
		}
	}
	sortStrings(tags)
	return tags
}

func ensureDone(b *Board) int {
	if idx, ok := b.DoneIndex(); ok {
		if b.DoneColumn == "" || b.ColumnIndex(b.DoneColumn) < 0 {
			b.DoneColumn = b.Columns[idx].Name
		}
		return idx
	}
	if b.DoneColumn != "" {
		// A named Done column is missing. Fall back to the last column when
		// there is one, otherwise recreate it.
		if len(b.Columns) > 1 {
			b.DoneColumn = b.Columns[len(b.Columns)-1].Name
			return len(b.Columns) - 1
		}
		b.DoneColumn = ""
	}
	b.Columns = append(b.Columns, Column{Name: "Done"})
	b.DoneColumn = "Done"
	return len(b.Columns) - 1
}

func rewriteMilestone(b *Board, from, to string) {
	for ci := range b.Columns {
		for ti := range b.Columns[ci].Tasks {
			if b.Columns[ci].Tasks[ti].Milestone == from {
				b.Columns[ci].Tasks[ti].Milestone = to
			}
		}
	}
	for i := range b.Archive {
		if b.Archive[i].Milestone == from {
			b.Archive[i].Milestone = to
		}
	}
	for i := range b.Milestones {
		if b.Milestones[i].Name == from {
			b.Milestones[i].Name = to
		}
	}
}

func detachMilestone(b *Board, name string) {
	for ci := range b.Columns {
		for ti := range b.Columns[ci].Tasks {
			if b.Columns[ci].Tasks[ti].Milestone == name {
				b.Columns[ci].Tasks[ti].Milestone = ""
			}
		}
	}
	for i := range b.Archive {
		if b.Archive[i].Milestone == name {
			b.Archive[i].Milestone = ""
		}
	}
}

func clone(b Board) Board {
	out := b
	out.Columns = make([]Column, len(b.Columns))
	for i, col := range b.Columns {
		out.Columns[i] = Column{
			Name:     col.Name,
			Preamble: col.Preamble,
			Tasks:    append([]Task(nil), col.Tasks...),
		}
	}
	out.Milestones = append([]Milestone(nil), b.Milestones...)
	out.Archive = append([]ArchivedTask(nil), b.Archive...)
	if b.RawFrontmatter != nil {
		out.RawFrontmatter = make(map[string]any, len(b.RawFrontmatter))
		for k, v := range b.RawFrontmatter {
			out.RawFrontmatter[k] = v
		}
	}
	return out
}

// normalizeNotes gives every non-empty note line at least the indentation that
// marks it as belonging to its task. Parsed notes already carry it.
func normalizeNotes(notes string) string {
	notes = strings.TrimRight(notes, "\n")
	if notes == "" {
		return ""
	}
	lines := strings.Split(notes, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) != "" && !isIndented(line) {
			lines[i] = "  " + line
		}
	}
	return strings.Join(lines, "\n")
}

func dedupeTags(tags []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(strings.TrimPrefix(tag, "#"))
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		out = append(out, tag)
	}
	return out
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}
