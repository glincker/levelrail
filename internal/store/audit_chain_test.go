package store

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func seedChain(t *testing.T, db *DB, n int) {
	t.Helper()
	for i := 1; i <= n; i++ {
		id := fmt.Sprintf("aud_%d", i)
		ts := FormatAuditTime(time.Date(2026, 1, i, 0, 0, 0, 0, time.UTC))
		if err := db.SaveAuditEntry(context.Background(), testAuditEntry(id, ts)); err != nil {
			t.Fatalf("SaveAuditEntry(%s) error = %v", id, err)
		}
	}
}

func TestAuditChain_IntactAndUnchainedLegacyRows(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `INSERT INTO audit_log (id, actor_type, actor_id, actor_name, ability, method, path, status_code, remote_addr, created_at) VALUES ('legacy', 's', 'u', 'n', 'write', 'POST', '/x', 200, '', '2025-01-01T00:00:00.000000000Z')`); err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}
	seedChain(t, db, 3)

	rep, err := db.VerifyAuditChain(ctx)
	if err != nil {
		t.Fatalf("VerifyAuditChain error = %v", err)
	}
	if rep.FirstBroken != nil || rep.Chained != 3 || rep.Unchained != 1 || rep.HeadSeq != 3 {
		t.Fatalf("report = %+v, want 3 chained, 1 unchained, intact", rep)
	}
}

func TestAuditChain_DetectsTampering(t *testing.T) {
	cases := []struct {
		name       string
		mutate     string
		wantSeq    int64
		wantReason string
	}{
		{"edited field", `UPDATE audit_log SET path = '/evil' WHERE seq = 2`, 2, AuditBreakHashMismatch},
		{"deleted middle", `DELETE FROM audit_log WHERE seq = 2`, 3, AuditBreakSeqGap},
		{"deleted tail", `DELETE FROM audit_log WHERE seq = 3`, 2, AuditBreakTruncated},
		{"relinked", `UPDATE audit_log SET prev_hash = 'x' WHERE seq = 3`, 3, AuditBreakPrevMismatch},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := openTestDB(t)
			ctx := context.Background()
			seedChain(t, db, 3)
			if _, err := db.ExecContext(ctx, tc.mutate); err != nil {
				t.Fatalf("mutate: %v", err)
			}
			rep, err := db.VerifyAuditChain(ctx)
			if err != nil {
				t.Fatalf("VerifyAuditChain error = %v", err)
			}
			if rep.FirstBroken == nil || rep.FirstBroken.Seq != tc.wantSeq || rep.FirstBroken.Reason != tc.wantReason {
				t.Fatalf("FirstBroken = %+v, want seq %d reason %s", rep.FirstBroken, tc.wantSeq, tc.wantReason)
			}
		})
	}
}

func TestAuditChain_RetentionOfOldestRowsKeepsChainValid(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	seedChain(t, db, 4)
	if _, err := db.DeleteAuditEntriesOlderThan(ctx, time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("DeleteAuditEntriesOlderThan error = %v", err)
	}
	rep, err := db.VerifyAuditChain(ctx)
	if err != nil {
		t.Fatalf("VerifyAuditChain error = %v", err)
	}
	if rep.FirstBroken != nil || rep.Chained != 2 {
		t.Fatalf("report = %+v, want 2 chained rows, intact", rep)
	}
}
