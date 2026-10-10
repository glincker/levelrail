package upgradehistory

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const pseudo = "v0.2.0-beta.18.0.20261009061912-d9ff03786c3e"

func TestClassify(t *testing.T) {
	cases := []struct {
		name           string
		prev, cur      string
		hasPrev, rollM bool
		want           string
	}{
		{"first boot", "", "v1.0.0", false, false, KindInstalled},
		{"upgrade", "v1.0.0", "v1.1.0", true, false, KindUpgraded},
		{"beta to stable", "v1.0.0-beta.2", "v1.0.0", true, false, KindUpgraded},
		{"downgrade", "v1.1.0", "v1.0.0", true, false, KindRolledBack},
		{"dev build", "v1.0.0", "dev", true, false, KindDevelopment},
		{"pseudo version", "v1.0.0", pseudo, true, false, KindDevelopment},
		{"from dev to release", "dev", "v1.0.0", true, false, KindChanged},
		{"from dev with rollback marker", "dev", "v1.0.0", true, true, KindRolledBack},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Classify(c.prev, c.cur, c.hasPrev, c.rollM); got != c.want {
				t.Fatalf("Classify = %s, want %s", got, c.want)
			}
		})
	}
}

func TestBuildLinks(t *testing.T) {
	repo := "https://github.com/acme/widget"
	cases := []struct {
		name, from, to string
		release, cmp   string
	}{
		{"both releases", "v1.0.0", "v1.1.0", repo + "/releases/tag/v1.1.0", repo + "/compare/v1.0.0...v1.1.0"},
		{"from dev", "dev", "v1.1.0", repo + "/releases/tag/v1.1.0", ""},
		{"to pseudo", "v1.0.0", pseudo, "", ""},
		{"to dev", "v1.0.0", "dev", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			l := BuildLinks(repo+"/", c.from, c.to)
			if l.ReleaseURL != c.release || l.CompareURL != c.cmp {
				t.Fatalf("BuildLinks = %+v", l)
			}
		})
	}
	if l := BuildLinks("http://insecure.example/x", "v1.0.0", "v1.1.0"); l != (Links{}) {
		t.Fatalf("non-https repo must yield no links, got %+v", l)
	}
}

func TestMarkerConsumedExactlyOnce(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	m := Marker{ToVersion: "v1.1.0", Initiator: "root", Method: MethodInstallScript, WrittenAt: now}
	if err := WriteMarker(dir, m); err != nil {
		t.Fatal(err)
	}
	if _, ok := ConsumeMarker(dir, "v1.0.0", now); ok {
		t.Fatal("marker for another version must not be consumed")
	}
	got, ok := ConsumeMarker(dir, "v1.1.0", now)
	if !ok || got.Initiator != "root" {
		t.Fatalf("first consume = %+v, %v", got, ok)
	}
	if _, ok := ConsumeMarker(dir, "v1.1.0", now); ok {
		t.Fatal("marker consumed twice")
	}
}

func TestMarkerStaleIsDiscarded(t *testing.T) {
	dir := t.TempDir()
	old := time.Now().Add(-MarkerMaxAge - time.Hour)
	if err := WriteMarker(dir, Marker{ToVersion: "v1.1.0", Initiator: "root", WrittenAt: old}); err != nil {
		t.Fatal(err)
	}
	if _, ok := ConsumeMarker(dir, "v1.1.0", time.Now()); ok {
		t.Fatal("stale marker was trusted")
	}
}

type harness struct {
	db  *store.DB
	dir string
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	dir := t.TempDir()
	db, err := store.Open(context.Background(), filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &harness{db: db, dir: dir}
}

func (h *harness) boot(t *testing.T, version string, schema int, obs Observed) (store.UpgradeHistoryEntry, bool) {
	t.Helper()
	r := &Recorder{
		Store: h.db, DataDir: h.dir, Version: version,
	}
	e, ok, err := r.Record(context.Background(), schema, obs)
	if err != nil {
		t.Fatal(err)
	}
	return e, ok
}

func TestRecorderTransitions(t *testing.T) {
	h := newHarness(t)
	steps := []struct {
		name    string
		version string
		schema  int
		kind    string
		wrote   bool
	}{
		{"first boot", "v0.0.1", 10, KindInstalled, true},
		{"same version restart", "v0.0.1", 10, "", false},
		{"crash loop repeat", "v0.0.1", 10, "", false},
		{"upgrade", "v0.0.2", 12, KindUpgraded, true},
		{"upgrade restart", "v0.0.2", 12, "", false},
		{"rollback", "v0.0.1", 12, KindRolledBack, true},
		{"dev build", "dev", 12, KindDevelopment, true},
	}
	for _, s := range steps {
		e, ok := h.boot(t, s.version, s.schema, Observed{SchemaBefore: -1})
		if ok != s.wrote || (ok && e.Kind != s.kind) {
			t.Fatalf("%s: wrote=%v kind=%q, want wrote=%v kind=%q", s.name, ok, e.Kind, s.wrote, s.kind)
		}
		if ok && e.Initiator != InitiatorUnknown {
			t.Fatalf("%s: initiator %q without a marker", s.name, e.Initiator)
		}
	}
	list, err := h.db.ListUpgradeHistory(context.Background(), 50)
	if err != nil || len(list) != 4 {
		t.Fatalf("history rows = %d, err %v", len(list), err)
	}
	if list[2].FromVersion != "v0.0.1" || list[2].ToVersion != "v0.0.2" || list[2].SchemaBefore != 10 || list[2].SchemaAfter != 12 {
		t.Fatalf("upgrade row = %+v", list[2])
	}
}

func TestRecorderUsesMarker(t *testing.T) {
	h := newHarness(t)
	h.boot(t, "v0.0.1", 10, Observed{SchemaBefore: -1})
	err := WriteMarker(h.dir, Marker{ToVersion: "v0.0.2", Initiator: "gagan", Method: MethodInstallScript, BackupName: "b1", WrittenAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	e, ok := h.boot(t, "v0.0.2", 11, Observed{SchemaBefore: 10, BackupName: "pre-migrate"})
	if !ok || e.Initiator != "gagan" || e.Health != HealthBooted || e.BackupName != "b1" || e.Method != MethodInstallScript {
		t.Fatalf("entry = %+v ok=%v", e, ok)
	}
}

func TestRecorderManualMarkerConsumedOnce(t *testing.T) {
	h := newHarness(t)
	h.boot(t, "v0.0.1", 10, Observed{SchemaBefore: -1})
	m := Marker{ToVersion: "v0.0.2", Initiator: "gagan", Method: MethodManual, Reason: "hand swap", WrittenAt: time.Now()}
	if err := WriteMarker(h.dir, m); err != nil {
		t.Fatal(err)
	}
	e, ok := h.boot(t, "v0.0.2", 10, Observed{SchemaBefore: -1})
	if !ok || e.Initiator != "gagan" || e.Method != MethodManual {
		t.Fatalf("entry = %+v ok=%v", e, ok)
	}
	if _, err := os.Stat(filepath.Join(h.dir, MarkerFile)); !os.IsNotExist(err) {
		t.Fatalf("marker still present after boot: %v", err)
	}
}

func TestHistoryIsAppendOnly(t *testing.T) {
	h := newHarness(t)
	e, _ := h.boot(t, "v0.0.1", 10, Observed{SchemaBefore: -1})
	ctx := context.Background()
	if _, err := h.db.ExecContext(ctx, `UPDATE upgrade_history SET to_version = 'x' WHERE id = ?`, e.ID); err == nil {
		t.Fatal("update of to_version must be refused")
	}
	if _, err := h.db.ExecContext(ctx, `DELETE FROM upgrade_history WHERE id = ?`, e.ID); err == nil {
		t.Fatal("delete must be refused")
	}
	first, changed, err := h.db.AcknowledgeUpgrade(ctx, e.ID, "session", "u1", "Ada", time.Now())
	if err != nil || !changed || !first.Acknowledged() {
		t.Fatalf("ack: %+v %v", first, err)
	}
	second, changed2, err := h.db.AcknowledgeUpgrade(ctx, e.ID, "session", "u2", "Bob", time.Now())
	if err != nil || changed2 || second.AckedByName != "Ada" {
		t.Fatalf("second ack changed the first: %+v %v", second, err)
	}
	if _, _, err := h.db.AcknowledgeUpgrade(ctx, "uh_missing", "session", "u1", "Ada", time.Now()); !errors.Is(err, store.ErrUpgradeHistoryNotFound) {
		t.Fatalf("missing id err = %v", err)
	}
}

func TestBoundNotes(t *testing.T) {
	in := "keep<!-- hidden -->me\x00" + string(make([]rune, 0))
	if got := boundNotes(in); got != "keepme" {
		t.Fatalf("boundNotes = %q", got)
	}
	long := make([]rune, notesMaxRunes+50)
	for i := range long {
		long[i] = 'a'
	}
	if got := []rune(boundNotes(string(long))); len(got) != notesMaxRunes {
		t.Fatalf("len = %d", len(got))
	}
}
