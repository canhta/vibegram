package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var ErrNotFound = errors.New("state record not found")

type Store struct {
	root string
}

func NewStore(root string) *Store {
	return &Store{root: filepath.Clean(root)}
}

func (s *Store) Init() error {
	for _, dir := range []string{
		filepath.Join(s.root, "sessions"),
		filepath.Join(s.root, "runs"),
		filepath.Join(s.root, "snapshots"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create state dir: %w", err)
		}
	}

	return nil
}

func (s *Store) SaveSession(session Session) error {
	if session.ID == "" {
		return fmt.Errorf("session_id is required")
	}

	return s.writeJSONAtomic(s.sessionPath(session.ID), session)
}

func (s *Store) LoadSession(id SessionID) (Session, error) {
	var session Session
	if id == "" {
		return session, fmt.Errorf("session_id is required")
	}

	if err := s.readJSON(s.sessionPath(id), &session); err != nil {
		return Session{}, err
	}

	return session, nil
}

func (s *Store) SaveRun(run Run) error {
	if run.ID == "" {
		return fmt.Errorf("run_id is required")
	}

	return s.writeJSONAtomic(s.runPath(run.ID), run)
}

func (s *Store) LoadRun(id RunID) (Run, error) {
	var run Run
	if id == "" {
		return run, fmt.Errorf("run_id is required")
	}

	if err := s.readJSON(s.runPath(id), &run); err != nil {
		return Run{}, err
	}

	return run, nil
}

func (s *Store) SaveSnapshot(sessionID string, snap Snapshot) error {
	if sessionID == "" {
		return fmt.Errorf("session_id is required")
	}
	return s.writeJSONAtomic(s.snapshotPath(sessionID), snap)
}

func (s *Store) LoadSnapshot(sessionID string) (Snapshot, error) {
	var snap Snapshot
	if sessionID == "" {
		return snap, fmt.Errorf("session_id is required")
	}
	if err := s.readJSON(s.snapshotPath(sessionID), &snap); err != nil {
		return Snapshot{}, err
	}
	return snap, nil
}

func (s *Store) snapshotPath(sessionID string) string {
	return filepath.Join(s.root, "snapshots", sessionID+".json")
}

func (s *Store) sessionPath(id SessionID) string {
	return filepath.Join(s.root, "sessions", string(id)+".json")
}

func (s *Store) runPath(id RunID) string {
	return filepath.Join(s.root, "runs", string(id)+".json")
}

func (s *Store) writeJSONAtomic(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}

	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal json: %w", err)
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(filepath.Dir(path), "*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}

	return nil
}

func (s *Store) readJSON(path string, dest any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: %s", ErrNotFound, path)
		}
		return fmt.Errorf("read file: %w", err)
	}

	if err := json.Unmarshal(data, dest); err != nil {
		return fmt.Errorf("unmarshal json: %w", err)
	}

	return nil
}
