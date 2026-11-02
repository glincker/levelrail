package store

import (
	"context"
	"strings"
	"testing"
	"time"
)

func anchorProblem(rep AuditAnchorReport, sub string) bool {
	for _, p := range rep.Problems {
		if strings.Contains(p.Reason, sub) {
			return true
		}
	}
	return false
}

func headAnchor(t *testing.T, db *DB) AuditAnchor {
	t.Helper()
	a, ok, err := db.LatestAuditAnchor(context.Background())
	if err != nil || !ok {
		t.Fatalf("latest anchor: ok=%v err=%v", ok, err)
	}
	return a
}

func TestAuditAnchor_IntactChainMatches(t *testing.T) {
	useChainKey(t, "anchor-key")
	db := openTestDB(t)
	seedChain(t, db, 5, nil)
	a := headAnchor(t, db)
	rep, err := db.CheckAuditAnchors(context.Background(), []AuditAnchor{a})
	if err != nil || !rep.OK || rep.Matched != 1 {
		t.Fatalf("report = %+v err=%v", rep, err)
	}
}

func TestAuditAnchor_DetectsRollback(t *testing.T) {
	useChainKey(t, "anchor-key")
	db := openTestDB(t)
	seedChain(t, db, 6, nil)
	a := headAnchor(t, db)
	if _, err := db.Exec(`DELETE FROM audit_log WHERE chain_seq > 3`); err != nil {
		t.Fatal(err)
	}
	rep, _ := db.CheckAuditAnchors(context.Background(), []AuditAnchor{a})
	if rep.OK || !anchorProblem(rep, "rolled back") {
		t.Fatalf("rollback not detected: %+v", rep)
	}
}

func TestAuditAnchor_DetectsRewriteAndResign(t *testing.T) {
	useChainKey(t, "anchor-key")
	db := openTestDB(t)
	seedChain(t, db, 4, nil)
	a := headAnchor(t, db)
	// An attacker with the key wipes the log and rebuilds a different, fully valid chain.
	if _, err := db.Exec(`DELETE FROM audit_log`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM audit_chain_checkpoints`); err != nil {
		t.Fatal(err)
	}
	seedChain(t, db, 6, func(_ int, e *AuditEntry) { e.ActorName = "forged" })
	if rep, _ := db.VerifyAuditChain(context.Background()); !rep.OK {
		t.Fatalf("rebuilt chain should verify on its own: %+v", rep)
	}
	rep, _ := db.CheckAuditAnchors(context.Background(), []AuditAnchor{a})
	if rep.OK || !anchorProblem(rep, "rewritten") {
		t.Fatalf("rewrite not detected: %+v", rep)
	}
}

func TestAuditAnchor_ForgedExportRejected(t *testing.T) {
	useChainKey(t, "anchor-key")
	db := openTestDB(t)
	seedChain(t, db, 3, nil)
	a := headAnchor(t, db)
	a.Hash = strings.Repeat("0", 64)
	rep, _ := db.CheckAuditAnchors(context.Background(), []AuditAnchor{a})
	if rep.OK || !anchorProblem(rep, "signature is invalid") {
		t.Fatalf("edited anchor accepted: %+v", rep)
	}
}

func TestAuditAnchor_PurgedRangeIsCompactedNotFailed(t *testing.T) {
	useChainKey(t, "anchor-key")
	db := openTestDB(t)
	seedChain(t, db, 5, nil)
	a := headAnchor(t, db)
	if _, err := db.DeleteAuditEntriesTiered(context.Background(), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	rep, _ := db.CheckAuditAnchors(context.Background(), []AuditAnchor{a})
	if !rep.OK || rep.Matched != 1 {
		t.Fatalf("purged head at link end should still match: %+v", rep)
	}
}

func TestAuditAnchor_EmptyLogHasNoAnchor(t *testing.T) {
	db := openTestDB(t)
	if _, ok, err := db.LatestAuditAnchor(context.Background()); ok || err != nil {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}
