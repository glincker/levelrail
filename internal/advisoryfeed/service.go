package advisoryfeed

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Environment variables. Everything is off while EnvFeed is empty.
const (
	EnvFeed      = "APP_ADVISORY_FEED"
	EnvPubKey    = "APP_ADVISORY_FEED_PUBKEY"
	EnvRefresh   = "APP_ADVISORY_FEED_REFRESH"
	EnvMaxBytes  = "APP_ADVISORY_FEED_MAX_BYTES"
	defaultFresh = 6 * time.Hour
	defaultMax   = 4 << 20
)

// Config is read once from the environment.
type Config struct {
	Source   string
	PubKey   string
	Refresh  time.Duration
	MaxBytes int64
}

// ConfigFromEnv reads the feed settings through lookup.
func ConfigFromEnv(lookup func(string) string) Config {
	c := Config{Source: strings.TrimSpace(lookup(EnvFeed)), PubKey: strings.TrimSpace(lookup(EnvPubKey)), Refresh: defaultFresh, MaxBytes: defaultMax}
	if d, err := time.ParseDuration(lookup(EnvRefresh)); err == nil && d > 0 {
		c.Refresh = d
	}
	if n, err := strconv.ParseInt(lookup(EnvMaxBytes), 10, 64); err == nil && n > 0 {
		c.MaxBytes = n
	}
	return c
}

// Snapshot is the loaded feed and how far it can be trusted.
type Snapshot struct {
	Enabled     bool      `json:"enabled"`
	Source      string    `json:"source,omitempty"`
	Signature   string    `json:"signature,omitempty"`
	Error       string    `json:"error,omitempty"`
	GeneratedAt time.Time `json:"generated_at,omitempty"`
	LoadedAt    time.Time `json:"loaded_at,omitempty"`
	Entries     []Entry   `json:"entries"`
	Fingerprint string    `json:"-"`
}

// UsesDigests reports whether any entry selects by digest, so callers can
// skip resolving digests when none can match.
func (s Snapshot) UsesDigests() bool {
	for _, e := range s.Entries {
		for _, a := range e.Affected {
			if len(a.Digests) > 0 {
				return true
			}
		}
	}
	return false
}

// Service loads the feed lazily and caches matches until the feed or the
// image set changes. There is no background loop.
type Service struct {
	cfg  Config
	http *http.Client

	mu       sync.Mutex
	snap     Snapshot
	loaded   bool
	statSig  string
	matchKey string
	matches  []Match
}

// New returns a service; a zero Source makes every call a no-op.
func New(cfg Config, client *http.Client) *Service {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &Service{cfg: cfg, http: client}
}

// Enabled reports whether a feed is configured.
func (s *Service) Enabled() bool { return s != nil && s.cfg.Source != "" }

// Snapshot returns the current feed, reloading only when the file changed or
// a remote feed is older than the refresh interval.
func (s *Service) Snapshot(ctx context.Context) Snapshot {
	if !s.Enabled() {
		return Snapshot{Entries: []Entry{}}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fresh() {
		return s.snap
	}
	s.reload(ctx)
	return s.snap
}

func (s *Service) isURL() bool {
	return strings.HasPrefix(s.cfg.Source, "https://") || strings.HasPrefix(s.cfg.Source, "http://")
}

func (s *Service) fresh() bool {
	if !s.loaded {
		return false
	}
	if s.isURL() {
		return time.Since(s.snap.LoadedAt) < s.cfg.Refresh
	}
	st, err := os.Stat(s.cfg.Source)
	return err == nil && fileSig(st) == s.statSig
}

func fileSig(st os.FileInfo) string {
	return strconv.FormatInt(st.ModTime().UnixNano(), 10) + ":" + strconv.FormatInt(st.Size(), 10)
}

func (s *Service) reload(ctx context.Context) {
	snap := Snapshot{Enabled: true, Source: s.cfg.Source, Signature: SigUnsigned, LoadedAt: time.Now().UTC(), Entries: []Entry{}}
	s.loaded = true
	s.matchKey = ""
	s.statSig = ""
	if !s.isURL() {
		if st, err := os.Stat(s.cfg.Source); err == nil {
			s.statSig = fileSig(st)
		}
	}
	data, sig, err := s.fetch(ctx)
	if err == nil {
		snap, err = s.accept(snap, data, sig)
	}
	if err != nil {
		snap.Error = err.Error()
		snap.Entries = []Entry{}
	}
	s.snap = snap
}

// accept verifies when a key is configured, then parses. A bad signature
// yields no entries, so an unauthenticated feed is never trusted by accident.
func (s *Service) accept(snap Snapshot, data, sig []byte) (Snapshot, error) {
	if s.cfg.PubKey != "" {
		key, err := ParsePublicKey(s.cfg.PubKey)
		if err != nil {
			snap.Signature = SigInvalid
			return snap, err
		}
		if err := verifyWith(key, data, sig); err != nil {
			snap.Signature = SigInvalid
			return snap, err
		}
		snap.Signature = SigVerified
	}
	f, err := Parse(data)
	if err != nil {
		return snap, err
	}
	snap.Entries = f.Entries
	snap.GeneratedAt = f.GeneratedAt
	sum := sha256.Sum256(data)
	snap.Fingerprint = hex.EncodeToString(sum[:8])
	return snap, nil
}

func verifyWith(key ed25519.PublicKey, data, sig []byte) error {
	if len(sig) == 0 {
		return errors.New("a public key is configured but the feed has no detached signature")
	}
	return Verify(key, data, sig)
}

// fetch reads the feed and, only when a key is configured, its ".sig" file.
func (s *Service) fetch(ctx context.Context) (data, sig []byte, err error) {
	data, err = s.read(ctx, s.cfg.Source)
	if err != nil || s.cfg.PubKey == "" {
		return data, nil, err
	}
	sig, serr := s.read(ctx, s.cfg.Source+".sig")
	if serr != nil {
		return data, nil, nil
	}
	return data, sig, nil
}

func (s *Service) read(ctx context.Context, src string) ([]byte, error) {
	var r io.ReadCloser
	if s.isURL() {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
		if err != nil {
			return nil, fmt.Errorf("advisory feed url: %w", err)
		}
		resp, err := s.http.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetch advisory feed: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("fetch advisory feed: status %d", resp.StatusCode)
		}
		r = resp.Body
	} else {
		f, err := os.Open(src) //nolint:gosec // operator configured path
		if err != nil {
			return nil, fmt.Errorf("open advisory feed: %w", err)
		}
		r = f
	}
	defer func() { _ = r.Close() }()
	b, err := io.ReadAll(io.LimitReader(r, s.cfg.MaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read advisory feed: %w", err)
	}
	if int64(len(b)) > s.cfg.MaxBytes {
		return nil, fmt.Errorf("advisory feed is larger than %d bytes", s.cfg.MaxBytes)
	}
	return b, nil
}

// Match returns the feed and its matches for images. The result is cached
// until the feed or the image set changes.
func (s *Service) Match(ctx context.Context, images []Image) (Snapshot, []Match) {
	snap := s.Snapshot(ctx)
	if !snap.Enabled || len(snap.Entries) == 0 {
		return snap, nil
	}
	key := snap.Fingerprint + "|" + imagesKey(images)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.matchKey != key {
		s.matches = MatchImages(Feed{Version: SchemaVersion, Entries: snap.Entries}, images)
		s.matchKey = key
	}
	return snap, append([]Match(nil), s.matches...)
}

func imagesKey(images []Image) string {
	parts := make([]string, len(images))
	for i, im := range images {
		parts[i] = im.App + "=" + im.Ref + "@" + im.Digest
	}
	sort.Strings(parts)
	return strings.Join(parts, ";")
}
