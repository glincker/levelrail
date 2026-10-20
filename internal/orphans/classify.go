// Package orphans is the single definition of a leftover container or volume
// that desired state no longer accounts for. The reaper, the API and the CLI
// all classify through it so they cannot disagree.
package orphans

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Kind is the type of resource an Orphan describes.
type Kind string

// Resource kinds.
const (
	KindContainer Kind = "container"
	KindVolume    Kind = "volume"
	// KindCertificate is a stored TLS certificate for a hostname nothing serves.
	KindCertificate Kind = "certificate"
)

// Reason says why a resource is considered an orphan.
type Reason string

// Orphan reasons.
const (
	ReasonServiceDeleted  Reason = "service_deleted"
	ReasonDatabaseDeleted Reason = "database_deleted"
	ReasonMisplaced       Reason = "misplaced"
	ReasonStaleGeneration Reason = "stale_generation"
	ReasonUnmanaged       Reason = "unmanaged"
	ReasonForeignInstance Reason = "foreign_instance"
	ReasonUnrecognized    Reason = "unrecognized_name"
	ReasonUnreferenced    Reason = "unreferenced_volume"
	ReasonUnserved        Reason = "unserved_certificate"
)

// Skip reasons explain why an orphan is listed but not auto-removable.
const (
	SkipTeardownPending = "app teardown is still pending"
	SkipSupersededBuild = "an older generation of a live app, owned by its reconciler"
	SkipNotOurs         = "not created by this control plane"
	SkipNoRunningCopy   = "the node it is placed on has no running copy yet"
	SkipUnrecognized    = "name does not match a managed container shape"
	SkipDatabaseVolume  = "database data volumes are never removed automatically"
	SkipMounted         = "volume is mounted by a container"
	SkipVolumesOff      = "automatic volume removal is disabled"
)

// Orphan is one leftover resource on one node.
type Orphan struct {
	Kind      Kind   `json:"kind"`
	Name      string `json:"name"`
	ID        string `json:"id,omitempty"`
	NodeID    string `json:"node_id"`
	Reason    Reason `json:"reason"`
	Image     string `json:"image,omitempty"`
	SizeBytes int64  `json:"size_bytes,omitempty"`
	// PlacedOn is the node desired state now places a misplaced container on.
	PlacedOn string `json:"placed_on,omitempty"`
	// Skip is non-empty when the reaper must leave this resource alone.
	Skip string `json:"skip,omitempty"`
}

// Reapable reports whether the reaper may remove o once its grace elapsed.
func (o Orphan) Reapable() bool { return o.Skip == "" }

// Desired is the slice of desired state classification needs.
type Desired struct {
	Services       map[string]bool
	ContainerNames map[string]bool
	// ContainerNode maps each desired container name to the node it belongs on.
	ContainerNode   map[string]string
	Databases       map[string]bool
	VolumeNames     map[string]bool
	PendingTeardown map[string]bool
}

// Empty reports whether nothing at all is desired.
func (d Desired) Empty() bool { return len(d.Services) == 0 && len(d.Databases) == 0 }

// Source is the persistence LoadDesired reads. *store.DB satisfies it.
type Source interface {
	ListDesiredServices(ctx context.Context) ([]store.DesiredService, error)
	ListDesiredDatabases(ctx context.Context) ([]store.DesiredDatabase, error)
	ListPendingTeardowns(ctx context.Context) ([]store.PendingTeardown, error)
}

// DesiredServiceContainerNames is every replica container name svc's desired
// state can produce.
func DesiredServiceContainerNames(svc store.DesiredService) []string {
	replicas := svc.Replicas
	if replicas < 1 {
		replicas = 1
	}
	base := application.ContainerName(svc.Name, application.NameImage(svc), svc.RestartNonce)
	names := make([]string, 0, replicas)
	names = append(names, base)
	for i := 1; i < replicas; i++ {
		names = append(names, base+"-r"+strconv.Itoa(i))
	}
	return names
}

// LoadDesired snapshots desired state from src. normNode maps a stored node ID
// to the ID the scanner uses for it ("" for the control plane's own node).
func LoadDesired(ctx context.Context, src Source, normNode func(string) string) (Desired, error) {
	if normNode == nil {
		normNode = func(id string) string { return id }
	}
	services, err := src.ListDesiredServices(ctx)
	if err != nil {
		return Desired{}, fmt.Errorf("list desired services: %w", err)
	}
	databases, err := src.ListDesiredDatabases(ctx)
	if err != nil {
		return Desired{}, fmt.Errorf("list desired databases: %w", err)
	}
	pending, err := src.ListPendingTeardowns(ctx)
	if err != nil {
		return Desired{}, fmt.Errorf("list pending teardowns: %w", err)
	}
	d := Desired{
		Services:        make(map[string]bool, len(services)),
		ContainerNames:  make(map[string]bool, len(services)+len(databases)),
		ContainerNode:   make(map[string]string, len(services)+len(databases)),
		Databases:       make(map[string]bool, len(databases)),
		VolumeNames:     make(map[string]bool, len(services)+len(databases)*3),
		PendingTeardown: make(map[string]bool, len(pending)),
	}
	for _, svc := range services {
		d.Services[svc.Name] = true
		for _, n := range DesiredServiceContainerNames(svc) {
			d.ContainerNames[n] = true
			d.ContainerNode[n] = normNode(svc.NodeID)
		}
		for _, v := range svc.Volumes {
			d.VolumeNames[v.Name] = true
		}
	}
	for _, db := range databases {
		d.Databases[db.Name] = true
		d.ContainerNames["db-"+db.Name] = true
		d.ContainerNode["db-"+db.Name] = normNode(db.NodeID)
		for _, suffix := range []string{"-data", "-certs", "-wal-archive"} {
			d.VolumeNames["db-"+db.Name+suffix] = true
		}
	}
	for _, p := range pending {
		d.PendingTeardown[p.Name] = true
	}
	return d, nil
}

const egressSuffix = "-egress"

// ClassifyContainer reports whether c is an orphan. instanceID is this
// control plane's own instance label, empty when instances are not scoped.
func ClassifyContainer(c docker.ContainerState, d Desired, instanceID, nodeID string) (Orphan, bool) {
	label := c.Labels[spec.InstanceLabelKey]
	base := strings.TrimSuffix(c.Name, egressSuffix)
	o := Orphan{Kind: KindContainer, Name: c.Name, ID: c.ID, NodeID: nodeID, Image: c.Image}
	if label != "" {
		desiredName := base
		if d.ContainerNames[c.Name] {
			desiredName = c.Name
		}
		if d.ContainerNames[desiredName] {
			if want := d.ContainerNode[desiredName]; want != nodeID && (instanceID == "" || label == instanceID) {
				o.Reason, o.PlacedOn = ReasonMisplaced, want
				return o, true
			}
			return Orphan{}, false
		}
	}
	switch {
	case label == "":
		o.Reason, o.Skip = ReasonUnmanaged, SkipNotOurs
		return o, true
	case instanceID != "" && label != instanceID:
		o.Reason, o.Skip = ReasonForeignInstance, SkipNotOurs
		return o, true
	}
	if svc, ok := application.ParseContainerName(c.Name); ok {
		switch {
		case d.Services[svc]:
			o.Reason, o.Skip = ReasonStaleGeneration, SkipSupersededBuild
		case d.PendingTeardown[svc]:
			o.Reason, o.Skip = ReasonServiceDeleted, SkipTeardownPending
		default:
			o.Reason = ReasonServiceDeleted
		}
		return o, true
	}
	if dbName, ok := strings.CutPrefix(c.Name, "db-"); ok && dbName != "" {
		o.Reason = ReasonDatabaseDeleted
		return o, true
	}
	o.Reason, o.Skip = ReasonUnrecognized, SkipUnrecognized
	return o, true
}

// ClassifyVolume reports whether v is an orphan. reapVolumes is the
// operator's opt-in for automatic volume removal.
func ClassifyVolume(v docker.NamedVolume, d Desired, nodeID string, reapVolumes bool) (Orphan, bool) {
	if d.VolumeNames[v.Name] {
		return Orphan{}, false
	}
	o := Orphan{Kind: KindVolume, Name: v.Name, NodeID: nodeID, Reason: ReasonUnreferenced}
	if v.SizeBytes > 0 {
		o.SizeBytes = v.SizeBytes
	}
	switch {
	case strings.HasPrefix(v.Name, "db-"):
		o.Skip = SkipDatabaseVolume
	case v.Mounted:
		o.Skip = SkipMounted
	case !reapVolumes:
		o.Skip = SkipVolumesOff
	}
	return o, true
}

// IsManaged is the shared "belongs to a live app or database" test the
// container list uses.
func IsManaged(c docker.ContainerState, d Desired, instanceID string) bool {
	_, orphan := ClassifyContainer(c, d, instanceID, "")
	return !orphan
}

// ServedSet turns served hostnames into the storage names certificates are
// filed under. A wildcard cert is kept when any host it could cover is served.
func ServedSet(hosts []string) map[string]bool {
	set := make(map[string]bool, len(hosts)*2)
	for _, h := range hosts {
		h = strings.ToLower(strings.TrimSpace(h))
		set[strings.ReplaceAll(h, "*", "wildcard_")] = true
		if _, parent, ok := strings.Cut(h, "."); ok && !strings.HasPrefix(h, "*") {
			set["wildcard_."+parent] = true
		}
	}
	return set
}

// ClassifyCertificate reports whether the certificate filed under domain (a
// Caddy storage directory name) is for a hostname nothing serves.
func ClassifyCertificate(issuer, domain string, served map[string]bool) (Orphan, bool) {
	if served[strings.ToLower(domain)] {
		return Orphan{}, false
	}
	return Orphan{Kind: KindCertificate, Name: issuer + "/" + domain, Reason: ReasonUnserved}, true
}
