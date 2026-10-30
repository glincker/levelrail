package advisoryfeed

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const testFeed = `{"version":1,"entries":[
{"id":"TEST-0001","severity":"high","summary":"fake runtime flaw","fixed_in":"20.5.0","reference":"https://example.invalid/TEST-0001",
 "affected":[{"repository":"docker.io/library/node","ranges":[{"from":"20.0.0","before":"20.5.0"}]}]},
{"id":"TEST-0002","severity":"critical","summary":"fake digest flaw",
 "affected":[{"repository":"ghcr.io/example/app","digests":["sha256:abc"]}]}]}`

func TestParseRejectsBadFeeds(t *testing.T) {
	for name, doc := range map[string]string{
		"version":  `{"version":2,"entries":[]}`,
		"selector": `{"version":1,"entries":[{"id":"TEST-1","severity":"low","summary":"x","affected":[{"repository":"node"}]}]}`,
		"severity": `{"version":1,"entries":[{"id":"TEST-1","severity":"bad","summary":"x","affected":[{"repository":"node","tags":["1"]}]}]}`,
	} {
		if _, err := Parse([]byte(doc)); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

func TestMatchImages(t *testing.T) {
	f, err := Parse([]byte(testFeed))
	if err != nil {
		t.Fatal(err)
	}
	got := MatchImages(f, []Image{
		{App: "a", Ref: "node:20.2.1-alpine"},
		{App: "b", Ref: "node:20.5.0"},
		{App: "c", Ref: "ghcr.io/example/app:latest", Digest: "sha256:ABC"},
		{App: "d", Ref: "ghcr.io/example/app:latest"},
	})
	if len(got) != 2 || got[0].AdvisoryID != "TEST-0002" || got[1].App != "a" || got[1].Hint != "Move to 20.5.0 or newer, then redeploy." {
		t.Fatalf("unexpected matches: %+v", got)
	}
}

func TestServiceSignatureStates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "feed.json")
	if err := os.WriteFile(path, []byte(testFeed), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if s := New(Config{Source: path, MaxBytes: 1 << 20}, nil).Snapshot(ctx); s.Signature != SigUnsigned || len(s.Entries) != 2 {
		t.Fatalf("unsigned: %+v", s)
	}
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	cfg := Config{Source: path, PubKey: base64.StdEncoding.EncodeToString(pub), MaxBytes: 1 << 20}
	if s := New(cfg, nil).Snapshot(ctx); s.Signature != SigInvalid || len(s.Entries) != 0 || s.Error == "" {
		t.Fatalf("missing sig must not be trusted: %+v", s)
	}
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(priv, []byte(testFeed)))
	if err := os.WriteFile(path+".sig", []byte(sig), 0o600); err != nil {
		t.Fatal(err)
	}
	if s := New(cfg, nil).Snapshot(ctx); s.Signature != SigVerified || len(s.Entries) != 2 {
		t.Fatalf("verified: %+v", s)
	}
}

func TestServiceOffIsInert(t *testing.T) {
	s := New(ConfigFromEnv(func(string) string { return "" }), nil)
	if s.Enabled() {
		t.Fatal("must be off")
	}
	if snap, m := s.Match(context.Background(), []Image{{App: "a", Ref: "node:1"}}); snap.Enabled || m != nil {
		t.Fatal("must do nothing")
	}
}

func TestRemoteFeedWithoutKeyFailsClosed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(testFeed)) }))
	defer srv.Close()
	s := New(Config{Source: srv.URL, MaxBytes: 1 << 20, Refresh: time.Hour}, nil).Snapshot(context.Background())
	if len(s.Entries) != 0 || s.Error == "" || s.Signature != SigInvalid {
		t.Fatalf("remote feed without a key must be rejected: %+v", s)
	}
}
