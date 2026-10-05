package orphans

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Store is the persistence the Reaper needs. *store.DB satisfies it.
type Store interface {
	Source
	RecordOrphanSighting(ctx context.Context, kind, nodeID, name string, at time.Time) error
	ClearOrphanSighting(ctx context.Context, kind, nodeID, name string) error
	ListOrphanSightings(ctx context.Context) ([]store.OrphanSighting, error)
	SaveAuditEntry(ctx context.Context, e store.AuditEntry) error
}

// VolumeBackend lists and removes named volumes on the control plane's own
// node. *docker.Client satisfies it. Remote nodes are containers-only.
type VolumeBackend interface {
	ListNamedVolumes(ctx context.Context) ([]docker.NamedVolume, error)
	RemoveVolume(ctx context.Context, name string) error
}

// CertBackend lists and deletes stored certificate files. *store.DB satisfies it.
type CertBackend interface {
	ListCertStorageKeys(ctx context.Context, prefix string, recursive bool) ([]string, error)
	DeleteCertStorageValue(ctx context.Context, key string) error
}

// Deps are the Reaper's collaborators.
type Deps struct {
	Store    Store
	NodeIDs  func(ctx context.Context) ([]string, error)
	NormNode func(nodeID string) string
	Resolve  func(nodeID string) (docker.Runtime, error)
	Volumes  VolumeBackend
	Certs    CertBackend
	// ServedHosts reports the hostnames ingress serves; known is false until it has applied a config.
	ServedHosts func(ctx context.Context) (hosts []string, known bool, err error)
	InstanceID  string
	Config      Config
	Logger      *slog.Logger
	Now         func() time.Time
}

// Reaper removes orphans once they have stayed orphaned past the grace period.
type Reaper struct {
	Deps
	mu      sync.Mutex
	lastRun time.Time
}

// New builds a Reaper.
func New(d Deps) *Reaper {
	if d.Logger == nil {
		d.Logger = slog.Default()
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	return &Reaper{Deps: d}
}

// Finding is an Orphan plus where it stands against the grace period.
type Finding struct {
	Orphan
	FirstSeenAt *time.Time `json:"first_seen_at,omitempty"`
	ReapAfter   *time.Time `json:"reap_after,omitempty"`
	Due         bool       `json:"due"`
}

// Failure is an orphan the reaper could not remove.
type Failure struct {
	Orphan
	Error string `json:"error"`
}

// Report is what one pass saw and did.
type Report struct {
	DryRun   bool      `json:"dry_run"`
	Findings []Finding `json:"findings"`
	Removed  []Orphan  `json:"removed"`
	Failed   []Failure `json:"failed,omitempty"`
	// Unreachable lists nodes that could not be scanned this pass.
	Unreachable []string `json:"unreachable,omitempty"`
	// Halted explains why removal was withheld, empty when it was not.
	Halted string `json:"halted,omitempty"`
}

type sightingKey struct {
	kind   Kind
	nodeID string
	name   string
}

// Scan lists every orphan and its grace status without removing anything or
// writing any state.
func (r *Reaper) Scan(ctx context.Context) (Report, error) {
	return r.scan(ctx, false)
}

func (r *Reaper) scan(ctx context.Context, record bool) (Report, error) {
	rep := Report{Findings: []Finding{}, Removed: []Orphan{}}
	desired, err := LoadDesired(ctx, r.Store, r.NormNode)
	if err != nil {
		return rep, fmt.Errorf("orphans: %w", err)
	}
	sightings, err := r.Store.ListOrphanSightings(ctx)
	if err != nil {
		return rep, fmt.Errorf("orphans: %w", err)
	}
	seenAt := make(map[sightingKey]time.Time, len(sightings))
	for _, s := range sightings {
		seenAt[sightingKey{Kind(s.Kind), s.NodeID, s.Name}] = s.FirstSeenAt
	}
	nodeIDs, err := r.NodeIDs(ctx)
	if err != nil {
		return rep, fmt.Errorf("orphans: list nodes: %w", err)
	}

	now := r.Now()
	live := map[sightingKey]bool{}
	scannedNode := map[string]bool{}
	listed := make(map[string]bool, len(nodeIDs))
	for _, id := range nodeIDs {
		listed[id] = true
	}
	add := func(o Orphan) {
		k := sightingKey{o.Kind, o.NodeID, o.Name}
		live[k] = true
		f := Finding{Orphan: o}
		if o.Reapable() {
			first, ok := seenAt[k]
			if !ok {
				first = now
				if record {
					if err := r.Store.RecordOrphanSighting(ctx, string(o.Kind), o.NodeID, o.Name, now); err != nil {
						r.Logger.Warn("orphans: record sighting", slog.String("error", err.Error()), slog.String("name", o.Name))
					}
				}
			}
			after := first.Add(r.Config.graceFor(o.Kind))
			f.FirstSeenAt, f.ReapAfter, f.Due = &first, &after, !now.Before(after) && ok
		}
		rep.Findings = append(rep.Findings, f)
	}

	containers := map[string][]docker.ContainerState{}
	running := map[string]map[string]bool{}
	for _, id := range nodeIDs {
		rt, err := r.Resolve(id)
		if err == nil {
			var cs []docker.ContainerState
			if cs, err = rt.ListByPrefix(ctx, ""); err == nil {
				scannedNode[id] = true
				containers[id] = cs
				running[id] = map[string]bool{}
				for _, c := range cs {
					if c.Running {
						running[id][c.Name] = true
					}
				}
			}
		}
		if err != nil {
			r.Logger.Warn("orphans: node not scanned", slog.String("node_id", id), slog.String("error", err.Error()))
			rep.Unreachable = append(rep.Unreachable, id)
		}
	}
	for _, id := range nodeIDs {
		for _, c := range containers[id] {
			o, isOrphan := ClassifyContainer(c, desired, r.InstanceID, id)
			if !isOrphan {
				continue
			}
			if o.Reason == ReasonMisplaced && !running[o.PlacedOn][o.Name] {
				o.Skip = SkipNoRunningCopy
			}
			add(o)
		}
	}
	if r.Volumes != nil {
		vols, err := r.Volumes.ListNamedVolumes(ctx)
		if err != nil {
			r.Logger.Warn("orphans: volumes not scanned", slog.String("error", err.Error()))
		} else {
			for _, v := range vols {
				if o, isOrphan := ClassifyVolume(v, desired, "", r.Config.ReapVolumes); isOrphan {
					add(o)
				}
			}
		}
	}

	if r.Certs != nil && r.ServedHosts != nil && !desired.Empty() {
		r.scanCertificates(ctx, add)
	}

	if record {
		for k := range seenAt {
			if live[k] || (k.kind == KindContainer && listed[k.nodeID] && !scannedNode[k.nodeID]) {
				continue
			}
			if err := r.Store.ClearOrphanSighting(ctx, string(k.kind), k.nodeID, k.name); err != nil {
				r.Logger.Warn("orphans: clear sighting", slog.String("error", err.Error()), slog.String("name", k.name))
			}
		}
	}
	if desired.Empty() && len(rep.Findings) > 0 {
		rep.Halted = "nothing is desired at all; refusing to remove anything until an operator confirms"
	}
	sort.Slice(rep.Findings, func(i, j int) bool {
		a, b := rep.Findings[i], rep.Findings[j]
		if a.NodeID != b.NodeID {
			return a.NodeID < b.NodeID
		}
		return a.Name < b.Name
	})
	return rep, nil
}

// Reap runs one pass: it records sightings, then removes every due orphan
// unless dryRun. At most Config.MaxPerPass resources are removed per pass.
func (r *Reaper) Reap(ctx context.Context, dryRun bool) (Report, error) {
	rep, err := r.scan(ctx, true)
	rep.DryRun = dryRun
	if err != nil {
		return rep, err
	}
	if dryRun || rep.Halted != "" {
		return rep, nil
	}
	removed := 0
	for _, f := range rep.Findings {
		if !f.Due {
			continue
		}
		if removed >= r.Config.MaxPerPass {
			rep.Halted = fmt.Sprintf("per-pass limit of %d reached; the rest is removed on later passes", r.Config.MaxPerPass)
			break
		}
		if err := r.remove(ctx, f.Orphan); err != nil {
			rep.Failed = append(rep.Failed, Failure{Orphan: f.Orphan, Error: err.Error()})
			r.Logger.Error("orphans: remove failed", slog.String("kind", string(f.Kind)), slog.String("name", f.Name), slog.String("node_id", f.NodeID), slog.String("error", err.Error()))
			continue
		}
		removed++
		rep.Removed = append(rep.Removed, f.Orphan)
		r.audit(ctx, f.Orphan)
		if err := r.Store.ClearOrphanSighting(ctx, string(f.Kind), f.NodeID, f.Name); err != nil {
			r.Logger.Warn("orphans: clear sighting", slog.String("error", err.Error()), slog.String("name", f.Name))
		}
	}
	return rep, nil
}

func (r *Reaper) scanCertificates(ctx context.Context, add func(Orphan)) {
	hosts, known, err := r.ServedHosts(ctx)
	if err != nil || !known {
		if err != nil {
			r.Logger.Warn("orphans: served hosts unavailable, certificates not scanned", slog.String("error", err.Error()))
		}
		return
	}
	served := ServedSet(hosts)
	keys, err := r.Certs.ListCertStorageKeys(ctx, "certificates", true)
	if err != nil {
		r.Logger.Warn("orphans: certificates not scanned", slog.String("error", err.Error()))
		return
	}
	seen := map[string]bool{}
	for _, key := range keys {
		parts := strings.Split(key, "/")
		if len(parts) < 4 || parts[0] != "certificates" || seen[parts[1]+"/"+parts[2]] {
			continue
		}
		seen[parts[1]+"/"+parts[2]] = true
		if o, isOrphan := ClassifyCertificate(parts[1], parts[2], served); isOrphan {
			add(o)
		}
	}
}

func (r *Reaper) removeCertificate(ctx context.Context, o Orphan) error {
	keys, err := r.Certs.ListCertStorageKeys(ctx, "certificates/"+o.Name, true)
	if err != nil {
		return fmt.Errorf("list certificate files: %w", err)
	}
	for _, k := range keys {
		if err := r.Certs.DeleteCertStorageValue(ctx, k); err != nil {
			return fmt.Errorf("delete %s: %w", k, err)
		}
	}
	return nil
}

func (r *Reaper) remove(ctx context.Context, o Orphan) error {
	switch o.Kind {
	case KindCertificate:
		if r.Certs == nil {
			return errors.New("no certificate backend")
		}
		return r.removeCertificate(ctx, o)
	case KindVolume:
		if r.Volumes == nil {
			return errors.New("no volume backend")
		}
		return r.Volumes.RemoveVolume(ctx, o.Name)
	case KindContainer:
		rt, err := r.Resolve(o.NodeID)
		if err != nil {
			return fmt.Errorf("resolve node %q: %w", o.NodeID, err)
		}
		if err := rt.Remove(ctx, o.ID, true); err != nil {
			return fmt.Errorf("remove container: %w", err)
		}
		return nil
	}
	return fmt.Errorf("unknown orphan kind %q", o.Kind)
}

func (r *Reaper) audit(ctx context.Context, o Orphan) {
	id, err := store.NewAuditEntryID()
	if err != nil {
		r.Logger.Warn("orphans: audit id", slog.String("error", err.Error()))
		return
	}
	e := store.AuditEntry{
		ID: id, ActorType: "system", ActorName: "orphan-reaper", Ability: "root",
		Method: "REAP", Path: fmt.Sprintf("/system/orphans/%s/%s?node=%s&reason=%s", o.Kind, o.Name, o.NodeID, o.Reason),
		StatusCode: 200, CreatedAt: store.FormatAuditTime(r.Now()), ClientKind: "system",
	}
	if err := r.Store.SaveAuditEntry(ctx, e); err != nil {
		r.Logger.Warn("orphans: audit entry", slog.String("error", err.Error()), slog.String("name", o.Name))
	}
	r.Logger.Info("orphans: removed", slog.String("kind", string(o.Kind)), slog.String("name", o.Name), slog.String("node_id", o.NodeID), slog.String("reason", string(o.Reason)))
}

// due reports whether a scheduled pass should run now and claims the slot.
// Reconcile passes fire on every Docker event, far more often than the
// grace period needs.
func (r *Reaper) due() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.Now()
	if !r.lastRun.IsZero() && now.Sub(r.lastRun) < r.Config.Interval {
		return false
	}
	r.lastRun = now
	return true
}

// Controller adapts a Reaper to the reconcile loop.
type Controller struct{ reaper *Reaper }

// NewController wraps r.
func NewController(r *Reaper) *Controller { return &Controller{reaper: r} }

// Name implements reconcile.Controller.
func (c *Controller) Name() string { return "orphans/reaper" }

// Reconcile implements reconcile.Controller.
func (c *Controller) Reconcile(ctx context.Context) (reconcile.Result, error) {
	mode := c.reaper.Config.Mode
	if mode == ModeOff {
		return result(reconcile.ConditionTrue, "Disabled", ""), nil
	}
	if !c.reaper.due() {
		return result(reconcile.ConditionTrue, "Converged", "waiting for the next scheduled pass"), nil
	}
	rep, err := c.reaper.Reap(ctx, mode == ModeDryRun)
	if err != nil {
		return result(reconcile.ConditionFalse, "ScanFailed", err.Error()), fmt.Errorf("orphans/reaper: %w", err)
	}
	if len(rep.Failed) > 0 {
		msg := fmt.Sprintf("%d orphan(s) could not be removed, first: %s: %s", len(rep.Failed), rep.Failed[0].Name, rep.Failed[0].Error)
		return result(reconcile.ConditionTrue, "ReapFailed", msg), fmt.Errorf("orphans/reaper: %s", msg)
	}
	if mode == ModeDryRun && len(rep.Findings) > 0 {
		return result(reconcile.ConditionTrue, "DryRun", fmt.Sprintf("%d orphan(s) found, none removed", len(rep.Findings))), nil
	}
	return result(reconcile.ConditionTrue, "Converged", ""), nil
}

func result(s reconcile.ConditionStatus, reason, msg string) reconcile.Result {
	return reconcile.Result{Conditions: []reconcile.Condition{{Type: reconcile.ConditionTypeReady, Status: s, Reason: reason, Message: msg}}}
}
