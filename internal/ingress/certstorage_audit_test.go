package ingress

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

// openAuditTestDB opens a real, temp-file *store.DB (migrations included),
// not fakeCertStore: this test's whole point is proving Store's audit hook
// against the same SaveAuditEntry/ListAuditEntries code path a real
// deployment uses, the same "real DB over a hand-written fake" bar
// internal/reconcile/ingress/controller_live_test.go sets for cert storage
// itself, just without that test's Docker/Caddy/ACME layers, which this
// one has no need for.
func openAuditTestDB(t *testing.T) *store.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "levelrail.db")
	db, err := store.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("store.Open(%q) error = %v", path, err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("closing test db: %v", err)
		}
	})
	return db
}

// TestSQLiteStorage_Store_RecordsCertEventAuditEntries is the real,
// end-to-end proof that a certificate write SQLiteStorage.Store makes
// (whether Caddy's own background ACME renewal triggered it or
// handleRenewDomainCertificate's reconciler nudge did, Store can't tell
// the two apart and doesn't need to) lands as a system-actor audit_log
// row: first write to a ".crt" key is "cert.issued", a second write to
// the same key is "cert.renewed", and a write to a sibling ".key" key
// (every certmagic site prefix also stores one) produces no row at all.
func TestSQLiteStorage_Store_RecordsCertEventAuditEntries(t *testing.T) {
	ctx := context.Background()
	db := openAuditTestDB(t)

	storage := NewSQLiteStorage(db, nil).WithAuditRecorder(db)

	const certKey = "certificates/acme-v02.api.letsencrypt.org-directory/example.com/example.com.crt"
	const keyKey = "certificates/acme-v02.api.letsencrypt.org-directory/example.com/example.com.key"

	if err := storage.Store(ctx, certKey, []byte("leaf-v1")); err != nil {
		t.Fatalf("Store(cert, v1) error = %v", err)
	}
	if err := storage.Store(ctx, keyKey, []byte("private-key")); err != nil {
		t.Fatalf("Store(key) error = %v", err)
	}
	if err := storage.Store(ctx, certKey, []byte("leaf-v2-renewed")); err != nil {
		t.Fatalf("Store(cert, v2) error = %v", err)
	}

	entries, err := db.ListAuditEntries(ctx, 10, nil, store.AuditEntryFilter{})
	if err != nil {
		t.Fatalf("ListAuditEntries error = %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2 (one per .crt write, none for the .key write); entries = %+v", len(entries), entries)
	}

	// ListAuditEntries orders newest first: entries[0] is the renewal.
	renewed, issued := entries[0], entries[1]

	for _, e := range []store.AuditEntry{issued, renewed} {
		if e.ActorType != "system" {
			t.Errorf("entry %q: ActorType = %q, want %q", e.ID, e.ActorType, "system")
		}
		if e.ClientKind != "system" {
			t.Errorf("entry %q: ClientKind = %q, want %q", e.ID, e.ClientKind, "system")
		}
		if e.Path != "/api/v1/certificates/example.com" {
			t.Errorf("entry %q: Path = %q, want %q", e.ID, e.Path, "/api/v1/certificates/example.com")
		}
		if e.StatusCode != 200 {
			t.Errorf("entry %q: StatusCode = %d, want 200", e.ID, e.StatusCode)
		}
	}
	if issued.Ability != CertEventAbilityIssued {
		t.Errorf("first write: Ability = %q, want %q", issued.Ability, CertEventAbilityIssued)
	}
	if renewed.Ability != CertEventAbilityRenewed {
		t.Errorf("second write: Ability = %q, want %q", renewed.Ability, CertEventAbilityRenewed)
	}
}

// TestSQLiteStorage_Store_NoAuditRecorderSkipsEventsWithoutError proves
// WithAuditRecorder is genuinely optional: Store must still succeed, and
// write no audit_log row, when it was never called (the shape every
// current production wiring is in today).
func TestSQLiteStorage_Store_NoAuditRecorderSkipsEventsWithoutError(t *testing.T) {
	ctx := context.Background()
	db := openAuditTestDB(t)

	storage := NewSQLiteStorage(db, nil) // no WithAuditRecorder

	if err := storage.Store(ctx, "certificates/acme-v02.api.letsencrypt.org-directory/example.com/example.com.crt", []byte("leaf")); err != nil {
		t.Fatalf("Store error = %v", err)
	}

	entries, err := db.ListAuditEntries(ctx, 10, nil, store.AuditEntryFilter{})
	if err != nil {
		t.Fatalf("ListAuditEntries error = %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("len(entries) = %d, want 0 with no audit recorder wired", len(entries))
	}
}
