package walship

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"filippo.io/age"

	"github.com/GLINCKER/levelrail/internal/objectstore"
	"github.com/GLINCKER/levelrail/internal/objectstore/objectstoretest"
)

func newObjectSink(t *testing.T) (*ObjectSink, *objectstoretest.Server) {
	t.Helper()
	srv := objectstoretest.New("wal")
	t.Cleanup(srv.Close)
	cl, err := objectstore.New(objectstore.Config{Endpoint: srv.URL, Bucket: "wal", AccessKeyID: "k", SecretAccessKey: "s", PathStyle: true, MaxAttempts: 1, HTTPClient: &http.Client{Timeout: 10 * time.Second}})
	if err != nil {
		t.Fatal(err)
	}
	id, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	return &ObjectSink{Bucket: cl, Prefix: "wal/inst-1/", Recipients: []age.Recipient{id.Recipient()}, Identities: []age.Identity{id}, Timeout: 10 * time.Second}, srv
}

func TestObjectSinkRoundTripIsCiphertext(t *testing.T) {
	s, srv := newObjectSink(t)
	if err := s.Put("gen-000001/a.seg.gz", strings.NewReader("secret-wal-bytes")); err != nil {
		t.Fatal(err)
	}
	raw, ok := srv.Data("wal/inst-1/gen-000001/a.seg.gz")
	if !ok || strings.Contains(string(raw), "secret-wal-bytes") {
		t.Fatalf("object missing or stored in plaintext (ok=%v)", ok)
	}
	got, err := Reader{Sink: s}.Fetch("gen-000001/a.seg.gz")
	if err != nil || string(got) != "secret-wal-bytes" {
		t.Fatalf("fetch = %q, %v", got, err)
	}
	other, _ := age.GenerateX25519Identity()
	s2 := *s
	s2.Identities = []age.Identity{other}
	if _, err := s2.Get("gen-000001/a.seg.gz"); err == nil {
		t.Fatal("wrong identity must not decrypt")
	}
	s3 := *s
	s3.Identities = nil
	if _, err := s3.Get("gen-000001/a.seg.gz"); !errors.Is(err, ErrNoIdentity) {
		t.Fatalf("want ErrNoIdentity, got %v", err)
	}
}

func TestObjectSinkListPrefixAndOrder(t *testing.T) {
	s, _ := newObjectSink(t)
	for _, n := range []string{"gen-000002/x", "gen-000001/b", "gen-000001/a", "other/z"} {
		if err := s.Put(n, strings.NewReader("x")); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.List("gen-")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "gen-000001/a,gen-000001/b,gen-000002/x" {
		t.Fatalf("list = %v", got)
	}
}

func TestShipThroughObjectSinkThenRestoreAndRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cp.db")
	db := openPrimary(t, path)
	defer func() { _ = db.Close() }()
	s, _ := newObjectSink(t)
	sh, err := New(path, s, 128<<10)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sh.Close() }()
	ctx := context.Background()
	if err := sh.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	r := newRand(7)
	for i := 0; i < 6; i++ {
		if _, err := churn(db, r, 8, 0, nil); err != nil {
			t.Fatal(err)
		}
		if err := sh.Sync(ctx); err != nil {
			t.Fatal(err)
		}
	}
	rd := Reader{Sink: s}
	gens, err := rd.Generations()
	if err != nil || len(gens) == 0 {
		t.Fatalf("generations = %v, %v", gens, err)
	}
	segs, err := rd.Segments(gens[len(gens)-1])
	if err != nil || len(segs) == 0 || segs[0].Offset != 0 {
		t.Fatalf("segments = %+v, %v", segs, err)
	}
	dest := filepath.Join(dir, "restored.db")
	res, err := Restore(ctx, s, dest)
	if err != nil || res.Truncated {
		t.Fatalf("restore = %+v, %v", res, err)
	}
	pd, pe, _ := counts(t, path)
	rdc, re, check := counts(t, dest)
	if check != "ok" || rdc != pd || re != pe {
		t.Fatalf("mismatch primary %d/%d replica %d/%d %s", pd, pe, rdc, re, check)
	}
}
