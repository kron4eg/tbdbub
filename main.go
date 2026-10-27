package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/kron4eg/tbdbub/internal/board"
	"github.com/kron4eg/tbdbub/internal/tui"
)

const usage = `tbdbub — a keyboard-first kanban board backed by Markdown

Usage:
  tbdbub [board.md]        open or create a board
  tbdbub add <title...>    add a task to the capture column and exit
  tbdbub ls [--json]       list tasks and exit

The board path falls back to $TBDBUB_BOARD, then ./board.md.
`

func main() {
	args := os.Args[1:]
	if len(args) > 0 {
		switch args[0] {
		case "add":
			runAdd(args[1:])
			return
		case "ls":
			runList(args[1:])
			return
		case "-h", "--help", "help":
			fmt.Print(usage)
			return
		}
	}

	path := resolvePath(args)
	b, hash, warnings, err := loadOrCreate(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "tbdbub:", err)
		os.Exit(1)
	}
	for _, warning := range warnings {
		fmt.Fprintln(os.Stderr, "tbdbub: warning:", warning)
	}

	program := tea.NewProgram(tui.NewFile(b, path, hash, warnings))
	if _, err := program.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "tbdbub:", err)
		os.Exit(1)
	}
}

func resolvePath(args []string) string {
	for _, arg := range args {
		if arg != "" && !strings.HasPrefix(arg, "-") {
			return arg
		}
	}
	if env := os.Getenv("TBDBUB_BOARD"); env != "" {
		return env
	}
	return "board.md"
}

// loadOrCreate opens a board, creating a one-column Markdown file if needed.
func loadOrCreate(path string) (board.Board, string, []board.Warning, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		b := board.NewBoard(titleFromPath(path))
		var buf bytes.Buffer
		if err := board.Write(&buf, b); err != nil {
			return board.Board{}, "", nil, err
		}
		if err := writeFileAtomic(path, buf.Bytes()); err != nil {
			return board.Board{}, "", nil, err
		}
		return b, hashBytes(buf.Bytes()), nil, nil
	}
	if err != nil {
		return board.Board{}, "", nil, err
	}
	b, warnings, err := board.Parse(bytes.NewReader(data))
	if err != nil {
		return board.Board{}, "", warnings, err
	}
	return b, hashBytes(data), warnings, nil
}

func loadExisting(path string) (board.Board, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return board.Board{}, err
	}
	b, _, err := board.Parse(bytes.NewReader(data))
	return b, err
}

func runAdd(args []string) {
	path := defaultPath()
	var title []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-b", "--board":
			if i+1 < len(args) {
				path = args[i+1]
				i++
			}
		default:
			title = append(title, args[i])
		}
	}
	if strings.TrimSpace(strings.Join(title, " ")) == "" {
		fmt.Fprintln(os.Stderr, "tbdbub add: a task title is required")
		os.Exit(1)
	}

	b, _, _, err := loadOrCreate(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "tbdbub:", err)
		os.Exit(1)
	}
	next, err := b.AddTask(b.CaptureIndex(), strings.Join(title, " "))
	if err != nil {
		fmt.Fprintln(os.Stderr, "tbdbub add:", err)
		os.Exit(1)
	}
	var buf bytes.Buffer
	if err := board.Write(&buf, next); err != nil {
		fmt.Fprintln(os.Stderr, "tbdbub:", err)
		os.Exit(1)
	}
	if err := writeFileAtomic(path, buf.Bytes()); err != nil {
		fmt.Fprintln(os.Stderr, "tbdbub:", err)
		os.Exit(1)
	}
	task := next.Columns[next.CaptureIndex()].Tasks
	fmt.Printf("added %s\n", task[len(task)-1].ID)
}

func runList(args []string) {
	jsonOut := false
	path := defaultPath()
	for _, arg := range args {
		switch arg {
		case "--json":
			jsonOut = true
		case "-b", "--board":
			// handled below for simplicity
		default:
			if !strings.HasPrefix(arg, "-") {
				path = arg
			}
		}
	}
	for i := 0; i < len(args); i++ {
		if (args[i] == "-b" || args[i] == "--board") && i+1 < len(args) {
			path = args[i+1]
			i++
		}
	}

	b, err := loadExisting(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "tbdbub ls:", err)
		os.Exit(1)
	}

	type row struct {
		Column    string   `json:"column"`
		ID        string   `json:"id"`
		Title     string   `json:"title"`
		Tags      []string `json:"tags,omitempty"`
		Milestone string   `json:"milestone,omitempty"`
		Done      bool     `json:"done"`
	}
	var rows []row
	for _, col := range b.Columns {
		for _, task := range col.Tasks {
			rows = append(rows, row{
				Column:    col.Name,
				ID:        string(task.ID),
				Title:     task.Title,
				Tags:      task.Tags,
				Milestone: task.Milestone,
				Done:      task.Done,
			})
		}
	}
	if jsonOut {
		out, err := json.MarshalIndent(rows, "", "  ")
		if err != nil {
			fmt.Fprintln(os.Stderr, "tbdbub ls:", err)
			os.Exit(1)
		}
		fmt.Println(string(out))
		return
	}
	for _, task := range rows {
		check := " "
		if task.Done {
			check = "x"
		}
		fmt.Printf("[%s] %-12s %-6s %s\n", check, task.Column, task.ID, task.Title)
	}
}

func defaultPath() string {
	if env := os.Getenv("TBDBUB_BOARD"); env != "" {
		return env
	}
	return "board.md"
}

func titleFromPath(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	if name == "" {
		return "tbdbub"
	}
	return name
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	tmp, err := os.CreateTemp(dir, ".tbdbub-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
