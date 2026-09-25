package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func saveCertNode(t *testing.T, db *DB, notAfter time.Time) {
	t.Helper()
	const id, fp = "n1", "fp1"
	n := testNode(id, "name-"+id)
	n.CertFingerprint = fp
	n.CertNotAfter = &notAfter
	n.CertKeyOrigin = CertKeyOriginAgent
	if err := db.SaveNode(context.Background(), n); err != nil {
		t.Fatalf("SaveNode() error = %v", err)
	}
}

func TestNodeAcceptsCert(t *testing.T) {
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	later := now.Add(time.Hour)
	earlier := now.Add(-time.Hour)
	tests := []struct {
		name string
		node Node
		fp   string
		want bool
	}{
		{"current", Node{CertFingerprint: "a"}, "a", true},
		{"empty presented", Node{CertFingerprint: ""}, "", false},
		{"unknown", Node{CertFingerprint: "a"}, "b", false},
		{"previous inside window", Node{CertFingerprint: "a", PrevCertFingerprint: "b", PrevCertValidUntil: &later}, "b", true},
		{"previous after window", Node{CertFingerprint: "a", PrevCertFingerprint: "b", PrevCertValidUntil: &earlier}, "b", false},
		{"previous with no window", Node{CertFingerprint: "a", PrevCertFingerprint: "b"}, "b", false},
		{"revoked current", Node{CertFingerprint: "a", CertRevokedAt: &earlier}, "a", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.node.AcceptsCert(tt.fp, now); got != tt.want {
				t.Errorf("AcceptsCert(%q) = %v, want %v", tt.fp, got, tt.want)
			}
		})
	}
}

func TestRotateNodeCert(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	grace := 24 * time.Hour

	t.Run("current cert rotates and becomes previous", func(t *testing.T) {
		db := openTestDB(t)
		saveCertNode(t, db, now.Add(30*24*time.Hour))
		if err := db.RotateNodeCert(ctx, "n1", "fp1", NodeCert{Fingerprint: "fp2", Serial: "s2", NotAfter: now.Add(90 * 24 * time.Hour), KeyOrigin: CertKeyOriginAgent}, grace, now); err != nil {
			t.Fatalf("RotateNodeCert() error = %v", err)
		}
		n, err := db.GetNode(ctx, "n1")
		if err != nil {
			t.Fatalf("GetNode() error = %v", err)
		}
		if n.CertFingerprint != "fp2" || n.PrevCertFingerprint != "fp1" || n.CertSerial != "s2" || n.CertGeneration != 2 {
			t.Fatalf("after rotate: %+v", n)
		}
		if n.PrevCertValidUntil == nil || !n.PrevCertValidUntil.Equal(now.Add(grace)) {
			t.Errorf("PrevCertValidUntil = %v, want %v", n.PrevCertValidUntil, now.Add(grace))
		}
		if n.CertRenewedAt == nil || !n.CertRenewedAt.Equal(now) {
			t.Errorf("CertRenewedAt = %v, want %v", n.CertRenewedAt, now)
		}
		if !n.AcceptsCert("fp1", now.Add(time.Hour)) || !n.AcceptsCert("fp2", now) {
			t.Error("both certs must be accepted inside the overlap window")
		}
	})

	t.Run("lost response retry with previous cert keeps the original window", func(t *testing.T) {
		db := openTestDB(t)
		saveCertNode(t, db, now.Add(30*24*time.Hour))
		if err := db.RotateNodeCert(ctx, "n1", "fp1", NodeCert{Fingerprint: "fp2", NotAfter: now.Add(time.Hour)}, grace, now); err != nil {
			t.Fatalf("first rotate: %v", err)
		}
		retryAt := now.Add(time.Hour)
		if err := db.RotateNodeCert(ctx, "n1", "fp1", NodeCert{Fingerprint: "fp3", NotAfter: now.Add(2 * time.Hour)}, grace, retryAt); err != nil {
			t.Fatalf("retry rotate: %v", err)
		}
		n, _ := db.GetNode(ctx, "n1")
		if n.CertFingerprint != "fp3" || n.PrevCertFingerprint != "fp1" || n.CertGeneration != 3 {
			t.Fatalf("after retry: %+v", n)
		}
		if !n.PrevCertValidUntil.Equal(now.Add(grace)) {
			t.Errorf("retry extended the window to %v, want %v", n.PrevCertValidUntil, now.Add(grace))
		}
		if n.AcceptsCert("fp2", retryAt) {
			t.Error("the orphaned intermediate cert must not be accepted")
		}
	})

	t.Run("previous cert past its window is refused", func(t *testing.T) {
		db := openTestDB(t)
		saveCertNode(t, db, now.Add(30*24*time.Hour))
		_ = db.RotateNodeCert(ctx, "n1", "fp1", NodeCert{Fingerprint: "fp2", NotAfter: now.Add(time.Hour)}, grace, now)
		err := db.RotateNodeCert(ctx, "n1", "fp1", NodeCert{Fingerprint: "fp3", NotAfter: now.Add(time.Hour)}, grace, now.Add(grace+time.Second))
		if !errors.Is(err, ErrNodeCertNotAccepted) {
			t.Fatalf("err = %v, want ErrNodeCertNotAccepted", err)
		}
	})

	t.Run("revoked node is refused", func(t *testing.T) {
		db := openTestDB(t)
		saveCertNode(t, db, now.Add(30*24*time.Hour))
		if err := db.RevokeNodeCert(ctx, "n1", now); err != nil {
			t.Fatalf("RevokeNodeCert() error = %v", err)
		}
		err := db.RotateNodeCert(ctx, "n1", "fp1", NodeCert{Fingerprint: "fp2", NotAfter: now.Add(time.Hour)}, grace, now)
		if !errors.Is(err, ErrNodeCertRevoked) {
			t.Fatalf("err = %v, want ErrNodeCertRevoked", err)
		}
	})

	t.Run("unknown node", func(t *testing.T) {
		db := openTestDB(t)
		err := db.RotateNodeCert(ctx, "missing", "fp1", NodeCert{Fingerprint: "fp2"}, grace, now)
		if !errors.Is(err, ErrNodeNotFound) {
			t.Fatalf("err = %v, want ErrNodeNotFound", err)
		}
	})
}

func TestReenrollNodeCert_ClearsRevocationAndPrevious(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	saveCertNode(t, db, now.Add(-time.Hour))
	_ = db.RotateNodeCert(ctx, "n1", "fp1", NodeCert{Fingerprint: "fp2", NotAfter: now}, time.Hour, now.Add(-2*time.Hour))
	if err := db.RevokeNodeCert(ctx, "n1", now); err != nil {
		t.Fatalf("RevokeNodeCert() error = %v", err)
	}
	if err := db.ReenrollNodeCert(ctx, "n1", NodeCert{Fingerprint: "fp9", Serial: "s9", NotAfter: now.Add(90 * 24 * time.Hour), KeyOrigin: CertKeyOriginAgent}, now); err != nil {
		t.Fatalf("ReenrollNodeCert() error = %v", err)
	}
	n, _ := db.GetNode(ctx, "n1")
	if n.CertRevokedAt != nil || n.PrevCertFingerprint != "" || n.PrevCertValidUntil != nil || n.CertFingerprint != "fp9" {
		t.Fatalf("after re-enroll: %+v", n)
	}
	if n.Name != "name-n1" {
		t.Errorf("re-enroll must keep the node's identity, got name %q", n.Name)
	}
	if err := db.ReenrollNodeCert(ctx, "missing", NodeCert{Fingerprint: "x"}, now); !errors.Is(err, ErrNodeNotFound) {
		t.Errorf("missing node err = %v, want ErrNodeNotFound", err)
	}
}

func TestSyncNodeCertDetails_OnlyForCurrentCert(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	estimate := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	saveCertNode(t, db, estimate)
	actual := time.Date(2026, 11, 30, 8, 0, 0, 0, time.UTC)

	if err := db.SyncNodeCertDetails(ctx, "n1", "other", "s", actual); err != nil {
		t.Fatalf("SyncNodeCertDetails() error = %v", err)
	}
	n, _ := db.GetNode(ctx, "n1")
	if !n.CertNotAfter.Equal(estimate) {
		t.Fatalf("a non-current fingerprint must not change the record, got %v", n.CertNotAfter)
	}
	if err := db.SyncNodeCertDetails(ctx, "n1", "fp1", "serial-1", actual); err != nil {
		t.Fatalf("SyncNodeCertDetails() error = %v", err)
	}
	n, _ = db.GetNode(ctx, "n1")
	if !n.CertNotAfter.Equal(actual) || n.CertSerial != "serial-1" {
		t.Fatalf("got not_after %v serial %q", n.CertNotAfter, n.CertSerial)
	}
}

func TestUpdateNodeAgentInfo(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	saveCertNode(t, db, time.Now().Add(time.Hour))
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	info := NodeAgentInfo{Version: "v0.9.0", Commit: "abc123", OS: "linux", Arch: "arm64"}
	if err := db.UpdateNodeAgentInfo(ctx, "n1", info, now); err != nil {
		t.Fatalf("UpdateNodeAgentInfo() error = %v", err)
	}
	n, _ := db.GetNode(ctx, "n1")
	if n.AgentVersion != "v0.9.0" || n.AgentCommit != "abc123" || n.AgentOS != "linux" || n.AgentArch != "arm64" || !n.AgentReportedAt.Equal(now) {
		t.Fatalf("got %+v", n)
	}
	if err := db.UpdateNodeAgentInfo(ctx, "missing", info, now); !errors.Is(err, ErrNodeNotFound) {
		t.Errorf("missing node err = %v, want ErrNodeNotFound", err)
	}
}

func TestNodeJoinToken_ReenrollPurposeRoundTrips(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	saveCertNode(t, db, time.Now().Add(time.Hour))
	now := time.Now()
	for _, tok := range []NodeJoinToken{
		{ID: "t1", TokenHash: "h1", CreatedAt: now, ExpiresAt: now.Add(time.Hour)},
		{ID: "t2", TokenHash: "h2", CreatedAt: now, ExpiresAt: now.Add(time.Hour), Purpose: NodeJoinTokenPurposeReenroll, NodeID: "n1"},
	} {
		if err := db.SaveNodeJoinToken(ctx, tok); err != nil {
			t.Fatalf("SaveNodeJoinToken(%s) error = %v", tok.ID, err)
		}
	}
	t1, _ := db.GetNodeJoinTokenByHash(ctx, "h1")
	if t1.Purpose != NodeJoinTokenPurposeEnroll || t1.NodeID != "" {
		t.Errorf("enroll token = %+v", t1)
	}
	t2, _ := db.GetNodeJoinTokenByHash(ctx, "h2")
	if t2.Purpose != NodeJoinTokenPurposeReenroll || t2.NodeID != "n1" {
		t.Errorf("reenroll token = %+v", t2)
	}
}

func TestMigration0135_BackfillsCertNotAfterFromCreatedAt(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	created := time.Date(2026, 6, 1, 10, 0, 0, 123456789, time.UTC)
	n := testNode("old", "old-node")
	n.CertFingerprint = "fp"
	n.CreatedAt = created
	if err := db.SaveNode(ctx, n); err != nil {
		t.Fatalf("SaveNode() error = %v", err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE nodes SET cert_not_after = strftime('%Y-%m-%dT%H:%M:%fZ', created_at, '+90 days') WHERE id = 'old'`); err != nil {
		t.Fatalf("backfill statement: %v", err)
	}
	got, err := db.GetNode(ctx, "old")
	if err != nil {
		t.Fatalf("GetNode() error = %v", err)
	}
	want := created.Add(90 * 24 * time.Hour).Truncate(time.Millisecond)
	if got.CertNotAfter == nil || !got.CertNotAfter.Equal(want) {
		t.Fatalf("CertNotAfter = %v, want %v", got.CertNotAfter, want)
	}
}
