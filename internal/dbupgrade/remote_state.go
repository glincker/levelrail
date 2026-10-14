package dbupgrade

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/dbcatalogbuild"
)

// Settings of the remote catalog.
const (
	EnvCatalogRemote = "APP_DB_CATALOG_REMOTE"
	EnvCatalogURL    = "APP_DB_CATALOG_URL"
	EnvCatalogSource = "APP_DB_CATALOG_SOURCE"

	CatalogSourcePublished = "published"
	CatalogSourceDirect    = "direct"
	CatalogSourceLive      = "live"
	CatalogSourceEmbedded  = "embedded"

	// DefaultCatalogURL is the published file; its signature sits at the same URL plus ".sig".
	DefaultCatalogURL = "https://cdn.jsdelivr.net/gh/glincker/levelrail@data/catalog/databases.v1.json"
)

// Freshness labels, measured from generated_at or fetched_at.
const (
	FreshnessFresh = "fresh"
	FreshnessOK    = "ok"
	FreshnessAging = "aging"
	FreshnessStale = "stale"
)

// Every cache, freshness and rate threshold of the remote catalog lives here.
const (
	freshMaxAge       = 7 * 24 * time.Hour
	okMaxAge          = 35 * 24 * time.Hour
	agingMaxAge       = 90 * 24 * time.Hour
	staleServeMax     = 90 * 24 * time.Hour
	publishedInterval = 24 * time.Hour
	directInterval    = 30 * 24 * time.Hour
	liveCooldown      = 10 * time.Minute
	upstreamTimeout   = 10 * time.Second
	maxCatalogBytes   = 1 << 20
	maxSigBytes       = 4 << 10
	backoffBase       = time.Hour
	backoffMax        = 24 * time.Hour
	startupDelay      = 30 * time.Second
	cacheFileName     = "db-catalog-cache.json"
)

// FreshnessOf labels data of the given age reference time.
func FreshnessOf(at, now time.Time) string {
	if at.IsZero() {
		return FreshnessStale
	}
	age := now.Sub(at)
	switch {
	case age < freshMaxAge:
		return FreshnessFresh
	case age < okMaxAge:
		return FreshnessOK
	case age < agingMaxAge:
		return FreshnessAging
	}
	return FreshnessStale
}

type liveEntry struct {
	Engine    dbcatalogbuild.Engine `json:"engine"`
	FetchedAt time.Time             `json:"fetched_at"`
}

type remoteState struct {
	RemoteEnabled *bool                 `json:"remote_enabled,omitempty"`
	PublishedRaw  string                `json:"published_raw,omitempty"`
	Signature     string                `json:"signature,omitempty"`
	ETag          string                `json:"etag,omitempty"`
	LastChecked   time.Time             `json:"last_checked,omitempty"`
	FetchedAt     time.Time             `json:"fetched_at,omitempty"`
	FailCount     int                   `json:"fail_count,omitempty"`
	NextAttempt   time.Time             `json:"next_attempt,omitempty"`
	LastError     string                `json:"last_error,omitempty"`
	Direct        *dbcatalogbuild.File  `json:"direct,omitempty"`
	DirectAt      time.Time             `json:"direct_at,omitempty"`
	Live          map[string]*liveEntry `json:"live,omitempty"`
	LiveChecked   map[string]time.Time  `json:"live_checked,omitempty"`
}

func (r *RemoteCatalog) cachePath() string {
	if r.cfg.DataDir == "" {
		return ""
	}
	return filepath.Join(r.cfg.DataDir, cacheFileName)
}

func (r *RemoteCatalog) loadState() {
	path := r.cachePath()
	if path == "" {
		return
	}
	raw, err := os.ReadFile(path) //nolint:gosec // fixed file name under the data dir
	if err != nil {
		return
	}
	var st remoteState
	if json.Unmarshal(raw, &st) != nil {
		return
	}
	r.st = st
}

func (r *RemoteCatalog) saveStateLocked() error {
	path := r.cachePath()
	if path == "" {
		return nil
	}
	raw, err := json.Marshal(r.st)
	if err != nil {
		return fmt.Errorf("dbupgrade: marshal catalog cache: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return fmt.Errorf("dbupgrade: create catalog cache dir: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("dbupgrade: write catalog cache: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("dbupgrade: replace catalog cache: %w", err)
	}
	return nil
}

// remoteOffReason is why every remote lookup is disabled, "" when enabled.
func (r *RemoteCatalog) remoteOffReason() string {
	if v, ok := r.cfg.lookupEnv(EnvCatalogRemote); ok {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "off", "0", "false", "no":
			return "disabled by " + EnvCatalogRemote
		}
	}
	if r.st.RemoteEnabled != nil && !*r.st.RemoteEnabled {
		return "disabled in settings"
	}
	return ""
}

// publishedOffReason is why the published file is not fetched, "" when it is.
func (r *RemoteCatalog) publishedOffReason() string {
	if off := r.remoteOffReason(); off != "" {
		return off
	}
	if r.cfg.Source == CatalogSourcePublished && r.cfg.PublicKey == "" {
		return "no catalog signing key is configured in this build, so unsigned data is not trusted"
	}
	return ""
}

// SetRemoteEnabled persists the operator's remote refresh setting.
func (r *RemoteCatalog) SetRemoteEnabled(enabled bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.st.RemoteEnabled = &enabled
	return r.saveStateLocked()
}

var (
	// ErrRemoteDisabled means remote lookups are switched off.
	ErrRemoteDisabled = errors.New("dbupgrade: remote catalog lookups are disabled")
	// ErrUnsupportedEngine means no registry source is known for the engine.
	ErrUnsupportedEngine = errors.New("dbupgrade: no live version source for this engine")
)
