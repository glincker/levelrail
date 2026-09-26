package cpbackup

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"filippo.io/age"

	"github.com/GLINCKER/levelrail/internal/secrets"
)

func TestBuildEscrow_RoundTripMultiRecipient(t *testing.T) {
	e := newDREnv(t)
	second := mustIdentity(t)
	e.configure(func(u *ConfigUpdate) { u.Recipients = []string{e.id.Recipient().String(), second.Recipient().String()} })
	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatal(err)
	}

	b, err := e.svc.BuildEscrow(context.Background(), mk.String(), nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if b.RecipientCount != 2 || b.UploadedKey != "" || strings.Contains(b.Armored, mk.String()) || !strings.Contains(b.Armored, "BEGIN AGE ENCRYPTED FILE") {
		t.Fatalf("bundle = %+v", b)
	}
	for _, id := range []age.Identity{e.id, second} {
		p, err := OpenEscrow([]byte(b.Armored), []age.Identity{id})
		if err != nil {
			t.Fatalf("recipient could not open the bundle: %v", err)
		}
		if p.MasterKey != mk.String() || p.InstallID == "" || p.Instructions == "" {
			t.Fatalf("payload = %+v", p)
		}
		if _, err := secrets.LoadMasterKey(p.MasterKey); err != nil {
			t.Fatalf("escrowed key does not load: %v", err)
		}
	}
	if _, err := OpenEscrow([]byte(b.Armored), []age.Identity{mustIdentity(t)}); !errors.Is(err, ErrWrongIdentity) {
		t.Fatalf("stranger opened the bundle: %v", err)
	}
	if s := e.settings(); s.EscrowGeneratedAt.IsZero() || !s.EscrowAckedAt.IsZero() {
		t.Fatalf("escrow not stamped: %+v", s)
	}
	if err := e.svc.AckEscrow(context.Background()); err != nil {
		t.Fatal(err)
	}
	if e.settings().EscrowAckedAt.IsZero() {
		t.Fatal("ack not recorded")
	}
}

func TestBuildEscrow_DrillIdentityIsNeverARecipient(t *testing.T) {
	e := newDREnv(t)
	drill := mustIdentity(t)
	e.svc.DrillIdentities = []age.Identity{drill}
	e.configure(nil)
	mk, _ := secrets.GenerateMasterKey()
	b, err := e.svc.BuildEscrow(context.Background(), mk.String(), nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if b.RecipientCount != 1 {
		t.Fatalf("recipient count = %d", b.RecipientCount)
	}
	if _, err := OpenEscrow([]byte(b.Armored), []age.Identity{drill}); !errors.Is(err, ErrWrongIdentity) {
		t.Fatalf("drill identity opened escrow: %v", err)
	}
}

func TestBuildEscrow_RecipientOverrideAndBadKey(t *testing.T) {
	e := newDREnv(t)
	e.configure(nil)
	other := mustIdentity(t)
	mk, _ := secrets.GenerateMasterKey()
	b, err := e.svc.BuildEscrow(context.Background(), mk.String(), []string{other.Recipient().String()}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := OpenEscrow([]byte(b.Armored), []age.Identity{other}); err != nil {
		t.Fatalf("override recipient cannot open: %v", err)
	}
	if _, err := OpenEscrow([]byte(b.Armored), []age.Identity{e.id}); err == nil {
		t.Fatal("configured recipient should not open an overridden bundle")
	}
	if _, err := e.svc.BuildEscrow(context.Background(), "garbage", nil, false); err == nil || strings.Contains(err.Error(), "garbage") {
		t.Fatalf("bad key err = %v", err)
	}
}

func TestBuildEscrow_UploadNeedsSeparateBucket(t *testing.T) {
	e := newDREnv(t)
	mk, _ := secrets.GenerateMasterKey()

	e.configure(nil)
	if _, err := e.svc.BuildEscrow(context.Background(), mk.String(), nil, true); err == nil {
		t.Fatal("upload without an escrow destination must fail")
	}

	e.configure(func(u *ConfigUpdate) { u.EscrowTargetID = "alias" })
	if _, err := e.svc.BuildEscrow(context.Background(), mk.String(), nil, true); !errors.Is(err, ErrEscrowSameBucket) {
		t.Fatalf("same-bucket alias err = %v", err)
	}
	if len(e.srv.Keys()) != 0 {
		t.Fatal("escrow reached the backup bucket")
	}
	st, err := e.svc.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !hasWarning(st, "escrow_same_bucket") {
		t.Fatalf("warnings = %+v", st.Warnings)
	}

	e.configure(func(u *ConfigUpdate) { u.EscrowTargetID = "esc" })
	b, err := e.svc.BuildEscrow(context.Background(), mk.String(), nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(b.UploadedKey, "cp-escrow/inst-") || len(e.srv2.Keys()) != 1 {
		t.Fatalf("upload = %q, keys = %v", b.UploadedKey, e.srv2.Keys())
	}
	st, _ = e.svc.Status(context.Background())
	if hasWarning(st, "escrow_same_bucket") {
		t.Fatal("separate bucket still warned")
	}
}

func TestStatus_ChecklistAndWarnings(t *testing.T) {
	e := newDREnv(t)
	st, err := e.svc.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.Configured || !hasWarning(st, "no_destination") || !hasWarning(st, "no_recipients") || st.Checklist.DestinationChosen {
		t.Fatalf("empty status = %+v", st)
	}

	e.configure(nil)
	e.backup()
	st, _ = e.svc.Status(context.Background())
	if !st.Configured || !st.Checklist.DestinationChosen || !st.Checklist.RecipientSet || st.Checklist.EscrowAcknowledged || st.Checklist.DrillPassed {
		t.Fatalf("checklist = %+v", st.Checklist)
	}
	if !hasWarning(st, "escrow_missing") || !hasWarning(st, "no_drill") || st.NextBackupAt == nil {
		t.Fatalf("status = %+v", st)
	}
	if st.Schedule != "0 2 * * *" || st.RetainDaily != 7 {
		t.Fatalf("effective config = %s %d", st.Schedule, st.RetainDaily)
	}
	e.clock.Advance(time.Minute)
}

func TestRetentionSelect(t *testing.T) {
	day := func(m time.Month, d int) time.Time { return time.Date(2026, m, d, 2, 0, 0, 0, time.UTC) }
	var entries []Entry
	for d := 1; d <= 30; d++ {
		entries = append(entries, Entry{Key: day(6, d).Format("0102"), CreatedAt: day(6, d)})
	}
	entries = append(entries, Entry{Key: "may", CreatedAt: day(5, 15)}, Entry{Key: "apr", CreatedAt: day(4, 15)})

	cases := []struct {
		name string
		r    Retention
		want int
	}{
		{"daily only", Retention{Daily: 3}, 3},
		{"weekly only", Retention{Weekly: 2}, 2},
		{"monthly picks newest per month", Retention{Monthly: 3}, 3},
		{"union of tiers", Retention{Daily: 2, Monthly: 3}, 4},
		{"zero policy still keeps newest", Retention{}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := len(tc.r.Select(entries)); got != tc.want {
				t.Fatalf("kept %d, want %d", got, tc.want)
			}
		})
	}
	keep := Retention{Monthly: 3}.Select(entries)
	if !keep["0630"] || !keep["may"] || !keep["apr"] {
		t.Fatalf("monthly keep set = %v", keep)
	}
	if len(Retention{Daily: 5}.Select(nil)) != 0 {
		t.Fatal("nothing to keep from nothing")
	}
}

func TestKeyRoundTrip(t *testing.T) {
	ts := time.Date(2026, 9, 25, 2, 3, 4, 0, time.UTC)
	key := DataKey("inst-abc", ts)
	id, got, err := ParseKey(key)
	if err != nil || id != "inst-abc" || !got.Equal(ts) {
		t.Fatalf("ParseKey(%q) = %q, %v, %v", key, id, got, err)
	}
	if ManifestKey(key) != "cp-backups/inst-abc/2026/09/25/20260925T020304Z.json" {
		t.Fatalf("manifest key = %q", ManifestKey(key))
	}
	if _, _, err := ParseKey("cp-backups/x/bad"); err == nil {
		t.Fatal("bad key parsed")
	}
}

func TestGenerateIdentity(t *testing.T) {
	for _, hybrid := range []bool{false, true} {
		id, err := GenerateIdentity(hybrid)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ParseRecipients([]string{id.Recipient}); err != nil {
			t.Fatalf("hybrid=%v recipient rejected: %v", hybrid, err)
		}
		if !strings.HasPrefix(id.Secret, "AGE-SECRET-KEY") {
			t.Fatalf("hybrid=%v secret = %q", hybrid, id.Secret[:8])
		}
	}
}

func TestParseRecipients_RejectsMixedKeyKinds(t *testing.T) {
	classic, err := GenerateIdentity(false)
	if err != nil {
		t.Fatal(err)
	}
	hybrid, err := GenerateIdentity(true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseRecipients([]string{classic.Recipient, hybrid.Recipient}); err == nil {
		t.Fatal("mixing classic and post-quantum recipients must be rejected up front")
	}
	if _, err := ParseRecipients([]string{hybrid.Recipient}); err != nil {
		t.Fatalf("hybrid alone: %v", err)
	}
}
