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

// deployStatic copies the static build output into
// "<staticRootDir>/<ServiceName>/<CommitSHA>" and saves it as a
// store.StaticSite. The copy is required because the webhook checkout is
// removed as soon as Deploy returns. Returns the directory now served.
func (p *Pipeline) deployStatic(ctx context.Context, req Request) (string, error) {
	if p.staticSites == nil {
		return "", fmt.Errorf("deploy: service %q: build.type %q needs a static site store configured (WithStaticSiteStore)", req.ServiceName, spec.BuildStatic)
	}
	if p.staticRootDir == "" {
		return "", fmt.Errorf("deploy: service %q: build.type %q needs a static root directory configured (WithStaticRootDir)", req.ServiceName, spec.BuildStatic)
	}

	buildRoot, err := resolveBuildRoot(req.SourceDir, req.Service.Build.BaseDirectory)
	if err != nil {
		return "", fmt.Errorf("deploy: service %q: %w", req.ServiceName, err)
	}

	srcDir := buildRoot
	if buildPath := req.Service.Build.Path; buildPath != "" {
		if !filepath.IsLocal(buildPath) {
			return "", fmt.Errorf("deploy: service %q: build.path %q must be a relative path inside the build root", req.ServiceName, buildPath)
		}
		srcDir = filepath.Join(buildRoot, buildPath)
	}
	srcDir, err = containedRealPath(req.SourceDir, srcDir)
	if err != nil {
		return "", fmt.Errorf("deploy: service %q: static source: %w", req.ServiceName, err)
	}
	info, err := os.Stat(srcDir)
	if err != nil {
		return "", fmt.Errorf("deploy: service %q: static source directory %q: %w", req.ServiceName, srcDir, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("deploy: service %q: static source path %q is not a directory", req.ServiceName, srcDir)
	}

	destDir, err := staticDestDir(p.staticRootDir, req.ServiceName, req.CommitSHA)
	if err != nil {
		return "", fmt.Errorf("deploy: service %q: %w", req.ServiceName, err)
	}

	start := time.Now()
	if err := copyStaticDir(srcDir, destDir); err != nil {
		return "", fmt.Errorf("deploy: service %q: copy static files: %w", req.ServiceName, err)
	}
	duration := time.Since(start)

	if err := commitPoint(req); err != nil {
		return "", fmt.Errorf("deploy: service %q: %w", req.ServiceName, err)
	}
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

// containedRealPath resolves symlinks in dir and refuses a result outside the
// checkout, since a repository could link build.path to a control plane file
// that the static site would then serve publicly.
func containedRealPath(checkout, dir string) (string, error) {
	realRoot, err := filepath.EvalSymlinks(checkout)
	if err != nil {
		return "", fmt.Errorf("resolve checkout %q: %w", checkout, err)
	}
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", fmt.Errorf("resolve %q: %w", dir, err)
	}
	rel, err := filepath.Rel(realRoot, realDir)
	if err != nil || !filepath.IsLocal(rel) {
		return "", fmt.Errorf("%q resolves outside the repository", dir)
	}
	return realDir, nil
}

// staticDestDir joins service and commit under root, refusing any value
// that could leave root: destDir is removed before the copy, so an
// escaping value would delete a directory outside it. commit may be
// empty or a ref with slashes, matching what callers already pass.
func staticDestDir(root, service, commit string) (string, error) {
	if !filepath.IsLocal(service) || filepath.Base(service) != service {
		return "", fmt.Errorf("static site service name %q must be a single relative name", service)
	}
	if commit == "" {
		return filepath.Join(root, service), nil
	}
	if !filepath.IsLocal(commit) {
		return "", fmt.Errorf("static site commit %q must be a relative name", commit)
	}
	return filepath.Join(root, service, commit), nil
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
