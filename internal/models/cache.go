package models

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	cacheVolumeMid    = "-model-"
	cacheVolumeSuffix = "-cache"
	hoursPerDay       = 24
)

// CacheNote states what the cache listing can and cannot see.
const CacheNote = "Each model keeps its weights in its own Docker volume, so the cache is listed per volume, not per file. " +
	"Last used comes from gateway traffic when a model has any, else from the model's last change. " +
	"Only the control plane's own host is inspected; remote nodes are not listed yet."

// CacheBackend is the Docker surface the cache manager needs.
type CacheBackend interface {
	ListVolumesByPrefix(ctx context.Context, prefix string) ([]docker.NamedVolume, error)
	RemoveVolume(ctx context.Context, name string) error
}

type cacheDeps struct {
	backend CacheBackend
	prefix  string
	now     func() time.Time
}

// SetCacheBackend enables the cache manager. prefix is the container and
// volume name prefix the model controller uses.
func (s *Service) SetCacheBackend(b CacheBackend, prefix string) {
	if prefix == "" {
		prefix = defaultPrefix
	}
	s.cache = cacheDeps{backend: b, prefix: prefix, now: time.Now}
}

// CacheEnabled reports whether the cache manager is available.
func (s *Service) CacheEnabled() bool { return s.cache.backend != nil }

// CacheUnusedDays is APP_MODEL_CACHE_UNUSED_DAYS (default 30).
func CacheUnusedDays() int {
	if n, err := strconv.Atoi(os.Getenv("APP_MODEL_CACHE_UNUSED_DAYS")); err == nil && n > 0 {
		return n
	}
	return 30
}

// CacheEntry is one cached weights volume.
type CacheEntry struct {
	Volume         string     `json:"volume"`
	Model          string     `json:"model,omitempty"`
	Engine         string     `json:"engine,omitempty"`
	ModelRef       string     `json:"model_ref,omitempty"`
	SizeBytes      *int64     `json:"size_bytes"`
	LastUsedAt     *time.Time `json:"last_used_at"`
	LastUsedSource string     `json:"last_used_source"`
	UnusedDays     int        `json:"unused_days"`
	Unused         bool       `json:"unused"`
	InUse          bool       `json:"in_use"`
	Configured     bool       `json:"configured"`
	// DuplicateOf names the volume holding the same weights that is
	// counted in UniqueBytes instead of this one.
	DuplicateOf string `json:"duplicate_of,omitempty"`
	Prunable    bool   `json:"prunable"`
	KeepReason  string `json:"keep_reason,omitempty"`
}

// CacheNode is one node's cache.
type CacheNode struct {
	NodeID           string       `json:"node_id"`
	Name             string       `json:"name"`
	IsLocal          bool         `json:"is_local"`
	Supported        bool         `json:"supported"`
	Message          string       `json:"message,omitempty"`
	Entries          []CacheEntry `json:"entries"`
	TotalBytes       int64        `json:"total_bytes"`
	UniqueBytes      int64        `json:"unique_bytes"`
	ReclaimableBytes int64        `json:"reclaimable_bytes"`
	DiskFreeBytes    *int64       `json:"disk_free_bytes"`
}

// CacheReport lists every node's model cache.
type CacheReport struct {
	UnusedDays int         `json:"unused_days"`
	Nodes      []CacheNode `json:"nodes"`
	Note       string      `json:"note"`
}

// CacheSkip is a volume a prune left alone.
type CacheSkip struct {
	Volume string `json:"volume"`
	Reason string `json:"reason"`
}

// CachePruneResult reports a prune. With DryRun set nothing was removed
// and Candidates is what a real run would remove.
type CachePruneResult struct {
	DryRun         bool         `json:"dry_run"`
	Candidates     []CacheEntry `json:"candidates"`
	Removed        []string     `json:"removed"`
	Skipped        []CacheSkip  `json:"skipped"`
	ReclaimedBytes int64        `json:"reclaimed_bytes"`
}

func (s *Service) modelVolumePrefix() string { return s.cache.prefix + cacheVolumeMid }

func (s *Service) modelFromVolume(volume string) string {
	name, ok := strings.CutPrefix(volume, s.modelVolumePrefix())
	if !ok {
		return ""
	}
	return strings.TrimSuffix(name, cacheVolumeSuffix)
}

// CacheList reports model weight volumes with size, last use and whether
// each is safe to prune.
func (s *Service) CacheList(ctx context.Context) (CacheReport, error) {
	if s.cache.backend == nil {
		return CacheReport{}, errors.New("models: cache manager is not configured")
	}
	days := CacheUnusedDays()
	report := CacheReport{UnusedDays: days, Nodes: []CacheNode{}, Note: CacheNote}
	local, err := s.localCache(ctx, days)
	if err != nil {
		return CacheReport{}, err
	}
	report.Nodes = append(report.Nodes, local)
	nodes, err := s.store.ListNodes(ctx)
	if err != nil {
		return CacheReport{}, fmt.Errorf("models: list nodes: %w", err)
	}
	for _, n := range nodes {
		if n.ID == s.localID {
			continue
		}
		report.Nodes = append(report.Nodes, CacheNode{NodeID: n.ID, Name: n.Name, Entries: []CacheEntry{},
			Message: "Cache listing for remote nodes is not available yet."})
	}
	return report, nil
}

func (s *Service) localCache(ctx context.Context, days int) (CacheNode, error) {
	node := CacheNode{NodeID: s.localID, Name: LocalNodeName, IsLocal: true, Supported: true, Entries: []CacheEntry{}}
	vols, err := s.cache.backend.ListVolumesByPrefix(ctx, s.modelVolumePrefix())
	if err != nil {
		return node, fmt.Errorf("models: list cache volumes: %w", err)
	}
	rows, err := s.store.ListModels(ctx)
	if err != nil {
		return node, fmt.Errorf("models: list models: %w", err)
	}
	byName := make(map[string]store.Model, len(rows))
	for _, m := range rows {
		byName[m.Name] = m
	}
	now := s.cache.now()
	for _, v := range vols {
		name := s.modelFromVolume(v.Name)
		if name == "" || !strings.HasSuffix(v.Name, cacheVolumeSuffix) {
			continue
		}
		node.Entries = append(node.Entries, s.cacheEntry(ctx, v, name, byName, days, now))
	}
	markDuplicates(node.Entries)
	tallyCache(&node)
	if s.preflight.diskFree != nil {
		if free, _, ok := s.preflight.diskFree(ctx, s.localID); ok {
			node.DiskFreeBytes = &free
		}
	}
	return node, nil
}

func (s *Service) cacheEntry(ctx context.Context, v docker.NamedVolume, name string, rows map[string]store.Model, days int, now time.Time) CacheEntry {
	e := CacheEntry{Volume: v.Name, InUse: v.Mounted}
	if v.SizeBytes >= 0 {
		size := v.SizeBytes
		e.SizeBytes = &size
	}
	created, _ := time.Parse(time.RFC3339Nano, v.CreatedAt)
	m, configured := rows[name]
	if configured {
		e.Model, e.Engine, e.ModelRef, e.Configured = m.Name, m.Engine, m.ModelRef, true
		e.LastUsedAt, e.LastUsedSource = s.modelLastUsed(ctx, m, days, now)
	} else if !created.IsZero() {
		e.LastUsedAt, e.LastUsedSource = &created, "volume_created"
	} else {
		e.LastUsedSource = "unknown"
	}
	if e.LastUsedAt != nil {
		e.UnusedDays = max(int(now.Sub(*e.LastUsedAt).Hours()/hoursPerDay), 0)
		e.Unused = e.UnusedDays >= days
	}
	switch {
	case e.InUse:
		e.KeepReason = "mounted by a container"
	case e.Configured:
		e.KeepReason = "used by model " + e.Model
	case e.LastUsedAt == nil:
		e.KeepReason = "last use is unknown"
	case !e.Unused:
		e.KeepReason = fmt.Sprintf("used within the last %d days", days)
	default:
		e.Prunable = true
	}
	return e
}

func (s *Service) modelLastUsed(ctx context.Context, m store.Model, days int, now time.Time) (*time.Time, string) {
	from := now.Add(-time.Duration(days+1) * hoursPerDay * time.Hour)
	rows, err := s.store.ListModelUsage(ctx, m.Name, from, now.Add(time.Hour))
	if err != nil {
		slog.Warn("models: cache last-use lookup", slog.String("model", m.Name), slog.String("error", err.Error()))
	}
	var last time.Time
	for _, r := range rows {
		if r.Requests > 0 && r.HourStart.After(last) {
			last = r.HourStart
		}
	}
	if !last.IsZero() {
		return &last, "gateway_traffic"
	}
	updated := m.UpdatedAt
	return &updated, "model_updated"
}

// markDuplicates flags volumes caching the same engine and model
// reference; the largest of each group is canonical.
func markDuplicates(entries []CacheEntry) {
	type group struct{ canonical int }
	groups := map[string]*group{}
	size := func(e CacheEntry) int64 {
		if e.SizeBytes == nil {
			return 0
		}
		return *e.SizeBytes
	}
	for i, e := range entries {
		if e.ModelRef == "" {
			continue
		}
		key := e.Engine + "|" + strings.ToLower(e.ModelRef)
		g, ok := groups[key]
		if !ok {
			groups[key] = &group{canonical: i}
			continue
		}
		if size(e) > size(entries[g.canonical]) {
			g.canonical = i
		}
	}
	for i, e := range entries {
		if e.ModelRef == "" {
			continue
		}
		g := groups[e.Engine+"|"+strings.ToLower(e.ModelRef)]
		if g.canonical != i {
			entries[i].DuplicateOf = entries[g.canonical].Volume
		}
	}
}

func tallyCache(n *CacheNode) {
	sort.Slice(n.Entries, func(i, j int) bool {
		a, b := n.Entries[i], n.Entries[j]
		if a.Prunable != b.Prunable {
			return a.Prunable
		}
		return a.Volume < b.Volume
	})
	for _, e := range n.Entries {
		var size int64
		if e.SizeBytes != nil {
			size = *e.SizeBytes
		}
		n.TotalBytes += size
		if e.DuplicateOf == "" {
			n.UniqueBytes += size
		}
		if e.Prunable {
			n.ReclaimableBytes += size
		}
	}
}

// CachePrune removes prunable volumes: not mounted, not owned by any
// configured model, and unused for CacheUnusedDays. volumes narrows the
// candidates; empty means all. With dryRun nothing is removed.
func (s *Service) CachePrune(ctx context.Context, volumes []string, dryRun bool) (CachePruneResult, error) {
	res := CachePruneResult{DryRun: dryRun, Candidates: []CacheEntry{}, Removed: []string{}, Skipped: []CacheSkip{}}
	if s.cache.backend == nil {
		return res, errors.New("models: cache manager is not configured")
	}
	local, err := s.localCache(ctx, CacheUnusedDays())
	if err != nil {
		return res, err
	}
	want := map[string]bool{}
	for _, v := range volumes {
		want[v] = true
	}
	known := map[string]bool{}
	for _, e := range local.Entries {
		known[e.Volume] = true
		if len(want) > 0 && !want[e.Volume] {
			continue
		}
		if !e.Prunable {
			if len(want) > 0 {
				res.Skipped = append(res.Skipped, CacheSkip{Volume: e.Volume, Reason: e.KeepReason})
			}
			continue
		}
		res.Candidates = append(res.Candidates, e)
	}
	for _, v := range volumes {
		if !known[v] {
			res.Skipped = append(res.Skipped, CacheSkip{Volume: v, Reason: "not a model cache volume on this node"})
		}
	}
	if dryRun {
		return res, nil
	}
	for _, e := range res.Candidates {
		if reason := s.stillPrunable(ctx, e); reason != "" {
			res.Skipped = append(res.Skipped, CacheSkip{Volume: e.Volume, Reason: reason})
			continue
		}
		if err := s.cache.backend.RemoveVolume(ctx, e.Volume); err != nil {
			slog.Warn("models: remove cache volume", slog.String("volume", e.Volume), slog.String("error", err.Error()))
			res.Skipped = append(res.Skipped, CacheSkip{Volume: e.Volume, Reason: "remove failed: " + err.Error()})
			continue
		}
		res.Removed = append(res.Removed, e.Volume)
		if e.SizeBytes != nil {
			res.ReclaimedBytes += *e.SizeBytes
		}
		slog.Info("models: pruned cache volume", slog.String("volume", e.Volume))
	}
	return res, nil
}

// stillPrunable rechecks a candidate right before removal, since a model
// may have been created or a container started since the listing.
func (s *Service) stillPrunable(ctx context.Context, e CacheEntry) string {
	name := s.modelFromVolume(e.Volume)
	if _, err := s.store.GetModel(ctx, name); err == nil {
		return "a model with this name now exists"
	} else if !errors.Is(err, store.ErrModelNotFound) {
		return "could not verify that no model uses this volume: " + err.Error()
	}
	vols, err := s.cache.backend.ListVolumesByPrefix(ctx, e.Volume)
	if err != nil {
		return "could not verify that no container mounts this volume: " + err.Error()
	}
	for _, v := range vols {
		if v.Name == e.Volume && v.Mounted {
			return "mounted by a container"
		}
	}
	return ""
}
