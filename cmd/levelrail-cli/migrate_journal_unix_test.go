//go:build !windows

package main

import (
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestMigrateJournal_FIFONeverBlocks(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "run.json")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Skipf("mkfifo: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		f, err := openJournalNoFollow(fifo)
		if err == nil {
			_ = f.Close()
		}
		_, err = loadJournal(fifo, "coolify", journalMaxBytesDflt)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Error("a FIFO journal was accepted")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("opening a FIFO journal blocked")
	}
}
