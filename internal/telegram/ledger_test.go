package telegram

import (
	"testing"
)

func TestLedgerNewKeyNotSent(t *testing.T) {
	dir := t.TempDir()
	l := NewLedger(dir)
	if l.AlreadySent("key-1") {
		t.Error("expected new key to not be marked as sent")
	}
}

func TestLedgerMarkSentThenAlreadySent(t *testing.T) {
	dir := t.TempDir()
	l := NewLedger(dir)
	if err := l.MarkSent("key-2"); err != nil {
		t.Fatalf("MarkSent: %v", err)
	}
	if !l.AlreadySent("key-2") {
		t.Error("expected key to be marked as sent after MarkSent")
	}
}

func TestLedgerPersistsAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	l1 := NewLedger(dir)
	if err := l1.MarkSent("key-3"); err != nil {
		t.Fatalf("MarkSent: %v", err)
	}

	l2 := NewLedger(dir)
	if !l2.AlreadySent("key-3") {
		t.Error("expected key to persist across ledger restart")
	}
}

func TestLedgerMarkSentIdempotent(t *testing.T) {
	dir := t.TempDir()
	l := NewLedger(dir)
	if err := l.MarkSent("key-4"); err != nil {
		t.Fatalf("first MarkSent: %v", err)
	}
	if err := l.MarkSent("key-4"); err != nil {
		t.Fatalf("second MarkSent: %v", err)
	}
	if !l.AlreadySent("key-4") {
		t.Error("expected key to still be marked sent")
	}
}
