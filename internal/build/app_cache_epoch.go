package build

import (
	"context"
	"sync/atomic"
)

// AppCacheEpochs is the store view that gives each app instance its own cache token.
type AppCacheEpochs interface {
	GetAppCacheEpoch(ctx context.Context, name string) (string, error)
	EnsureAppCacheEpoch(ctx context.Context, name string) (string, error)
}

var appCacheEpochs atomic.Pointer[AppCacheEpochs]

// SetAppCacheEpochs wires the store in at boot; nil keeps the bare app name keys.
func SetAppCacheEpochs(e AppCacheEpochs) {
	if e == nil {
		appCacheEpochs.Store(nil)
		return
	}
	appCacheEpochs.Store(&e)
}

// AppCacheKey is the cache id of an existing app instance. An app that has not
// built since the token existed keeps the legacy bare name key.
func AppCacheKey(ctx context.Context, name string) string {
	return appCacheKey(ctx, name, false)
}

// EnsureAppCacheKey is AppCacheKey for a build: it mints the instance token on first use.
func EnsureAppCacheKey(ctx context.Context, name string) string {
	return appCacheKey(ctx, name, true)
}

func appCacheKey(ctx context.Context, name string, create bool) string {
	base := SanitizeCacheID(name, "")
	p := appCacheEpochs.Load()
	if p == nil || base == "" {
		return base
	}
	var epoch string
	var err error
	if create {
		epoch, err = (*p).EnsureAppCacheEpoch(ctx, name)
	} else {
		epoch, err = (*p).GetAppCacheEpoch(ctx, name)
	}
	if err != nil || epoch == "" {
		return base
	}
	return FitCacheID(name + "-" + epoch)
}
