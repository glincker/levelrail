package deploy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	defaultStaticKeepReleases = 10
	minStaticKeepReleases     = 3
)

// StaticRetentionStore is the read surface pruning needs to know which
// releases are still in use. *store.DB satisfies it.
type StaticRetentionStore interface {
	ListDeployAttempts(ctx context.Context, serviceName string) ([]store.DeployAttempt, error)
	ListStaticSites(ctx context.Context) ([]store.StaticSite, error)
	GetPublishLock(ctx context.Context, name string) (*store.PublishLock, error)
}

// StaticKeepReleases reads APP_STATIC_KEEP_RELEASES: how many releases of one
// static site stay on disk. Unset or invalid gives 10, anything below 3 gives 3.
func StaticKeepReleases() int {
	n, err := strconv.Atoi(strings.TrimSpace(os.Getenv("APP_STATIC_KEEP_RELEASES")))
	if err != nil {
		return defaultStaticKeepReleases
	}
	return max(n, minStaticKeepReleases)
}

// PruneStaticReleases deletes the oldest release directories of one static
// site under <root>/<service>, keeping the newest keep (never fewer than 3).
// It never removes the live release, the previous release, a release used by a
// queued, running, held or awaiting approval attempt, a release any site still
// serves (previews and branch deploys included), or a release newer than the
// live one. It does nothing while a publish lock is set. Safe to repeat.
func PruneStaticReleases(ctx context.Context, root, service string, keep int, st StaticRetentionStore) ([]string, error) {
	if root == "" || service == "" || service == "." || !filepath.IsLocal(service) || filepath.Base(service) != service {
		return nil, fmt.Errorf("%w: %q", ErrUnsafeStaticName, service)
	}
	keep = max(keep, minStaticKeepReleases)
	lock, err := st.GetPublishLock(ctx, service)
	if err != nil {
		return nil, fmt.Errorf("deploy: retention: check publish lock: %w", err)
	}
	if lock != nil {
		return nil, nil
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("deploy: retention: resolve static root: %w", err)
	}
	svcDir := filepath.Join(root, service)
	realSvc, err := filepath.EvalSymlinks(svcDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("deploy: retention: resolve site dir: %w", err)
	}
	if filepath.Dir(realSvc) != realRoot {
		return nil, fmt.Errorf("deploy: retention: site dir %q resolves outside the static root", svcDir)
	}

	protected, liveName, err := protectedReleases(ctx, st, service, svcDir, realSvc)
	if err != nil {
		return nil, err
	}
	if protected == nil {
		return nil, nil
	}
	rels, err := listReleases(realSvc)
	if err != nil {
		return nil, err
	}
	var liveTime int64
	for _, r := range rels {
		if r.name == liveName {
			liveTime = r.mtime
		}
	}

	dirRoot, err := os.OpenRoot(realSvc)
	if err != nil {
		return nil, fmt.Errorf("deploy: retention: open site dir: %w", err)
	}
	defer dirRoot.Close() //nolint:errcheck // handle only used for confined removal
	var removed []string
	var errs []error
	for i, r := range rels {
		if i < keep || protected[r.name] || (liveName != "" && r.mtime > liveTime) {
			continue
		}
		if err := dirRoot.RemoveAll(r.name); err != nil {
			errs = append(errs, fmt.Errorf("remove release %q: %w", r.name, err))
			continue
		}
		removed = append(removed, r.name)
	}
	return removed, errors.Join(errs...)
}

type releaseEntry struct {
	name  string
	mtime int64
}

// listReleases returns real directories only, newest first. Symlinks are
// skipped so a link can never lead the removal outside the site dir.
func listReleases(dir string) ([]releaseEntry, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("deploy: retention: list releases: %w", err)
	}
	var out []releaseEntry
	for _, e := range ents {
		if !e.IsDir() || e.Type()&os.ModeSymlink != 0 || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, releaseEntry{name: e.Name(), mtime: info.ModTime().UnixNano()})
	}
	slices.SortFunc(out, func(a, b releaseEntry) int {
		if a.mtime != b.mtime {
			return -cmpInt64(a.mtime, b.mtime)
		}
		return -strings.Compare(a.name, b.name)
	})
	return out, nil
}

func cmpInt64(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// releaseChild maps a path to its first segment under the site dir, or false
// when the path is not inside it.
func releaseChild(path string, dirs ...string) (string, bool) {
	for _, d := range dirs {
		rel, err := filepath.Rel(d, path)
		if err != nil || rel == "." || !filepath.IsLocal(rel) {
			continue
		}
		return strings.Split(filepath.ToSlash(rel), "/")[0], true
	}
	return "", false
}

// protectedReleases returns the release names that must stay and the live
// release name. A nil map means the whole site dir is served as one release,
// so nothing may be pruned.
func protectedReleases(ctx context.Context, st StaticRetentionStore, service, svcDir, realSvc string) (map[string]bool, string, error) {
	protected := map[string]bool{}
	liveName := ""
	sites, err := st.ListStaticSites(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("deploy: retention: list static sites: %w", err)
	}
	for _, s := range sites {
		paths := []string{filepath.Clean(s.RootDir)}
		if resolved, err := filepath.EvalSymlinks(s.RootDir); err == nil {
			paths = append(paths, resolved)
		}
		for _, p := range paths {
			if p == svcDir || p == realSvc {
				return nil, "", nil
			}
			if name, ok := releaseChild(p, svcDir, realSvc); ok {
				protected[name] = true
				if s.Name == service {
					liveName = name
				}
			}
		}
	}

	attempts, err := st.ListDeployAttempts(ctx, service)
	if err != nil {
		return nil, "", fmt.Errorf("deploy: retention: list attempts: %w", err)
	}
	// The previous release is the rollback target: rollback re-hashes the tree
	// a past succeeded deploy recorded, so deleting it would turn that
	// rollback into a 410. Attempts arrive newest first.
	previousFound := false
	for _, a := range attempts {
		name, ok := attemptRelease(a)
		if !ok {
			continue
		}
		switch a.Status {
		case store.DeployAttemptStatusQueued, store.DeployAttemptStatusRunning, store.DeployAttemptStatusPending,
			store.DeployAttemptStatusHeld, store.DeployAttemptStatusAwaitingApproval:
			protected[name] = true
		case store.DeployAttemptStatusSucceeded:
			if !previousFound && a.ImageDigest != "" && name != liveName {
				protected[name] = true
				previousFound = true
			}
		}
	}
	return protected, liveName, nil
}

// attemptRelease names the release directory an attempt published to.
func attemptRelease(a store.DeployAttempt) (string, bool) {
	if a.CommitSHA == "" || !filepath.IsLocal(a.CommitSHA) {
		return "", false
	}
	return strings.Split(filepath.ToSlash(filepath.Clean(a.CommitSHA)), "/")[0], true
}

// pruneStaticReleases runs retention after a successful publish. A failure
// only logs: the deploy itself already succeeded.
func (p *Pipeline) pruneStaticReleases(ctx context.Context, service string) {
	st, ok := p.staticSites.(StaticRetentionStore)
	if !ok {
		return
	}
	removed, err := PruneStaticReleases(ctx, p.staticRootDir, service, StaticKeepReleases(), st)
	if err != nil {
		p.logger.Warn("deploy: prune old static releases failed", slog.String("service", service), slog.String("error", err.Error()))
	}
	if len(removed) > 0 {
		p.logger.Info("deploy: pruned old static releases", slog.String("service", service), slog.Int("removed", len(removed)))
	}
}
