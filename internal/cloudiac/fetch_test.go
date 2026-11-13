package cloudiac

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

var fakeTofu = []byte("#!/bin/false fixture tofu binary\n")

func fixtureArchive(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range map[string][]byte{"LICENSE": []byte("license"), "tofu": fakeTofu} {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

type fixtureRelease struct {
	srv  *httptest.Server
	hits atomic.Int32
}

func serveRelease(t *testing.T, handler func(w http.ResponseWriter)) *fixtureRelease {
	t.Helper()
	fr := &fixtureRelease{}
	want := "/v" + TofuVersion + "/tofu_" + TofuVersion + "_linux_amd64.tar.gz"
	fr.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fr.hits.Add(1)
		if r.URL.Path != want {
			http.NotFound(w, r)
			return
		}
		handler(w)
	}))
	t.Cleanup(fr.srv.Close)
	return fr
}

func newTestFetcher(t *testing.T, fr *fixtureRelease, digest string, mut func(*FetchConfig)) *Fetcher {
	t.Helper()
	cfg := FetchConfig{
		DataDir:           t.TempDir(),
		MaxBytes:          1 << 20,
		MinFreeBytes:      1 << 20,
		Timeout:           5 * time.Second,
		FreeBytes:         func(string) (int64, error) { return 1 << 30, nil },
		HTTPClient:        fr.srv.Client(),
		BaseURL:           fr.srv.URL,
		AllowChecksumOnly: true,
		GOOS:              "linux",
		GOARCH:            "amd64",
	}
	if mut != nil {
		mut(&cfg)
	}
	f, err := NewFetcher(cfg)
	if err != nil {
		t.Fatal(err)
	}
	f.digests = map[string]map[string]string{TofuVersion: {"linux/amd64": digest}}
	return f
}

func sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func TestFetchGoodDigest(t *testing.T) {
	archive := fixtureArchive(t)
	fr := serveRelease(t, func(w http.ResponseWriter) { _, _ = w.Write(archive) })
	f := newTestFetcher(t, fr, sum(archive), nil)

	bin, err := f.Ensure(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(bin) //nolint:gosec // path from the fetcher under test
	if err != nil || !bytes.Equal(got, fakeTofu) {
		t.Fatalf("binary content mismatch: %v", err)
	}
	st, _ := os.Stat(bin)
	if st.Mode().Perm() != 0o500 {
		t.Fatalf("binary mode %v, want 0500", st.Mode().Perm())
	}
	for d := filepath.Dir(bin); d != f.cfg.DataDir; d = filepath.Dir(d) {
		ds, _ := os.Stat(d)
		if ds.Mode().Perm() != 0o700 {
			t.Fatalf("dir %s mode %v, want 0700", d, ds.Mode().Perm())
		}
	}
	entries, _ := os.ReadDir(filepath.Dir(bin))
	if len(entries) != 1 {
		t.Fatalf("temp files left behind: %v", entries)
	}
	if _, err := f.Ensure(context.Background()); err != nil || fr.hits.Load() != 1 {
		t.Fatalf("cache hit should not download again: hits=%d err=%v", fr.hits.Load(), err)
	}
}

func assertNoBinary(t *testing.T, f *Fetcher) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(f.binDir(), tofuBinaryName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("binary must not exist after a failed fetch: %v", err)
	}
}

func TestFetchTamperedDigestFailsClosed(t *testing.T) {
	archive := fixtureArchive(t)
	tampered := append([]byte{}, archive...)
	tampered[len(tampered)/2] ^= 0xff
	fr := serveRelease(t, func(w http.ResponseWriter) { _, _ = w.Write(tampered) })
	f := newTestFetcher(t, fr, sum(archive), nil)
	if _, err := f.Ensure(context.Background()); !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("want checksum mismatch, got %v", err)
	}
	assertNoBinary(t, f)
}

func TestFetchTruncatedBodyFailsClosed(t *testing.T) {
	archive := fixtureArchive(t)
	fr := serveRelease(t, func(w http.ResponseWriter) {
		w.Header().Set("Content-Length", strconv.Itoa(len(archive)))
		_, _ = w.Write(archive[:len(archive)/2])
	})
	f := newTestFetcher(t, fr, sum(archive), nil)
	if _, err := f.Ensure(context.Background()); err == nil {
		t.Fatal("truncated body must fail")
	}
	assertNoBinary(t, f)
}

func TestFetchPlaceholderDigestRefuses(t *testing.T) {
	archive := fixtureArchive(t)
	fr := serveRelease(t, func(w http.ResponseWriter) { _, _ = w.Write(archive) })
	f := newTestFetcher(t, fr, placeholderDigest, nil)
	if _, err := f.Ensure(context.Background()); !errors.Is(err, ErrPlaceholderDigest) {
		t.Fatalf("want placeholder refusal, got %v", err)
	}
	if fr.hits.Load() != 0 {
		t.Fatal("placeholder digest must refuse before any network call")
	}
}

func TestFetchRefusals(t *testing.T) {
	archive := fixtureArchive(t)
	cases := map[string]struct {
		mut  func(*FetchConfig)
		want error
	}{
		"no signature check": {func(c *FetchConfig) { c.AllowChecksumOnly = false }, ErrSignatureUnverified},
		"low disk":           {func(c *FetchConfig) { c.FreeBytes = func(string) (int64, error) { return 1024, nil } }, ErrLowDisk},
		"unpinned version":   {func(c *FetchConfig) { c.Version = "0.0.1" }, ErrUnpinnedVersion},
		"other platform":     {func(c *FetchConfig) { c.GOARCH = "riscv64" }, ErrUnsupportedPlatform},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			fr := serveRelease(t, func(w http.ResponseWriter) { _, _ = w.Write(archive) })
			f := newTestFetcher(t, fr, sum(archive), tc.mut)
			if _, err := f.Ensure(context.Background()); !errors.Is(err, tc.want) {
				t.Fatalf("want %v, got %v", tc.want, err)
			}
			if fr.hits.Load() != 0 {
				t.Fatal("refusal must happen before any network call")
			}
		})
	}
}

func TestFetchSizeCap(t *testing.T) {
	archive := fixtureArchive(t)
	fr := serveRelease(t, func(w http.ResponseWriter) { _, _ = w.Write(archive) })
	f := newTestFetcher(t, fr, sum(archive), func(c *FetchConfig) { c.MaxBytes = 16 })
	if _, err := f.Ensure(context.Background()); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("want size cap error, got %v", err)
	}
	assertNoBinary(t, f)
}

func TestNewFetcherRejectsBadConfig(t *testing.T) {
	ok := FetchConfig{DataDir: "/var/lib/x", MaxBytes: 1, Timeout: time.Second, FreeBytes: func(string) (int64, error) { return 0, nil }}
	for name, mut := range map[string]func(*FetchConfig){
		"relative dir": func(c *FetchConfig) { c.DataDir = "rel" },
		"no cap":       func(c *FetchConfig) { c.MaxBytes = 0 },
		"no timeout":   func(c *FetchConfig) { c.Timeout = 0 },
		"no probe":     func(c *FetchConfig) { c.FreeBytes = nil },
		"plain http":   func(c *FetchConfig) { c.BaseURL = "http://example.invalid" },
	} {
		cfg := ok
		mut(&cfg)
		if _, err := NewFetcher(cfg); !errors.Is(err, ErrFetchConfig) {
			t.Fatalf("%s: want config error, got %v", name, err)
		}
	}
}

func TestCompiledDigestsAreReal(t *testing.T) {
	pinned, ok := tofuDigests[TofuVersion]
	if !ok || len(pinned) == 0 {
		t.Fatal("pinned version has no digests")
	}
	for p, d := range pinned {
		if raw, err := hex.DecodeString(d); err != nil || len(raw) != sha256.Size {
			t.Fatalf("%s: digest is not a sha256: %q", p, d)
		}
	}
}
