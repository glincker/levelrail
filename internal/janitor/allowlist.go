package janitor

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Actor kinds recorded on a run.
const (
	ActorUser     = "user"
	ActorToken    = "api_token"
	ActorSchedule = "schedule"
)

// Names of items a stage reports as one lump rather than one row per thing.
const (
	aggContainers = "stopped containers"
	aggCache      = " cache records"
	aggAnonymous  = " anonymous volumes"
)

// Actor is who started a run.
type Actor struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// ItemSelection lists the item names one stage may act on.
type ItemSelection struct {
	Stage Stage    `json:"stage"`
	Names []string `json:"names"`
}

// IgnoredItem is an allowlist entry a pass did not act on, and why.
type IgnoredItem struct {
	Stage  Stage  `json:"stage"`
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// scope is the parsed allowlist of one pass.
type scope struct {
	names   map[Stage]map[string]bool
	ignored []IgnoredItem
}

func newScope(sel []ItemSelection) *scope {
	s := &scope{names: map[Stage]map[string]bool{}}
	for _, it := range sel {
		for _, n := range it.Names {
			n = strings.TrimSpace(n)
			if n == "" {
				continue
			}
			if !validStage(it.Stage) {
				s.ignored = append(s.ignored, IgnoredItem{Stage: it.Stage, Name: n, Reason: "unknown stage"})
				continue
			}
			if it.Stage == StageVolumes {
				if prot, why := volumeProtected(n); prot {
					s.ignored = append(s.ignored, IgnoredItem{Stage: it.Stage, Name: n, Reason: "protected: " + why})
					continue
				}
			}
			if s.names[it.Stage] == nil {
				s.names[it.Stage] = map[string]bool{}
			}
			s.names[it.Stage][n] = true
		}
	}
	return s
}

func (s *scope) has(st Stage) bool { return len(s.names[st]) > 0 }

func (s *scope) allows(st Stage, name string) bool { return s.names[st][name] }

func (s *scope) allowsSuffix(st Stage, suffix string) bool {
	for n := range s.names[st] {
		if strings.HasSuffix(n, suffix) {
			return true
		}
	}
	return false
}

// finish reports every listed name the stage found no candidate for.
func (s *scope) finish(st Stage, res StageResult) {
	for n := range s.names[st] {
		if !matched(st, n, res.Items) {
			s.ignored = append(s.ignored, IgnoredItem{Stage: st, Name: n, Reason: "no matching cleanup candidate"})
		}
	}
}

func matched(st Stage, name string, items []Item) bool {
	if st == StageContainers && name == aggContainers {
		return len(items) > 0
	}
	for _, it := range items {
		switch {
		case it.Name == name:
			return true
		case st == StageBuildCache && strings.HasSuffix(name, aggCache) && strings.HasSuffix(it.Name, aggCache):
			return true
		case st == StageVolumes && strings.HasSuffix(name, aggAnonymous) && strings.HasSuffix(it.Name, aggAnonymous):
			return true
		}
	}
	return false
}

// engine returns a copy of e whose collaborators only see the allowed items,
// so the unchanged stage code acts on exactly the allowlist.
func (s *scope) engine(e *Engine) *Engine {
	d := e.Deps
	d.Docker = scopedDocker{Docker: e.Docker, s: s}
	d.Store = scopedStore{Store: e.Store, s: s}
	if orig := e.OrphanVolumes; orig != nil {
		d.OrphanVolumes = func(ctx context.Context) ([]docker.NamedVolume, error) {
			all, err := orig(ctx)
			var out []docker.NamedVolume
			for _, v := range all {
				if s.allows(StageVolumes, v.Name) {
					out = append(out, v)
				}
			}
			return out, err
		}
	}
	if orig := e.TokenWindows; orig != nil {
		d.TokenWindows = func(ctx context.Context) TokenWindows {
			w := orig(ctx)
			if !s.allows(StageTokens, string(store.TokenPurgeRevoked)+" tokens") {
				w.Revoked = 0
			}
			if !s.allows(StageTokens, string(store.TokenPurgeExpired)+" tokens") {
				w.Expired = 0
			}
			return w
		}
	}
	return &Engine{Deps: d}
}

type scopedStore struct {
	Store
	s *scope
}

func (t scopedStore) ListJanitorQuarantine(ctx context.Context, kind string) ([]store.JanitorQuarantine, error) {
	all, err := t.Store.ListJanitorQuarantine(ctx, kind)
	var out []store.JanitorQuarantine
	for _, q := range all {
		if kind != volumeKind || t.s.allows(StageVolumes, q.Name) {
			out = append(out, q)
		}
	}
	return out, err
}

func (t scopedStore) ListPrunableDeployAttempts(ctx context.Context, cutoff time.Time, keep, limit int) ([]store.PrunableAttempt, error) {
	all, err := t.Store.ListPrunableDeployAttempts(ctx, cutoff, keep, limit)
	var out []store.PrunableAttempt
	for _, a := range all {
		if t.s.allows(StageAttempts, a.ServiceName) {
			out = append(out, a)
		}
	}
	return out, err
}

type scopedDocker struct {
	Docker
	s *scope
}

type imageRemover interface {
	RemoveImage(ctx context.Context, id string) error
}

func (d scopedDocker) DiskUsage(ctx context.Context) (docker.DiskUsage, error) {
	u, err := d.Docker.DiskUsage(ctx)
	if !d.s.allows(StageContainers, aggContainers) {
		u.ContainersReclaimableBytes = 0
	}
	return u, err
}

func (d scopedDocker) PruneContainers(ctx context.Context, keep []string) (docker.PruneContainersResult, error) {
	if !d.s.allows(StageContainers, aggContainers) {
		return docker.PruneContainersResult{}, nil
	}
	return d.Docker.PruneContainers(ctx, keep)
}

func (d scopedDocker) ListAllImages(ctx context.Context) ([]docker.ImageRecord, error) {
	all, err := d.Docker.ListAllImages(ctx)
	var out []docker.ImageRecord
	for _, r := range all {
		if d.s.allows(StageImages, shortID(r.ID)) {
			out = append(out, r)
		}
	}
	return out, err
}

// PruneDanglingImages removes only the listed dangling images, one by one,
// because the daemon prune has no way to name them.
func (d scopedDocker) PruneDanglingImages(ctx context.Context) (docker.PruneImagesResult, error) {
	rm, ok := d.Docker.(imageRemover)
	if !ok {
		return docker.PruneImagesResult{}, errors.New("removing single images is not available on this daemon")
	}
	recs, err := d.ListAllImages(ctx)
	if err != nil {
		return docker.PruneImagesResult{}, err
	}
	var res docker.PruneImagesResult
	for _, r := range recs {
		if len(r.Tags) > 0 || r.Containers > 0 {
			continue
		}
		if err := rm.RemoveImage(ctx, r.ID); err != nil {
			return res, err
		}
		res.Removed = append(res.Removed, r.ID)
		res.ReclaimedBytes += u64ClampNonNeg(r.SizeBytes)
	}
	return res, nil
}

func u64ClampNonNeg(v int64) uint64 {
	if v < 0 {
		return 0
	}
	return uint64(v)
}

func (d scopedDocker) BuildCacheRecords(ctx context.Context) ([]docker.BuildCacheRecord, error) {
	if !d.s.allowsSuffix(StageBuildCache, aggCache) {
		return nil, nil
	}
	return d.Docker.BuildCacheRecords(ctx)
}

func (d scopedDocker) PruneAnonymousVolumes(ctx context.Context) (docker.PruneVolumesResult, error) {
	if !d.s.allowsSuffix(StageVolumes, aggAnonymous) {
		return docker.PruneVolumesResult{}, nil
	}
	return d.Docker.PruneAnonymousVolumes(ctx)
}

func (d scopedDocker) ListDanglingAnonymousVolumes(ctx context.Context) ([]docker.AnonymousVolume, error) {
	l, ok := d.Docker.(anonymousVolumeLister)
	if !ok || !d.s.allowsSuffix(StageVolumes, aggAnonymous) {
		return nil, nil
	}
	return l.ListDanglingAnonymousVolumes(ctx)
}
