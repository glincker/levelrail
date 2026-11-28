package announcements

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

var testNow = time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)

func testLimits() Limits {
	return Limits{MaxBytes: 1 << 18, MaxItems: 50, MaxTitle: 120, MaxBody: 2000, MaxAge: 720 * time.Hour, ClockSkew: 24 * time.Hour}
}

type testKey struct {
	key  Key
	priv ed25519.PrivateKey
}

func newKey(t *testing.T, id string, tiers ...string) testKey {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	return testKey{key: Key{ID: id, Public: pub, Tiers: tiers}, priv: priv}
}

func sign(data []byte, keys ...testKey) []byte {
	var sigs []Signature
	for _, k := range keys {
		sigs = append(sigs, Signature{KeyID: k.key.ID, Sig: base64.StdEncoding.EncodeToString(ed25519.Sign(k.priv, data))})
	}
	b, _ := json.Marshal(sigs)
	return b
}

func item(id, tier string) map[string]any {
	it := map[string]any{
		"id": id, "tier": tier, "kind": "announcement", "title": "Title " + id, "body": "Plain body text.",
		"issued_at": testNow.Add(-time.Hour), "expires_at": testNow.Add(48 * time.Hour),
		"future_field": "ignored",
	}
	if tier == TierAdvisory || tier == TierCritical {
		it["applies_to"] = "<1.5.14"
	}
	return it
}

func feedBytes(seq int64, items ...map[string]any) []byte {
	b, _ := json.Marshal(map[string]any{
		"schema": 1, "sequence": seq, "issued_at": testNow, "expires_at": testNow.Add(240 * time.Hour),
		"items": items, "unknown_top": true,
	})
	return b
}

func verify(t *testing.T, data []byte, sigs []byte, last int64, keys ...Key) (Verified, error) {
	t.Helper()
	return VerifyFeed(VerifyInput{Data: data, SigData: sigs, Keys: keys, LastSequence: last, Now: testNow, Limits: testLimits()})
}

func ids(items []Item) string {
	var out []string
	for _, it := range items {
		out = append(out, it.ID)
	}
	return strings.Join(out, ",")
}

func TestVerifyFeedSignatures(t *testing.T) {
	n := newKey(t, "N", TierNewsletter, TierFeature, TierInfo, TierMaintenance)
	s1 := newKey(t, "S1", TierAdvisory, TierCritical)
	s2 := newKey(t, "S2", TierAdvisory, TierCritical)
	other := newKey(t, "N", TierNewsletter)
	data := feedBytes(5, item("news", TierNewsletter), item("adv", TierAdvisory), item("crit", TierCritical))

	got, err := verify(t, data, sign(data, n, s1), 0, n.key, s1.key, s2.key)
	if err != nil {
		t.Fatal(err)
	}
	if ids(got.Items) != "news,adv" {
		t.Fatalf("one security key must not pass critical: got %q", ids(got.Items))
	}
	got, err = verify(t, data, sign(data, n, s1, s2), 0, n.key, s1.key, s2.key)
	if err != nil || ids(got.Items) != "news,adv,crit" {
		t.Fatalf("two security keys pass critical: %q %v", ids(got.Items), err)
	}
	got, err = verify(t, data, sign(data, n), 0, n.key, s1.key, s2.key)
	if err != nil || ids(got.Items) != "news" {
		t.Fatalf("newsletter key must not sign higher tiers: %q %v", ids(got.Items), err)
	}
	if _, err = verify(t, data, sign(data, other), 0, n.key); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("wrong key material under a known id: %v", err)
	}
	if _, err = verify(t, data, sign(data, s1), 0); !errors.Is(err, ErrNoTrustedKeys) {
		t.Fatalf("empty production list must fail closed: %v", err)
	}
	tampered := append([]byte(nil), data...)
	tampered[len(tampered)-3] ^= 1
	if _, err = verify(t, tampered, sign(data, n), 0, n.key); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("tampered body: %v", err)
	}
}

func TestVerifyFeedRevokedKey(t *testing.T) {
	n := newKey(t, "N", TierNewsletter)
	data := feedBytes(1, item("news", TierNewsletter))
	_, err := VerifyFeed(VerifyInput{Data: data, SigData: sign(data, n), Keys: []Key{n.key}, Revoked: map[string]bool{"N": true}, Now: testNow, Limits: testLimits()})
	if !errors.Is(err, ErrNoTrustedKeys) {
		t.Fatalf("revoked key is not trusted: %v", err)
	}
}

func TestVerifyFeedSequenceAndExpiry(t *testing.T) {
	n := newKey(t, "N", TierNewsletter, TierInfo)
	data := feedBytes(5, item("a", TierNewsletter))
	if _, err := verify(t, data, sign(data, n), 9, n.key); !errors.Is(err, ErrReplay) {
		t.Fatalf("older sequence must be rejected: %v", err)
	}
	if _, err := verify(t, data, sign(data, n), 5, n.key); !errors.Is(err, ErrUnchanged) {
		t.Fatalf("same sequence is a no-op: %v", err)
	}
	expired := item("old", TierNewsletter)
	expired["expires_at"] = testNow.Add(-48 * time.Hour)
	data = feedBytes(6, expired, item("live", TierNewsletter))
	got, err := verify(t, data, sign(data, n), 5, n.key)
	if err != nil || ids(got.Items) != "live" {
		t.Fatalf("expired item skipped: %q %v", ids(got.Items), err)
	}
	stale, _ := json.Marshal(map[string]any{"schema": 1, "sequence": 7, "issued_at": testNow, "expires_at": testNow.Add(-72 * time.Hour), "items": []any{}})
	if _, err := verify(t, stale, sign(stale, n), 0, n.key); !errors.Is(err, ErrFeedExpired) {
		t.Fatalf("expired feed: %v", err)
	}
	long, _ := json.Marshal(map[string]any{"schema": 1, "sequence": 7, "issued_at": testNow, "expires_at": testNow.Add(5000 * time.Hour), "items": []any{}})
	if _, err := verify(t, long, sign(long, n), 0, n.key); !errors.Is(err, ErrFeedLifetime) {
		t.Fatalf("feed lifetime beyond the cap: %v", err)
	}
}

func TestParseFeedTolerance(t *testing.T) {
	if _, err := ParseFeed([]byte(`{not json`), testLimits()); err == nil {
		t.Fatal("malformed JSON must fail")
	}
	if _, err := ParseFeed([]byte(`{"schema":2,"sequence":1}`), testLimits()); !errors.Is(err, ErrSchema) {
		t.Fatalf("unknown schema: %v", err)
	}
	big := feedBytes(1, item("a", TierInfo))
	lim := testLimits()
	lim.MaxBytes = 10
	if _, err := ParseFeed(big, lim); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("oversize feed: %v", err)
	}
	lim = testLimits()
	lim.MaxItems = 1
	if _, err := ParseFeed(feedBytes(1, item("a", TierInfo), item("b", TierInfo)), lim); !errors.Is(err, ErrTooManyItem) {
		t.Fatalf("item cap: %v", err)
	}
	weird := item("w", "mystery")
	f, err := ParseFeed(feedBytes(1, weird, item("ok", TierInfo)), testLimits())
	if err != nil || len(f.Items) != 1 || f.Dropped != 1 {
		t.Fatalf("unknown tier dropped, unknown fields ignored: %+v %v", f, err)
	}
}

func TestValidateItemRejectsUnsafeContent(t *testing.T) {
	cases := map[string]func(m map[string]any){
		"html":         func(m map[string]any) { m["body"] = "hello <script>alert(1)</script>" },
		"javascript":   func(m map[string]any) { m["body"] = "[x](javascript:alert(1))" },
		"data link":    func(m map[string]any) { m["body"] = "see data:text/html;base64,AAAA" },
		"image":        func(m map[string]any) { m["body"] = "![x](https://levelrail.com/a.png)" },
		"code fence":   func(m map[string]any) { m["body"] = "```\nls\n```" },
		"command":      func(m map[string]any) { m["body"] = "run `curl https://levelrail.com/x`" },
		"pipe in code": func(m map[string]any) { m["body"] = "run `a | b`" },
		"http link":    func(m map[string]any) { m["body"] = "[x](http://levelrail.com/a)" },
		"other host":   func(m map[string]any) { m["body"] = "[x](https://evil.example/a)" },
		"bare url":     func(m map[string]any) { m["body"] = "get https://evil.example/installer" },
		"lookalike":    func(m map[string]any) { m["body"] = "[x](https://levelrail.com.evil.example/a)" },
		"userinfo":     func(m map[string]any) { m["body"] = "[x](https://levelrail.com@evil.example/a)" },
		"other org": func(m map[string]any) {
			m["links"] = []map[string]string{{"text": "t", "url": "https://github.com/someone/else"}}
		},
		"query": func(m map[string]any) {
			m["links"] = []map[string]string{{"text": "t", "url": "https://levelrail.com/a?x=1"}}
		},
		"external cta":  func(m map[string]any) { m["cta"] = "https://levelrail.com/upgrade" },
		"no applies_to": func(m map[string]any) { m["tier"] = "advisory"; delete(m, "applies_to") },
		"bad range":     func(m map[string]any) { m["applies_to"] = "*" },
		"control byte":  func(m map[string]any) { m["title"] = "bad\x1b[31m" },
		"channel":       func(m map[string]any) { m["channels"] = []string{"nightly"} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			m := item("x", TierInfo)
			m["applies_to"] = "<1.5.14"
			mutate(m)
			raw, _ := json.Marshal(m)
			var it Item
			if err := json.Unmarshal(raw, &it); err != nil {
				t.Fatal(err)
			}
			if err := ValidateItem(it, testLimits()); !errors.Is(err, ErrContent) {
				t.Fatalf("want rejection, got %v", err)
			}
		})
	}
	good := item("good", TierInfo)
	good["body"] = "Read **the notes** at [release](https://github.com/GLINCKER/levelrail/releases/tag/v1.5.14) or `levelrail version`."
	raw, _ := json.Marshal(good)
	var it Item
	_ = json.Unmarshal(raw, &it)
	if err := ValidateItem(it, testLimits()); err != nil {
		t.Fatalf("good item rejected: %v", err)
	}
}

func TestVersionRange(t *testing.T) {
	r, err := ParseRange(">=1.5.0,<1.5.14")
	if err != nil {
		t.Fatal(err)
	}
	for v, want := range map[string]bool{"v1.5.13": true, "1.5.0": true, "v1.5.14": false, "v1.4.9": false, "v1.5.14-rc1": false, "dev": false} {
		if r.Match(v) != want {
			t.Errorf("Match(%q) = %v", v, !want)
		}
	}
	for _, bad := range []string{"", "*", "<abc"} {
		if _, err := ParseRange(bad); err == nil {
			t.Errorf("range %q must not parse", bad)
		}
	}
}

func rowOf(t *testing.T, m map[string]any) store.AnnouncementRow {
	t.Helper()
	raw, _ := json.Marshal(m)
	var it Item
	if err := json.Unmarshal(raw, &it); err != nil {
		t.Fatal(err)
	}
	return store.AnnouncementRow{ID: it.ID, Tier: it.Tier, Kind: it.Kind, PayloadJSON: string(raw)}
}

func TestVisibleFiltersByAbilityVersionAndChannel(t *testing.T) {
	adv := item("adv", TierAdvisory)
	beta := item("beta", TierFeature)
	beta["channels"] = []string{"beta"}
	news := item("news", TierNewsletter)
	late := item("late", TierAdvisory)
	late["applies_to"] = ">=2.0.0"
	rows := []store.AnnouncementRow{rowOf(t, adv), rowOf(t, beta), rowOf(t, news), rowOf(t, late)}
	v := Viewer{CanRead: true, Version: "v1.5.13", Channel: "stable", Mode: ModeAll, Newsletter: true, Now: testNow}
	if got := ids(Visible(rows, v)); got != "news" {
		t.Fatalf("non-root sees no security tiers, wrong channel hidden: %q", got)
	}
	v.IsRoot = true
	if got := ids(Visible(rows, v)); got != "adv,news" {
		t.Fatalf("root, version targeting: %q", got)
	}
	v.Mode = ModeSecurity
	if got := ids(Visible(rows, v)); got != "adv" {
		t.Fatalf("security mode shows only security tiers: %q", got)
	}
	v.Mode, v.Newsletter, v.Channel = ModeAll, false, "beta"
	if got := ids(Visible(rows, v)); got != "adv,beta" {
		t.Fatalf("newsletter hidden, beta channel item shown: %q", got)
	}
	v.Mode = ModeOff
	if len(Visible(rows, v)) != 0 {
		t.Fatal("off shows nothing")
	}
}

func TestDevKeysRefusedInProductionMode(t *testing.T) {
	dev := newKey(t, "D", TierInfo)
	entry := "D:" + base64.StdEncoding.EncodeToString(dev.key.Public) + ":info"
	keys, ignored, err := EffectiveKeys(false, []string{entry})
	if err != nil || !ignored || len(keys) != 0 {
		t.Fatalf("production mode must ignore dev keys: %v %v %v", keys, ignored, err)
	}
	keys, ignored, err = EffectiveKeys(true, []string{entry})
	if err != nil || ignored || len(keys) != 1 || keys[0].ID != "D" {
		t.Fatalf("dev mode honours dev keys: %v %v %v", keys, ignored, err)
	}
	if _, _, err = EffectiveKeys(true, []string{"nonsense"}); err == nil {
		t.Fatal("malformed dev key must error")
	}
	if len(ProductionKeys()) != 0 {
		t.Fatal("no production key may be embedded until the owner adds one")
	}
}

type fakeSettings map[string]string

func (f fakeSettings) String(_ context.Context, k string) string { return f[k] }
func (f fakeSettings) Bool(_ context.Context, k string) bool     { return f[k] == "true" }
func (f fakeSettings) Int(_ context.Context, k string) int       { n, _ := strconv.Atoi(f[k]); return n }
func (f fakeSettings) Duration(_ context.Context, k string) time.Duration {
	d, _ := time.ParseDuration(f[k])
	return d
}
func (f fakeSettings) List(_ context.Context, k string) []string {
	if f[k] == "" {
		return nil
	}
	return strings.Split(f[k], ",")
}

type memStore struct {
	state   store.AnnouncementFeedState
	rows    []store.AnnouncementRow
	revoked []string
}

func (m *memStore) GetAnnouncementFeedState(context.Context) (store.AnnouncementFeedState, error) {
	return m.state, nil
}
func (m *memStore) RecordAnnouncementFetch(_ context.Context, etag, at, lastErr string) error {
	if etag != "" {
		m.state.ETag = etag
	}
	m.state.FetchedAt, m.state.LastError = at, lastErr
	return nil
}
func (m *memStore) SaveAnnouncementFeed(_ context.Context, st store.AnnouncementFeedState, rows []store.AnnouncementRow, revoked []string) error {
	m.state, m.rows = st, rows
	m.revoked = append(m.revoked, revoked...)
	return nil
}
func (m *memStore) ListAnnouncements(context.Context) ([]store.AnnouncementRow, error) {
	return m.rows, nil
}
func (m *memStore) ListRevokedAnnouncementKeys(context.Context) ([]string, error) {
	return m.revoked, nil
}

type feedServer struct {
	srv        *httptest.Server
	hits       atomic.Int32
	conditions atomic.Int32
	data, sigs []byte
}

func newFeedServer(data, sigs []byte) *feedServer {
	fs := &feedServer{data: data, sigs: sigs}
	fs.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fs.hits.Add(1)
		if r.Header.Get("User-Agent") != UserAgent || r.URL.RawQuery != "" || r.Header.Get("Cookie") != "" {
			http.Error(w, "unexpected identifying data", http.StatusBadRequest)
			return
		}
		if strings.HasSuffix(r.URL.Path, ".sig") {
			_, _ = w.Write(fs.sigs)
			return
		}
		if r.Header.Get("If-None-Match") == `"v1"` {
			fs.conditions.Add(1)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		_, _ = w.Write(fs.data)
	}))
	return fs
}

func pollSetup(t *testing.T, mode string, fs *feedServer, keys ...Key) (*Manager, *memStore, *time.Time) {
	t.Helper()
	ms := &memStore{}
	now := testNow
	var devKeys []string
	for _, k := range keys {
		devKeys = append(devKeys, k.ID+":"+base64.StdEncoding.EncodeToString(k.Public)+":"+strings.Join(k.Tiers, "|"))
	}
	set := fakeSettings{
		runtimesettings.KeyAnnouncements: mode, runtimesettings.KeyAnnouncementsURL: fs.srv.URL + "/feed.v1.json",
		runtimesettings.KeyAnnouncementsInterval: "6h", runtimesettings.KeyAnnouncementsJitter: "0",
		runtimesettings.KeyAnnouncementsMaxBytes: "262144", runtimesettings.KeyAnnouncementsMaxItems: "50",
		runtimesettings.KeyAnnouncementsMaxTitle: "120", runtimesettings.KeyAnnouncementsMaxBody: "2000",
		runtimesettings.KeyAnnouncementsMaxAge: "720h", runtimesettings.KeyAnnouncementsClockSkew: "24h",
		runtimesettings.KeyAnnouncementsDevKeys: strings.Join(devKeys, ","),
	}
	m := &Manager{Store: ms, Settings: set, Fetcher: Fetcher{Client: fs.srv.Client()}, DevMode: true, Now: func() time.Time { return now }}
	return m, ms, &now
}

func TestPollOffMeansNoFetch(t *testing.T) {
	n := newKey(t, "N", TierInfo)
	data := feedBytes(1, item("a", TierInfo))
	fs := newFeedServer(data, sign(data, n))
	defer fs.srv.Close()
	m, _, _ := pollSetup(t, "off", fs, n.key)
	if err := m.Poll(context.Background()); err != nil || fs.hits.Load() != 0 {
		t.Fatalf("off must not fetch: hits=%d err=%v", fs.hits.Load(), err)
	}
}

func TestPollWithoutTrustedKeysIsInert(t *testing.T) {
	n := newKey(t, "N", TierInfo)
	data := feedBytes(1, item("a", TierInfo))
	fs := newFeedServer(data, sign(data, n))
	defer fs.srv.Close()
	m, _, _ := pollSetup(t, "all", fs, n.key)
	m.DevMode = false
	if err := m.Poll(context.Background()); err != nil || fs.hits.Load() != 0 {
		t.Fatalf("production mode with the empty key list must not fetch: hits=%d err=%v", fs.hits.Load(), err)
	}
}

func TestPollStoresVerifiedFeedAndUsesETag(t *testing.T) {
	n := newKey(t, "N", TierInfo)
	data := feedBytes(3, item("a", TierInfo))
	fs := newFeedServer(data, sign(data, n))
	defer fs.srv.Close()
	m, ms, now := pollSetup(t, "all", fs, n.key)
	ctx := context.Background()
	if err := m.Poll(ctx); err != nil {
		t.Fatal(err)
	}
	if len(ms.rows) != 1 || ms.state.LastSequence != 3 || ms.state.ETag != `"v1"` {
		t.Fatalf("feed not stored: %+v %+v", ms.rows, ms.state)
	}
	if err := m.Poll(ctx); err != nil || fs.hits.Load() != 2 {
		t.Fatalf("not due yet, no second request: hits=%d err=%v", fs.hits.Load(), err)
	}
	*now = now.Add(7 * time.Hour)
	if err := m.Poll(ctx); err != nil || fs.conditions.Load() != 1 {
		t.Fatalf("due poll must send If-None-Match and get 304: conditions=%d err=%v", fs.conditions.Load(), err)
	}
	if len(ms.rows) != 1 {
		t.Fatal("304 must keep the cache")
	}
}

func TestPollRejectsBadSignatureAndKeepsCache(t *testing.T) {
	n := newKey(t, "N", TierInfo)
	attacker := newKey(t, "N", TierInfo)
	data := feedBytes(4, item("evil", TierInfo))
	fs := newFeedServer(data, sign(data, attacker))
	defer fs.srv.Close()
	m, ms, _ := pollSetup(t, "all", fs, n.key)
	ms.rows = []store.AnnouncementRow{{ID: "kept"}}
	if err := m.Poll(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(ms.rows) != 1 || ms.rows[0].ID != "kept" || ms.state.LastError == "" {
		t.Fatalf("bad signature must keep the cache and record the error: %+v %+v", ms.rows, ms.state)
	}
}

func TestPollOfflineIsSilent(t *testing.T) {
	n := newKey(t, "N", TierInfo)
	fs := newFeedServer(nil, nil)
	m, ms, _ := pollSetup(t, "all", fs, n.key)
	fs.srv.Close()
	if err := m.Poll(context.Background()); err != nil {
		t.Fatalf("offline must not return an error: %v", err)
	}
	if ms.state.LastError == "" {
		t.Fatal("offline should record last_error for Settings")
	}
	if got := fmt.Sprint(m.failures); got != "1" {
		t.Fatalf("backoff should count the failure: %s", got)
	}
}

func TestFetcherRefusesPlainHTTP(t *testing.T) {
	if _, err := (Fetcher{}).Get(context.Background(), "http://example.invalid/feed", "", 10); !errors.Is(err, ErrNotHTTPS) {
		t.Fatalf("plain http must be refused: %v", err)
	}
}
