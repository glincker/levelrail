package deploy

// This file implements build.type: static (spec.BuildStatic), the
// architecture's one deliberate exception to "deploy means build an
// image": the design states it directly, "static sites get served by
// the embedded Caddy directly with no container." There is nothing here
// for internal/build.Client to do: the "build" step for a static site is
// copying its output into a directory internal/reconcile/ingress's
// controller can point Caddy's file_server handler at, and saving that
// directory's location as a store.StaticSite (migrations/0015), which
// bypasses store.DesiredService (and therefore the application
// controller and every other consumer of "what containers should be
// running") entirely.

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// StaticSiteStore is the narrow surface this package needs from
// internal/store for build.type: static, so tests can fake it without a
// real database, the same narrow-interface pattern ServiceStore already
// establishes for container services. *store.DB satisfies this.
type StaticSiteStore interface {
	SaveStaticSite(ctx context.Context, site store.StaticSite) error
}

// WithStaticSiteStore enables build.type: static deploys. Without one
// configured (the default), a service declaring build.type: static fails
// its deploy loudly rather than silently discarding the copied files,
// the same "fail loudly without configuration" shape WithSecretChecker
// already establishes for { secret: true } env vars.
func WithStaticSiteStore(s StaticSiteStore) Option {
	return func(p *Pipeline) { p.staticSites = s }
}

// WithStaticRootDir sets the local filesystem directory static site
// output is copied into, under which every static service gets its own
// "<ServiceName>/<CommitSHA>" subdirectory. Without one configured (the
// default, empty string), build.type: static fails its deploy loudly:
// this package deliberately does not invent a fallback location (e.g.
// os.TempDir()) for content the ingress controller and Caddy need to
// keep serving after this process's own temp-file lifecycle might have
// cleaned it up. The caller (cmd/levelrail) is expected to pass a path
// under the control plane's own data directory, matching every other
// on-disk state this codebase keeps there (internal/ingress's
// WithStorageDir doc comment makes the identical point for Caddy's own
// certificate storage).
func WithStaticRootDir(dir string) Option {
	return func(p *Pipeline) { p.staticRootDir = dir }
}

// deployStatic copies req.Service.Build.Path (relative to req.SourceDir,
// or req.SourceDir itself when Build.Path is empty) into
// "<staticRootDir>/<req.ServiceName>/<req.CommitSHA>", then saves that
// directory plus req.Service.Domains as a store.StaticSite. Unlike
// deployDockerfile, there is no image, no build.ProgressEvent to forward
// (progress is accepted for signature symmetry with the other
// deployXxx methods but never invoked), and no container for the
// application controller to converge to: the returned string is the
// directory now being served, not an image tag.
//
// The copy must land under p.staticRootDir, never serve directly from
// req.SourceDir: internal/webhook's Handler defers removing the checkout
// directory the instant Deploy returns (a real git checkout is temporary
// by design, see cloneAndCheckout's own doc comment), so anything Caddy
// needs to keep serving after this call returns has to already live
// somewhere durable by the time it does.
func (p *Pipeline) deployStatic(ctx context.Context, req Request) (string, error) {
	if p.staticSites == nil {
		return "", fmt.Errorf("deploy: service %q: build.type %q needs a static site store configured (WithStaticSiteStore)", req.ServiceName, spec.BuildStatic)
	}
	if p.staticRootDir == "" {
		return "", fmt.Errorf("deploy: service %q: build.type %q needs a static root directory configured (WithStaticRootDir)", req.ServiceName, spec.BuildStatic)
	}

	srcDir := req.SourceDir
	if req.Service.Build.Path != "" {
		srcDir = filepath.Join(req.SourceDir, req.Service.Build.Path)
	}
	info, err := os.Stat(srcDir)
	if err != nil {
		return "", fmt.Errorf("deploy: service %q: static source directory %q: %w", req.ServiceName, srcDir, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("deploy: service %q: static source path %q is not a directory", req.ServiceName, srcDir)
	}

	destDir := filepath.Join(p.staticRootDir, req.ServiceName, req.CommitSHA)

	start := time.Now()
	if err := copyStaticDir(srcDir, destDir); err != nil {
		return "", fmt.Errorf("deploy: service %q: copy static files: %w", req.ServiceName, err)
	}
	duration := time.Since(start)

	if err := p.staticSites.SaveStaticSite(ctx, store.StaticSite{
		Name:    req.ServiceName,
		Domains: req.Service.Domains,
		RootDir: destDir,
	}); err != nil {
		return "", fmt.Errorf("deploy: service %q: save static site: %w", req.ServiceName, err)
	}

	// Best-effort, secondary to the deploy itself, the identical
	// "must never block the real operation" choice deployDockerfile
	// already makes for the same metric.
	if p.metrics != nil {
		if err := p.metrics.RecordBuildDuration(ctx, req.ServiceName, duration, time.Now()); err != nil {
			p.logger.Warn("deploy: record build duration metric failed",
				slog.String("service", req.ServiceName), slog.String("error", err.Error()))
		}
	}

	return destDir, nil
}

// copyStaticDir replaces dest's entire contents with a copy of every
// regular file and directory under src. dest is removed first (not
// merged into) so a redeploy of the same commit SHA, or a source tree
// that dropped a file since the previous deploy, never leaves a stale
// file being served that the current source tree no longer has.
//
// Symlinks are deliberately skipped, not followed or recreated: a
// symlink inside a user's repo could point outside src (an "escape" a
// plain recursive copy has no business resolving), and Caddy's
// file_server needs actual files under destDir, not a second layer of
// indirection this package would have to reason about the safety of.
func copyStaticDir(src, dest string) error {
	if err := os.RemoveAll(dest); err != nil {
		return fmt.Errorf("clear destination %q: %w", dest, err)
	}
	if err := os.MkdirAll(dest, 0o750); err != nil {
		return fmt.Errorf("create destination %q: %w", dest, err)
	}

	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk %q: %w", path, err)
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return fmt.Errorf("relative path for %q: %w", path, err)
		}
		if rel == "." {
			return nil // src itself, already created above.
		}
		target := filepath.Join(dest, rel)

		switch {
		case d.Type()&os.ModeSymlink != 0:
			return nil // See the doc comment above: symlinks are skipped.
		case d.IsDir():
			return os.MkdirAll(target, 0o750)
		case d.Type().IsRegular():
			return copyFile(path, target)
		default:
			return nil // Sockets, devices, etc.: not valid static site content.
		}
	})
}

// copyFile copies a single regular file's contents from src to dest.
// dest's parent directory is assumed to already exist (copyStaticDir's
// WalkDir visits a directory before any of its children).
func copyFile(src, dest string) (err error) {
	in, err := os.Open(src) //nolint:gosec // src is a path this package itself derived from a git checkout under SourceDir, not user-supplied request input
	if err != nil {
		return fmt.Errorf("open %q: %w", src, err)
	}
	defer func() {
		if cerr := in.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close %q: %w", src, cerr)
		}
	}()

	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o640) //nolint:gosec // dest is derived from the control plane's own configured static root dir, not user-supplied request input
	if err != nil {
		return fmt.Errorf("create %q: %w", dest, err)
	}
	defer func() {
		if cerr := out.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close %q: %w", dest, cerr)
		}
	}()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy %q to %q: %w", src, dest, err)
	}
	return nil
}
