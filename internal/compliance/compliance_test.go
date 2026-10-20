package compliance

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/envredact"
	"github.com/GLINCKER/levelrail/internal/store"
)

func testSigner() *Signer { return NewSigner([]byte("0123456789abcdef0123456789abcdef")) }

func buildTestPack(t *testing.T, signer *Signer, body string) []byte {
	t.Helper()
	var buf bytes.Buffer
	secs := []Section{{Name: "a.json", Write: func(_ context.Context, w io.Writer) error {
		_, err := io.WriteString(w, body)
		return err
	}}}
	meta := Meta{PackID: "evp_t", RangeFrom: time.Unix(0, 0), RangeTo: time.Unix(100, 0), Now: time.Unix(200, 0)}
	if err := Build(context.Background(), &buf, meta, secs, signer, nil); err != nil {
		t.Fatalf("Build: %v", err)
	}
	return buf.Bytes()
}

func rewriteEntry(t *testing.T, archive []byte, name string, mutate func([]byte) []byte) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, f := range zr.File {
		rc, _ := f.Open()
		data, _ := io.ReadAll(rc)
		_ = rc.Close()
		if f.Name == name {
			data = mutate(data)
		}
		w, _ := zw.Create(f.Name)
		_, _ = w.Write(data)
	}
	_ = zw.Close()
	return out.Bytes()
}

func TestVerify_DetectsTampering(t *testing.T) {
	signer := testSigner()
	good := buildTestPack(t, signer, `{"ok":true}`)
	cases := []struct {
		name  string
		pack  []byte
		valid bool
	}{
		{"untouched", good, true},
		{"file content changed", rewriteEntry(t, good, "a.json", func([]byte) []byte { return []byte(`{"ok":false}`) }), false},
		{"manifest edited", rewriteEntry(t, good, manifestName, func(b []byte) []byte {
			return bytes.Replace(b, []byte(`"version": 1`), []byte(`"version": 2`), 1)
		}), false},
		{"signature replaced", rewriteEntry(t, good, signatureName, func([]byte) []byte { return []byte(strings.Repeat("00", 32)) }), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rep, err := Verify(bytes.NewReader(tc.pack), int64(len(tc.pack)), signer)
			if err != nil {
				t.Fatalf("Verify: %v", err)
			}
			if rep.Valid != tc.valid {
				t.Fatalf("Valid = %v, want %v (report %+v)", rep.Valid, tc.valid, rep)
			}
		})
	}
}

func TestVerify_WrongKeyRejected(t *testing.T) {
	pack := buildTestPack(t, testSigner(), "{}")
	other := NewSigner([]byte("ffffffffffffffffffffffffffffffff"))
	rep, err := Verify(bytes.NewReader(pack), int64(len(pack)), other)
	if err != nil || rep.Valid || rep.SignatureValid {
		t.Fatalf("pack verified under a different key: %+v, %v", rep, err)
	}
}

func TestBuild_RequiresSigner(t *testing.T) {
	err := Build(context.Background(), io.Discard, Meta{}, nil, nil, nil)
	if err == nil {
		t.Fatal("Build without a signer must fail")
	}
}

func TestRedact_MasksSecretsAndTokens(t *testing.T) {
	in := map[string]any{
		"db_password": "hunter2hunter2",
		"name":        "primary",
		"nested":      []any{map[string]any{"api_key": "abc123abc123"}},
		"note":        "bearer eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.c2lnbmF0dXJl",
	}
	out, err := Redact(in)
	if err != nil {
		t.Fatal(err)
	}
	m := out.(map[string]any)
	if m["db_password"] != envredact.Placeholder {
		t.Errorf("db_password = %v, want placeholder", m["db_password"])
	}
	if m["name"] != "primary" {
		t.Errorf("name = %v", m["name"])
	}
	nested := m["nested"].([]any)[0].(map[string]any)
	if nested["api_key"] != envredact.Placeholder {
		t.Errorf("api_key = %v, want placeholder", nested["api_key"])
	}
	if strings.Contains(m["note"].(string), "c2lnbmF0dXJl") {
		t.Errorf("token-shaped text survived: %v", m["note"])
	}
}

func openDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "levelrail.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func newTestService(t *testing.T, db *store.DB, users UserRemover) *Service {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewService(context.Background(), db, t.TempDir(), testSigner(),
		func(context.Context) (any, error) { return map[string]string{"retention": "90d"}, nil }, users, logger)
}

func seedToken(t *testing.T, db *store.DB, id string) {
	t.Helper()
	err := db.SaveAPIToken(context.Background(), store.APIToken{ID: id, Name: "ci " + id, TokenHash: "hash-" + id,
		Abilities: []string{"read"}, CreatedAt: time.Now().UTC()})
	if err != nil {
		t.Fatalf("SaveAPIToken: %v", err)
	}
}

type fakeRemover struct{ removed []string }

func (f *fakeRemover) RemoveUser(_ context.Context, id, _ string) error {
	f.removed = append(f.removed, id)
	return nil
}

func TestCampaign_RevokeAppliesAndIsAudited(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	seedToken(t, db, "tok_keep")
	seedToken(t, db, "tok_drop")
	svc := newTestService(t, db, &fakeRemover{})
	actor := Actor{Type: "session", ID: "u1", Name: "root"}

	c, err := svc.CreateCampaign(ctx, "Q4", 7, 0, actor)
	if err != nil {
		t.Fatalf("CreateCampaign: %v", err)
	}
	items, _ := db.ListAccessReviewItems(ctx, c.ID)
	var keepID, dropID string
	for _, it := range items {
		switch it.SubjectID {
		case "tok_keep":
			keepID = it.ID
		case "tok_drop":
			dropID = it.ID
		}
	}
	if keepID == "" || dropID == "" {
		t.Fatalf("tokens missing from snapshot: %+v", items)
	}
	if _, err := svc.Complete(ctx, c.ID, actor); !errors.Is(err, ErrUndecided) {
		t.Fatalf("Complete with undecided rows = %v, want ErrUndecided", err)
	}
	for _, it := range items {
		decision := store.AccessDecisionKeep
		if it.ID == dropID {
			decision = store.AccessDecisionRevoke
		}
		if err := svc.Decide(ctx, c.ID, it.ID, decision, actor); err != nil {
			t.Fatalf("Decide: %v", err)
		}
	}
	res, err := svc.Complete(ctx, c.ID, actor)
	if err != nil || res.Revoked != 1 {
		t.Fatalf("Complete = %+v, %v", res, err)
	}
	drop, _ := db.GetAPITokenByID(ctx, "tok_drop")
	keep, _ := db.GetAPITokenByID(ctx, "tok_keep")
	if drop.RevokedAt == nil || keep.RevokedAt != nil {
		t.Fatalf("revoked state wrong: drop=%v keep=%v", drop.RevokedAt, keep.RevokedAt)
	}
	got, _ := db.GetAccessReviewCampaign(ctx, c.ID)
	if got.Status != store.AccessReviewCompleted {
		t.Fatalf("status = %q", got.Status)
	}
	entries, err := db.ListAuditEntries(ctx, 50, nil, store.AuditEntryFilter{Action: ActionAccessRevoked})
	if err != nil || len(entries) != 1 {
		t.Fatalf("revoke audit rows = %d, %v", len(entries), err)
	}
}

type failingRemover struct{}

func (failingRemover) RemoveUser(context.Context, string, string) error {
	return errors.New("last root user")
}

func TestCampaign_FailedRevocationKeepsCampaignOpen(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	if err := db.CreateUser(ctx, store.User{ID: "usr_a", Email: "a@example.com", Abilities: []string{"read"}, CreatedAt: time.Now()}); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	svc := newTestService(t, db, failingRemover{})
	actor := Actor{Type: "session", ID: "u1", Name: "root"}
	c, err := svc.CreateCampaign(ctx, "", 0, 0, actor)
	if err != nil {
		t.Fatal(err)
	}
	items, _ := db.ListAccessReviewItems(ctx, c.ID)
	for _, it := range items {
		d := store.AccessDecisionKeep
		if it.Kind == store.AccessSubjectUser {
			d = store.AccessDecisionRevoke
		}
		if err := svc.Decide(ctx, c.ID, it.ID, d, actor); err != nil {
			t.Fatal(err)
		}
	}
	res, err := svc.Complete(ctx, c.ID, actor)
	if !errors.Is(err, ErrApplyFailed) || len(res.Failures) != 1 {
		t.Fatalf("Complete = %+v, %v", res, err)
	}
	got, _ := db.GetAccessReviewCampaign(ctx, c.ID)
	if got.Status != store.AccessReviewOpen {
		t.Fatalf("campaign status = %q, want open", got.Status)
	}
	if entries, _ := db.ListAuditEntries(ctx, 10, nil, store.AuditEntryFilter{Action: ActionReviewCompleted}); len(entries) != 0 {
		t.Fatal("a half-applied campaign must not be audited as completed")
	}
}

func TestEvidencePack_FailedGenerationLeavesNoCompletePack(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	svc := newTestService(t, db, nil)
	svc.sections = func(time.Time, time.Time) []Section {
		return []Section{
			{Name: "ok.json", Write: func(_ context.Context, w io.Writer) error { _, err := io.WriteString(w, "{}"); return err }},
			{Name: "bad.json", Write: func(context.Context, io.Writer) error { return errors.New("disk full") }},
		}
	}
	p, err := svc.StartPack(ctx, time.Now().Add(-24*time.Hour), time.Now(), Actor{Name: "root"})
	if err != nil {
		t.Fatal(err)
	}
	svc.Wait()
	got, _ := db.GetEvidencePack(ctx, p.ID)
	if got.Status != store.EvidencePackFailed || got.Error == "" {
		t.Fatalf("pack = %+v, want failed with an error", got)
	}
	entries, _ := os.ReadDir(svc.dir)
	if len(entries) != 0 {
		t.Fatalf("partial files left behind: %v", entries)
	}
	if _, err := svc.VerifyPack(ctx, p.ID); !errors.Is(err, ErrPackNotReady) {
		t.Fatalf("VerifyPack on a failed pack = %v", err)
	}
}

func TestEvidencePack_GeneratesVerifiableArchive(t *testing.T) {
	ctx := context.Background()
	db := openDB(t)
	svc := newTestService(t, db, nil)
	p, err := svc.StartPack(ctx, time.Now().Add(-24*time.Hour), time.Now().Add(time.Hour), Actor{Name: "root"})
	if err != nil {
		t.Fatal(err)
	}
	svc.Wait()
	got, _ := db.GetEvidencePack(ctx, p.ID)
	if got.Status != store.EvidencePackComplete || got.SHA256 == "" {
		t.Fatalf("pack = %+v", got)
	}
	rep, err := svc.VerifyPack(ctx, p.ID)
	if err != nil || !rep.Valid || !rep.SignatureValid {
		t.Fatalf("VerifyPack = %+v, %v", rep, err)
	}
	names := map[string]bool{}
	for _, f := range rep.Files {
		names[f.Name] = f.OK
	}
	for _, want := range []string{"audit-log.ndjson", "access-reviews.json", "backups.json", "deploy-approvals.json", "security-settings.json", "tls-certificates.json"} {
		if !names[want] {
			t.Errorf("section %s missing or failed: %+v", want, rep.Files)
		}
	}
}

func TestStartPack_RejectsBadRange(t *testing.T) {
	svc := newTestService(t, openDB(t), nil)
	now := time.Now()
	for _, r := range [][2]time.Time{{now, now}, {now, now.Add(-time.Hour)}, {now.AddDate(-3, 0, 0), now}} {
		if _, err := svc.StartPack(context.Background(), r[0], r[1], Actor{}); !errors.Is(err, ErrInvalidRange) {
			t.Errorf("range %v: err = %v, want ErrInvalidRange", r, err)
		}
	}
}
