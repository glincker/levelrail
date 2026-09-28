package deploy

import (
	"context"
	"log/slog"

	"github.com/GLINCKER/levelrail/internal/build"
)

// BuildCacheProvider resolves an app's BuildKit remote cache and records how
// the build went. internal/objectstore.BuildCache satisfies it.
type BuildCacheProvider interface {
	S3Cache(ctx context.Context, app string) (*build.S3Cache, error)
	RecordBuild(ctx context.Context, app, warning string)
}

// WithBuildCache enables the object storage BuildKit cache for Dockerfile
// builds. A resolution failure downgrades to no cache with a warning; it never
// fails the deploy.
func WithBuildCache(c BuildCacheProvider) Option {
	return func(p *Pipeline) { p.buildCache = c }
}

// prepareBuildCache returns the cache to build with, a progress func that
// remembers any cache warning, and a done func that records the outcome.
func (p *Pipeline) prepareBuildCache(ctx context.Context, app string, progress func(build.ProgressEvent)) (*build.S3Cache, func(build.ProgressEvent), func()) {
	if p.buildCache == nil {
		return nil, progress, func() {}
	}
	var warning string
	wrapped := func(ev build.ProgressEvent) {
		if ev.CacheWarning != "" {
			warning = ev.CacheWarning
		}
		if progress != nil {
			progress(ev)
		}
	}
	s3, err := p.buildCache.S3Cache(ctx, app)
	if err != nil {
		p.logger.Warn("deploy: build cache unavailable", slog.String("service", app), slog.String("error", err.Error()))
		build.EmitCacheWarning(wrapped, "build cache skipped: could not resolve the storage destination")
		s3 = nil
	}
	done := func() {
		if s3 == nil && warning == "" {
			return
		}
		p.buildCache.RecordBuild(ctx, app, warning)
	}
	return s3, wrapped, done
}
