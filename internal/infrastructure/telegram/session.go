package telegram

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// fileSession is a thread-safe file-backed MTProto session store.
// It satisfies the gotd/td session.Storage interface through structural typing —
// no import of github.com/gotd/td/session required.
//
// Files are written with mode 0600 (owner-only) since they contain credentials.
type fileSession struct {
	path string
	mu   sync.Mutex
}

func newFileSession(path string) (*fileSession, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create session dir %q: %w", dir, err)
	}
	return &fileSession{path: path}, nil
}

// LoadSession reads the saved MTProto session bytes.
// Returns nil data (no error) when no session exists yet.
func (s *fileSession) LoadSession(_ context.Context) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load session: %w", err)
	}
	return data, nil
}

// StoreSession atomically writes session bytes to disk.
// Uses a temp file + rename to avoid partial writes on crash.
func (s *fileSession) StoreSession(_ context.Context, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := filepath.Dir(s.path)
	tmp, err := os.CreateTemp(dir, "session.*.tmp")
	if err != nil {
		return fmt.Errorf("create temp session file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write session data: %w", err)
	}
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("chmod session file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close temp session file: %w", err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename session file: %w", err)
	}
	return nil
}
