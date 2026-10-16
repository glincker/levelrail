package models

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type hubFake struct {
	data        []byte
	status      int
	ignoreRange bool
	etag        string
	cutAfter    int
	mu          sync.Mutex
	ranges      []string
	redirectTo  string
}

func (h *hubFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	h.ranges = append(h.ranges, r.Header.Get("Range"))
	h.mu.Unlock()
	if h.redirectTo != "" {
		http.Redirect(w, r, h.redirectTo, http.StatusFound)
		return
	}
	if h.status != 0 {
		w.WriteHeader(h.status)
		return
	}
	if h.etag != "" {
		w.Header().Set("X-Linked-Etag", `"`+h.etag+`"`)
	}
	data, start := h.data, 0
	if rg := r.Header.Get("Range"); rg != "" && !h.ignoreRange {
		_, _ = fmt.Sscanf(rg, "bytes=%d-", &start)
		if start >= len(data) {
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, len(data)-1, len(data)))
		w.Header().Set("Content-Length", strconv.Itoa(len(data)-start))
		w.WriteHeader(http.StatusPartialContent)
	} else {
		start = 0
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	}
	body := data[start:]
	if h.cutAfter > 0 && start == 0 && h.cutAfter < len(body) {
		_, _ = w.Write(body[:h.cutAfter])
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		if hj, ok := w.(http.Hijacker); ok {
			conn, _, _ := hj.Hijack()
			_ = conn.Close()
		}
		return
	}
	_, _ = w.Write(body)
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path) //nolint:gosec // test temp path
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func sum(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

func payload(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(i * 7)
	}
	return b
}

func newTestDownloader(t *testing.T, srv *httptest.Server, mut func(*DownloadConfig)) (*Downloader, *progressLog) {
	t.Helper()
	pl := &progressLog{}
	cfg := DownloadConfig{Root: t.TempDir(), BaseURL: srv.URL, MaxBytes: 1 << 20, RetryAfter: 150 * time.Millisecond, BlockedRetryAfter: time.Hour,
		HostAllowed: func(h string) bool { return h == "127.0.0.1" }, Client: srv.Client(),
		DiskFree: func(string) (int64, error) { return 1 << 30, nil }}
	if mut != nil {
		mut(&cfg)
	}
	return NewDownloader(cfg, nil, pl.add), pl
}

type progressLog struct {
	mu   sync.Mutex
	last [2]int64
}

func (p *progressLog) add(_ string, done, total int64) {
	p.mu.Lock()
	p.last = [2]int64{done, total}
	p.mu.Unlock()
}

func waitPhase(t *testing.T, d *Downloader, spec WeightsSpec, want string) DownloadState {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var st DownloadState
	for time.Now().Before(deadline) {
		st = d.Ensure(spec)
		if st.Phase == want {
			return st
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("phase never reached %q, last %+v err=%v", want, st, st.Err)
	return st
}

func spec1() WeightsSpec {
	return WeightsSpec{Name: "m", Repo: "org/repo-GGUF", File: "w.gguf", Key: "k"}
}

func TestDownloader_HappyPathVerifiesChecksum(t *testing.T) {
	data := payload(300_000)
	h := &hubFake{data: data}
	srv := httptest.NewServer(h)
	defer srv.Close()
	d, pl := newTestDownloader(t, srv, nil)
	s := spec1()
	s.SHA256 = sum(data)
	st := waitPhase(t, d, s, DLDone)
	if st.Total != int64(len(data)) {
		t.Errorf("total = %d", st.Total)
	}
	got := mustRead(t, filepath.Join(d.Dir("m"), "w.gguf"))
	if sum(got) != s.SHA256 {
		t.Fatal("file content wrong")
	}
	if _, err := os.Stat(filepath.Join(d.Dir("m"), "w.gguf.part")); !os.IsNotExist(err) {
		t.Error("partial file must be gone")
	}
	if pl.last[0] != int64(len(data)) {
		t.Errorf("final progress = %v", pl.last)
	}
}

func TestDownloader_UsesHubEtagWhenNoDigestGiven(t *testing.T) {
	data := payload(10_000)
	h := &hubFake{data: data, etag: strings.Repeat("0", 64)}
	srv := httptest.NewServer(h)
	defer srv.Close()
	d, _ := newTestDownloader(t, srv, nil)
	st := waitPhase(t, d, spec1(), DLFailed)
	if st.Err == nil || st.Err.Code != DLChecksum || st.Err.Retryable {
		t.Fatalf("err = %+v", st.Err)
	}
}

func TestDownloader_ChecksumMismatchDiscardsFile(t *testing.T) {
	data := payload(50_000)
	srv := httptest.NewServer(&hubFake{data: data})
	defer srv.Close()
	d, _ := newTestDownloader(t, srv, nil)
	s := spec1()
	s.SHA256 = strings.Repeat("a", 64)
	st := waitPhase(t, d, s, DLFailed)
	if st.Err.Code != DLChecksum {
		t.Fatalf("code = %s", st.Err.Code)
	}
	for _, f := range []string{"w.gguf", "w.gguf.part"} {
		if _, err := os.Stat(filepath.Join(d.Dir("m"), f)); !os.IsNotExist(err) {
			t.Errorf("%s must not remain after a mismatch", f)
		}
	}
}

func TestDownloader_ResumesWithRange(t *testing.T) {
	data := payload(200_000)
	h := &hubFake{data: data}
	srv := httptest.NewServer(h)
	defer srv.Close()
	d, _ := newTestDownloader(t, srv, nil)
	dir := d.Dir("m")
	_ = os.MkdirAll(dir, 0o750)
	if err := os.WriteFile(filepath.Join(dir, "w.gguf.part"), data[:80_000], 0o600); err != nil {
		t.Fatal(err)
	}
	s := spec1()
	s.SHA256 = sum(data)
	waitPhase(t, d, s, DLDone)
	if len(h.ranges) == 0 || h.ranges[0] != "bytes=80000-" {
		t.Fatalf("ranges = %v, want a resume from 80000", h.ranges)
	}
	got := mustRead(t, filepath.Join(dir, "w.gguf"))
	if sum(got) != s.SHA256 {
		t.Fatal("resumed file differs")
	}
}

func TestDownloader_ServerIgnoresRangeRestartsFromZero(t *testing.T) {
	data := payload(120_000)
	h := &hubFake{data: data, ignoreRange: true}
	srv := httptest.NewServer(h)
	defer srv.Close()
	d, _ := newTestDownloader(t, srv, nil)
	dir := d.Dir("m")
	_ = os.MkdirAll(dir, 0o750)
	_ = os.WriteFile(filepath.Join(dir, "w.gguf.part"), data[:50_000], 0o600)
	s := spec1()
	s.SHA256 = sum(data)
	waitPhase(t, d, s, DLDone)
	got := mustRead(t, filepath.Join(dir, "w.gguf"))
	if len(got) != len(data) || sum(got) != s.SHA256 {
		t.Fatalf("file has %d bytes, want %d and matching content", len(got), len(data))
	}
}

func TestDownloader_InterruptedKeepsPartialAndResumes(t *testing.T) {
	data := payload(400_000)
	h := &hubFake{data: data, cutAfter: 100_000}
	srv := httptest.NewServer(h)
	defer srv.Close()
	d, _ := newTestDownloader(t, srv, nil)
	s := spec1()
	s.SHA256 = sum(data)
	st := waitPhase(t, d, s, DLFailed)
	if st.Err.Code != DLInterrupted || !st.Err.Retryable {
		t.Fatalf("err = %+v", st.Err)
	}
	part, err := os.Stat(filepath.Join(d.Dir("m"), "w.gguf.part"))
	if err != nil || part.Size() == 0 {
		t.Fatalf("partial missing: %v", err)
	}
	h.mu.Lock()
	h.cutAfter = 0
	h.mu.Unlock()
	waitPhase(t, d, s, DLDone)
}

func TestDownloader_Failures(t *testing.T) {
	big := payload(5000)
	tests := []struct {
		name  string
		hub   *hubFake
		mut   func(*DownloadConfig)
		token string
		code  string
		retry bool
	}{
		{"gated without token", &hubFake{status: 401}, nil, "", DLGated, false},
		{"gated with token lacking access", &hubFake{status: 403}, nil, "tok", DLGated, false},
		{"not found", &hubFake{status: 404}, nil, "", DLNotFound, false},
		{"hub unavailable", &hubFake{status: 503}, nil, "", DLUnavailable, true},
		{"size cap", &hubFake{data: big}, func(c *DownloadConfig) { c.MaxBytes = 1000 }, "", DLTooLarge, false},
		{"disk full before start", &hubFake{data: big}, func(c *DownloadConfig) { c.DiskFree = func(string) (int64, error) { return 100, nil } }, "", DLDiskFull, true},
		{"redirect to a disallowed host", &hubFake{redirectTo: "http://evil.example.com/x.gguf"}, nil, "", DLHostNotAllowed, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(tt.hub)
			defer srv.Close()
			d, _ := newTestDownloader(t, srv, tt.mut)
			s := spec1()
			s.Token = tt.token
			st := waitPhase(t, d, s, DLFailed)
			if st.Err.Code != tt.code || st.Err.Retryable != tt.retry {
				t.Fatalf("err = %+v, want %s retryable=%v", st.Err, tt.code, tt.retry)
			}
			if st.Err.Message == "" {
				t.Error("failure needs a message")
			}
		})
	}
}

func TestDownloader_HostPolicy(t *testing.T) {
	d := NewDownloader(DownloadConfig{Root: t.TempDir(), BaseURL: "https://huggingface.co"}, nil, nil)
	tests := map[string]bool{
		"huggingface.co": true, "cdn-lfs.huggingface.co": true, "us.aws.cdn.hf.co": true, "hf.co": true, "cas-bridge.xethub.hf.co": true,
		"evil.com": false, "huggingface.co.evil.com": false, "nothf.co": false, "127.0.0.1": false, "169.254.169.254": false,
	}
	for host, want := range tests {
		if got := d.hostAllowed(host); got != want {
			t.Errorf("hostAllowed(%q) = %v, want %v", host, got, want)
		}
	}
}

func TestDownloader_RejectsBadNames(t *testing.T) {
	d := NewDownloader(DownloadConfig{Root: t.TempDir(), BaseURL: "https://huggingface.co"}, nil, nil)
	for _, s := range []WeightsSpec{
		{Name: "m", Repo: "org/repo", File: "../etc/passwd.gguf"},
		{Name: "m", Repo: "org/repo", File: "a/b.gguf"},
		{Name: "m", Repo: "not a repo", File: "a.gguf"},
		{Name: "m", Repo: "org/repo", File: "a.bin"},
	} {
		if st := d.Ensure(s); st.Phase != DLFailed || st.Err.Code != DLInvalid {
			t.Errorf("%+v accepted: %+v", s, st)
		}
	}
}

func TestDownloader_ExistingFileIsDone(t *testing.T) {
	d := NewDownloader(DownloadConfig{Root: t.TempDir(), BaseURL: "https://huggingface.co"}, nil, nil)
	_ = os.MkdirAll(d.Dir("m"), 0o750)
	_ = os.WriteFile(filepath.Join(d.Dir("m"), "w.gguf"), []byte("xx"), 0o600)
	if st := d.Ensure(spec1()); st.Phase != DLDone || st.Total != 2 {
		t.Fatalf("state = %+v", st)
	}
}
