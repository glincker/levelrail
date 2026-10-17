package models

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/GLINCKER/levelrail/kit/diskspace"
	"github.com/GLINCKER/levelrail/kit/netguard"
)

// Download failure codes, shown in the resource status.
const (
	DLGated           = "gated"
	DLNotFound        = "not_found"
	DLDiskFull        = "disk_full"
	DLInterrupted     = "interrupted"
	DLChecksum        = "checksum_mismatch"
	DLHostNotAllowed  = "host_not_allowed"
	DLTooLarge        = "too_large"
	DLUnavailable     = "unavailable"
	DLInvalid         = "invalid"
	downloadChunk     = 256 << 10
	downloadFlushStep = 8 << 20
)

// Download phases.
const (
	DLPending     = "pending"
	DLDownloading = "downloading"
	DLVerifying   = "verifying"
	DLDone        = "done"
	DLFailed      = "failed"
)

var (
	dlFileRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,200}\.gguf$`)
	hexRe    = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// DownloadError is a classified download failure.
type DownloadError struct {
	Code      string
	Message   string
	Retryable bool
}

func (e *DownloadError) Error() string { return e.Message }

// DownloadState is the observable state of one weights download.
type DownloadState struct {
	Phase string
	Done  int64
	Total int64
	Err   *DownloadError
}

// WeightsSpec names one file to fetch into a model's weights directory.
type WeightsSpec struct {
	Name string
	Repo string
	File string
	// SHA256 is the expected digest; empty falls back to the digest the
	// Hub reports on the response (X-Linked-Etag), and to none for a file
	// the Hub gives no digest for.
	SHA256 string
	Token  string
	// Key changes when the download should be retried from scratch, for
	// example after a new token or a restart.
	Key string
}

// DownloadConfig tunes the downloader. Zero fields take the defaults.
type DownloadConfig struct {
	Root     string
	BaseURL  string
	MaxBytes int64
	// HeadroomPercent is free disk required beyond the remaining bytes.
	HeadroomPercent int
	RetryAfter      time.Duration
	// BlockedRetryAfter spaces retries of failures a retry will not fix soon.
	BlockedRetryAfter time.Duration
	// HostAllowed overrides the Hugging Face host policy (tests).
	HostAllowed func(host string) bool
	Client      *http.Client
	DiskFree    func(path string) (int64, error)
	Now         func() time.Time
}

// LoadDownloadConfig reads APP_MODEL_WEIGHTS_DIR, APP_MODEL_DOWNLOAD_MAX_BYTES,
// APP_MODEL_DOWNLOAD_RETRY_SECONDS and reuses APP_HF_BASE_URL and APP_DATA_DIR.
func LoadDownloadConfig() DownloadConfig {
	data := os.Getenv("APP_DATA_DIR")
	if data == "" {
		data = "/var/lib/levelrail-data"
	}
	root := os.Getenv("APP_MODEL_WEIGHTS_DIR")
	if root == "" {
		root = filepath.Join(data, "model-weights")
	}
	return DownloadConfig{
		Root:              root,
		BaseURL:           LoadHFConfig().BaseURL,
		MaxBytes:          envInt64("APP_MODEL_DOWNLOAD_MAX_BYTES", 16<<30),
		HeadroomPercent:   envInt("APP_MODEL_DISK_HEADROOM_PERCENT", 10),
		RetryAfter:        time.Duration(envInt64("APP_MODEL_DOWNLOAD_RETRY_SECONDS", 15)) * time.Second,
		BlockedRetryAfter: 5 * time.Minute,
	}
}

// Downloader fetches GGUF weights from Hugging Face into per-model
// directories, in the background, with resume and checksum verification.
type Downloader struct {
	cfg      DownloadConfig
	client   *http.Client
	logger   *slog.Logger
	progress func(name string, done, total int64)

	mu   sync.Mutex
	jobs map[string]*dlJob
}

type dlJob struct {
	key      string
	state    DownloadState
	cancel   context.CancelFunc
	finished time.Time
}

// NewDownloader builds a Downloader. progress may be nil; it is called
// with throttled byte counts.
func NewDownloader(cfg DownloadConfig, logger *slog.Logger, progress func(name string, done, total int64)) *Downloader {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.DiskFree == nil {
		cfg.DiskFree = diskspace.Free
	}
	if cfg.RetryAfter <= 0 {
		cfg.RetryAfter = 15 * time.Second
	}
	if cfg.BlockedRetryAfter <= 0 {
		cfg.BlockedRetryAfter = 5 * time.Minute
	}
	d := &Downloader{cfg: cfg, logger: logger, progress: progress, jobs: map[string]*dlJob{}}
	d.client = cfg.Client
	if d.client == nil {
		d.client = netguard.NewClient()
	}
	c := *d.client
	c.CheckRedirect = d.checkRedirect
	d.client = &c
	return d
}

// Dir is the host directory holding name's weights.
func (d *Downloader) Dir(name string) string { return filepath.Join(d.cfg.Root, name) }

func (d *Downloader) hostAllowed(host string) bool {
	if d.cfg.HostAllowed != nil {
		return d.cfg.HostAllowed(host)
	}
	host = strings.ToLower(host)
	if u, err := url.Parse(d.cfg.BaseURL); err == nil && strings.EqualFold(u.Hostname(), host) && u.Scheme == "https" {
		return true
	}
	for _, base := range []string{"huggingface.co", "hf.co"} {
		if host == base || strings.HasSuffix(host, "."+base) {
			return true
		}
	}
	return false
}

func (d *Downloader) checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 6 {
		return &DownloadError{Code: DLUnavailable, Message: "too many redirects from Hugging Face"}
	}
	if d.cfg.HostAllowed == nil && req.URL.Scheme != "https" {
		return &DownloadError{Code: DLHostNotAllowed, Message: "redirect to a non-HTTPS address was refused"}
	}
	if !d.hostAllowed(req.URL.Hostname()) {
		return &DownloadError{Code: DLHostNotAllowed, Message: fmt.Sprintf("redirect to %q was refused: only Hugging Face hosts are allowed", req.URL.Hostname())}
	}
	return nil
}

// Ensure returns the download state for spec, starting or restarting the
// background job when needed. It never blocks on the network.
func (d *Downloader) Ensure(spec WeightsSpec) DownloadState {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !hfRepoRe.MatchString(spec.Repo) || !dlFileRe.MatchString(spec.File) {
		return DownloadState{Phase: DLFailed, Err: &DownloadError{Code: DLInvalid, Message: "the weights repository or file name is not valid"}}
	}
	final := filepath.Join(d.Dir(spec.Name), spec.File)
	if st, err := os.Stat(final); err == nil && st.Mode().IsRegular() && st.Size() > 0 {
		if j, ok := d.jobs[spec.Name]; ok && j.state.Phase == DLDownloading {
			return j.state
		}
		return DownloadState{Phase: DLDone, Done: st.Size(), Total: st.Size()}
	}
	j := d.jobs[spec.Name]
	if j != nil {
		switch {
		case j.state.Phase == DLDownloading || j.state.Phase == DLVerifying || j.state.Phase == DLPending:
			return j.state
		case j.key == spec.Key && j.state.Err != nil:
			wait := d.cfg.RetryAfter
			if !j.state.Err.Retryable {
				wait = d.cfg.BlockedRetryAfter
			}
			if d.cfg.Now().Sub(j.finished) < wait {
				return j.state
			}
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	nj := &dlJob{key: spec.Key, state: DownloadState{Phase: DLPending}, cancel: cancel}
	d.jobs[spec.Name] = nj
	go d.run(ctx, spec, nj)
	return nj.state
}

// Forget cancels name's job and drops its state. Files are kept.
func (d *Downloader) Forget(name string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if j, ok := d.jobs[name]; ok {
		j.cancel()
		delete(d.jobs, name)
	}
}

func (d *Downloader) set(j *dlJob, st DownloadState) {
	d.mu.Lock()
	defer d.mu.Unlock()
	j.state = st
	if st.Phase == DLFailed || st.Phase == DLDone {
		j.finished = d.cfg.Now()
	}
}

func (d *Downloader) fail(j *dlJob, name string, done, total int64, e *DownloadError) {
	d.logger.Warn("model weights download failed", slog.String("model", name), slog.String("code", e.Code), slog.String("error", e.Message))
	d.set(j, DownloadState{Phase: DLFailed, Done: done, Total: total, Err: e})
}

func (d *Downloader) run(ctx context.Context, spec WeightsSpec, j *dlJob) {
	dir := d.Dir(spec.Name)
	if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // public weights must be readable by the engine container's user
		d.fail(j, spec.Name, 0, 0, classifyWriteErr(err))
		return
	}
	part := filepath.Join(dir, spec.File+".part")
	final := filepath.Join(dir, spec.File)
	done, total, etag, derr := d.fetch(ctx, spec, j, part)
	if derr != nil {
		d.fail(j, spec.Name, done, total, derr)
		return
	}
	want := strings.ToLower(spec.SHA256)
	if want == "" && hexRe.MatchString(etag) {
		want = etag
	}
	if want != "" {
		d.set(j, DownloadState{Phase: DLVerifying, Done: done, Total: total})
		got, err := fileSHA256(ctx, part)
		if err != nil {
			d.fail(j, spec.Name, done, total, &DownloadError{Code: DLInterrupted, Message: "could not verify the download: " + err.Error(), Retryable: true})
			return
		}
		if got != want {
			_ = os.Remove(part)
			d.fail(j, spec.Name, 0, total, &DownloadError{Code: DLChecksum, Message: "the downloaded file does not match its sha256; it was discarded"})
			return
		}
	}
	if err := os.Rename(part, final); err != nil {
		d.fail(j, spec.Name, done, total, classifyWriteErr(err))
		return
	}
	d.report(spec.Name, total, total)
	d.logger.Info("model weights downloaded", slog.String("model", spec.Name), slog.String("file", spec.File), slog.Int64("bytes", total))
	d.set(j, DownloadState{Phase: DLDone, Done: total, Total: total})
}

func (d *Downloader) report(name string, done, total int64) {
	if d.progress != nil {
		d.progress(name, done, total)
	}
}

func classifyWriteErr(err error) *DownloadError {
	if errors.Is(err, syscall.ENOSPC) {
		return &DownloadError{Code: DLDiskFull, Message: "the disk is full; free space and retry (the partial download is kept)", Retryable: true}
	}
	return &DownloadError{Code: DLUnavailable, Message: "cannot write the weights file: " + err.Error(), Retryable: true}
}

func (d *Downloader) fetch(ctx context.Context, spec WeightsSpec, j *dlJob, part string) (done, total int64, etag string, derr *DownloadError) {
	var offset int64
	if st, err := os.Stat(part); err == nil {
		offset = st.Size()
	}
	u := strings.TrimRight(d.cfg.BaseURL, "/") + "/" + spec.Repo + "/resolve/main/" + url.PathEscape(spec.File)
	pu, err := url.Parse(u)
	if err != nil || !d.hostAllowed(pu.Hostname()) {
		return 0, 0, "", &DownloadError{Code: DLHostNotAllowed, Message: "the Hugging Face base URL host is not allowed"}
	}
	resp, offset, derr := d.open(ctx, u, spec.Token, offset)
	if derr != nil {
		return offset, 0, "", derr
	}
	defer func() { _ = resp.Body.Close() }()
	etag = strings.ToLower(strings.Trim(resp.Header.Get("X-Linked-Etag"), `"`))
	total = responseTotal(resp, offset)
	if total > 0 && total > d.cfg.MaxBytes {
		return offset, total, etag, &DownloadError{Code: DLTooLarge, Message: fmt.Sprintf("the file is %s, above the %s limit (APP_MODEL_DOWNLOAD_MAX_BYTES)", formatGiB(total), formatGiB(d.cfg.MaxBytes))}
	}
	if total > 0 {
		need := total - offset
		need += need * int64(d.cfg.HeadroomPercent) / 100
		if free, err := d.cfg.DiskFree(filepath.Dir(part)); err == nil && free < need {
			return offset, total, etag, &DownloadError{Code: DLDiskFull, Message: fmt.Sprintf("the download needs about %s more but only %s is free", formatGiB(need), formatGiB(free)), Retryable: true}
		}
	}
	flags := os.O_WRONLY | os.O_CREATE
	if offset == 0 {
		flags |= os.O_TRUNC
	} else {
		flags |= os.O_APPEND
	}
	f, err := os.OpenFile(part, flags, 0o644) //nolint:gosec // validated path; the engine container's user must read public weights
	if err != nil {
		return offset, total, etag, classifyWriteErr(err)
	}
	defer func() { _ = f.Close() }()
	done = offset
	d.set(j, DownloadState{Phase: DLDownloading, Done: done, Total: total})
	buf := make([]byte, downloadChunk)
	var sinceFlush int64
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				return done, total, etag, classifyWriteErr(werr)
			}
			done += int64(n)
			sinceFlush += int64(n)
			if done > d.cfg.MaxBytes {
				return done, total, etag, &DownloadError{Code: DLTooLarge, Message: "the download grew past the size limit and was stopped"}
			}
			if sinceFlush >= downloadFlushStep {
				sinceFlush = 0
				d.set(j, DownloadState{Phase: DLDownloading, Done: done, Total: total})
				d.report(spec.Name, done, total)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return done, total, etag, &DownloadError{Code: DLInterrupted, Message: "the download was interrupted and will resume: " + rerr.Error(), Retryable: true}
		}
	}
	if total > 0 && done != total {
		return done, total, etag, &DownloadError{Code: DLInterrupted, Message: fmt.Sprintf("the download ended early at %s of %s and will resume", formatGiB(done), formatGiB(total)), Retryable: true}
	}
	if total <= 0 {
		total = done
	}
	return done, total, etag, nil
}

// open issues the GET, resuming from offset with a Range header. A server
// that ignores Range (200) restarts the file from zero.
func (d *Downloader) open(ctx context.Context, u, token string, offset int64) (*http.Response, int64, *DownloadError) {
	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, offset, &DownloadError{Code: DLInvalid, Message: "invalid download URL"}
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if offset > 0 {
			req.Header.Set("Range", "bytes="+strconv.FormatInt(offset, 10)+"-")
		}
		resp, err := d.client.Do(req)
		if err != nil {
			var de *DownloadError
			if errors.As(err, &de) {
				return nil, offset, de
			}
			return nil, offset, &DownloadError{Code: DLInterrupted, Message: "cannot reach Hugging Face: " + err.Error(), Retryable: true}
		}
		switch {
		case resp.StatusCode == http.StatusPartialContent && offset > 0:
			return resp, offset, nil
		case resp.StatusCode == http.StatusOK:
			return resp, 0, nil
		case resp.StatusCode == http.StatusRequestedRangeNotSatisfiable && offset > 0 && attempt == 0:
			_ = resp.Body.Close()
			offset = 0
			continue
		}
		_ = resp.Body.Close()
		return nil, offset, statusError(resp.StatusCode, token != "")
	}
	return nil, offset, &DownloadError{Code: DLInterrupted, Message: "could not resume the download", Retryable: true}
}

func statusError(code int, hadToken bool) *DownloadError {
	switch code {
	case http.StatusUnauthorized, http.StatusForbidden:
		msg := "this repository is gated: accept its licence on huggingface.co and provide a Hugging Face token"
		if hadToken {
			msg = "this repository is gated and the Hugging Face token does not have access yet: accept the licence with the token's account"
		}
		return &DownloadError{Code: DLGated, Message: msg}
	case http.StatusNotFound:
		return &DownloadError{Code: DLNotFound, Message: "the repository or file was not found on Hugging Face"}
	}
	return &DownloadError{Code: DLUnavailable, Message: fmt.Sprintf("Hugging Face answered HTTP %d", code), Retryable: true}
}

func responseTotal(resp *http.Response, offset int64) int64 {
	if resp.StatusCode == http.StatusPartialContent {
		if cr := resp.Header.Get("Content-Range"); cr != "" {
			if i := strings.LastIndex(cr, "/"); i >= 0 {
				if n, err := strconv.ParseInt(strings.TrimSpace(cr[i+1:]), 10, 64); err == nil {
					return n
				}
			}
		}
		if resp.ContentLength > 0 {
			return offset + resp.ContentLength
		}
		return 0
	}
	if n, err := strconv.ParseInt(resp.Header.Get("X-Linked-Size"), 10, 64); err == nil && n > 0 {
		return n
	}
	if resp.ContentLength > 0 {
		return resp.ContentLength
	}
	return 0
}

func fileSHA256(ctx context.Context, path string) (string, error) {
	f, err := os.Open(path) //nolint:gosec // path is built from a validated model name and file name
	if err != nil {
		return "", fmt.Errorf("open: %w", err)
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	buf := make([]byte, 1<<20)
	for {
		if err := ctx.Err(); err != nil {
			return "", fmt.Errorf("verify cancelled: %w", err)
		}
		n, rerr := f.Read(buf)
		h.Write(buf[:n])
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return "", fmt.Errorf("read: %w", rerr)
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
