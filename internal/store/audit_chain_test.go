package store

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/secretmask"
)

func seedChain(t *testing.T, db *DB, n int, mutate func(i int, e *AuditEntry)) {
	t.Helper()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 1; i <= n; i++ {
		e := testAuditEntry(fmt.Sprintf("aud_%03d", i), FormatAuditTime(base.Add(time.Duration(i)*time.Hour)))
		if mutate != nil {
			mutate(i, &e)
		}
		if err := db.SaveAuditEntry(context.Background(), e); err != nil {
			t.Fatalf("save entry %d: %v", i, err)
		}
	}
}

func useChainKey(t *testing.T, key string) {
	t.Helper()
	SetAuditChainKey([]byte(key))
	t.Cleanup(func() { SetAuditChainKey(nil) })
}

func TestAuditChain_IntactChainVerifies(t *testing.T) {
	useChainKey(t, "test-key-one")
	db := openTestDB(t)
	seedChain(t, db, 10, nil)
	rep, err := db.VerifyAuditChain(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !rep.OK || rep.Checked != 10 || rep.Head != 10 {
		t.Fatalf("report = %+v, want ok with 10 checked", rep)
	}
}

func TestAuditChain_DetectsEditedRow(t *testing.T) {
	useChainKey(t, "test-key-one")
	db := openTestDB(t)
	seedChain(t, db, 5, nil)
	if _, err := db.Exec(`UPDATE audit_log SET status_code = 200, actor_name = 'someone else' WHERE id = 'aud_003'`); err != nil {
		t.Fatal(err)
	}
	rep, err := db.VerifyAuditChain(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rep.OK || len(rep.Problems) == 0 || !strings.Contains(rep.Problems[0].Reason, "modified") {
		t.Fatalf("edited row not detected: %+v", rep)
	}
}

func TestAuditChain_EditWithPlainHashIsStillDetectedWhenKeyed(t *testing.T) {
	useChainKey(t, "test-key-one")
	db := openTestDB(t)
	seedChain(t, db, 3, nil)
	var prev string
	if err := db.QueryRow(`SELECT chain_prev FROM audit_log WHERE id = 'aud_002'`).Scan(&prev); err != nil {
		t.Fatal(err)
	}
	e := testAuditEntry("aud_002", "2026-01-01T02:00:00.000000000Z")
	e.Path = "/forged"
	forged := auditDigest(nil, auditAlgPlain, auditChainPayload(prev, e))
	if _, err := db.Exec(`UPDATE audit_log SET path = '/forged', chain_hash = ?, chain_alg = 'h' WHERE id = 'aud_002'`, forged); err != nil {
		t.Fatal(err)
	}
	rep, err := db.VerifyAuditChain(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rep.OK {
		t.Fatalf("a recomputed plain hash must break the next link: %+v", rep)
	}
}

func TestAuditChain_DetectsDeletedMiddleRow(t *testing.T) {
	useChainKey(t, "test-key-one")
	db := openTestDB(t)
	seedChain(t, db, 6, nil)
	if _, err := db.Exec(`DELETE FROM audit_log WHERE id = 'aud_003'`); err != nil {
		t.Fatal(err)
	}
	rep, err := db.VerifyAuditChain(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rep.OK || !strings.Contains(rep.Problems[0].Reason, "missing") {
		t.Fatalf("deleted row not detected: %+v", rep)
	}
}

func TestAuditChain_DetectsTruncationPastCheckpoint(t *testing.T) {
	useChainKey(t, "test-key-one")
	db := openTestDB(t)
	seedChain(t, db, auditCheckpointEvery+3, nil)
	if _, err := db.Exec(`DELETE FROM audit_log WHERE chain_seq >= ?`, auditCheckpointEvery); err != nil {
		t.Fatal(err)
	}
	rep, err := db.VerifyAuditChain(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rep.OK || rep.Checkpoints != 1 {
		t.Fatalf("truncation past a checkpoint not detected: %+v", rep)
	}
}

func TestAuditChain_ForgedCheckpointRejected(t *testing.T) {
	useChainKey(t, "test-key-one")
	db := openTestDB(t)
	seedChain(t, db, auditCheckpointEvery, nil)
	if _, err := db.Exec(`UPDATE audit_chain_checkpoints SET sig = 'k:deadbeef'`); err != nil {
		t.Fatal(err)
	}
	rep, err := db.VerifyAuditChain(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rep.OK {
		t.Fatalf("forged checkpoint signature accepted: %+v", rep)
	}
}

func TestAuditChain_WrongKeyFailsKeyedRows(t *testing.T) {
	useChainKey(t, "test-key-one")
	db := openTestDB(t)
	seedChain(t, db, 3, nil)
	SetAuditChainKey([]byte("another-key"))
	rep, err := db.VerifyAuditChain(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if rep.OK {
		t.Fatalf("rows verified under the wrong key: %+v", rep)
	}
}

func TestAuditChain_MissingKeyIsUnverifiedNotTampered(t *testing.T) {
	useChainKey(t, "test-key-one")
	db := openTestDB(t)
	seedChain(t, db, 3, nil)
	SetAuditChainKey(nil)
	rep, err := db.VerifyAuditChain(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !rep.OK || rep.Unverified != 3 {
		t.Fatalf("keyed rows without a key should be unverified, not failed: %+v", rep)
	}
}

func TestAuditChain_PlainRowsBeforeKeyStillVerify(t *testing.T) {
	db := openTestDB(t)
	seedChain(t, db, 3, nil)
	useChainKey(t, "late-key")
	seedChainFrom(t, db, 4, 3)
	rep, err := db.VerifyAuditChain(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !rep.OK || rep.Checked != 6 {
		t.Fatalf("mixed plain and keyed chain failed: %+v", rep)
	}
}

func seedChainFrom(t *testing.T, db *DB, start, count int) {
	t.Helper()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := start; i < start+count; i++ {
		e := testAuditEntry(fmt.Sprintf("aud_%03d", i), FormatAuditTime(base.Add(time.Duration(i)*time.Hour)))
		if err := db.SaveAuditEntry(context.Background(), e); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAuditChain_TieredPurgeKeepsSecurityRowsAndChainVerifies(t *testing.T) {
	useChainKey(t, "test-key-one")
	db := openTestDB(t)
	seedChain(t, db, 12, func(i int, e *AuditEntry) {
		if i%4 == 0 {
			e.Action = "step_up.failed"
		}
	})
	cutoff := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Add(100 * time.Hour)
	securityCutoff := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	deleted, err := db.DeleteAuditEntriesTiered(context.Background(), cutoff, securityCutoff)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 9 {
		t.Fatalf("deleted %d rows, want the 9 non-security ones", deleted)
	}
	var kept int
	if err := db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action = 'step_up.failed'`).Scan(&kept); err != nil || kept != 3 {
		t.Fatalf("security rows kept = %d (%v), want 3", kept, err)
	}
	rep, err := db.VerifyAuditChain(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !rep.OK {
		t.Fatalf("chain must stay verifiable after a tiered purge: %+v", rep)
	}
	seedChainFrom(t, db, 13, 2)
	if rep, err = db.VerifyAuditChain(context.Background()); err != nil || !rep.OK {
		t.Fatalf("appending after a purge broke the chain: %+v %v", rep, err)
	}
}

func TestAuditChain_PurgeOfEverythingThenAppendStaysLinked(t *testing.T) {
	useChainKey(t, "test-key-one")
	db := openTestDB(t)
	seedChain(t, db, 5, nil)
	if _, err := db.DeleteAuditEntriesOlderThan(context.Background(), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	seedChainFrom(t, db, 6, 2)
	rep, err := db.VerifyAuditChain(context.Background())
	if err != nil || !rep.OK || rep.Head != 7 {
		t.Fatalf("report = %+v err = %v, want ok with head 7", rep, err)
	}
}

func TestAuditEntry_SecretValuesAreMaskedBeforeStorage(t *testing.T) {
	db := openTestDB(t)
	secretValue := "sk-live-abcdefgh12345678"
	secretmask.Add(secretValue)
	e := testAuditEntry("aud_secret", "2026-01-01T00:00:00.000000000Z")
	e.Path = "/api/v1/apps/x?token=" + secretValue
	e.Detail = `{"note":"` + secretValue + `"}`
	if err := db.SaveAuditEntry(context.Background(), e); err != nil {
		t.Fatal(err)
	}
	var path, detail string
	if err := db.QueryRow(`SELECT path, detail FROM audit_log WHERE id = 'aud_secret'`).Scan(&path, &detail); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(path+detail, secretValue) {
		t.Fatalf("secret stored in audit row: %q %q", path, detail)
	}
}

func TestAuditChain_RangeNamesBreakAndHead(t *testing.T) {
	useChainKey(t, "test-key-one")
	db := openTestDB(t)
	seedChain(t, db, 6, nil)
	if _, err := db.Exec(`UPDATE audit_log SET actor_name = 'x' WHERE id = 'aud_004'`); err != nil {
		t.Fatal(err)
	}
	rep, err := db.VerifyAuditChainRange(context.Background(), 1, 3)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.OK || rep.ChainOK || rep.ProblemsOutside != 1 || rep.Checked != 3 || rep.RangeEnd != 3 || rep.RangeEndHash == "" || rep.Head != 6 || rep.HeadHash == "" {
		t.Fatalf("range 1-3 = %+v, want range ok, chain not ok, head 6", rep)
	}
	rep, err = db.VerifyAuditChainRange(context.Background(), 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if rep.OK || rep.FirstBreak == nil || rep.FirstBreak.Seq != 4 {
		t.Fatalf("full run = %+v, want first break at seq 4", rep)
	}
}

func TestAuditChain_RangeAfterEditStaysNotChainOK(t *testing.T) {
	useChainKey(t, "test-key-one")
	db := openTestDB(t)
	seedChain(t, db, 10, nil)
	if _, err := db.Exec(`UPDATE audit_log SET actor_name = 'x' WHERE id = 'aud_004'`); err != nil {
		t.Fatal(err)
	}
	rep, err := db.VerifyAuditChainRange(context.Background(), 6, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.OK || rep.ChainOK || rep.ProblemsOutside == 0 {
		t.Fatalf("range 6- = %+v, want range ok but chain_ok false", rep)
	}
}

func TestAuditChain_RangeCoversDeletedRows(t *testing.T) {
	useChainKey(t, "test-key-one")
	db := openTestDB(t)
	seedChain(t, db, 10, nil)
	if _, err := db.Exec(`DELETE FROM audit_log WHERE id IN ('aud_005','aud_006','aud_007','aud_008')`); err != nil {
		t.Fatal(err)
	}
	for _, r := range [][2]int64{{5, 8}, {6, 7}, {3, 6}, {8, 9}} {
		rep, err := db.VerifyAuditChainRange(context.Background(), r[0], r[1])
		if err != nil {
			t.Fatal(err)
		}
		if rep.OK {
			t.Errorf("range %v = %+v, want not ok: rows 5-8 are gone", r, rep)
		}
	}
	rep, err := db.VerifyAuditChainRange(context.Background(), 1, 3)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.OK || rep.ChainOK {
		t.Errorf("range 1-3 = %+v, want range ok, chain not ok", rep)
	}
}

func TestAuditChain_RangePastHeadIsNotOK(t *testing.T) {
	useChainKey(t, "test-key-one")
	db := openTestDB(t)
	seedChain(t, db, 10, nil)
	for _, r := range [][2]int64{{1, 20}, {50, 0}, {50, 60}} {
		rep, err := db.VerifyAuditChainRange(context.Background(), r[0], r[1])
		if err != nil {
			t.Fatal(err)
		}
		if rep.OK {
			t.Errorf("range %v = %+v, want not ok past head 10", r, rep)
		}
	}
}

func TestAuditChain_RangeSeesTruncation(t *testing.T) {
	useChainKey(t, "test-key-one")
	db := openTestDB(t)
	seedChain(t, db, 130, nil)
	if _, err := db.Exec(`DELETE FROM audit_log WHERE chain_seq > 100`); err != nil {
		t.Fatal(err)
	}
	rep, err := db.VerifyAuditChainRange(context.Background(), 1, 120)
	if err != nil {
		t.Fatal(err)
	}
	if rep.OK {
		t.Fatalf("range 1-120 = %+v, want not ok: the chain was cut off at 100", rep)
	}
}

func TestAuditChain_PurgedRangeIsCoveredByLinks(t *testing.T) {
	useChainKey(t, "test-key-one")
	db := openTestDB(t)
	seedChain(t, db, 10, nil)
	cut := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Add(5*time.Hour + time.Minute)
	if _, err := db.DeleteAuditEntriesTiered(context.Background(), cut, cut); err != nil {
		t.Fatal(err)
	}
	rep, err := db.VerifyAuditChainRange(context.Background(), 1, 4)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.OK {
		t.Fatalf("purged range 1-4 = %+v, want ok through the compacted link", rep)
	}
}

func TestAuditChain_RangeSeesDeletedOldestRows(t *testing.T) {
	useChainKey(t, "test-key-one")
	db := openTestDB(t)
	seedChain(t, db, 10, nil)
	if _, err := db.Exec(`DELETE FROM audit_log WHERE chain_seq <= 4`); err != nil {
		t.Fatal(err)
	}
	rep, err := db.VerifyAuditChainRange(context.Background(), 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if rep.OK || rep.ChainOK {
		t.Fatalf("range 1-10 = %+v, want not ok: rows 1-4 are gone", rep)
	}
	rep, err = db.VerifyAuditChainRange(context.Background(), 5, 10)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.OK || rep.ChainOK {
		t.Fatalf("range 5-10 = %+v, want range ok, chain not ok", rep)
	}
}

func TestAuditChain_CompactedLinkAtOneIsIntact(t *testing.T) {
	useChainKey(t, "test-key-one")
	db := openTestDB(t)
	seedChain(t, db, 10, nil)
	cut := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Add(5*time.Hour + time.Minute)
	if _, err := db.DeleteAuditEntriesTiered(context.Background(), cut, cut); err != nil {
		t.Fatal(err)
	}
	rep, err := db.VerifyAuditChainRange(context.Background(), 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.OK || !rep.ChainOK {
		t.Fatalf("range 1-10 over a compacted prefix = %+v, want intact", rep)
	}
}
