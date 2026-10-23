package cliupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/selfupgrade"
)

const testAsset = "tool-cli-linux-amd64"

func fakeBinary(version string) string {
	return "#!/bin/sh\necho '{\"cli\":{\"version\":\"" + version + "\"}}'\n"
}

type fakeRelease struct {
	binary   string
	checksum string
	bundle   bool
}

type fakeServer struct {
	*httptest.Server
	hits     atomic.Int64
	releases string
	byTag    map[string]fakeRelease
}

func newFakeServer(t *testing.T, listing string, byTag map[string]fakeRelease) *fakeServer {
	t.Helper()
	f := &fakeServer{releases: listing, byTag: byTag}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.hits.Add(1)
		if strings.HasPrefix(r.URL.Path, "/repos/o/r/releases") {
			_, _ = w.Write([]byte(f.releases))
			return
		}
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) != 3 || parts[0] != "dl" {
			http.NotFound(w, r)
			return
		}
		rel, ok := f.byTag[parts[1]]
		if !ok {
			http.NotFound(w, r)
			return
		}
		switch parts[2] {
		case testAsset:
			_, _ = w.Write([]byte(rel.binary))
		case "checksums.txt":
			sum := rel.checksum
			if sum == "" {
				h := sha256.Sum256([]byte(rel.binary))
				sum = hex.EncodeToString(h[:])
			}
			_, _ = fmt.Fprintf(w, "%s  %s\n", sum, testAsset)
		case "checksums.txt.sigstore.json":
			if !rel.bundle {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write([]byte("{}"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.Close)
	return f
}

const listing = `[
 {"tag_name":"v1.3.0-beta.1","prerelease":true,"assets":[{"name":"tool-cli-linux-amd64"}]},
 {"tag_name":"v1.2.0","prerelease":false,"assets":[{"name":"tool-cli-linux-amd64"}]},
 {"tag_name":"v1.4.0","prerelease":false,"assets":[{"name":"tool-cli-darwin-arm64"}]},
 {"tag_name":"v1.1.0","prerelease":false,"assets":[{"name":"tool-cli-linux-amd64"}]},
 {"tag_name":"kit/v9.0.0","prerelease":false,"assets":[{"name":"tool-cli-linux-amd64"}]},
 {"tag_name":"v2.0.0","prerelease":false,"draft":true,"assets":[{"name":"tool-cli-linux-amd64"}]}
]`

func (f *fakeServer) source() Source {
	return Source{Repo: "o/r", APIBase: f.URL, DownloadBase: f.URL + "/dl", Asset: testAsset, AllowHTTP: true}
}

func okVerify(context.Context, string, string) (string, error) { return "test signature verified", nil }

func setup(t *testing.T) (exe string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixtures")
	}
	exe = filepath.Join(t.TempDir(), "tool-cli")
	if err := os.WriteFile(exe, []byte(fakeBinary("v1.0.0")), 0o755); err != nil { //nolint:gosec // test fixture must be executable
		t.Fatal(err)
	}
	return exe
}

func content(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p) //nolint:gosec // test temp path
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		cur, latest string
		want        bool
	}{
		{"v1.0.0", "v1.0.1", true}, {"v1.0.0", "v1.0.0", false}, {"v1.2.0", "v1.1.0", false},
		{"v1.0.0-beta.9", "v1.0.0-beta.15", true}, {"dev", "v9.0.0", false}, {"v1.0.0", "", false},
	} {
		if got := Newer(c.cur, c.latest); got != c.want {
			t.Errorf("Newer(%q,%q)=%v want %v", c.cur, c.latest, got, c.want)
		}
	}
}

func TestLatestPicksHighestStableWithAsset(t *testing.T) {
	f := newFakeServer(t, listing, nil)
	got, err := f.source().Latest(context.Background())
	if err != nil || got != "v1.2.0" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestLatestFallsBackToPrerelease(t *testing.T) {
	f := newFakeServer(t, `[{"tag_name":"v1.0.0-beta.2","prerelease":true,"assets":[{"name":"tool-cli-linux-amd64"}]}]`, nil)
	got, err := f.source().Latest(context.Background())
	if err != nil || got != "v1.0.0-beta.2" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestLatestRefusesPlainHTTPByDefault(t *testing.T) {
	f := newFakeServer(t, listing, nil)
	s := f.source()
	s.AllowHTTP = false
	if _, err := s.Latest(context.Background()); err == nil {
		t.Fatal("plain http base accepted")
	}
}

func TestUpdateSuccess(t *testing.T) {
	exe := setup(t)
	f := newFakeServer(t, listing, map[string]fakeRelease{"v1.2.0": {binary: fakeBinary("v1.2.0"), bundle: true}})
	res, err := Update(context.Background(), Options{Source: f.source(), Current: "v1.0.0", ExePath: exe, Verify: okVerify})
	if err != nil || res.Action != ActionUpdated || res.Target != "v1.2.0" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if got := content(t, exe); got != fakeBinary("v1.2.0") {
		t.Fatalf("binary not replaced: %q", got)
	}
	if st, _ := os.Stat(exe); st.Mode().Perm()&0o111 == 0 {
		t.Fatal("replaced binary lost its executable bit")
	}
	entries, _ := os.ReadDir(filepath.Dir(exe))
	if len(entries) != 1 {
		t.Fatalf("staging file left behind: %v", entries)
	}
}

func TestUpdateUpToDateMakesNoDownload(t *testing.T) {
	exe := setup(t)
	f := newFakeServer(t, listing, nil)
	res, err := Update(context.Background(), Options{Source: f.source(), Current: "v1.2.0", ExePath: exe, Verify: okVerify})
	if err != nil || res.Action != ActionUpToDate {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if content(t, exe) != fakeBinary("v1.0.0") {
		t.Fatal("binary changed")
	}
}

func TestUpdateFailuresLeaveBinaryUntouched(t *testing.T) {
	good := fakeBinary("v1.2.0")
	cases := map[string]struct {
		rel    fakeRelease
		verify func(context.Context, string, string) (string, error)
		want   error
	}{
		"checksum mismatch": {fakeRelease{binary: good, checksum: strings.Repeat("0", 64), bundle: true}, okVerify, selfupgrade.ErrChecksumMismatch},
		"signature invalid": {fakeRelease{binary: good, bundle: true}, func(context.Context, string, string) (string, error) {
			return "", selfupgrade.ErrSignatureInvalid
		}, selfupgrade.ErrSignatureInvalid},
		"no signature bundle, default verifier": {fakeRelease{binary: good, bundle: false}, nil, selfupgrade.ErrSignatureRequired},
		"reports other version":                 {fakeRelease{binary: fakeBinary("v9.9.9"), bundle: true}, okVerify, selfupgrade.ErrVersionMismatch},
		"does not run":                          {fakeRelease{binary: "not an executable", bundle: true}, okVerify, nil},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			exe := setup(t)
			f := newFakeServer(t, listing, map[string]fakeRelease{"v1.2.0": c.rel})
			res, err := Update(context.Background(), Options{Source: f.source(), Current: "v1.0.0", ExePath: exe, Verify: c.verify})
			if err == nil || res.Action == ActionUpdated {
				t.Fatalf("expected failure, res=%+v err=%v", res, err)
			}
			if c.want != nil && !errors.Is(err, c.want) {
				t.Fatalf("err=%v want %v", err, c.want)
			}
			if content(t, exe) != fakeBinary("v1.0.0") {
				t.Fatal("old binary was modified")
			}
			if entries, _ := os.ReadDir(filepath.Dir(exe)); len(entries) != 1 {
				t.Fatalf("staging file left behind: %v", entries)
			}
		})
	}
}

func TestUpdateNotWritable(t *testing.T) {
	exe := setup(t)
	dir := filepath.Dir(exe)
	if err := os.Chmod(dir, 0o500); err != nil { //nolint:gosec // read-only dir for the test
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) }) //nolint:gosec // restore for cleanup
	if f, err := os.CreateTemp(dir, "probe"); err == nil {
		_ = f.Close()
		t.Skip("directory is writable despite chmod (running as root)")
	}
	srv := newFakeServer(t, listing, map[string]fakeRelease{"v1.2.0": {binary: fakeBinary("v1.2.0"), bundle: true}})
	_, err := Update(context.Background(), Options{Source: srv.source(), Current: "v1.0.0", ExePath: exe, Verify: okVerify})
	if !errors.Is(err, ErrNotWritable) || !strings.Contains(err.Error(), "sudo") {
		t.Fatalf("err=%v", err)
	}
}

func TestUpdateDryRunChangesNothing(t *testing.T) {
	exe := setup(t)
	f := newFakeServer(t, listing, map[string]fakeRelease{"v1.2.0": {binary: fakeBinary("v1.2.0"), bundle: true}})
	res, err := Update(context.Background(), Options{Source: f.source(), Current: "v1.0.0", ExePath: exe, DryRun: true, Verify: okVerify})
	if err != nil || res.Action != ActionWouldApply || res.Target != "v1.2.0" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if content(t, exe) != fakeBinary("v1.0.0") {
		t.Fatal("dry run changed the binary")
	}
	if entries, _ := os.ReadDir(filepath.Dir(exe)); len(entries) != 1 {
		t.Fatalf("dry run left files: %v", entries)
	}
}

func TestUpdatePinnedVersionAllowsDowngrade(t *testing.T) {
	exe := setup(t)
	if err := os.WriteFile(exe, []byte(fakeBinary("v1.2.0")), 0o755); err != nil { //nolint:gosec // test fixture must be executable
		t.Fatal(err)
	}
	f := newFakeServer(t, listing, map[string]fakeRelease{"v1.1.0": {binary: fakeBinary("v1.1.0"), bundle: true}})
	res, err := Update(context.Background(), Options{Source: f.source(), Current: "v1.2.0", Target: "v1.1.0", ExePath: exe, Verify: okVerify})
	if err != nil || res.Action != ActionUpdated {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	if content(t, exe) != fakeBinary("v1.1.0") {
		t.Fatal("not downgraded")
	}
}

func TestUpdateRefusesUnsafeTagAndConfirmAbort(t *testing.T) {
	exe := setup(t)
	f := newFakeServer(t, listing, nil)
	if _, err := Update(context.Background(), Options{Source: f.source(), Current: "v1.0.0", Target: "../v1", ExePath: exe}); err == nil {
		t.Fatal("unsafe tag accepted")
	}
	before := f.hits.Load()
	_, err := Update(context.Background(), Options{
		Source: f.source(), Current: "v1.0.0", ExePath: exe,
		Confirm: func(string, string) error { return errors.New("cancelled") },
	})
	if err == nil || f.hits.Load() != before+1 {
		t.Fatalf("expected abort after only the release lookup, err=%v hits=%d->%d", err, before, f.hits.Load())
	}
}

func TestHinterCachesAndNotifiesOnce(t *testing.T) {
	f := newFakeServer(t, listing, nil)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	h := Hinter{Source: f.source(), Current: "v1.0.0", Dir: t.TempDir(), IntervalDays: 7, Now: func() time.Time { return now }}
	if got := h.Check(context.Background()); !strings.Contains(got, "v1.2.0") {
		t.Fatalf("hint=%q", got)
	}
	hits := f.hits.Load()
	now = now.Add(24 * time.Hour)
	if got := h.Check(context.Background()); got != "" || f.hits.Load() != hits {
		t.Fatalf("second check within interval: hint=%q, new requests=%d", got, f.hits.Load()-hits)
	}
	now = now.Add(7 * 24 * time.Hour)
	if got := h.Check(context.Background()); got == "" {
		t.Fatal("no hint after the interval")
	}
}

func TestHinterDisabledAndDevMakeNoRequests(t *testing.T) {
	f := newFakeServer(t, listing, nil)
	for _, h := range []Hinter{
		{Source: f.source(), Current: "v1.0.0", Dir: t.TempDir(), IntervalDays: 0},
		{Source: f.source(), Current: "dev", Dir: t.TempDir(), IntervalDays: 7},
	} {
		if got := h.Check(context.Background()); got != "" {
			t.Fatalf("hint=%q", got)
		}
	}
	if f.hits.Load() != 0 {
		t.Fatalf("made %d requests", f.hits.Load())
	}
}

func TestHinterBacksOffAfterFailure(t *testing.T) {
	f := newFakeServer(t, `not json`, nil)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	h := Hinter{Source: f.source(), Current: "v1.0.0", Dir: t.TempDir(), IntervalDays: 7, Now: func() time.Time { return now }}
	_ = h.Check(context.Background())
	hits := f.hits.Load()
	now = now.Add(5 * time.Minute)
	_ = h.Check(context.Background())
	if f.hits.Load() != hits {
		t.Fatal("retried within the backoff window")
	}
}
