package board

import "strings"

// TagMode selects whether a task must match any or all selected tags.
type TagMode int

const (
	TagAny TagMode = iota
	TagAll
)

// NoMilestone is the Filter.Milestone sentinel for "tasks without a milestone".
const NoMilestone = "\x00"

// Filter is a pure view-layer predicate. It is ephemeral and never persisted.
type Filter struct {
	Text      string
	Tags      []string
	TagMode   TagMode
	Milestone string // empty = any; "\x00" = none
	ShowDone  bool
}

// NewFilter returns the default filter: everything visible.
func NewFilter() Filter {
	return Filter{ShowDone: true}
}

// Active reports whether the filter hides anything at all.
func (f Filter) Active() bool {
	return strings.TrimSpace(f.Text) != "" || len(f.Tags) > 0 || f.Milestone != "" || !f.ShowDone
}

// Matches applies the filter to one task.
func (f Filter) Matches(task Task) bool {
	if !f.ShowDone && task.Done {
		return false
	}
	if f.Milestone != "" {
		if f.Milestone == NoMilestone {
			if task.Milestone != "" {
				return false
			}
		} else if task.Milestone != f.Milestone {
			return false
		}
	}
	if len(f.Tags) > 0 {
		matched := 0
		for _, want := range f.Tags {
			for _, tag := range task.Tags {
				if tag == want {
					matched++
					break
				}
			}
		}
		if f.TagMode == TagAll && matched != len(f.Tags) {
			return false
		}
		if f.TagMode == TagAny && matched == 0 {
			return false
		}
	}
	if text := strings.TrimSpace(f.Text); text != "" {
		if !taskMatchesText(task, text) {
			return false
		}
	}
	return true
}

func taskMatchesText(task Task, text string) bool {
	text = strings.ToLower(text)
	fields := []string{task.Title, task.Notes, task.Milestone}
	fields = append(fields, task.Tags...)
	for _, field := range fields {
		if strings.Contains(strings.ToLower(field), text) {
			return true
		}
	}
	return false
}

// VisibleIndices returns the indexes of tasks that pass the filter.
func (f Filter) VisibleIndices(col Column) []int {
	indices := make([]int, 0, len(col.Tasks))
	for i, task := range col.Tasks {
		if f.Matches(task) {
			indices = append(indices, i)
		}
	}
	return indices
}

// VisibleCount returns how many tasks in a column pass the filter.
func (f Filter) VisibleCount(col Column) int {
	if !f.Active() {
		return len(col.Tasks)
	}
	n := 0
	for _, task := range col.Tasks {
		if f.Matches(task) {
			n++
		}
	}
	return n
}

// MatchesArchive applies the text portion of the filter to an archived task.
func (f Filter) MatchesArchive(task Task) bool {
	text := strings.TrimSpace(f.Text)
	if text == "" {
		return true
	}
	return taskMatchesText(task, text)
}
