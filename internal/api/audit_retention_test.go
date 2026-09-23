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
	tests := []struct {
		name              string
		retention         time.Duration
		expectedRetention time.Duration
		returnedDeleted   int64
	}{
		{
			name:              "CustomRetention",
			retention:         24 * time.Hour,
			expectedRetention: 24 * time.Hour,
			returnedDeleted:   42,
		},
		{
			name:              "DefaultRetention",
			retention:         0,
			expectedRetention: defaultAuditLogRetention,
			returnedDeleted:   7,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockAuditStore{returnedDeleted: tt.returnedDeleted}
			rt := &Router{
				auditLog:          mock,
				auditLogRetention: tt.retention,
			}

			before := time.Now().UTC()
			n, err := rt.PurgeOldAuditEntries(context.Background())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if n != tt.returnedDeleted {
				t.Errorf("deleted = %d, want %d", n, tt.returnedDeleted)
			}
			after := time.Now().UTC()

			expectedMin := before.Add(-tt.expectedRetention)
			expectedMax := after.Add(-tt.expectedRetention)

			if mock.deletedOlderThan.Before(expectedMin) || mock.deletedOlderThan.After(expectedMax) {
				t.Errorf("cutoff = %v, want between %v and %v", mock.deletedOlderThan, expectedMin, expectedMax)
			}
		})
	}
}

func TestRunAuditLogSweeper(t *testing.T) {
	tests := []struct {
		name          string
		returnedError error
	}{
		{"Success", nil},
		{"Error", errors.New("mock error")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockAuditStore{
				returnedDeleted: 1,
				returnedError:   tt.returnedError,
			}
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
		})
	}
}
