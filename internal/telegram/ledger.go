package telegram

import (
	"crypto/sha1"
	"fmt"
	"os"
	"path/filepath"
)

type Ledger struct {
	dir string
}

func NewLedger(dir string) *Ledger {
	return &Ledger{dir: dir}
}

func (l *Ledger) AlreadySent(key string) bool {
	_, err := os.Stat(l.keyPath(key))
	return err == nil
}

func (l *Ledger) MarkSent(key string) error {
	if err := os.MkdirAll(l.dir, 0o755); err != nil {
		return fmt.Errorf("create ledger dir: %w", err)
	}

	path := l.keyPath(key)
	tmp, err := os.CreateTemp(l.dir, "*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}

	return nil
}

func (l *Ledger) keyPath(key string) string {
	h := sha1.Sum([]byte(key))
	name := fmt.Sprintf("%x", h[:8]) + ".sent"
	return filepath.Join(l.dir, name)
}
