package objectstore

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/netguard"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Env names for the build cache bounds.
const (
	EnvBuildCachePrefix          = "APP_BUILD_CACHE_PREFIX"
	EnvBuildCacheClearMaxObjects = "APP_BUILD_CACHE_CLEAR_MAX_OBJECTS"
	EnvBuildCacheStatsMaxObjects = "APP_BUILD_CACHE_STATS_MAX_OBJECTS"

	buildCachePageSize = 1000
)

// ErrInvalidBuildCacheApp is returned for an app name that cannot be a key prefix.
var ErrInvalidBuildCacheApp = errors.New("objectstore: invalid app name for build cache")

var buildCacheAppPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// BuildCacheOptions bounds the build cache bucket operations.
type BuildCacheOptions struct {
	Prefix          string
	ClearMaxObjects int
	StatsMaxObjects int
}

// DefaultBuildCacheOptions returns the defaults.
func DefaultBuildCacheOptions() BuildCacheOptions {
	return BuildCacheOptions{Prefix: "build-cache", ClearMaxObjects: 5000, StatsMaxObjects: 2000}
}

// BuildCacheOptionsFromEnv applies env overrides to the defaults.
func BuildCacheOptionsFromEnv(lookup func(string) (string, bool)) BuildCacheOptions {
	o := DefaultBuildCacheOptions()
	if v, ok := lookup(EnvBuildCachePrefix); ok && strings.Trim(v, "/") != "" {
		o.Prefix = strings.Trim(v, "/")
	}
	if n, ok := envInt(lookup, EnvBuildCacheClearMaxObjects); ok {
		o.ClearMaxObjects = n
	}
	if n, ok := envInt(lookup, EnvBuildCacheStatsMaxObjects); ok {
		o.StatsMaxObjects = n
	}
	return o
}

// BuildCacheStore is the persistence surface the build cache service needs.
type BuildCacheStore interface {
	GetBuildCacheSetting(ctx context.Context, appName string) (store.BuildCacheSetting, error)
	RecordBuildCacheResult(ctx context.Context, scope, at, result, warning string) error
	RecordBuildCacheCleared(ctx context.Context, scope, at string) error
}

// BuildCacheResolver turns a storage destination id into credentials and a client.
type BuildCacheResolver interface {
	Config(ctx context.Context, targetID string) (Config, store.BackupTarget, error)
	Client(ctx context.Context, targetID string) (*Client, store.BackupTarget, error)
}

// BuildCache resolves per-app BuildKit remote cache settings against storage
// destinations and runs cache hygiene operations on the bucket.
type BuildCache struct {
	Store    BuildCacheStore
	Resolver BuildCacheResolver
	Options  BuildCacheOptions
	Logger   *slog.Logger
	// ResolveHost is net.DefaultResolver.LookupNetIP unless a test overrides it.
	ResolveHost func(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// Effective returns the setting an app builds with: its own row when present
// (an explicit disable wins), else the global default. The bool is false when
// no enabled setting applies.
func (b *BuildCache) Effective(ctx context.Context, app string) (store.BuildCacheSetting, bool, error) {
	own, err := b.Store.GetBuildCacheSetting(ctx, app)
	switch {
	case err == nil:
		return own, own.Enabled, nil
	case !errors.Is(err, store.ErrBuildCacheSettingNotFound):
		return store.BuildCacheSetting{}, false, fmt.Errorf("objectstore: load build cache setting: %w", err)
	}
	global, err := b.Store.GetBuildCacheSetting(ctx, "")
	switch {
	case err == nil:
		return global, global.Enabled, nil
	case errors.Is(err, store.ErrBuildCacheSettingNotFound):
		return store.BuildCacheSetting{}, false, nil
	default:
		return store.BuildCacheSetting{}, false, fmt.Errorf("objectstore: load global build cache setting: %w", err)
	}
}

// KeyPrefix is the per-app bucket prefix the cache lives under.
func (b *BuildCache) KeyPrefix(app string) (string, error) {
	if !buildCacheAppPattern.MatchString(app) {
		return "", ErrInvalidBuildCacheApp
	}
	prefix := b.Options.Prefix
	if prefix == "" {
		prefix = DefaultBuildCacheOptions().Prefix
	}
	return prefix + "/" + app + "/", nil
}

// S3Cache builds the BuildKit s3 cache config for app, or nil when the app has
// no enabled setting. The result carries credentials.
func (b *BuildCache) S3Cache(ctx context.Context, app string) (*build.S3Cache, error) {
	setting, ok, err := b.Effective(ctx, app)
	if err != nil || !ok {
		return nil, err
	}
	prefix, err := b.KeyPrefix(app)
	if err != nil {
		return nil, err
	}
	cfg, _, err := b.Resolver.Config(ctx, setting.TargetID)
	if err != nil {
		return nil, fmt.Errorf("objectstore: resolve build cache destination: %w", err)
	}
	if err := b.checkEndpoint(ctx, cfg.Endpoint); err != nil {
		return nil, err
	}
	return &build.S3Cache{
		Region: cfg.Region, Bucket: cfg.Bucket, Prefix: prefix, Endpoint: cfg.Endpoint, PathStyle: cfg.PathStyle,
		AccessKeyID: cfg.AccessKeyID, SecretAccessKey: cfg.SecretAccessKey, Mode: setting.Mode,
	}, nil
}

// checkEndpoint applies the netguard policy to a custom endpoint. BuildKit
// dials the bucket itself, so the guarded HTTP client cannot do it for us.
func (b *BuildCache) checkEndpoint(ctx context.Context, endpoint string) error {
	if endpoint == "" || netguard.AllowPrivate() {
		return nil
	}
	if err := ValidateEndpoint(endpoint); err != nil {
		return fmt.Errorf("objectstore: %w", err)
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("objectstore: parse endpoint: %w", err)
	}
	host := u.Hostname()
	if ip, err := netip.ParseAddr(host); err == nil {
		return blockedAddrErr(ip)
	}
	lookup := b.ResolveHost
	if lookup == nil {
		lookup = net.DefaultResolver.LookupNetIP
	}
	addrs, err := lookup(ctx, "ip", host)
	if err != nil {
		return fmt.Errorf("objectstore: resolve endpoint host: %w", err)
	}
	for _, ip := range addrs {
		if err := blockedAddrErr(ip); err != nil {
			return err
		}
	}
	return nil
}

func blockedAddrErr(ip netip.Addr) error {
	if netguard.IsBlocked(ip) {
		return fmt.Errorf("objectstore: %w", netguard.ErrBlockedAddress)
	}
	return nil
}

// RecordBuild stores the latest build outcome on the setting the app resolved
// to. Failures are logged, never returned: recording must not fail a build.
func (b *BuildCache) RecordBuild(ctx context.Context, app, warning string) {
	scope, ok := b.scope(ctx, app)
	if !ok {
		return
	}
	result := "ok"
	if warning != "" {
		result = "fallback"
	}
	if err := b.Store.RecordBuildCacheResult(ctx, scope, time.Now().UTC().Format(time.RFC3339), result, warning); err != nil && b.Logger != nil {
		b.Logger.Warn("build cache: record result failed", slog.String("app", app), slog.String("error", err.Error()))
	}
}

func (b *BuildCache) scope(ctx context.Context, app string) (string, bool) {
	setting, ok, err := b.Effective(ctx, app)
	if err != nil || !ok {
		return "", false
	}
	return setting.AppName, true
}

// CacheStats summarises the objects under an app's cache prefix.
type CacheStats struct {
	Prefix       string    `json:"prefix"`
	Objects      int       `json:"objects"`
	Bytes        int64     `json:"bytes"`
	LastModified time.Time `json:"last_modified,omitzero"`
	// Truncated means the listing hit the bound, so counts are lower bounds.
	Truncated bool `json:"truncated"`
}

// Stats lists the app's cache prefix, bounded by StatsMaxObjects.
func (b *BuildCache) Stats(ctx context.Context, app string) (CacheStats, error) {
	client, prefix, err := b.appClient(ctx, app)
	if err != nil {
		return CacheStats{}, err
	}
	stats := CacheStats{Prefix: prefix}
	token := ""
	for stats.Objects < b.Options.StatsMaxObjects {
		page, err := client.List(ctx, prefix, token, buildCachePageSize)
		if err != nil {
			return CacheStats{}, fmt.Errorf("objectstore: build cache stats: %w", err)
		}
		for _, o := range page.Objects {
			stats.Objects++
			stats.Bytes += o.Size
			if o.LastModified.After(stats.LastModified) {
				stats.LastModified = o.LastModified
			}
		}
		if page.Next == "" {
			return stats, nil
		}
		token = page.Next
	}
	stats.Truncated = true
	return stats, nil
}

// ClearResult reports a cache clear.
type ClearResult struct {
	Deleted int `json:"deleted"`
	// More means the ClearMaxObjects bound was hit; run the clear again.
	More bool `json:"more"`
}

// Clear deletes objects under the app's cache prefix, up to ClearMaxObjects.
func (b *BuildCache) Clear(ctx context.Context, app string) (ClearResult, error) {
	client, prefix, err := b.appClient(ctx, app)
	if err != nil {
		return ClearResult{}, err
	}
	var res ClearResult
	for {
		page, err := client.List(ctx, prefix, "", buildCachePageSize)
		if err != nil {
			return res, fmt.Errorf("objectstore: clear build cache: %w", err)
		}
		if len(page.Objects) == 0 {
			break
		}
		for _, o := range page.Objects {
			if res.Deleted >= b.Options.ClearMaxObjects {
				res.More = true
				return res, b.stampCleared(ctx, app)
			}
			if err := client.Delete(ctx, o.Key); err != nil {
				return res, fmt.Errorf("objectstore: clear build cache: %w", err)
			}
			res.Deleted++
		}
		if page.Next == "" {
			break
		}
	}
	return res, b.stampCleared(ctx, app)
}

func (b *BuildCache) stampCleared(ctx context.Context, app string) error {
	scope, ok := b.scope(ctx, app)
	if !ok {
		return nil
	}
	if err := b.Store.RecordBuildCacheCleared(ctx, scope, time.Now().UTC().Format(time.RFC3339)); err != nil {
		return fmt.Errorf("objectstore: %w", err)
	}
	return nil
}

func (b *BuildCache) appClient(ctx context.Context, app string) (*Client, string, error) {
	setting, ok, err := b.Effective(ctx, app)
	if err != nil {
		return nil, "", err
	}
	if !ok {
		return nil, "", ErrBuildCacheNotConfigured
	}
	prefix, err := b.KeyPrefix(app)
	if err != nil {
		return nil, "", err
	}
	client, _, err := b.Resolver.Client(ctx, setting.TargetID)
	if err != nil {
		return nil, "", fmt.Errorf("objectstore: resolve build cache destination: %w", err)
	}
	return client, prefix, nil
}

// ErrBuildCacheNotConfigured is returned when an app has no enabled build cache setting.
var ErrBuildCacheNotConfigured = errors.New("objectstore: build cache is not configured for this app")
