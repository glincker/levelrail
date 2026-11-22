package cleanup

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/registrycatalog"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

// Built-in registry target ids.
const (
	TargetRegistryTags = "docker.registry_tags"
	TargetRegistryGC   = "docker.registry_gc"
)

const bytesPerMB = 1 << 20

// RegistryGCReport is what garbage collection found unreferenced in the registry volume.
type RegistryGCReport struct {
	Blobs int
	Bytes int64
}

// RegistryTags is the built-in registry as the cleanup center sees it.
type RegistryTags interface {
	Repositories(ctx context.Context) ([]string, error)
	Tags(ctx context.Context, repository string) ([]registrycatalog.TagInfo, error)
	Delete(ctx context.Context, repository, digest string) error
	// InUse maps a repository to the tags apps use; a "*" tag protects the whole repository.
	InUse(ctx context.Context) (map[string]map[string]bool, error)
	GarbageCollect(ctx context.Context, dryRun bool) (RegistryGCReport, error)
}

// SplitImageRef returns the repository path and tag of an image reference, without any registry
// host. A digest reference yields the tag "*", meaning every tag of the repository.
func SplitImageRef(ref string) (repo, tag string) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", ""
	}
	digest := false
	if i := strings.Index(ref, "@"); i >= 0 {
		ref, digest = ref[:i], true
	}
	tag = "latest"
	if i := strings.LastIndex(ref, ":"); i > strings.LastIndex(ref, "/") {
		ref, tag = ref[:i], ref[i+1:]
	}
	if i := strings.Index(ref, "/"); i > 0 {
		if first := ref[:i]; strings.ContainsAny(first, ".:") || first == "localhost" {
			ref = ref[i+1:]
		}
	}
	if digest {
		tag = "*"
	}
	return ref, tag
}

func registryTargets(d BuiltinDeps) []Target {
	tags := baseMeta(TargetRegistryTags, CategoryDocker, "Registry tags",
		"Old tags in the built-in registry beyond the newest N per repository. Tags an app uses are never removed.", runtimesettings.KeyRegistryKeepTags)
	tags.Origin, tags.Destructive, tags.Floor, tags.Presets = OriginNew, true, 1, []int{3, 5, 10, 20}
	gc := baseMeta(TargetRegistryGC, CategoryDocker, "Registry garbage collection",
		"Layers no tag references. Runs only when the unreferenced size reaches the threshold in MB; pushes during a run can fail.", runtimesettings.KeyRegistryGCMinMB)
	gc.Origin, gc.Destructive, gc.Presets = OriginNew, true, []int{100, 500, 1000, 5000}
	return []Target{&registryTagsTarget{meta: tags, src: d.RegistryTags}, &registryGCTarget{meta: gc, src: d.RegistryTags}}
}

type registryTagsTarget struct {
	meta Meta
	src  RegistryTags
}

func (t *registryTagsTarget) Meta() Meta { return t.meta }

func (t *registryTagsTarget) Measure(ctx context.Context) (Usage, error) {
	repos, err := t.src.Repositories(ctx)
	if err != nil {
		return Usage{}, err
	}
	var n int64
	for _, r := range repos {
		tags, err := t.src.Tags(ctx, r)
		if err != nil {
			return Usage{}, err
		}
		n += int64(len(tags))
	}
	return Usage{Rows: n, NoBytes: true}, nil
}

type prunePlan struct {
	repo string
	tags []registrycatalog.TagInfo
}

// plan is the shared dry-run: what would go, never touching a tag an app uses. days is the keep count.
func (t *registryTagsTarget) plan(ctx context.Context, keep int) ([]prunePlan, error) {
	repos, err := t.src.Repositories(ctx)
	if err != nil {
		return nil, err
	}
	use, err := t.src.InUse(ctx)
	if err != nil {
		return nil, err
	}
	sort.Strings(repos)
	var out []prunePlan
	for _, r := range repos {
		if use[r]["*"] {
			continue
		}
		tags, err := t.src.Tags(ctx, r)
		if err != nil {
			return nil, err
		}
		if drop := registrycatalog.PlanPrune(tags, keep, use[r]); len(drop) > 0 {
			out = append(out, prunePlan{repo: r, tags: drop})
		}
	}
	return out, nil
}

func (t *registryTagsTarget) Preview(ctx context.Context, keep int) (Impact, error) {
	p, err := t.plan(ctx, keep)
	if err != nil {
		return Impact{}, err
	}
	var n int64
	for _, e := range p {
		n += int64(len(e.tags))
	}
	return Impact{Rows: n}, nil
}

func (t *registryTagsTarget) Run(ctx context.Context, keep int, opts RunOptions) Result {
	p, err := t.plan(ctx, keep)
	if err != nil {
		return Result{Err: fmt.Errorf("cleanup: %s: %w", t.meta.ID, err)}
	}
	var res Result
	for _, e := range p {
		for _, tag := range e.tags {
			if opts.MaxRows > 0 && res.Rows >= opts.MaxRows {
				res.More = true
				return res
			}
			if !opts.DryRun {
				if err := t.src.Delete(ctx, e.repo, tag.Digest); err != nil {
					res.Err = fmt.Errorf("cleanup: %s: delete %s:%s: %w", t.meta.ID, e.repo, tag.Tag, err)
					return res
				}
			}
			res.Rows++
		}
	}
	return res
}

type registryGCTarget struct {
	meta Meta
	src  RegistryTags
}

func (t *registryGCTarget) Meta() Meta { return t.meta }

func (t *registryGCTarget) Measure(ctx context.Context) (Usage, error) {
	rep, err := t.src.GarbageCollect(ctx, true)
	if err != nil {
		return Usage{}, err
	}
	return Usage{Rows: int64(rep.Blobs), Bytes: rep.Bytes}, nil
}

func (t *registryGCTarget) Preview(ctx context.Context, minMB int) (Impact, error) {
	rep, err := t.src.GarbageCollect(ctx, true)
	if err != nil {
		return Impact{}, err
	}
	if rep.Bytes < int64(minMB)*bytesPerMB {
		return Impact{}, nil
	}
	return Impact{Rows: int64(rep.Blobs), Bytes: rep.Bytes}, nil
}

// Run always looks first with a dry run, and collects only when the threshold is met.
func (t *registryGCTarget) Run(ctx context.Context, minMB int, opts RunOptions) Result {
	imp, err := t.Preview(ctx, minMB)
	if err != nil || opts.DryRun || imp.Rows == 0 {
		return Result{Rows: imp.Rows, Bytes: imp.Bytes, Err: err}
	}
	rep, err := t.src.GarbageCollect(ctx, false)
	if err != nil {
		return Result{Err: fmt.Errorf("cleanup: %s: %w", t.meta.ID, err)}
	}
	return Result{Rows: int64(rep.Blobs), Bytes: rep.Bytes}
}
