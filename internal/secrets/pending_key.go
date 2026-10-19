package secrets

import (
	"context"
	"errors"
	"fmt"
	"os"
)

const pendingKeySuffix = ".new"

var errProbeDone = errors.New("secrets: probe complete")

// PendingMasterKeyPath is where a rotated master key is staged before the DB rotation commits.
func PendingMasterKeyPath(path string) string { return path + pendingKeySuffix }

// StagePendingMasterKeyFile durably writes serialized next to path as path.new (0600, fsynced).
func StagePendingMasterKeyFile(path, serialized string) error {
	pending := PendingMasterKeyPath(path)
	f, err := os.OpenFile(pending, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600) //nolint:gosec // operator-controlled key path
	if err != nil {
		return fmt.Errorf("secrets: stage master key file: %w", err)
	}
	if _, err := f.WriteString(serialized); err != nil {
		_ = f.Close()
		return fmt.Errorf("secrets: stage master key file: write: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("secrets: stage master key file: sync: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("secrets: stage master key file: close: %w", err)
	}
	return nil
}

// CommitPendingMasterKeyFile atomically renames path.new over path.
func CommitPendingMasterKeyFile(path string) error {
	if err := os.Rename(PendingMasterKeyPath(path), path); err != nil {
		return fmt.Errorf("secrets: commit master key file: %w", err)
	}
	return nil
}

// DiscardPendingMasterKeyFile removes a staged key; a missing file is not an error.
func DiscardPendingMasterKeyFile(path string) {
	_ = os.Remove(PendingMasterKeyPath(path))
}

// dekUnwrapsUnder reports whether the first stored DEK unwraps under mk (all DEKs share one master key,
// since rotation is a single transaction). ok is false when there are no DEKs to test. The probe always
// returns an error from the callback so the transaction rolls back and nothing is written.
func (m *Manager) dekUnwrapsUnder(ctx context.Context, mk *MasterKey) (unwraps, ok bool, err error) {
	err = m.store.RotateServiceDEKs(ctx, func(_ string, wrapped []byte) ([]byte, error) {
		ok = true
		_, uerr := mk.UnwrapDEK(WrappedDEK(wrapped))
		unwraps = uerr == nil
		return nil, errProbeDone
	})
	if errors.Is(err, errProbeDone) {
		return unwraps, ok, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("secrets: probe stored DEKs: %w", err)
	}
	return false, false, nil
}

// RecoverPendingMasterKey settles a rotation interrupted between the DB commit and the key file rename.
// open turns the file's raw content into a serialized key. It returns true when path.new was promoted.
func (m *Manager) RecoverPendingMasterKey(ctx context.Context, path string, open func(string) (string, error)) (bool, error) {
	raw, err := os.ReadFile(PendingMasterKeyPath(path)) //nolint:gosec // operator-controlled key path
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("secrets: read pending master key: %w", err)
	}

	unwraps, ok, err := m.dekUnwrapsUnder(ctx, m.masterKey())
	if err != nil {
		return false, err
	}
	if unwraps || !ok {
		DiscardPendingMasterKeyFile(path)
		return false, nil
	}

	plain, err := open(string(raw))
	if err != nil {
		return false, fmt.Errorf("secrets: open pending master key: %w", err)
	}
	next, err := LoadMasterKey(plain)
	if err != nil {
		return false, fmt.Errorf("secrets: load pending master key: %w", err)
	}
	unwraps, _, err = m.dekUnwrapsUnder(ctx, next)
	if err != nil {
		return false, err
	}
	if !unwraps {
		return false, errors.New("secrets: stored DEKs unwrap under neither the master key file nor its pending replacement")
	}
	if err := CommitPendingMasterKeyFile(path); err != nil {
		return false, err
	}
	m.mu.Lock()
	m.mk = next
	m.mu.Unlock()
	return true, nil
}
