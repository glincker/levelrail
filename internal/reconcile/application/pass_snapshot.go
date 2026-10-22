package application

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// EnvSnapshotMaxAge bounds how stale a shared container listing may get inside
// one pass, so a long deploy cannot leave later apps reading an old listing.
const EnvSnapshotMaxAge = "APP_RECONCILE_SNAPSHOT_MAX_AGE"

const defaultSnapshotMaxAge = 10 * time.Second

// stopGrace is how long a container gets to exit before Docker kills it.
const stopGrace = 10 * time.Second

// SnapshotMaxAgeFromEnv reads EnvSnapshotMaxAge.
func SnapshotMaxAgeFromEnv(getenv func(string) string) time.Duration {
	if getenv == nil {
		getenv = os.Getenv
	}
	if d, err := time.ParseDuration(getenv(EnvSnapshotMaxAge)); err == nil && d > 0 {
		return d
	}
	return defaultSnapshotMaxAge
}

type nodeListing struct {
	at  time.Time
	all []docker.ContainerState
}

type nodeInspect struct {
	at  time.Time
	set *docker.InspectSet
}

// PassSnapshot shares one container listing per node across a single pass.
// Build a fresh one per pass so nothing outlives it and reconciling stays
// level-triggered; a controller that changes containers invalidates it.
type PassSnapshot struct {
	maxAge time.Duration
	now    func() time.Time

	mu        sync.Mutex
	nodes     map[string]*nodeListing
	inspected map[string]*nodeInspect
	calls     int
}

// NewPassSnapshot builds an empty snapshot; maxAge <= 0 means the default.
func NewPassSnapshot(maxAge time.Duration) *PassSnapshot {
	if maxAge <= 0 {
		maxAge = defaultSnapshotMaxAge
	}
	return &PassSnapshot{maxAge: maxAge, now: time.Now, nodes: map[string]*nodeListing{}, inspected: map[string]*nodeInspect{}}
}

// Lists reports how many real listings the snapshot has issued.
func (s *PassSnapshot) Lists() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// ListByPrefix answers a prefix query for node from the shared listing,
// fetching it from rt when it is missing, stale or invalidated.
func (s *PassSnapshot) ListByPrefix(ctx context.Context, node string, rt docker.Runtime, prefix string) ([]docker.ContainerState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l := s.nodes[node]
	if l == nil || s.now().Sub(l.at) > s.maxAge {
		all, err := rt.ListByPrefix(ctx, "")
		if err != nil {
			return nil, fmt.Errorf("list containers on %q: %w", node, err)
		}
		s.calls++
		l = &nodeListing{at: s.now(), all: all}
		s.nodes[node] = l
	}
	var out []docker.ContainerState
	for _, cs := range l.all {
		if strings.HasPrefix(cs.Name, prefix) {
			out = append(out, cs)
		}
	}
	return out, nil
}

// Prefetch reads the states of names on node in one call, so the controllers
// of the pass answer their own container lookups from it. A runtime that
// cannot batch simply leaves every lookup to the runtime.
func (s *PassSnapshot) Prefetch(ctx context.Context, node string, rt docker.Runtime, names []string) {
	set := docker.FetchInspectSet(ctx, rt, names)
	if set == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inspected[node] = &nodeInspect{at: s.now(), set: set}
	s.calls++
}

// Inspect is rt.InspectByName answered from the prefetched states while they
// are fresh and no container on node has changed since.
func (s *PassSnapshot) Inspect(ctx context.Context, node string, rt docker.Runtime, name string) (*docker.ContainerState, error) {
	s.mu.Lock()
	n := s.inspected[node]
	if n != nil && s.now().Sub(n.at) > s.maxAge {
		delete(s.inspected, node)
		n = nil
	}
	var set *docker.InspectSet
	if n != nil {
		set = n.set
	}
	s.mu.Unlock()
	if state, covered := set.Lookup(name); covered {
		return state, nil
	}
	return rt.InspectByName(ctx, name)
}

// Invalidate drops node's cached views after a container there was created,
// started, stopped or removed.
func (s *PassSnapshot) Invalidate(node string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.nodes, node)
	delete(s.inspected, node)
}

// WithPassSnapshot makes the controller list containers through snap, keyed by
// the node its runtime talks to.
func WithPassSnapshot(snap *PassSnapshot, node string) Option {
	return func(c *Controller) { c.snap, c.snapNode = snap, node }
}

// listPrefix lists containers named with prefix, from the pass snapshot when one is attached.
func (c *Controller) listPrefix(ctx context.Context, prefix string) ([]docker.ContainerState, error) {
	if c.snap == nil {
		return c.runtime.ListByPrefix(ctx, prefix)
	}
	return c.snap.ListByPrefix(ctx, c.snapNode, c.runtime, prefix)
}

// inspectByName is the runtime's InspectByName, from the pass snapshot's batch when it covers name.
func (c *Controller) inspectByName(ctx context.Context, name string) (*docker.ContainerState, error) {
	if c.snap == nil {
		return c.runtime.InspectByName(ctx, name)
	}
	return c.snap.Inspect(ctx, c.snapNode, c.runtime, name)
}

func (c *Controller) invalidateSnapshot() {
	if c.snap != nil {
		c.snap.Invalidate(c.snapNode)
	}
}

func (c *Controller) createContainer(ctx context.Context, spec docker.ContainerSpec) (string, error) {
	defer c.invalidateSnapshot()
	return c.runtime.Create(ctx, spec)
}

func (c *Controller) startContainer(ctx context.Context, id string) error {
	defer c.invalidateSnapshot()
	return c.runtime.Start(ctx, id)
}

func (c *Controller) stopContainer(ctx context.Context, id string) error {
	defer c.invalidateSnapshot()
	return c.runtime.Stop(ctx, id, stopGrace)
}

func (c *Controller) removeContainer(ctx context.Context, id string) error {
	defer c.invalidateSnapshot()
	return c.runtime.Remove(ctx, id, true)
}

type imageIDEntry struct {
	id string
	at time.Time
}

// ImageIDCache remembers the local image ID a digest-pinned reference resolves
// to, per node, for ttl. A digest names fixed content, so re-asking the daemon
// every pass for every app only repeats an answer that cannot change.
type ImageIDCache struct {
	ttl time.Duration
	now func() time.Time

	mu sync.Mutex
	m  map[string]imageIDEntry
}

// NewImageIDCache builds a cache whose entries expire after ttl; ttl <= 0 caches nothing.
func NewImageIDCache(ttl time.Duration) *ImageIDCache {
	return &ImageIDCache{ttl: ttl, now: time.Now, m: map[string]imageIDEntry{}}
}

func (c *ImageIDCache) get(node, ref string) (string, bool) {
	if c == nil || c.ttl <= 0 {
		return "", false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[node+"\x00"+ref]
	if !ok || c.now().Sub(e.at) >= c.ttl {
		return "", false
	}
	return e.id, true
}

func (c *ImageIDCache) put(node, ref, id string) {
	if c == nil || c.ttl <= 0 || id == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[node+"\x00"+ref] = imageIDEntry{id: id, at: c.now()}
}

// WithImageIDCache lets the controller reuse digest-pinned image IDs across passes for node.
func WithImageIDCache(cache *ImageIDCache, node string) Option {
	return func(c *Controller) { c.imageIDs, c.imageNode = cache, node }
}
