package tui

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kron4eg/tbdbub/internal/board"
)

type saveTickMsg struct {
	version uint64
}

type savedMsg struct {
	version uint64
	hash    string
	force   bool
	err     error
}

// loadFile reads and parses a board file and returns its content hash.
func loadFile(path string) (board.Board, string, []board.Warning, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return board.Board{}, "", nil, err
	}
	b, warnings, err := board.Parse(bytes.NewReader(data))
	if err != nil {
		return board.Board{}, "", warnings, err
	}
	return b, hashBytes(data), warnings, nil
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (m Model) scheduleSave() tea.Cmd {
	if m.path == "" {
		return nil
	}
	version := m.version
	return tea.Tick(autosaveDelay, func(time.Time) tea.Msg {
		return saveTickMsg{version: version}
	})
}

func (m Model) handleSaveTick(msg saveTickMsg) (tea.Model, tea.Cmd) {
	if !m.dirty || msg.version != m.version || m.path == "" {
		return m, nil
	}
	return m.saveNow(false, false)
}

// saveNow writes the board atomically. Unless force is set, it first checks that
// the file still matches the hash from the last load/save.
func (m Model) saveNow(force, explicit bool) (tea.Model, tea.Cmd) {
	if m.path == "" {
		m.dirty = false
		m.savedVersion = m.version
		if explicit {
			m.status = "No file to save (demo board)"
		}
		return m, nil
	}
	if !force && m.fileHash != "" {
		data, err := os.ReadFile(m.path)
		switch {
		case err == nil && hashBytes(data) != m.fileHash:
			m.confirm = Confirm{
				kind:    confirmConflict,
				title:   "File changed on disk",
				message: "Reload the file, overwrite it, or cancel? (r reload · o overwrite · esc cancel)",
			}
			m.mode = ModeConfirm
			return m, nil
		case err != nil && !errors.Is(err, os.ErrNotExist):
			m.status = "Save failed: " + err.Error()
			return m, nil
		}
	}
	snapshot := m.Board
	version := m.version
	path := m.path
	if explicit {
		m.status = "Saving…"
	}
	return m, saveCmd(path, snapshot, version, explicit)
}

func saveCmd(path string, b board.Board, version uint64, force bool) tea.Cmd {
	return func() tea.Msg {
		var buf bytes.Buffer
		if err := board.Write(&buf, b); err != nil {
			return savedMsg{version: version, force: force, err: err}
		}
		if err := writeFileAtomic(path, buf.Bytes()); err != nil {
			return savedMsg{version: version, force: force, err: err}
		}
		return savedMsg{version: version, hash: hashBytes(buf.Bytes()), force: force}
	}
}

func (m Model) handleSaved(msg savedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.dirty = true
		m.status = "Save failed: " + msg.err.Error()
		m.quitAfterSave = false
		return m, nil
	}
	m.fileHash = msg.hash
	if msg.version == m.version {
		m.savedVersion = msg.version
		m.dirty = false
		if msg.force {
			m.status = "Saved"
		}
		if m.quitAfterSave {
			m.quitAfterSave = false
			return m, tea.Quit
		}
		return m, nil
	}
	// The board changed while the write was in flight; save again.
	m.dirty = true
	return m, m.scheduleSave()
}

func (m *Model) reloadFile() {
	if m.path == "" {
		return
	}
	b, hash, warnings, err := loadFile(m.path)
	if err != nil {
		m.status = "Reload failed: " + err.Error()
		return
	}
	m.Board = b
	m.fileHash = hash
	m.dirty = false
	m.version = 0
	m.savedVersion = 0
	m.undo = nil
	m.redo = nil
	m.warn = warnings
	m.filter = board.NewFilter()
	m.columnTask = map[string]int{}
	m.clampFocus()
	if len(warnings) > 0 {
		m.status = "Reloaded with warnings: " + warnings[0].String()
	} else {
		m.status = "Reloaded"
	}
}

// writeFileAtomic writes a temp file next to path, syncs it, and renames it.
// The original file mode is preserved.
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
	tmpName := tmp.Name()
	cleanup := func() {
		tmp.Close()
		os.Remove(tmpName)
	}
	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}
