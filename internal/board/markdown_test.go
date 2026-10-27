package board

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestParseHomeFixture(t *testing.T) {
	f, err := os.Open("testdata/home.md")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	b, warnings, err := Parse(f)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}

	if b.Title != "Home renovation" || b.CaptureColumn != "Backlog" {
		t.Fatalf("board = %+v", b)
	}
	if len(b.Columns) != 3 {
		t.Fatalf("columns = %d, want 3", len(b.Columns))
	}
	if len(b.Milestones) != 2 {
		t.Fatalf("milestones = %+v", b.Milestones)
	}
	if len(b.Archive) != 1 || b.Archive[0].ID != "t4qq" {
		t.Fatalf("archive = %+v", b.Archive)
	}
	if b.Archive[0].ArchivedAt.Format("2006-01") != "2026-10" {
		t.Fatalf("archived at = %v", b.Archive[0].ArchivedAt)
	}

	task := b.Columns[0].Tasks[0]
	if task.Title != "Replace faucet" || task.Milestone != "Kitchen" || len(task.Tags) != 2 {
		t.Fatalf("task = %+v", task)
	}
	if !strings.Contains(task.Notes, "Cartridge is 35mm") {
		t.Fatalf("notes = %q", task.Notes)
	}
	if !strings.Contains(b.Columns[0].Tasks[1].Notes, "shortlist") {
		t.Fatalf("nested notes = %q", b.Columns[0].Tasks[1].Notes)
	}
	if !b.IsDoneColumn(2) || !b.Columns[2].Tasks[0].Done {
		t.Fatal("Done column not detected")
	}
}

func TestCanonicalRoundTrip(t *testing.T) {
	src, err := os.ReadFile("testdata/home.md")
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := Parse(bytes.NewReader(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	var out bytes.Buffer
	if err := Write(&out, b); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if out.String() != string(src) {
		t.Fatalf("round-trip mismatch:\n--- want ---\n%s\n--- got ---\n%s", src, out.String())
	}

	// Writing twice must be byte-identical.
	var second bytes.Buffer
	b2, _, err := Parse(bytes.NewReader(out.Bytes()))
	if err != nil {
		t.Fatalf("Parse written board: %v", err)
	}
	if err := Write(&second, b2); err != nil {
		t.Fatalf("Write second: %v", err)
	}
	if second.String() != out.String() {
		t.Fatal("canonical output is not stable")
	}
}

func TestParseTolerant(t *testing.T) {
	src := `---
title: Broken
tbdbub: 1
milestones:
  - name: A
---

Preamble text.

## Backlog
- [ ] No ID #one @A @B ^abcd
  note
* [X] Star bullet #two
outside text

## Things

## Done
- [x] checked ^abcd
`
	b, warnings, err := Parse(strings.NewReader(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if b.Title != "Broken" {
		t.Fatalf("title = %q", b.Title)
	}
	if b.Preamble != "Preamble text." {
		t.Fatalf("preamble = %q", b.Preamble)
	}
	if len(b.Columns) != 3 {
		t.Fatalf("columns = %+v", b.Columns)
	}
	if b.Columns[0].Tasks[0].Milestone != "A" {
		t.Fatalf("first milestone should win: %+v", b.Columns[0].Tasks[0])
	}
	if !containsWarning(warnings, "extra milestone") {
		t.Fatalf("warnings = %v", warnings)
	}
	if !b.Columns[0].Tasks[1].Done {
		t.Fatal("uppercase X not parsed as done")
	}
	if b.Columns[0].Preamble != "outside text" {
		t.Fatalf("column preamble = %q", b.Columns[0].Preamble)
	}
	// Duplicate IDs are regenerated while keeping each task addressable.
	used := map[TaskID]bool{}
	for _, col := range b.Columns {
		for _, task := range col.Tasks {
			if used[task.ID] {
				t.Fatalf("duplicate ID %q", task.ID)
			}
			used[task.ID] = true
		}
	}
	// The "Done" column stays last even without done_column in frontmatter.
	if !b.IsDoneColumn(2) {
		t.Fatal("Done column not detected")
	}
	var out bytes.Buffer
	if err := Write(&out, b); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "## Done") || !strings.HasSuffix(out.String(), "\n") {
		t.Fatalf("written board:\n%s", out.String())
	}
}

func TestParseInlineEscapes(t *testing.T) {
	title, tags, milestone, id, warnings := ParseTaskText(`Fix \#42 and \@home ^t1ab`)
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	if title != "Fix #42 and @home" {
		t.Fatalf("title = %q", title)
	}
	if len(tags) != 0 || milestone != "" || id != "t1ab" {
		t.Fatalf("tags=%v milestone=%q id=%q", tags, milestone, id)
	}

	title, tags, milestone, _, _ = ParseTaskText("Ship #ui #urgent @v1.2")
	if title != "Ship" || len(tags) != 2 || tags[1] != "urgent" || milestone != "v1.2" {
		t.Fatalf("title=%q tags=%v milestone=%q", title, tags, milestone)
	}
}

func TestWriteNewBoard(t *testing.T) {
	b := NewBoard("Fresh")
	var out bytes.Buffer
	if err := Write(&out, b); err != nil {
		t.Fatal(err)
	}
	want := "---\ntbdbub: 1\ntitle: Fresh\ncapture_column: Backlog\n---\n\n## Backlog\n"
	if out.String() != want {
		t.Fatalf("got:\n%q\nwant:\n%q", out.String(), want)
	}
}

func TestParseEmpty(t *testing.T) {
	b, warnings, err := Parse(strings.NewReader(""))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v", warnings)
	}
	if len(b.Columns) != 1 || b.Columns[0].Name != "Backlog" {
		t.Fatalf("columns = %+v", b.Columns)
	}
	if b.CaptureColumn != "Backlog" {
		t.Fatalf("capture = %q", b.CaptureColumn)
	}
}

func FuzzParse(f *testing.F) {
	seed, err := os.ReadFile("testdata/home.md")
	if err != nil {
		f.Fatal(err)
	}
	f.Add(string(seed))
	f.Add("---\ntitle: x\n---\n## A\n- [ ] t ^abcd\n")
	f.Add("## A\n- [ ] t\n  note\n- [x] u\n")
	f.Add("<!-- tbdbub:archive -->\n## Archived 2026-01\n- [x] old ^abcd\n")

	f.Fuzz(func(t *testing.T, src string) {
		b, _, err := Parse(strings.NewReader(src))
		if err != nil {
			return
		}
		var out bytes.Buffer
		if err := Write(&out, b); err != nil {
			t.Fatalf("Write: %v", err)
		}
		if _, _, err := Parse(bytes.NewReader(out.Bytes())); err != nil {
			t.Fatalf("re-parse: %v\n%s", err, out.String())
		}
	})
}

func containsWarning(warnings []Warning, substr string) bool {
	for _, w := range warnings {
		if strings.Contains(w.Message, substr) {
			return true
		}
	}
	return false
}
