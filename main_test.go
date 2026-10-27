package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTitleFromPath(t *testing.T) {
	cases := map[string]string{
		"board.md":                "board",
		"/tmp/home renovation.md": "home renovation",
		"noext":                   "noext",
		"":                        "tbdbub",
	}
	for path, want := range cases {
		if got := titleFromPath(path); got != want {
			t.Errorf("titleFromPath(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestResolvePath(t *testing.T) {
	t.Setenv("TBDBUB_BOARD", "/tmp/from-env.md")
	if got := resolvePath(nil); got != "/tmp/from-env.md" {
		t.Fatalf("env path = %q", got)
	}
	if got := resolvePath([]string{"/tmp/explicit.md"}); got != "/tmp/explicit.md" {
		t.Fatalf("arg path = %q", got)
	}
	t.Setenv("TBDBUB_BOARD", "")
	if got := resolvePath(nil); got != "board.md" {
		t.Fatalf("default path = %q", got)
	}
}

func TestLoadOrCreate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "home.md")

	b, hash, warnings, err := loadOrCreate(path)
	if err != nil {
		t.Fatalf("loadOrCreate: %v", err)
	}
	if b.Title != "home" || len(warnings) != 0 || hash == "" {
		t.Fatalf("board=%+v hash=%q warnings=%v", b, hash, warnings)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("created file: %v", err)
	}
	if len(data) == 0 || hash != hashBytes(data) {
		t.Fatal("created file hash mismatch")
	}

	got, gotHash, _, err := loadOrCreate(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.Title != "home" || gotHash != hash {
		t.Fatalf("reloaded title=%q hash=%q", got.Title, gotHash)
	}
}

func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "board.md")
	if err := os.WriteFile(path, []byte("old"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := writeFileAtomic(path, []byte("new")); err != nil {
		t.Fatalf("writeFileAtomic: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new" {
		t.Fatalf("content = %q", data)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("temporary file left behind: %v", entries)
	}
}
