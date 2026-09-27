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
	"io/fs"
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
	checkoutRoot, srcRel, err := openContained(req.SourceDir, srcDir)
	if err != nil {
		return "", fmt.Errorf("deploy: service %q: static source: %w", req.ServiceName, err)
	}
	defer checkoutRoot.Close() //nolint:errcheck // read-only handle
	info, err := checkoutRoot.Stat(srcRel)
	if err != nil {
		return "", fmt.Errorf("deploy: service %q: static source directory %q: %w", req.ServiceName, srcDir, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("deploy: service %q: static source path %q is not a directory", req.ServiceName, srcDir)
	}

	destRel, err := staticDestRel(req.ServiceName, req.CommitSHA)
	if err != nil {
		return "", fmt.Errorf("deploy: service %q: %w", req.ServiceName, err)
	}
	if err := os.MkdirAll(p.staticRootDir, 0o750); err != nil {
		return "", fmt.Errorf("deploy: service %q: create static root %q: %w", req.ServiceName, p.staticRootDir, err)
	}
	staticRoot, err := os.OpenRoot(p.staticRootDir)
	if err != nil {
		return "", fmt.Errorf("deploy: service %q: open static root %q: %w", req.ServiceName, p.staticRootDir, err)
	}
	defer staticRoot.Close() //nolint:errcheck // handle only used for confined writes
	destDir := filepath.Join(p.staticRootDir, destRel)

	start := time.Now()
	if err := copyStaticDir(checkoutRoot, srcRel, staticRoot, destRel); err != nil {
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

// openContained opens the symlink-resolved checkout as an os.Root and returns
// it with dir's path relative to it. It refuses a dir that resolves outside the
// checkout, since a repository could link build.path to a control plane file
// that the static site would then serve publicly. Callers must Close the root.
func openContained(checkout, dir string) (*os.Root, string, error) {
	realRoot, err := filepath.EvalSymlinks(checkout)
	if err != nil {
		return nil, "", fmt.Errorf("resolve checkout %q: %w", checkout, err)
	}
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, "", fmt.Errorf("resolve %q: %w", dir, err)
	}
	rel, err := filepath.Rel(realRoot, realDir)
	if err != nil || !filepath.IsLocal(rel) {
		return nil, "", fmt.Errorf("%q resolves outside the repository", dir)
	}
	root, err := os.OpenRoot(realRoot)
	if err != nil {
		return nil, "", fmt.Errorf("open checkout %q: %w", realRoot, err)
	}
	return root, rel, nil
}

// staticDestRel returns the "<service>/<commit>" path under the static root,
// refusing any value that could leave it: the destination is removed before
// the copy, so an escaping value would delete a directory outside the root.
// commit may be empty or a ref with slashes, matching what callers pass.
func staticDestRel(service, commit string) (string, error) {
	if !filepath.IsLocal(service) || filepath.Base(service) != service {
		return "", fmt.Errorf("static site service name %q must be a single relative name", service)
	}
	if commit == "" {
		return service, nil
	}
	if !filepath.IsLocal(commit) {
		return "", fmt.Errorf("static site commit %q must be a relative name", commit)
	}
	return filepath.Join(service, commit), nil
}

// copyStaticDir replaces destRel under destRoot with a copy of every regular
// file and directory under srcRel in srcRoot. The destination is removed first
// so a stale file is never served. Symlinks are skipped, and both sides go
// through os.Root so no path can escape either tree.
func copyStaticDir(srcRoot *os.Root, srcRel string, destRoot *os.Root, destRel string) error {
	if err := destRoot.RemoveAll(destRel); err != nil {
		return fmt.Errorf("clear destination %q: %w", destRel, err)
	}
	if err := destRoot.MkdirAll(destRel, 0o750); err != nil {
		return fmt.Errorf("create destination %q: %w", destRel, err)
	}
	srcFS, err := fs.Sub(srcRoot.FS(), filepath.ToSlash(srcRel))
	if err != nil {
		return fmt.Errorf("scope source %q: %w", srcRel, err)
	}

	return fs.WalkDir(srcFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("walk %q: %w", path, err)
		}
		if path == "." {
			return nil
		}
		target := filepath.Join(destRel, filepath.FromSlash(path))

		switch {
		case d.Type()&fs.ModeSymlink != 0:
			return nil
		case d.IsDir():
			return destRoot.MkdirAll(target, 0o750)
		case d.Type().IsRegular():
			return copyFile(srcFS, path, destRoot, target)
		default:
			return nil // Sockets, devices, etc.: not valid static site content.
		}
	})
}

// copyFile copies one regular file from src to dest inside destRoot.
func copyFile(srcFS fs.FS, src string, destRoot *os.Root, dest string) (err error) {
	in, err := srcFS.Open(src)
	if err != nil {
		return fmt.Errorf("open %q: %w", src, err)
	}
	defer func() {
		if cerr := in.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close %q: %w", src, cerr)
		}
	}()

	out, err := destRoot.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o640)
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
