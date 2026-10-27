package board

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestUnknownFrontmatterAndPreamblePreserved(t *testing.T) {
	src := `---
tbdbub: 1
title: Custom
capture_column: Backlog
owner: kron
meta:
  nested: yes
---

Intro text.  

## Backlog

Column preamble.  

- [ ] Task #x ^abcd
  A note.  
`
	b, warnings, err := Parse(strings.NewReader(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	if b.Preamble != "Intro text.  " {
		t.Fatalf("preamble = %q", b.Preamble)
	}
	if b.Columns[0].Preamble != "Column preamble.  " {
		t.Fatalf("column preamble = %q", b.Columns[0].Preamble)
	}
	if b.RawFrontmatter["owner"] != "kron" {
		t.Fatalf("raw frontmatter = %+v", b.RawFrontmatter)
	}

	var out bytes.Buffer
	if err := Write(&out, b); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, line := range strings.Split(got, "\n") {
		if strings.HasSuffix(line, " ") {
			t.Fatalf("trailing space in %q", line)
		}
	}
	if !strings.Contains(got, "owner: kron") || !strings.Contains(got, "nested:") {
		t.Fatalf("unknown frontmatter lost:\n%s", got)
	}
	if !strings.Contains(got, "Intro text.") || !strings.Contains(got, "Column preamble.") {
		t.Fatalf("preamble lost:\n%s", got)
	}
}

func TestNotesSurviveArchiveRoundTrip(t *testing.T) {
	b := NewBoard("x")
	b, _ = b.AddTask(0, "task #a @m")
	id := b.Columns[0].Tasks[0].ID
	notes := "line one\n- [ ] nested\nline three"
	b, _ = b.UpdateTask(id, TaskPatch{Notes: &notes})
	b, _ = b.ToggleDone(id)

	archived, err := b.ArchiveTasks([]TaskID{id}, timeAt("2026-10"))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Write(&out, archived); err != nil {
		t.Fatal(err)
	}
	parsed, _, err := Parse(bytes.NewReader(out.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Archive) != 1 {
		t.Fatalf("archive = %+v", parsed.Archive)
	}
	got := parsed.Archive[0]
	wantNotes := "  line one\n  - [ ] nested\n  line three"
	if got.ID != id || got.Notes != wantNotes || got.Milestone != "m" || !got.Done {
		t.Fatalf("archived task = %+v", got)
	}
	if got.ArchivedAt.Format("2006-01") != "2026-10" {
		t.Fatalf("archived at = %v", got.ArchivedAt)
	}
}

func TestDuplicateIDRegenerated(t *testing.T) {
	src := `---
title: x
capture_column: Backlog
---

## Backlog

- [ ] one ^abcd
- [ ] two ^abcd

## Done

- [x] three ^abcd
`
	b, warnings, err := Parse(strings.NewReader(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !containsWarning(warnings, "duplicate task ID") {
		t.Fatalf("warnings = %v", warnings)
	}
	seen := map[TaskID]bool{}
	for _, col := range b.Columns {
		for _, task := range col.Tasks {
			if seen[task.ID] {
				t.Fatalf("duplicate ID %q remains", task.ID)
			}
			seen[task.ID] = true
		}
	}
	if !seen["abcd"] {
		t.Fatal("first occurrence of the ID was not preserved")
	}
}

func TestTaskWithoutIDStableAfterWrite(t *testing.T) {
	src := "---\ntitle: x\n---\n\n## Backlog\n\n- [ ] no id\n"
	b, _, err := Parse(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	id := b.Columns[0].Tasks[0].ID
	if id == "" {
		t.Fatal("parser did not assign an ID")
	}
	var out bytes.Buffer
	if err := Write(&out, b); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "^"+string(id)) {
		t.Fatalf("written board lost the assigned ID:\n%s", out.String())
	}
}

func TestWriteAssignsMissingIDs(t *testing.T) {
	b := Board{
		Title:         "x",
		CaptureColumn: "Backlog",
		Columns:       []Column{{Name: "Backlog", Tasks: []Task{{Title: "no id"}}}},
	}
	var out bytes.Buffer
	if err := Write(&out, b); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "no id ^") {
		t.Fatalf("writer did not assign an ID:\n%s", out.String())
	}
}

func timeAt(month string) (t time.Time) {
	t, _ = time.Parse("2006-01", month)
	return t
}

func TestImplicitMilestonesNotWritten(t *testing.T) {
	b := NewBoard("x")
	b, _ = b.AddTask(0, "task @v1")
	m, ok := b.MilestoneByName("v1")
	if !ok || !m.Implicit {
		t.Fatalf("milestone = %+v, %v", m, ok)
	}
	var out bytes.Buffer
	if err := Write(&out, b); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "milestones:") {
		t.Fatalf("implicit milestone written to frontmatter:\n%s", out.String())
	}
	parsed, _, err := Parse(strings.NewReader(out.String()))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := parsed.MilestoneByName("v1"); !ok {
		t.Fatal("inline milestone was not re-registered on parse")
	}

	due := "2026-12-01"
	b, err = b.UpdateMilestone("v1", MilestonePatch{Due: &due})
	if err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := Write(&out, b); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "due: 2026-12-01") {
		t.Fatalf("milestone metadata missing:\n%s", out.String())
	}
}
