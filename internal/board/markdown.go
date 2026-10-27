package board

import (
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"gopkg.in/yaml.v3"
)

const (
	archiveMarker = "<!-- tbdbub:archive -->"
	fileVersion   = 1
)

var (
	columnHeadingRE   = regexp.MustCompile(`^##[ \t]+(.+?)[ \t]*$`)
	archivedHeadingRE = regexp.MustCompile(`^##[ \t]+Archived(?:[ \t]+(\d{4}-\d{2}))?[ \t]*$`)
	taskLineRE        = regexp.MustCompile(`^[-*][ \t]+\[([ xX])\][ \t]*(.*)$`)
	trailingIDRE      = regexp.MustCompile(`(^|[ \t])\^([a-z0-9]{4,8})[ \t]*$`)
)

// Warning describes a recoverable problem found while parsing a board.
type Warning struct {
	Line    int
	Message string
}

func (w Warning) String() string {
	if w.Line > 0 {
		return fmt.Sprintf("line %d: %s", w.Line, w.Message)
	}
	return w.Message
}

type frontmatter struct {
	Tbdbub        int         `yaml:"tbdbub"`
	Title         string      `yaml:"title"`
	CaptureColumn string      `yaml:"capture_column"`
	DoneColumn    string      `yaml:"done_column"`
	Milestones    []Milestone `yaml:"milestones"`
}

// Parse reads a Markdown board. It is tolerant: malformed sections produce
// warnings and are preserved where possible rather than failing the load.
func Parse(r io.Reader) (Board, []Warning, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return Board{}, nil, err
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	text = strings.TrimPrefix(text, "\ufeff")
	lines := strings.Split(text, "\n")

	var warnings []Warning
	b := Board{RawFrontmatter: map[string]any{}}

	start := 0
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "---" {
		end := -1
		for i := 1; i < len(lines); i++ {
			if strings.TrimSpace(lines[i]) == "---" {
				end = i
				break
			}
		}
		if end < 0 {
			warnings = append(warnings, Warning{Line: 1, Message: "unterminated frontmatter; ignored"})
			start = 1
		} else {
			b, warnings = parseFrontmatter(strings.Join(lines[1:end], "\n"), warnings)
			start = end + 1
		}
	}

	var (
		preamble      []string
		colPreamble   []string
		colIndex      = -1
		inTask        bool
		taskIndex     = -1
		noteBuf       []string
		inArchive     bool
		archiveMonth  string
		archiveRaw    []string
		archiveInTask bool
		archiveTask   = -1
		archiveNotes  []string
	)

	finalizeTask := func() {
		if inTask && colIndex >= 0 && taskIndex >= 0 && taskIndex < len(b.Columns[colIndex].Tasks) {
			b.Columns[colIndex].Tasks[taskIndex].Notes = joinNotes(noteBuf)
		}
		noteBuf = nil
		inTask = false
		taskIndex = -1
	}
	finalizeArchiveTask := func() {
		if archiveInTask && archiveTask >= 0 && archiveTask < len(b.Archive) {
			b.Archive[archiveTask].Notes = joinNotes(archiveNotes)
		}
		archiveNotes = nil
		archiveInTask = false
		archiveTask = -1
	}
	flushColumnPreamble := func() {
		if colIndex >= 0 {
			b.Columns[colIndex].Preamble = trimBlankLines(colPreamble)
		}
		colPreamble = nil
	}

	for i := start; i < len(lines); i++ {
		line := lines[i]
		lineNo := i + 1
		trimmed := strings.TrimSpace(line)

		if line == archiveMarker {
			finalizeTask()
			flushColumnPreamble()
			inArchive = true
			colIndex = -1
			continue
		}

		if inArchive {
			if m := archivedHeadingRE.FindStringSubmatch(line); m != nil {
				finalizeArchiveTask()
				archiveMonth = m[1]
				continue
			}
			if m := taskLineRE.FindStringSubmatch(line); m != nil {
				finalizeArchiveTask()
				task, warns := parseTaskText(m[2], lineNo)
				for _, w := range warns {
					warnings = append(warnings, w)
				}
				task.Done = true
				at := time.Time{}
				if archiveMonth != "" {
					if t, err := time.Parse("2006-01", archiveMonth); err == nil {
						at = t
					}
				}
				b.Archive = append(b.Archive, ArchivedTask{Task: task, ArchivedAt: at})
				archiveTask = len(b.Archive) - 1
				archiveInTask = true
				archiveNotes = nil
				continue
			}
			if archiveInTask && (trimmed == "" || isIndented(line)) {
				archiveNotes = append(archiveNotes, line)
				continue
			}
			finalizeArchiveTask()
			if trimmed != "" {
				archiveRaw = append(archiveRaw, line)
			}
			continue
		}

		if m := columnHeadingRE.FindStringSubmatch(line); m != nil {
			finalizeTask()
			flushColumnPreamble()
			b.Columns = append(b.Columns, Column{Name: strings.TrimSpace(m[1])})
			colIndex = len(b.Columns) - 1
			continue
		}

		if m := taskLineRE.FindStringSubmatch(line); m != nil {
			if colIndex < 0 {
				finalizeTask()
				flushColumnPreamble()
				b.Columns = append(b.Columns, Column{Name: "Backlog"})
				colIndex = len(b.Columns) - 1
			}
			finalizeTask()
			task, warns := parseTaskText(m[2], lineNo)
			for _, w := range warns {
				warnings = append(warnings, w)
			}
			task.Done = strings.EqualFold(m[1], "x")
			b.Columns[colIndex].Tasks = append(b.Columns[colIndex].Tasks, task)
			taskIndex = len(b.Columns[colIndex].Tasks) - 1
			inTask = true
			noteBuf = nil
			continue
		}

		if inTask && (trimmed == "" || isIndented(line)) {
			noteBuf = append(noteBuf, line)
			continue
		}
		finalizeTask()

		if colIndex >= 0 {
			colPreamble = append(colPreamble, line)
		} else if trimmed != "" {
			preamble = append(preamble, line)
		}
	}
	finalizeTask()
	flushColumnPreamble()
	finalizeArchiveTask()

	b.Preamble = trimBlankLines(preamble)
	b.ArchiveRaw = trimBlankLines(archiveRaw)

	// A parsed file always has at least one column.
	if len(b.Columns) == 0 {
		name := b.CaptureColumn
		if name == "" {
			name = "Backlog"
		}
		b.Columns = []Column{{Name: name}}
	}

	normalizeColumns(&b)
	for ci := range b.Columns {
		if b.IsDoneColumn(ci) {
			for ti := range b.Columns[ci].Tasks {
				b.Columns[ci].Tasks[ti].Done = true
			}
		}
	}

	b, idWarnings := b.withIDs()
	warnings = append(warnings, idWarnings...)
	b.registerMilestones()

	return b, warnings, nil
}

func parseFrontmatter(src string, warnings []Warning) (Board, []Warning) {
	b := Board{RawFrontmatter: map[string]any{}}
	var raw map[string]any
	if err := yaml.Unmarshal([]byte(src), &raw); err != nil {
		warnings = append(warnings, Warning{Line: 2, Message: "frontmatter: " + err.Error()})
		return b, warnings
	}
	if raw == nil {
		return b, warnings
	}

	var meta frontmatter
	if err := yaml.Unmarshal([]byte(src), &meta); err != nil {
		warnings = append(warnings, Warning{Line: 2, Message: "frontmatter: " + err.Error()})
	}
	b.Title = meta.Title
	b.CaptureColumn = meta.CaptureColumn
	b.DoneColumn = meta.DoneColumn
	b.Milestones = meta.Milestones

	for key, value := range raw {
		switch key {
		case "tbdbub", "title", "capture_column", "done_column", "milestones":
		default:
			b.RawFrontmatter[key] = value
		}
	}
	return b, warnings
}

// normalizeColumns enforces the Done column convention: it is the last column,
// and a stale done_column name falls back to the last column.
func normalizeColumns(b *Board) {
	if len(b.Columns) == 0 {
		return
	}
	if b.DoneColumn != "" {
		idx := b.ColumnIndex(b.DoneColumn)
		if idx < 0 {
			if len(b.Columns) > 1 {
				b.DoneColumn = b.Columns[len(b.Columns)-1].Name
			} else {
				b.DoneColumn = ""
			}
		} else if idx != len(b.Columns)-1 {
			done := b.Columns[idx]
			b.Columns = append(b.Columns[:idx], b.Columns[idx+1:]...)
			b.Columns = append(b.Columns, done)
		}
	} else {
		for i, col := range b.Columns {
			if !strings.EqualFold(col.Name, "Done") {
				continue
			}
			if i != len(b.Columns)-1 {
				done := b.Columns[i]
				b.Columns = append(b.Columns[:i], b.Columns[i+1:]...)
				b.Columns = append(b.Columns, done)
			}
			break
		}
	}
	if b.CaptureColumn == "" {
		b.CaptureColumn = b.Columns[0].Name
	}
	idx := b.ColumnIndex(b.CaptureColumn)
	if idx < 0 || b.IsDoneColumn(idx) {
		b.CaptureColumn = b.Columns[b.CaptureIndex()].Name
	}
}

func (b *Board) registerMilestones() {
	known := make(map[string]bool, len(b.Milestones))
	for _, m := range b.Milestones {
		known[m.Name] = true
	}
	add := func(t Task) {
		if t.Milestone != "" && !known[t.Milestone] {
			known[t.Milestone] = true
			b.Milestones = append(b.Milestones, Milestone{Name: t.Milestone, Implicit: true})
		}
	}
	for _, col := range b.Columns {
		for _, task := range col.Tasks {
			add(task)
		}
	}
	for _, task := range b.Archive {
		add(task.Task)
	}
}

// parseTaskText splits inline tags, milestone, and ID from a task body.
func parseTaskText(text string, line int) (Task, []Warning) {
	var warnings []Warning
	var id TaskID

	if m := trailingIDRE.FindStringSubmatchIndex(text); m != nil {
		before := text[:m[0]]
		if !strings.HasSuffix(before, `\`) {
			id = TaskID(text[m[4]:m[5]])
			text = before
		}
	}

	var (
		title     strings.Builder
		tags      []string
		milestone string
	)
	runes := []rune(text)
	for i := 0; i < len(runes); {
		r := runes[i]
		if r == '\\' && i+1 < len(runes) && (runes[i+1] == '#' || runes[i+1] == '@' || runes[i+1] == '^') {
			title.WriteRune(runes[i+1])
			i += 2
			continue
		}
		if r == '#' || r == '@' {
			j := i + 1
			for j < len(runes) && tokenRune(runes[j], r == '@') {
				j++
			}
			if j > i+1 {
				token := string(runes[i+1 : j])
				if r == '#' {
					tags = append(tags, token)
				} else if milestone == "" {
					milestone = token
				} else {
					warnings = append(warnings, Warning{
						Line:    line,
						Message: "extra milestone @" + token + " ignored on task " + strings.TrimSpace(title.String()),
					})
				}
				i = j
				continue
			}
		}
		title.WriteRune(r)
		i++
	}

	return Task{
		ID:        id,
		Title:     strings.Join(strings.Fields(title.String()), " "),
		Tags:      tags,
		Milestone: milestone,
	}, warnings
}

// ParseTaskText exposes the inline-token parser for editors that want to warn
// before saving.
func ParseTaskText(text string) (title string, tags []string, milestone string, id TaskID, warnings []string) {
	task, warns := parseTaskText(text, 0)
	for _, w := range warns {
		warnings = append(warnings, w.Message)
	}
	return task.Title, task.Tags, task.Milestone, task.ID, warnings
}

func tokenRune(r rune, milestone bool) bool {
	if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
		return true
	}
	return milestone && r == '.'
}

func isIndented(line string) bool {
	return strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")
}

func joinNotes(lines []string) string {
	lines = trimBlankSlice(lines)
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n")
}

func trimBlankSlice(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func trimBlankLines(lines []string) string {
	return strings.Join(trimBlankSlice(lines), "\n")
}

// Write renders a board in canonical Markdown.
func Write(w io.Writer, b Board) error {
	b, _ = b.withIDs()
	out := b.render()
	_, err := io.WriteString(w, out)
	return err
}

func (b Board) render() string {
	var sb strings.Builder

	sb.WriteString("---\n")
	sb.WriteString("tbdbub: " + fmt.Sprint(fileVersion) + "\n")
	sb.WriteString("title: " + yamlScalar(b.Title) + "\n")
	if b.CaptureColumn != "" {
		sb.WriteString("capture_column: " + yamlScalar(b.CaptureColumn) + "\n")
	}
	if b.DoneColumn != "" {
		sb.WriteString("done_column: " + yamlScalar(b.DoneColumn) + "\n")
	}
	var milestones []Milestone
	for _, m := range b.Milestones {
		if m.Implicit && m.Description == "" && m.Due == "" && !m.Closed {
			continue
		}
		milestones = append(milestones, m)
	}
	if len(milestones) > 0 {
		sb.WriteString("milestones:\n")
		for _, m := range milestones {
			sb.WriteString("  - name: " + yamlScalar(m.Name) + "\n")
			if m.Description != "" {
				sb.WriteString("    description: " + yamlScalar(m.Description) + "\n")
			}
			if m.Due != "" {
				sb.WriteString("    due: " + yamlDue(m.Due) + "\n")
			}
			if m.Closed {
				sb.WriteString("    closed: true\n")
			}
		}
	}
	keys := make([]string, 0, len(b.RawFrontmatter))
	for key := range b.RawFrontmatter {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		raw, err := yaml.Marshal(map[string]any{key: b.RawFrontmatter[key]})
		if err != nil {
			continue
		}
		sb.WriteString(strings.TrimRight(string(raw), "\n"))
		sb.WriteString("\n")
	}
	sb.WriteString("---\n")

	if preamble := trimRightLines(b.Preamble); preamble != "" {
		sb.WriteString("\n")
		sb.WriteString(preamble)
		sb.WriteString("\n")
	}

	for _, col := range b.Columns {
		sb.WriteString("\n## " + col.Name + "\n")
		if pre := trimRightLines(col.Preamble); pre != "" {
			sb.WriteString("\n")
			sb.WriteString(pre)
			sb.WriteString("\n")
		}
		if len(col.Tasks) > 0 {
			sb.WriteString("\n")
		}
		for _, task := range col.Tasks {
			writeTask(&sb, task)
		}
	}

	if len(b.Archive) > 0 || strings.TrimSpace(b.ArchiveRaw) != "" {
		sb.WriteString("\n" + archiveMarker + "\n")
		writeArchive(&sb, b.Archive)
		if raw := trimRightLines(b.ArchiveRaw); raw != "" {
			sb.WriteString("\n")
			sb.WriteString(raw)
			sb.WriteString("\n")
		}
	}

	return strings.TrimRight(sb.String(), "\n") + "\n"
}

func writeTask(sb *strings.Builder, task Task) {
	box := "[ ]"
	if task.Done {
		box = "[x]"
	}
	line := "- " + box
	if title := escapeTitle(task.Title); title != "" {
		line += " " + title
	}
	for _, tag := range task.Tags {
		line += " #" + tag
	}
	if task.Milestone != "" {
		line += " @" + task.Milestone
	}
	if task.ID != "" {
		line += " ^" + string(task.ID)
	}
	sb.WriteString(strings.TrimRight(line, " ") + "\n")
	writeNotes(sb, task.Notes)
}

func writeArchive(sb *strings.Builder, archived []ArchivedTask) {
	order := make([]string, 0)
	groups := map[string][]ArchivedTask{}
	for _, task := range archived {
		key := ""
		if !task.ArchivedAt.IsZero() {
			key = task.ArchivedAt.UTC().Format("2006-01")
		}
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		groups[key] = append(groups[key], task)
	}
	sort.Slice(order, func(i, j int) bool {
		if order[i] == "" {
			return false
		}
		if order[j] == "" {
			return true
		}
		return order[i] < order[j]
	})
	for i, key := range order {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("## Archived")
		if key != "" {
			sb.WriteString(" " + key)
		}
		sb.WriteString("\n\n")
		for _, task := range groups[key] {
			task.Done = true
			writeTask(sb, task.Task)
		}
	}
}

func writeNotes(sb *strings.Builder, notes string) {
	if notes == "" {
		return
	}
	for _, line := range strings.Split(notes, "\n") {
		line = strings.TrimRight(line, " \t")
		if line == "" {
			sb.WriteString("\n")
			continue
		}
		if !isIndented(line) {
			line = "  " + line
		}
		sb.WriteString(line + "\n")
	}
}

// escapeTitle escapes characters that would otherwise be parsed as inline
// tokens. The canonical writer re-escapes them so a round-trip is stable.
func escapeTitle(title string) string {
	var sb strings.Builder
	for _, r := range title {
		if r == '#' || r == '@' || r == '^' {
			sb.WriteRune('\\')
		}
		sb.WriteRune(r)
	}
	return strings.Join(strings.Fields(sb.String()), " ")
}

// trimRightLines removes trailing whitespace from every line while keeping the
// line structure and removing leading/trailing blank lines.
func trimRightLines(text string) string {
	lines := strings.Split(text, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t")
	}
	return trimBlankLines(lines)
}

func yamlScalar(s string) string {
	out, err := yaml.Marshal(s)
	if err != nil {
		return `""`
	}
	return strings.TrimRight(string(out), "\n")
}

// yamlDue keeps ISO dates unquoted, matching the documented file format.
func yamlDue(s string) string {
	if len(s) == 10 && s[4] == '-' && s[7] == '-' {
		digits := s[0:4] + s[5:7] + s[8:10]
		numeric := true
		for _, r := range digits {
			if r < '0' || r > '9' {
				numeric = false
				break
			}
		}
		if numeric {
			return s
		}
	}
	return yamlScalar(s)
}
