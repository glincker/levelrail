package cloudiac

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// TofuVersion is the OpenTofu release this Levelrail release pins.
const TofuVersion = "1.13.2"

const (
	tofuReleaseBase   = "https://github.com/opentofu/opentofu/releases/download"
	placeholderDigest = "placeholder-not-a-real-digest"
	tofuBinaryName    = "tofu"
)

// Source: tofu_1.13.2_SHA256SUMS on the v1.13.2 GitHub release, matched against the asset API digests.
var tofuDigests = map[string]map[string]string{
	"1.13.2": {
		"linux/amd64":  "ac05d9952662c7db28dd57e93f0a68018837bfdf9342384ec0987977c16a9645",
		"linux/arm64":  "88e2d9e3f94f97b76ab2faf55ade87578f56aed19c5e45fa139d70e994366d1f",
		"darwin/amd64": "dc6e4eb1d3ed6550c2ef052596f933dbcd8c806957665ea83ae7537147cb3074",
		"darwin/arm64": "0d1cdc85f38992055b48e7fb161304a2ddfe2e5ee0ff68ffec9d0349c8bd1b59",
	},
}

// Fetch errors. Every one is a hard stop: there is no retry with another URL.
var (
	ErrFetchConfig          = errors.New("cloudiac: fetch is not configured")
	ErrUnpinnedVersion      = errors.New("cloudiac: OpenTofu version has no compiled checksum")
	ErrUnsupportedPlatform  = errors.New("cloudiac: no compiled checksum for this platform")
	ErrPlaceholderDigest    = errors.New("cloudiac: compiled checksum is a placeholder, refusing to run")
	ErrSignatureUnverified  = errors.New("cloudiac: release signature verification is not available")
	ErrLowDisk              = errors.New("cloudiac: not enough free disk for the OpenTofu download")
	ErrTooLarge             = errors.New("cloudiac: OpenTofu download exceeds the size cap")
	ErrChecksumMismatch     = errors.New("cloudiac: OpenTofu checksum mismatch")
	ErrBadArchive           = errors.New("cloudiac: OpenTofu archive is malformed")
	ErrFreeSpaceUnsupported = errors.New("cloudiac: free space probe is not supported on this platform")
)

// SignatureVerifier checks the release signature of a downloaded archive.
type SignatureVerifier interface {
	Verify(ctx context.Context, archiveName, archivePath string) error
}

// FetchConfig holds the fetch limits. Values come from runtime settings; zero values refuse.
type FetchConfig struct {
	DataDir      string
	Version      string
	MaxBytes     int64
	MinFreeBytes int64
	Timeout      time.Duration
	// FreeBytes reports free space under DataDir; the caller supplies it per platform.
	FreeBytes  func(dir string) (int64, error)
	HTTPClient *http.Client
	BaseURL    string
	Verifier   SignatureVerifier
	// AllowChecksumOnly accepts a release without a signature check. Default off.
	AllowChecksumOnly bool
	GOOS, GOARCH      string
}

// FetchLimits are the runtime settings that bound a fetch; NewFetcher refuses zero values.
type FetchLimits struct {
	MaxMB, MinFreeMB, TimeoutSeconds int
	AllowChecksumOnly                bool
}

// ApplyLimits copies the runtime settings into the config and installs the platform free space probe.
func (c *FetchConfig) ApplyLimits(l FetchLimits) {
	c.MaxBytes = int64(l.MaxMB) << 20
	c.MinFreeBytes = int64(l.MinFreeMB) << 20
	c.Timeout = time.Duration(l.TimeoutSeconds) * time.Second
	c.AllowChecksumOnly = l.AllowChecksumOnly
	if c.FreeBytes == nil {
		c.FreeBytes = FreeBytes
	}
}

// Fetcher downloads and caches the pinned OpenTofu binary. It does no I/O until Ensure.
type Fetcher struct {
	cfg     FetchConfig
	digests map[string]map[string]string
	mu      sync.Mutex
}

// NewFetcher validates the config without touching the network or disk.
func NewFetcher(cfg FetchConfig) (*Fetcher, error) {
	if cfg.Version == "" {
		cfg.Version = TofuVersion
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = tofuReleaseBase
	}
	if cfg.GOOS == "" {
		cfg.GOOS = runtime.GOOS
	}
	if cfg.GOARCH == "" {
		cfg.GOARCH = runtime.GOARCH
	}
	switch {
	case cfg.DataDir == "" || !filepath.IsAbs(cfg.DataDir):
		return nil, fmt.Errorf("%w: data dir must be an absolute path", ErrFetchConfig)
	case cfg.MaxBytes <= 0, cfg.MinFreeBytes < 0, cfg.Timeout <= 0:
		return nil, fmt.Errorf("%w: size cap, free space floor and timeout are required", ErrFetchConfig)
	case cfg.FreeBytes == nil:
		return nil, fmt.Errorf("%w: free space probe is required", ErrFetchConfig)
	}
	u, err := url.Parse(cfg.BaseURL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return nil, fmt.Errorf("%w: release URL must be https", ErrFetchConfig)
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: cfg.Timeout}
	}
	return &Fetcher{cfg: cfg, digests: tofuDigests}, nil
}

func (f *Fetcher) platform() string { return f.cfg.GOOS + "/" + f.cfg.GOARCH }

func (f *Fetcher) archiveName() string {
	return fmt.Sprintf("tofu_%s_%s_%s.tar.gz", f.cfg.Version, f.cfg.GOOS, f.cfg.GOARCH)
}

func (f *Fetcher) binDir() string {
	return filepath.Join(f.cfg.DataDir, "cloudiac", "tofu", f.cfg.Version, f.cfg.GOOS+"_"+f.cfg.GOARCH)
}

func (f *Fetcher) expectedDigest() (string, error) {
	byPlatform, ok := f.digests[f.cfg.Version]
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrUnpinnedVersion, f.cfg.Version)
	}
	d, ok := byPlatform[f.platform()]
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrUnsupportedPlatform, f.platform())
	}
	if raw, err := hex.DecodeString(d); err != nil || len(raw) != sha256.Size || d == placeholderDigest {
		return "", ErrPlaceholderDigest
	}
	return d, nil
}

// Ensure returns the path of a verified OpenTofu binary, downloading it on first use.
func (f *Fetcher) Ensure(ctx context.Context) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	digest, err := f.expectedDigest()
	if err != nil {
		return "", err
	}
	if f.cfg.Verifier == nil && !f.cfg.AllowChecksumOnly {
		return "", ErrSignatureUnverified
	}
	dir := f.binDir()
	bin := filepath.Join(dir, tofuBinaryName)
	if ok, err := cachedBinaryValid(bin); err != nil {
		return "", err
	} else if ok {
		return bin, nil
	}
	if err := mkdirPrivate(f.cfg.DataDir, dir); err != nil {
		return "", err
	}
	free, err := f.cfg.FreeBytes(f.cfg.DataDir)
	if err != nil {
		return "", fmt.Errorf("cloudiac: free space probe: %w", err)
	}
	// Room for the archive and the extracted binary, each at most MaxBytes.
	if free < 2*f.cfg.MaxBytes+f.cfg.MinFreeBytes {
		return "", ErrLowDisk
	}

	archive, err := f.download(ctx, dir, digest)
	if err != nil {
		return "", err
	}
	defer func() { _ = os.Remove(archive) }()
	if f.cfg.Verifier != nil {
		if err := f.cfg.Verifier.Verify(ctx, f.archiveName(), archive); err != nil {
			return "", fmt.Errorf("cloudiac: signature: %w", err)
		}
	}
	if err := f.extract(archive, dir, bin); err != nil {
		return "", err
	}
	return bin, nil
}

func (f *Fetcher) download(ctx context.Context, dir, digest string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, f.cfg.Timeout)
	defer cancel()
	src := strings.TrimRight(f.cfg.BaseURL, "/") + "/v" + f.cfg.Version + "/" + f.archiveName()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
	if err != nil {
		return "", err
	}
	resp, err := f.cfg.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("cloudiac: download: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("cloudiac: download: status %d", resp.StatusCode)
	}
	if resp.ContentLength > f.cfg.MaxBytes {
		return "", ErrTooLarge
	}

	tmp, err := os.CreateTemp(dir, ".archive-*")
	if err != nil {
		return "", err
	}
	keep := false
	defer func() {
		_ = tmp.Close()
		if !keep {
			_ = os.Remove(tmp.Name())
		}
	}()
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(resp.Body, f.cfg.MaxBytes+1))
	if err != nil {
		return "", fmt.Errorf("cloudiac: download: %w", err)
	}
	if n > f.cfg.MaxBytes {
		return "", ErrTooLarge
	}
	got := hex.EncodeToString(h.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(got), []byte(digest)) != 1 {
		return "", ErrChecksumMismatch
	}
	if err := tmp.Sync(); err != nil {
		return "", err
	}
	keep = true
	return tmp.Name(), nil
}

// extract writes only the tofu entry, and makes it executable only after the archive checksum passed.
func (f *Fetcher) extract(archive, dir, bin string) error {
	in, err := os.Open(archive) //nolint:gosec // path is our own temp file
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	gz, err := gzip.NewReader(in)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrBadArchive, err)
	}
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("%w: no %s entry", ErrBadArchive, tofuBinaryName)
		}
		if err != nil {
			return fmt.Errorf("%w: %w", ErrBadArchive, err)
		}
		if hdr.Name != tofuBinaryName || hdr.Typeflag != tar.TypeReg {
			continue
		}
		if hdr.Size <= 0 || hdr.Size > f.cfg.MaxBytes {
			return ErrTooLarge
		}
		return writeBinary(tr, hdr.Size, dir, bin)
	}
}

func writeBinary(r io.Reader, size int64, dir, bin string) error {
	out, err := os.CreateTemp(dir, ".tofu-*")
	if err != nil {
		return err
	}
	name := out.Name()
	ok := false
	defer func() {
		if !ok {
			_ = out.Close()
			_ = os.Remove(name)
		}
	}()
	if n, err := io.Copy(out, io.LimitReader(r, size)); err != nil || n != size {
		return fmt.Errorf("%w: short binary entry", ErrBadArchive)
	}
	if err := out.Sync(); err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	if err := os.Chmod(name, 0o500); err != nil { //nolint:gosec // owner read and execute only
		return err
	}
	if err := os.Rename(name, bin); err != nil {
		return err
	}
	ok = true
	return nil
}

// cachedBinaryValid accepts a cached binary only with the exact mode we set, so a loosened file is refetched.
func cachedBinaryValid(bin string) (bool, error) {
	st, err := os.Lstat(bin)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !st.Mode().IsRegular() || st.Mode().Perm() != 0o500 || st.Size() == 0 {
		if err := os.Remove(bin); err != nil {
			return false, err
		}
		return false, nil
	}
	return true, nil
}

// mkdirPrivate creates every directory below root with mode 0700, ignoring the umask.
func mkdirPrivate(root, dir string) error {
	rel, err := filepath.Rel(root, dir)
	if err != nil || strings.HasPrefix(rel, "..") {
		return fmt.Errorf("%w: cache dir outside data dir", ErrFetchConfig)
	}
	cur := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		cur = filepath.Join(cur, part)
		if err := os.Mkdir(cur, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
		if err := os.Chmod(cur, 0o700); err != nil { //nolint:gosec // directories need the search bit
			return err
		}
	}
	return nil
}
