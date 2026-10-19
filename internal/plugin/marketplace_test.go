package plugin

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const indexURL = "https://index.example.com/index.json"

type memMarketStore struct {
	sources []Source
	states  map[string]SourceState
}

func newMemMarketStore() *memMarketStore { return &memMarketStore{states: map[string]SourceState{}} }

func (s *memMarketStore) ListSources(context.Context) ([]Source, error) {
	return slices.Clone(s.sources), nil
}

func (s *memMarketStore) AddSource(_ context.Context, r Source) error {
	for _, x := range s.sources {
		if x.ID == r.ID {
			return ErrSourceExists
		}
	}
	s.sources = append(s.sources, r)
	return nil
}

func (s *memMarketStore) RemoveSource(_ context.Context, id string) error {
	for i, x := range s.sources {
		if x.ID == id {
			s.sources = slices.Delete(s.sources, i, i+1)
			delete(s.states, id)
			return nil
		}
	}
	return ErrSourceUnknown
}

func (s *memMarketStore) GetState(_ context.Context, id string) (SourceState, error) {
	if st, ok := s.states[id]; ok {
		return st, nil
	}
	return SourceState{SourceID: id}, nil
}

func (s *memMarketStore) SaveState(_ context.Context, st SourceState) error {
	s.states[st.SourceID] = st
	return nil
}

type failUpdateStore struct {
	*memStore
	failNext int
}

func (s *failUpdateStore) UpdatePlugin(ctx context.Context, r Record) error {
	if s.failNext > 0 {
		s.failNext--
		return errBoom
	}
	return s.memStore.UpdatePlugin(ctx, r)
}

type marketFixture struct {
	h         *harness
	mk        *Marketplace
	ms        *memMarketStore
	rootPriv  ed25519.PrivateKey
	rootPub   ed25519.PublicKey
	pubPriv   ed25519.PrivateKey
	pubPub    ed25519.PublicKey
	files     map[string][]byte
	index     []byte
	etag      string
	indexErr  error
	indexHits int
}

func newMarketFixture(t *testing.T) *marketFixture {
	t.Helper()
	f := &marketFixture{h: newHarness(t), ms: newMemMarketStore(), files: map[string][]byte{}}
	var err error
	if f.rootPub, f.rootPriv, err = ed25519.GenerateKey(rand.Reader); err != nil {
		t.Fatal(err)
	}
	if f.pubPub, f.pubPriv, err = ed25519.GenerateKey(rand.Reader); err != nil {
		t.Fatal(err)
	}
	f.h.store.keys = nil
	mk, err := NewMarketplace(f.ms, f.h.m, indexURL, b64(f.rootPub), "0.13.0")
	if err != nil {
		t.Fatal(err)
	}
	mk.now = func() time.Time { return f.h.now }
	mk.fetchFile = func(_ context.Context, u string, _ int64) ([]byte, error) {
		if b, ok := f.files[u]; ok {
			return b, nil
		}
		return nil, ErrSourceNotFound
	}
	mk.fetchIndex = func(_ context.Context, _ string, _ int64, etag string) (IndexFetchResult, error) {
		f.indexHits++
		if f.indexErr != nil {
			return IndexFetchResult{}, f.indexErr
		}
		if etag != "" && etag == f.etag {
			return IndexFetchResult{NotModified: true, ETag: etag}, nil
		}
		return IndexFetchResult{Body: f.index, ETag: f.etag}, nil
	}
	f.mk = mk
	return f
}

func (f *marketFixture) manifest(version, digest, abilities string) []byte {
	return manifestV(version, digest, abilities, "[]", "")
}

func (f *marketFixture) entryFor(t *testing.T, raw []byte, digest string) Entry {
	t.Helper()
	url := fmt.Sprintf("https://plugins.example.com/example/m%d.yaml", len(f.files))
	f.files[url] = raw
	e, err := BuildEntry(raw, url, Sign(f.pubPriv, raw, digest), f.pubPub, Entry{Name: "Example", Summary: "Delivers alerts", Categories: []string{"chat"}, Icon: "slack"})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func (f *marketFixture) publish(t *testing.T, seq int64, entries ...Entry) {
	t.Helper()
	idx := Index{
		Version: 1, GeneratedAt: f.h.now.Format(time.RFC3339), Sequence: seq,
		PublisherKeys: []IndexKey{{KeyID: Fingerprint(f.pubPub), Name: "Example Org", PublicKey: b64(f.pubPub)}},
		Plugins:       entries,
	}
	raw, err := SignIndex(f.rootPriv, idx)
	if err != nil {
		t.Fatal(err)
	}
	f.index = raw
}

func (f *marketFixture) req(e Entry, abilities ...string) MarketInstallRequest {
	return MarketInstallRequest{SourceID: BuiltinSourceID, ID: e.ID, Version: e.Version, Digest: e.Digest, Abilities: abilities, TrustKeyID: e.PublisherKeyID}
}

func TestIndexRejectsTamperWrongKeyAndOversize(t *testing.T) {
	f := newMarketFixture(t)
	raw := f.manifest("1.0.0", testDigest, "[read]")
	f.publish(t, 1, f.entryFor(t, raw, testDigest))
	if _, _, err := ParseIndex(f.index, f.rootPub); err != nil {
		t.Fatalf("valid index rejected: %v", err)
	}
	var env IndexEnvelope
	if err := json.Unmarshal(f.index, &env); err != nil {
		t.Fatal(err)
	}
	payload, _ := base64.StdEncoding.DecodeString(env.Payload)

	tamperedEntry := strings.Replace(string(payload), `"abilities":["read"]`, `"abilities":["read","read:sensitive"]`, 1)
	if tamperedEntry == string(payload) {
		t.Fatal("test setup: entry abilities not found in payload")
	}
	otherPub, _, _ := ed25519.GenerateKey(rand.Reader)
	mutate := func(p string) []byte {
		e := env
		e.Payload = b64([]byte(p))
		out, _ := json.Marshal(e)
		return out
	}
	cases := []struct {
		name string
		raw  []byte
		root ed25519.PublicKey
		want error
	}{
		{"tampered entry abilities", mutate(tamperedEntry), f.rootPub, ErrIndexSignature},
		{"tampered sequence", mutate(strings.Replace(string(payload), `"sequence":1`, `"sequence":9`, 1)), f.rootPub, ErrIndexSignature},
		{"wrong root key", f.index, otherPub, ErrIndexSignature},
		{"truncated signature", mutate(string(payload))[:len(mutate(string(payload)))-10], f.rootPub, ErrIndexFormat},
		{"oversized", []byte(strings.Repeat("x", maxIndexBytes+1)), f.rootPub, ErrIndexTooLarge},
		{"not json", []byte("nope"), f.rootPub, ErrIndexFormat},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := ParseIndex(tc.raw, tc.root); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestIndexValidationRefusesUnsafeEntries(t *testing.T) {
	f := newMarketFixture(t)
	good := f.entryFor(t, f.manifest("1.0.0", testDigest, "[read]"), testDigest)
	cases := map[string]func(e *Entry){
		"unknown publisher key": func(e *Entry) { e.PublisherKeyID = "ed25519:" + strings.Repeat("0", 64) },
		"root ability":          func(e *Entry) { e.Abilities = []string{"read", "root"} },
		"http manifest url":     func(e *Entry) { e.ManifestURL = "http://plugins.example.com/m.yaml" },
		"bad digest":            func(e *Entry) { e.Digest = "latest" },
		"bad signature":         func(e *Entry) { e.ManifestSignature = "AAAA" },
	}
	for name, mut := range cases {
		t.Run(name, func(t *testing.T) {
			e := good
			mut(&e)
			idx := Index{Version: 1, GeneratedAt: f.h.now.Format(time.RFC3339), Sequence: 1,
				PublisherKeys: []IndexKey{{KeyID: Fingerprint(f.pubPub), Name: "Example Org", PublicKey: b64(f.pubPub)}}, Plugins: []Entry{e}}
			if _, err := SignIndex(f.rootPriv, idx); err == nil {
				t.Fatal("unsafe entry was signed")
			}
		})
	}
}

func TestBuildEntryRefusesSignatureFromAnotherKey(t *testing.T) {
	f := newMarketFixture(t)
	raw := f.manifest("1.0.0", testDigest, "[read]")
	_, other, _ := ed25519.GenerateKey(rand.Reader)
	if _, err := BuildEntry(raw, "https://p.example.com/m.yaml", Sign(other, raw, testDigest), f.pubPub, Entry{}); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("err = %v, want bad signature", err)
	}
}

func TestReplayedOrRolledBackIndexIsRejected(t *testing.T) {
	ctx := context.Background()
	f := newMarketFixture(t)
	e := f.entryFor(t, f.manifest("1.0.0", testDigest, "[read]"), testDigest)
	f.publish(t, 5, e)
	old := f.index
	if cat, err := f.mk.Catalog(ctx, true); err != nil || cat.Sources[0].Status != "ok" || len(cat.Entries) != 1 {
		t.Fatalf("first fetch: %+v %v", cat, err)
	}
	f.publish(t, 4, e)
	cat, _ := f.mk.Catalog(ctx, true)
	if cat.Sources[0].Status != "error" || cat.Sources[0].ErrorKind != KindReplay || len(cat.Entries) != 0 {
		t.Fatalf("older sequence accepted: %+v", cat.Sources[0])
	}
	f.index = old
	f.publish(t, 5, f.entryFor(t, f.manifest("1.0.1", testDigest, "[read]"), testDigest))
	if cat, _ = f.mk.Catalog(ctx, true); cat.Sources[0].ErrorKind != KindReplay {
		t.Fatalf("same sequence with different content accepted: %+v", cat.Sources[0])
	}
	f.publish(t, 6, e)
	if cat, _ = f.mk.Catalog(ctx, true); cat.Sources[0].Status != "ok" || cat.Sources[0].Sequence != 6 {
		t.Fatalf("newer sequence refused: %+v", cat.Sources[0])
	}
}

func TestBadSignatureShowsTypedErrorAndServesNothing(t *testing.T) {
	ctx := context.Background()
	f := newMarketFixture(t)
	f.publish(t, 1, f.entryFor(t, f.manifest("1.0.0", testDigest, "[read]"), testDigest))
	if _, err := f.mk.Catalog(ctx, true); err != nil {
		t.Fatal(err)
	}
	other, otherPriv, _ := ed25519.GenerateKey(rand.Reader)
	_ = other
	raw, _ := SignIndex(otherPriv, Index{Version: 1, GeneratedAt: f.h.now.Format(time.RFC3339), Sequence: 2,
		PublisherKeys: []IndexKey{{KeyID: Fingerprint(f.pubPub), Name: "x", PublicKey: b64(f.pubPub)}}})
	f.index = raw
	cat, _ := f.mk.Catalog(ctx, true)
	if s := cat.Sources[0]; s.Status != "error" || s.ErrorKind != KindSignature || len(cat.Entries) != 0 {
		t.Fatalf("status = %+v", s)
	}
}

func TestCacheETagAndStaleFallback(t *testing.T) {
	ctx := context.Background()
	f := newMarketFixture(t)
	f.etag = `"v1"`
	f.publish(t, 1, f.entryFor(t, f.manifest("1.0.0", testDigest, "[read]"), testDigest))
	_, _ = f.mk.Catalog(ctx, false)
	_, _ = f.mk.Catalog(ctx, false)
	if f.indexHits != 1 {
		t.Fatalf("a fresh cache was refetched: %d hits", f.indexHits)
	}
	cat, _ := f.mk.Catalog(ctx, true)
	if f.indexHits != 2 || cat.Sources[0].Status != "ok" || len(cat.Entries) != 1 {
		t.Fatalf("conditional refetch: hits=%d %+v", f.indexHits, cat.Sources[0])
	}
	f.h.advance(48 * time.Hour)
	f.indexErr = errors.New("network down")
	cat, _ = f.mk.Catalog(ctx, false)
	if cat.Sources[0].Status != "stale" || cat.Sources[0].ErrorKind != KindFetch || len(cat.Entries) != 1 {
		t.Fatalf("stale fallback: %+v", cat.Sources[0])
	}
}

func TestOneClickInstallTrustsApprovesExactlyAndEnables(t *testing.T) {
	ctx := context.Background()
	f := newMarketFixture(t)
	e := f.entryFor(t, f.manifest("1.0.0", testDigest, "[read]"), testDigest)
	f.publish(t, 1, e)

	pv, err := f.mk.Preview(ctx, BuiltinSourceID, "example")
	if err != nil || pv.KeyTrusted || pv.Action != "install" || !slices.Equal(pv.AddedAbilities, []string{"read"}) {
		t.Fatalf("preview = %+v %v", pv, err)
	}
	declined := f.req(e, "read")
	declined.TrustKeyID = ""
	if _, err := f.mk.Install(ctx, declined, "admin"); err == nil {
		t.Fatal("installed without confirming the publisher key")
	}
	if recs, _ := f.h.m.List(ctx); len(recs) != 0 || len(f.h.store.keys) != 0 {
		t.Fatalf("declining trust changed state: %d plugins, %d keys", len(recs), len(f.h.store.keys))
	}

	res, err := f.mk.Install(ctx, f.req(e, "read"), "admin")
	if err != nil {
		t.Fatal(err)
	}
	rec := res.Plugin
	if rec.State != StateEnabled || !slices.Equal(rec.ApprovedAbilities, []string{"read"}) || !res.TrustAdded {
		t.Fatalf("record = %+v", rec)
	}
	if len(f.h.store.keys) != 1 || !strings.HasSuffix(f.h.store.keys[0].Name, "(added from marketplace)") {
		t.Fatalf("keys = %+v", f.h.store.keys)
	}
	for _, want := range []string{AuditTrustAdd, AuditInstall, AuditApprove, AuditEnable, AuditMarketplaceInstall} {
		if !slices.Contains(f.h.audit.actions(), want) {
			t.Fatalf("audit %v is missing %s", f.h.audit.actions(), want)
		}
	}
}

func TestInstallRefusesWhatTheSheetDidNotShow(t *testing.T) {
	ctx := context.Background()
	f := newMarketFixture(t)
	e := f.entryFor(t, f.manifest("1.0.0", testDigest, "[read]"), testDigest)
	f.publish(t, 1, e)
	cases := map[string]func(r *MarketInstallRequest){
		"abilities narrower": func(r *MarketInstallRequest) { r.Abilities = []string{} },
		"abilities wider":    func(r *MarketInstallRequest) { r.Abilities = []string{"read", "read:sensitive"} },
		"other version":      func(r *MarketInstallRequest) { r.Version = "0.9.0" },
		"other digest":       func(r *MarketInstallRequest) { r.Digest = testDigest2 },
	}
	for name, mut := range cases {
		t.Run(name, func(t *testing.T) {
			req := f.req(e, "read")
			mut(&req)
			if _, err := f.mk.Install(ctx, req, "admin"); !errors.Is(err, ErrChanged) {
				t.Fatalf("err = %v, want ErrChanged", err)
			}
		})
	}
	if recs, _ := f.h.m.List(ctx); len(recs) != 0 || len(f.h.store.keys) != 0 {
		t.Fatal("a refused install left state behind")
	}
}

func TestManifestThatDisagreesWithSignedListingIsRefused(t *testing.T) {
	ctx := context.Background()
	f := newMarketFixture(t)
	raw := f.manifest("1.0.0", testDigest, "[read]")
	e := f.entryFor(t, raw, testDigest)
	f.publish(t, 1, e)
	swapped := f.manifest("1.0.0", testDigest, "[read, read:sensitive]")
	f.files[e.ManifestURL] = swapped
	if _, err := f.mk.Install(ctx, f.req(e, "read"), "admin"); err == nil || !strings.Contains(err.Error(), "abilities") {
		t.Fatalf("ability escalation in the manifest was accepted: %v", err)
	}
	f.files[e.ManifestURL] = []byte(strings.Replace(string(raw), "publisher: Example Org", "publisher: Example Org\ndescription: edited", 1))
	if _, err := f.mk.Install(ctx, f.req(e, "read"), "admin"); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("edited manifest with a stale signature: %v", err)
	}
	f.files[e.ManifestURL] = f.manifest("1.0.0", testDigest2, "[read]")
	if _, err := f.mk.Install(ctx, f.req(e, "read"), "admin"); err == nil {
		t.Fatal("a manifest pinning another digest was accepted")
	}
}

func TestUpdateWithNewAbilityNeedsTheExactNewApproval(t *testing.T) {
	ctx := context.Background()
	f := newMarketFixture(t)
	v1 := f.entryFor(t, f.manifest("1.0.0", testDigest, "[read]"), testDigest)
	f.publish(t, 1, v1)
	if _, err := f.mk.Install(ctx, f.req(v1, "read"), "admin"); err != nil {
		t.Fatal(err)
	}
	v2 := f.entryFor(t, f.manifest("1.1.0", testDigest2, "[read, read:sensitive]"), testDigest2)
	f.publish(t, 2, v2)

	cat, _ := f.mk.Catalog(ctx, true)
	if !cat.Entries[0].UpdateAvailable || cat.Entries[0].InstalledVersion != "1.0.0" {
		t.Fatalf("update flag: %+v", cat.Entries[0])
	}
	pv, err := f.mk.Preview(ctx, BuiltinSourceID, "example")
	if err != nil || pv.Action != "upgrade" || !pv.Reapproval || !slices.Equal(pv.AddedAbilities, []string{"read:sensitive"}) {
		t.Fatalf("preview = %+v %v", pv, err)
	}
	if _, err := f.mk.Install(ctx, f.req(v2, "read"), "admin"); !errors.Is(err, ErrChanged) {
		t.Fatalf("update approved with the old ability set: %v", err)
	}
	res, err := f.mk.Install(ctx, f.req(v2, "read", "read:sensitive"), "admin")
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != "upgrade" || res.Plugin.Version != "1.1.0" || !slices.Equal(res.Plugin.ApprovedAbilities, []string{"read", "read:sensitive"}) || res.TrustAdded {
		t.Fatalf("upgrade result = %+v", res)
	}
}

func TestDowngradeAndPublisherSwitchAreRefused(t *testing.T) {
	ctx := context.Background()
	f := newMarketFixture(t)
	v2 := f.entryFor(t, f.manifest("2.0.0", testDigest2, "[read]"), testDigest2)
	f.publish(t, 1, v2)
	if _, err := f.mk.Install(ctx, f.req(v2, "read"), "admin"); err != nil {
		t.Fatal(err)
	}
	v1 := f.entryFor(t, f.manifest("1.0.0", testDigest, "[read]"), testDigest)
	f.publish(t, 2, v1)
	_, _ = f.mk.Refresh(ctx, "")
	if _, err := f.mk.Install(ctx, f.req(v1, "read"), "admin"); err == nil || !strings.Contains(err.Error(), "roll back") {
		t.Fatalf("version rollback accepted: %v", err)
	}
	rec, _ := f.h.store.GetPlugin(ctx, "example")
	rec.KeyID = "ed25519:" + strings.Repeat("a", 64)
	_ = f.h.store.UpdatePlugin(ctx, rec)
	v3 := f.entryFor(t, f.manifest("3.0.0", testDigest, "[read]"), testDigest)
	f.publish(t, 3, v3)
	_, _ = f.mk.Refresh(ctx, "")
	if _, err := f.mk.Install(ctx, f.req(v3, "read"), "admin"); err == nil || !strings.Contains(err.Error(), "different publisher") {
		t.Fatalf("publisher switch accepted: %v", err)
	}
}

func TestFailedInstallRollsBackWhatItAdded(t *testing.T) {
	ctx := context.Background()
	t.Run("enable fails", func(t *testing.T) {
		f := newMarketFixture(t)
		fs := &failUpdateStore{memStore: f.h.store}
		f.h.m.store = fs
		e := f.entryFor(t, f.manifest("1.0.0", testDigest, "[read]"), testDigest)
		f.publish(t, 1, e)
		fs.failNext = 1
		if _, err := f.mk.Install(ctx, f.req(e, "read"), "admin"); err == nil {
			t.Fatal("expected failure")
		}
		if recs, _ := f.h.m.List(ctx); len(recs) != 0 {
			t.Fatalf("a half-installed plugin was left: %+v", recs)
		}
	})
	t.Run("audit fails", func(t *testing.T) {
		f := newMarketFixture(t)
		e := f.entryFor(t, f.manifest("1.0.0", testDigest, "[read]"), testDigest)
		f.publish(t, 1, e)
		f.h.audit.fail = errBoom
		if _, err := f.mk.Install(ctx, f.req(e, "read"), "admin"); err == nil {
			t.Fatal("expected failure")
		}
		if len(f.h.store.keys) != 0 {
			t.Fatalf("the trust key added by the failed flow was kept: %+v", f.h.store.keys)
		}
	})
}

func TestIncompatibleControlPlaneIsRefused(t *testing.T) {
	ctx := context.Background()
	f := newMarketFixture(t)
	raw := f.manifest("1.0.0", testDigest, "[read]")
	url := "https://plugins.example.com/example/m.yaml"
	f.files[url] = raw
	e, err := BuildEntry(raw, url, Sign(f.pubPriv, raw, testDigest), f.pubPub, Entry{MinVersion: "9.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	f.publish(t, 1, e)
	cat, _ := f.mk.Catalog(ctx, true)
	if cat.Entries[0].Compatible {
		t.Fatal("listing should be flagged incompatible")
	}
	if _, err := f.mk.Install(ctx, f.req(e, "read"), "admin"); err == nil || !strings.Contains(err.Error(), "9.0.0") {
		t.Fatalf("err = %v", err)
	}
}

func TestSourcesAreSSRFSafeAndSizeCapped(t *testing.T) {
	ctx := context.Background()
	f := newMarketFixture(t)
	for _, u := range []string{"http://index.example.com/i.json", "https://127.0.0.1/i.json", "https://localhost/i.json", "https://10.0.0.5/i.json", "ftp://x.example.com/i.json"} {
		if _, err := f.mk.AddSource(ctx, "x", u, b64(f.rootPub), "admin"); err == nil {
			t.Errorf("source %s accepted", u)
		}
	}
	if _, err := f.mk.AddSource(ctx, "x", "https://other.example.com/i.json", "not-a-key", "admin"); err == nil {
		t.Error("a bad root key was accepted")
	}
	s, err := f.mk.AddSource(ctx, "", "https://other.example.com/i.json", b64(f.rootPub), "admin")
	if err != nil || s.Name != "other.example.com" {
		t.Fatalf("add = %+v %v", s, err)
	}
	if _, err := f.mk.AddSource(ctx, "", "https://other.example.com/i.json", b64(f.rootPub), "admin"); !errors.Is(err, ErrSourceExists) {
		t.Fatalf("duplicate = %v", err)
	}
	if err := f.mk.RemoveSource(ctx, BuiltinSourceID); err == nil {
		t.Error("the default source was removable")
	}
	if err := f.mk.RemoveSource(ctx, s.ID); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("{}")) }))
	defer srv.Close()
	if _, err := FetchIndex(ctx, srv.URL, maxIndexBytes, ""); err == nil {
		t.Fatal("FetchIndex reached a loopback server")
	}
	if _, err := FetchIndex(ctx, "http://example.com/i.json", maxIndexBytes, ""); err == nil {
		t.Fatal("FetchIndex accepted plain http")
	}
	if _, err := Fetch(ctx, srv.URL+"/m.yaml", maxManifestBytes); err == nil {
		t.Fatal("manifest fetch reached a loopback server")
	}
}

func TestSQLMarketStoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "m.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	s := NewSQLMarketStore(db)
	src := Source{ID: "src-1", Name: "n", URL: indexURL, RootKey: "k", AddedAt: "t", AddedBy: "a"}
	if err := s.AddSource(ctx, src); err != nil {
		t.Fatal(err)
	}
	if err := s.AddSource(ctx, src); !errors.Is(err, ErrSourceExists) {
		t.Fatalf("duplicate = %v", err)
	}
	if err := s.SaveState(ctx, SourceState{SourceID: "src-1", LastSequence: 3, Body: "b", ETag: "e"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveState(ctx, SourceState{SourceID: "src-1", LastSequence: 4, Body: "b2"}); err != nil {
		t.Fatal(err)
	}
	st, _ := s.GetState(ctx, "src-1")
	if st.LastSequence != 4 || st.Body != "b2" {
		t.Fatalf("state = %+v", st)
	}
	if got, _ := s.ListSources(ctx); len(got) != 1 || got[0].ID != "src-1" {
		t.Fatalf("sources = %+v", got)
	}
	if err := s.RemoveSource(ctx, "src-1"); err != nil {
		t.Fatal(err)
	}
	if st, _ = s.GetState(ctx, "src-1"); st.LastSequence != 0 {
		t.Fatal("state outlived its source")
	}
	if err := s.RemoveSource(ctx, "src-1"); !errors.Is(err, ErrSourceUnknown) {
		t.Fatalf("remove twice = %v", err)
	}
}
