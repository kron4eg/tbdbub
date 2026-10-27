package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/kron4eg/tbdbub/internal/board"
)

// View implements tea.Model.
func (m Model) View() tea.View {
	content := m.render()
	v := tea.NewView(content)
	v.AltScreen = true
	v.WindowTitle = "tbdbub"
	return v
}

func (m Model) render() string {
	switch m.mode {
	case ModeHelp:
		return m.helpView()
	case ModeTaskEditor:
		return m.overlayFrame(m.editorOverlay())
	case ModeColumnEditor:
		return m.overlayFrame(m.promptOverlay())
	case ModeFilter:
		return m.overlayFrame(m.filterOverlay())
	case ModeMilestones:
		return m.overlayFrame(m.milestonesOverlay())
	case ModeArchive:
		return m.overlayFrame(m.archiveOverlay())
	case ModeConfirm:
		return m.overlayFrame(m.confirmOverlay())
	default:
		return m.boardView()
	}
}

func (m Model) boardView() string {
	bodyHeight := max(1, m.height-2)
	return strings.Join([]string{
		m.headerView(),
		m.bodyView(bodyHeight),
		m.footerView(),
	}, "\n")
}

func (m Model) overlayFrame(box string) string {
	bodyHeight := max(1, m.height-2)
	body := lipgloss.Place(m.width, bodyHeight, lipgloss.Center, lipgloss.Center, box)
	body = fitBlock(body, m.width, bodyHeight)
	return strings.Join([]string{m.headerView(), body, m.footerView()}, "\n")
}

func (m Model) headerView() string {
	title := m.Board.Title
	if title == "" {
		title = "untitled"
	}
	left := " tbdbub · " + title

	indicator := "✓"
	if m.dirty {
		indicator = "●"
	}
	file := "demo"
	if m.path != "" {
		parts := strings.Split(m.path, "/")
		file = parts[len(parts)-1]
	}
	rightParts := []string{file + " " + indicator}
	if chips := m.filterChips(); chips != "" {
		rightParts = append(rightParts, chips)
	}
	if progress := m.milestoneProgress(); progress != "" {
		rightParts = append(rightParts, progress)
	}
	if len(m.warn) > 0 {
		rightParts = append(rightParts, fmt.Sprintf("%d warnings", len(m.warn)))
	}
	right := " " + strings.Join(rightParts, " · ") + " "

	return headerStyle.Width(m.width).Render(spread(left, right, m.width))
}

func (m Model) filterChips() string {
	var parts []string
	if text := strings.TrimSpace(m.filter.Text); text != "" {
		parts = append(parts, "/"+text)
	}
	if len(m.filter.Tags) > 0 {
		sep := ","
		if m.filter.TagMode == board.TagAll {
			sep = "+"
		}
		tags := make([]string, len(m.filter.Tags))
		for i, tag := range m.filter.Tags {
			tags[i] = "#" + tag
		}
		parts = append(parts, strings.Join(tags, sep))
	}
	switch m.filter.Milestone {
	case "":
	case board.NoMilestone:
		parts = append(parts, "no milestone")
	default:
		parts = append(parts, "@"+m.filter.Milestone)
	}
	if !m.filter.ShowDone {
		parts = append(parts, "hide done")
	}
	return strings.Join(parts, " ")
}

func (m Model) milestoneProgress() string {
	name := m.filter.Milestone
	if name == "" || name == board.NoMilestone {
		if len(m.Board.Milestones) == 0 {
			return ""
		}
		name = m.Board.Milestones[0].Name
	}
	done, total := m.Board.MilestoneProgress(name)
	if total == 0 {
		return name
	}
	return fmt.Sprintf("%s %d/%d", name, done, total)
}

func (m Model) footerView() string {
	left := m.footerKeys()
	if m.count > 0 && m.mode == ModeBoard {
		left = fmt.Sprintf(" %d× ", m.count) + strings.TrimLeft(left, " ")
	}
	right := m.status
	if m.mode == ModeArchive && m.archive.searching {
		right = "searching archive"
	}
	return footerStyle.Width(m.width).Render(spread(left, right, m.width))
}

func (m Model) footerKeys() string {
	switch m.mode {
	case ModeHelp:
		return " esc close help "
	case ModeGrab:
		return " GRAB  h/l column  j/k slot  1-9 jump  enter drop  esc cancel "
	case ModeTaskEditor:
		return " EDIT  tab field  ctrl+s save  esc cancel "
	case ModeColumnEditor:
		return " NAME  enter save  esc cancel "
	case ModeFilter:
		return " FILTER  tab section  space toggle  a any/all  c clear  ctrl+r reset  esc apply "
	case ModeMilestones:
		return " MILESTONES  j/k  enter assign  c clear  n new  e rename  d delete  esc close "
	case ModeArchive:
		if m.archive.searching {
			return " SEARCH  enter/esc finish  ctrl+c quit "
		}
		return " ARCHIVE  j/k  / search  u restore to Done  U reopen  esc close "
	case ModeConfirm:
		switch m.confirm.kind {
		case confirmQuit:
			return " s save & quit  d discard  esc cancel "
		case confirmConflict:
			return " r reload  o overwrite  esc cancel "
		default:
			return " y/enter confirm  n/esc cancel "
		}
	default:
		return " NORMAL  n new  N column  e edit  space done  m move  H/L move  / filter  ? help  q quit "
	}
}

func (m Model) bodyView(height int) string {
	if len(m.Board.Columns) == 0 {
		return lipgloss.Place(m.width, height, lipgloss.Center, lipgloss.Center,
			metaStyle.Render("no columns"))
	}
	if m.isNarrow() {
		return m.carouselView(height)
	}

	start, end := m.visibleColumns()
	colWidth := m.columnWidth(end - start)

	parts := make([]string, 0, end-start)
	gap := lipgloss.NewStyle().Width(1).Height(height).Render(" ")
	for i := start; i < end; i++ {
		if i > start {
			parts = append(parts, gap)
		}
		parts = append(parts, m.columnView(i, colWidth, height))
	}

	body := lipgloss.JoinHorizontal(lipgloss.Top, parts...)
	return fitBlock(body, m.width, height)
}

func (m Model) carouselView(height int) string {
	tabs := m.tabLine()
	columnHeight := max(1, height-1)
	col := m.columnView(m.column, m.width, columnHeight)
	return fitBlock(tabs+"\n"+col, m.width, height)
}

func (m Model) tabLine() string {
	names := make([]string, len(m.Board.Columns))
	for i, col := range m.Board.Columns {
		name := col.Name
		if m.grab != nil {
			name = fmt.Sprintf("%d %s", i+1, name)
		}
		if i == m.column {
			name = "▸ " + name
		}
		names[i] = name
	}
	start, end := 0, len(names)
	for end-start > 1 {
		line := " " + strings.Join(names[start:end], " │ ") + " "
		if lipglossWidth(line) <= m.width {
			break
		}
		if m.column <= start {
			end--
		} else if m.column >= end-1 {
			start++
		} else {
			start++
			end--
		}
	}
	line := " " + strings.Join(names[start:end], " │ ") + " "
	line = truncate(line, m.width)
	if pad := m.width - lipglossWidth(line); pad > 0 {
		line += strings.Repeat(" ", pad)
	}
	return tabBarStyle.Render(line)
}

func (m Model) columnView(index, width, height int) string {
	col := m.Board.Columns[index]
	focused := index == m.column
	grabTarget := m.grab != nil && index == m.grab.column

	vis := m.visibleIndices(index)
	count := fmt.Sprintf("%d", len(vis))
	if m.filter.Active() && m.grab == nil {
		count = fmt.Sprintf("%d/%d", len(vis), len(col.Tasks))
	}

	title := fmt.Sprintf("%s (%s)", col.Name, count)
	if m.grab != nil {
		title = fmt.Sprintf("%d %s", index+1, title)
	}
	if focused {
		title = "▸ " + title
	} else {
		title = "  " + title
	}
	title = truncate(title, max(1, width-4))
	titleStyle := columnTitleStyle
	if focused {
		titleStyle = columnTitleFocusedStyle
	}

	innerHeight := max(1, height-2)
	cardArea := max(1, innerHeight-1)
	if grabTarget && cardArea > 1 {
		cardArea--
	}
	visibleCards := max(1, cardArea/cardHeight)

	offset := 0
	if grabTarget && m.grab != nil {
		offset = clamp(m.grab.slot-visibleCards/2, 0, max(0, len(vis)-visibleCards))
	} else if focused {
		pos := 0
		for i, taskIndex := range vis {
			if taskIndex == m.task {
				pos = i
				break
			}
		}
		if pos >= visibleCards {
			offset = pos - visibleCards + 1
		}
	}
	if maxOffset := max(0, len(vis)-visibleCards); offset > maxOffset {
		offset = maxOffset
	}

	lines := []string{titleStyle.Render(title)}
	markerDone := false
	for pos := offset; pos < len(vis) && len(lines) < innerHeight; pos++ {
		if grabTarget && m.grab != nil && pos == m.grab.slot {
			lines = append(lines, m.grabMarker(width-4))
			markerDone = true
			if len(lines) >= innerHeight {
				break
			}
		}
		task := col.Tasks[vis[pos]]
		dim := m.grab != nil && task.ID == m.grab.id
		selected := focused && vis[pos] == m.task
		lines = append(lines, m.cardView(task, selected, dim, width-4))
	}
	if grabTarget && m.grab != nil && !markerDone && m.grab.slot >= len(vis) && len(lines) < innerHeight {
		lines = append(lines, m.grabMarker(width-4))
	}
	if len(vis) == 0 {
		if m.filter.Active() && m.grab == nil {
			lines = append(lines, metaStyle.Render("no matches"))
		} else {
			lines = append(lines, metaStyle.Render("empty"))
		}
	}
	for len(lines) < innerHeight {
		lines = append(lines, "")
	}
	if len(lines) > innerHeight {
		lines = lines[:innerHeight]
	}

	style := columnStyle.Width(width).Height(height)
	if focused {
		style = columnFocusedStyle.Width(width).Height(height)
	}
	if grabTarget {
		style = grabColumnStyle.Width(width).Height(height)
	}
	return style.Render(strings.Join(lines, "\n"))
}

func (m Model) grabMarker(width int) string {
	width = max(4, width)
	line := "  ── drop here "
	if lipglossWidth(line) > width {
		line = truncate(line, width)
	}
	return grabMarkerStyle.Render(line)
}

func (m Model) cardView(task board.Task, selected, dim bool, width int) string {
	width = max(8, width)
	titleWidth := max(1, width-4)

	prefix := "● "
	if task.Done {
		prefix = "✓ "
	}
	title := truncate(prefix+task.Title, titleWidth)

	titleStyle := cardTitleStyle
	if task.Done {
		titleStyle = cardTitleDoneStyle
	}
	if selected {
		titleStyle = cardTitleSelectedStyle
	}

	meta := truncate(taskMeta(task), titleWidth)
	content := titleStyle.Render(title) + "\n" + metaStyle.Render(meta)

	if dim {
		return dimCardStyle.Width(width).Height(cardHeight).Render(content)
	}
	style := cardStyle.Width(width).Height(cardHeight)
	if selected {
		style = cardSelectedStyle.Width(width).Height(cardHeight)
	}
	return style.Render(content)
}

func taskMeta(task board.Task) string {
	parts := make([]string, 0, len(task.Tags)+1)
	for _, tag := range task.Tags {
		parts = append(parts, "#"+tag)
	}
	if task.Milestone != "" {
		parts = append(parts, "@"+task.Milestone)
	}
	return strings.Join(parts, " ")
}

func (m Model) visibleColumns() (start, end int) {
	n := len(m.Board.Columns)
	if n == 0 {
		return 0, 0
	}
	fit := max(1, m.width/minColumnWidth)
	if fit >= n {
		return 0, n
	}
	start = m.column - fit/2
	if start < 0 {
		start = 0
	}
	if start+fit > n {
		start = n - fit
	}
	return start, start + fit
}

func (m Model) columnWidth(visible int) int {
	if visible <= 0 {
		return m.width
	}
	width := (m.width - (visible - 1)) / visible
	if width > maxColumnWidth {
		width = maxColumnWidth
	}
	return max(12, width)
}

// ---------- overlays ----------

func (m Model) editorOverlay() string {
	e := m.editor
	title := "Edit task"
	if e.taskID == "" {
		title = "New task"
	}
	lines := []string{
		fieldLabel("Title", e.focus == 0) + e.title.View(),
		fieldLabel("Notes", e.focus == 1),
	}
	lines = append(lines, strings.Split(e.notes.View(), "\n")...)
	lines = append(lines,
		fieldLabel("Tags", e.focus == 2)+e.tags.View(),
		fieldLabel("Milestone", e.focus == 3)+e.milestone.View(),
	)
	if e.err != "" {
		lines = append(lines, "", overlayErrorStyle.Render(e.err))
	}
	lines = append(lines, "", overlayHintStyle.Render("tab field · ctrl+s save · esc cancel"))
	return m.overlayBox(title, lines)
}

func fieldLabel(name string, focused bool) string {
	if focused {
		return overlaySelectedStyle.Render("▸ "+name) + "  "
	}
	return "  " + name + "  "
}

func (m Model) promptOverlay() string {
	p := m.prompt
	lines := []string{
		p.input.View(),
	}
	if p.err != "" {
		lines = append(lines, overlayErrorStyle.Render(p.err))
	}
	lines = append(lines, "", overlayHintStyle.Render("enter save · esc cancel"))
	return m.overlayBox(p.title, lines)
}

func (m Model) filterOverlay() string {
	p := m.filterPane
	lines := []string{
		fieldLabel("Search", p.focus == 0) + p.text.View(),
		"",
		sectionLabel(fmt.Sprintf("Tags · %s", tagModeName(p.tagMode)), p.focus == 1),
	}
	if len(p.tags) == 0 {
		lines = append(lines, overlayHintStyle.Render("  no tags in use"))
	}
	for i, tag := range p.tags {
		mark := "[ ]"
		if p.chosen[tag] {
			mark = "[x]"
		}
		prefix := "  "
		style := overlayHintStyle
		if p.focus == 1 && i == p.tagIdx {
			prefix = "▸ "
			style = overlaySelectedStyle
		}
		lines = append(lines, prefix+mark+" #"+style.Render(tag))
	}
	lines = append(lines, "", sectionLabel("Milestone", p.focus == 2))
	for i, milestone := range p.milestones {
		name := milestone
		switch milestone {
		case "":
			name = "any"
		case board.NoMilestone:
			name = "no milestone"
		default:
			name = "@" + milestone
		}
		mark := "( )"
		if i == p.milIdx {
			mark = "(•)"
		}
		prefix := "  "
		style := overlayHintStyle
		if p.focus == 2 && i == p.milIdx {
			prefix = "▸ "
			style = overlaySelectedStyle
		}
		lines = append(lines, prefix+mark+" "+style.Render(name))
	}
	showDone := "[ ]"
	if p.showDone {
		showDone = "[x]"
	}
	lines = append(lines, "", sectionLabel("Options", p.focus == 3))
	lines = append(lines, "  "+showDone+" show done tasks")
	lines = append(lines, "", overlayHintStyle.Render("tab section · space toggle · a any/all · c clear · ctrl+r reset · esc apply"))
	return m.overlayBox("Filter", lines)
}

func tagModeName(mode board.TagMode) string {
	if mode == board.TagAll {
		return "all"
	}
	return "any"
}

func sectionLabel(name string, focused bool) string {
	if focused {
		return overlaySelectedStyle.Render("▸ " + name)
	}
	return "  " + name
}

func (m Model) milestonesOverlay() string {
	lines := []string{}
	if len(m.Board.Milestones) == 0 {
		lines = append(lines, overlayHintStyle.Render("No milestones yet. Press n to add one."))
	}
	for i, milestone := range m.Board.Milestones {
		done, total := m.Board.MilestoneProgress(milestone.Name)
		extra := ""
		if milestone.Due != "" {
			extra += " due " + milestone.Due
		}
		if milestone.Closed {
			extra += " closed"
		}
		line := fmt.Sprintf("%s  %d/%d%s", milestone.Name, done, total, extra)
		if i == m.miles.index {
			lines = append(lines, overlaySelectedStyle.Render("▸ "+line))
		} else {
			lines = append(lines, "  "+line)
		}
	}
	lines = append(lines, "", overlayHintStyle.Render("j/k · enter assign · c clear · n new · e rename · u due · i description · d delete · esc close"))
	return m.overlayBox("Milestones", lines)
}

func (m Model) archiveOverlay() string {
	var lines []string
	if m.archive.searching {
		lines = append(lines, fieldLabel("Search", true)+m.archive.search.View())
	} else {
		lines = append(lines, overlayHintStyle.Render("Search: /"))
	}
	lines = append(lines, "")
	visible := m.archiveVisible()
	if len(visible) == 0 {
		if len(m.Board.Archive) == 0 {
			lines = append(lines, overlayHintStyle.Render("Archive is empty"))
		} else {
			lines = append(lines, overlayHintStyle.Render("No archived tasks match"))
		}
	}
	maxRows := max(1, m.height-10)
	for pos, index := range visible {
		if pos >= maxRows {
			break
		}
		task := m.Board.Archive[index]
		month := ""
		if !task.ArchivedAt.IsZero() {
			month = task.ArchivedAt.Format("2006-01") + " "
		}
		meta := taskMeta(task.Task)
		line := fmt.Sprintf("%s%s", month, task.Title)
		if meta != "" {
			line += "  " + meta
		}
		if pos == m.archive.index {
			lines = append(lines, overlaySelectedStyle.Render("▸ "+line))
		} else {
			lines = append(lines, "  "+line)
		}
	}
	lines = append(lines, "", overlayHintStyle.Render("j/k move · u restore to Done · U reopen · esc close"))
	return m.overlayBox("Archive", lines)
}

func (m Model) confirmOverlay() string {
	c := m.confirm
	lines := []string{c.message}
	switch c.kind {
	case confirmDeleteTask, confirmArchiveAll, confirmDeleteMilestone, confirmDeleteColumn, confirmArchiveDeleteDone:
		lines = append(lines, "", overlayHintStyle.Render("y/enter confirm · n/esc cancel"))
	}
	return m.overlayBox(c.title, lines)
}

func (m Model) overlayBox(title string, lines []string) string {
	width := clamp(m.width-2, 24, 78)
	inner := max(10, width-4)
	rendered := []string{overlayTitleStyle.Render(truncate(title, inner))}
	for _, line := range lines {
		for _, part := range strings.Split(line, "\n") {
			rendered = append(rendered, lipgloss.NewStyle().MaxWidth(inner).Render(part))
		}
	}
	maxLines := max(3, m.height-6)
	if len(rendered) > maxLines {
		rendered = rendered[:maxLines]
	}
	return overlayStyle.Width(width).Render(strings.Join(rendered, "\n"))
}

func (m Model) helpView() string {
	lines := []string{
		helpLine("h / l", "previous / next column"),
		helpLine("j / k", "previous / next task"),
		helpLine("g / G", "first / last task"),
		helpLine("H / L", "move task left / right"),
		helpLine("J / K", "reorder task down / up"),
		helpLine("m", "grab and drop"),
		helpLine("space", "toggle done"),
		helpLine("n / e", "new / edit task"),
		helpLine("t", "edit tags"),
		helpLine("N / r / D", "new / rename / delete column"),
		helpLine("M", "milestones · n/e/u/i/d"),
		helpLine("d / a", "delete / archive task"),
		helpLine("A", "archive all Done"),
		helpLine("V", "archive view"),
		helpLine("f / /", "filter / search"),
		helpLine("esc", "clear filters"),
		helpLine("u / ctrl+r", "undo / redo"),
		helpLine("s", "save"),
		helpLine("?", "toggle this help"),
		helpLine("q", "quit"),
	}
	return m.overlayFrame(m.overlayBox("tbdbub · keys", lines))
}

func helpLine(key, desc string) string {
	return helpKeyStyle.Width(11).Render(key) + helpDescStyle.Render(desc)
}

// fitBlock pads or trims a rendered block to exactly width x height cells.
func fitBlock(block string, width, height int) string {
	lines := strings.Split(block, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for i, line := range lines {
		line = lipgloss.NewStyle().MaxWidth(width).Render(line)
		if pad := width - lipgloss.Width(line); pad > 0 {
			line += strings.Repeat(" ", pad)
		}
		lines[i] = line
	}
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", max(0, width)))
	}
	return strings.Join(lines, "\n")
}

func lipglossWidth(s string) int {
	return lipgloss.Width(s)
}
