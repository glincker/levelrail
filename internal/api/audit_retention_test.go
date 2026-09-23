package api

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type mockAuditStore struct {
	deletedOlderThan time.Time
	returnedDeleted  int64
	returnedError    error
	callCount        int
}

func (m *mockAuditStore) DeleteAuditEntriesOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	m.deletedOlderThan = cutoff
	m.callCount++
	return m.returnedDeleted, m.returnedError
}

func (m *mockAuditStore) SaveAuditEntry(ctx context.Context, e store.AuditEntry) error {
	return nil
}

func (m *mockAuditStore) ListAuditEntries(ctx context.Context, limit int, before *time.Time, filter store.AuditEntryFilter) ([]store.AuditEntry, error) {
	return nil, nil
}

func TestPurgeOldAuditEntries(t *testing.T) {
	mock := &mockAuditStore{returnedDeleted: 42}

	rt := &Router{
		auditLog:          mock,
		auditLogRetention: 24 * time.Hour,
	}

	before := time.Now().UTC()

	n, err := rt.PurgeOldAuditEntries(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 42 {
		t.Errorf("deleted = %d, want 42", n)
	}

	after := time.Now().UTC()

	expectedMin := before.Add(-24 * time.Hour)
	expectedMax := after.Add(-24 * time.Hour)

	if mock.deletedOlderThan.Before(expectedMin) || mock.deletedOlderThan.After(expectedMax) {
		t.Errorf("cutoff = %v, want between %v and %v", mock.deletedOlderThan, expectedMin, expectedMax)
	}
}

func TestPurgeOldAuditEntries_DefaultRetention(t *testing.T) {
	mock := &mockAuditStore{returnedDeleted: 7}

	rt := &Router{
		auditLog: mock,
	}

	before := time.Now().UTC()

	n, err := rt.PurgeOldAuditEntries(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 7 {
		t.Errorf("deleted = %d, want 7", n)
	}

	after := time.Now().UTC()

	expectedMin := before.Add(-defaultAuditLogRetention)
	expectedMax := after.Add(-defaultAuditLogRetention)

	if mock.deletedOlderThan.Before(expectedMin) || mock.deletedOlderThan.After(expectedMax) {
		t.Errorf("cutoff = %v, want between %v and %v", mock.deletedOlderThan, expectedMin, expectedMax)
	}
}

func TestRunAuditLogSweeper(t *testing.T) {
	mock := &mockAuditStore{returnedDeleted: 1}
	rt := &Router{
		auditLog: mock,
		logger:   discardLogger(),
	}

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- rt.RunAuditLogSweeper(ctx, 5*time.Millisecond)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	err := <-errCh
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}

	if mock.callCount == 0 {
		t.Errorf("expected DeleteAuditEntriesOlderThan to be called at least once")
	}
}

func TestRunAuditLogSweeper_Error(t *testing.T) {
	mock := &mockAuditStore{returnedError: errors.New("mock error")}
	rt := &Router{
		auditLog: mock,
		logger:   discardLogger(),
	}

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- rt.RunAuditLogSweeper(ctx, 5*time.Millisecond)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	err := <-errCh
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}

	if mock.callCount == 0 {
		t.Errorf("expected DeleteAuditEntriesOlderThan to be called at least once")
	}
}
