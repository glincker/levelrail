package appsleep

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

// FleetStore is what Load needs, each call one query for the whole fleet.
type FleetStore interface {
	ListDesiredServices(ctx context.Context) ([]store.DesiredService, error)
	ListNodes(ctx context.Context) ([]store.Node, error)
	ListCanaryReleases(ctx context.Context) ([]store.CanaryRelease, error)
	ListDesiredDatabases(ctx context.Context) ([]store.DesiredDatabase, error)
	CountAppStreamsByService(ctx context.Context) (map[string]int, error)
}

// PortWaker is the port wake surface eligibility and the API read.
type PortWaker interface {
	// Supported says whether port wake can hold svc's pinned ports, and if not why.
	Supported(svc *store.DesiredService) (bool, string)
	// Status reports the live listener state for name.
	Status(name string) PortWakeStatus
}

// PortWakeStatus is what the manager knows about one app's pinned ports.
type PortWakeStatus struct {
	Active bool
	Ports  []int
	// Error is the last bind failure, empty when the listeners are fine.
	Error string
}

// Fleet is a snapshot of the facts Evaluate needs, loaded once so listing
// many apps costs a fixed number of queries.
type Fleet struct {
	Services  map[string]store.DesiredService
	nodes     map[string]store.Node
	canary    map[string]bool
	databases map[string]bool
	streams   map[string]int
	dependsOn map[string][]string
}

// Load reads the fleet snapshot.
func Load(ctx context.Context, fs FleetStore) (*Fleet, error) {
	svcs, err := fs.ListDesiredServices(ctx)
	if err != nil {
		return nil, fmt.Errorf("appsleep: list apps: %w", err)
	}
	nodes, err := fs.ListNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("appsleep: list nodes: %w", err)
	}
	canaries, err := fs.ListCanaryReleases(ctx)
	if err != nil {
		return nil, fmt.Errorf("appsleep: list canaries: %w", err)
	}
	dbs, err := fs.ListDesiredDatabases(ctx)
	if err != nil {
		return nil, fmt.Errorf("appsleep: list databases: %w", err)
	}
	streams, err := fs.CountAppStreamsByService(ctx)
	if err != nil {
		return nil, fmt.Errorf("appsleep: count streams: %w", err)
	}
	f := &Fleet{Services: map[string]store.DesiredService{}, nodes: map[string]store.Node{}, canary: map[string]bool{}, databases: map[string]bool{}, streams: streams, dependsOn: map[string][]string{}}
	for _, s := range svcs {
		f.Services[s.Name] = s
	}
	for _, n := range nodes {
		f.nodes[n.ID] = n
	}
	for _, c := range canaries {
		f.canary[c.ServiceName] = true
		f.canary[c.CanaryService] = true
	}
	for _, d := range dbs {
		f.databases[d.Name] = true
	}
	for _, s := range svcs {
		if s.AppID == "" {
			continue
		}
		for _, dep := range append(append([]string{}, s.DependsOn...), s.DependsOnHealthy...) {
			target := s.AppID + "-" + dep
			f.dependsOn[target] = append(f.dependsOn[target], s.Name)
		}
	}
	for k := range f.dependsOn {
		sort.Strings(f.dependsOn[k])
	}
	return f, nil
}

var statefulImages = []string{"postgres", "mysql", "mariadb", "mongo", "redis", "valkey", "clickhouse", "cockroach", "cassandra", "influxdb", "timescale", "elasticsearch", "opensearch"}

// PinnedPorts lists the host ports svc pins.
func PinnedPorts(svc *store.DesiredService) []int {
	if svc.HostPort != nil && *svc.HostPort != 0 {
		return []int{*svc.HostPort}
	}
	return nil
}

// Facts gathers the inputs Evaluate needs for svc under setting. pw may be nil.
func (f *Fleet) Facts(svc *store.DesiredService, setting store.AppSleep, pw PortWaker) Facts {
	facts := Facts{
		App: svc.Name, IdleMinutes: setting.IdleMinutes, PinnedPorts: PinnedPorts(svc), PortWakeEnabled: setting.PortWake,
		IsManagedDatabase: f.databases[svc.Name], NodeUnschedulable: f.nodeUnschedulable(svc.NodeID),
		CanaryTarget: f.canary[svc.Name], DependedOnBy: f.dependsOn[svc.Name], HoldRequests: setting.HoldRequests,
		HasDomain: len(svc.Domains) > 0, HasStreams: f.streams[svc.Name] > 0, IsTrial: svc.IsTrial, Replicas: svc.Replicas,
		LastWakeError: setting.WakeError, StatefulImage: isStatefulImage(svc.Image),
	}
	if pw != nil && len(facts.PinnedPorts) > 0 {
		facts.PortWakeSupported, facts.PortWakeUnsupportedReason = pw.Supported(svc)
		facts.PortWakeError = pw.Status(svc.Name).Error
	} else if len(facts.PinnedPorts) > 0 {
		facts.PortWakeUnsupportedReason = "port wake is not running on this control plane"
	}
	return facts
}

func (f *Fleet) nodeUnschedulable(id string) bool {
	if id == "" {
		return false
	}
	n, ok := f.nodes[id]
	return ok && !n.Schedulable
}

func isStatefulImage(image string) bool {
	repo := strings.ToLower(image)
	if i := strings.LastIndex(repo, "/"); i >= 0 {
		repo = repo[i+1:]
	}
	if i := strings.IndexAny(repo, ":@"); i >= 0 {
		repo = repo[:i]
	}
	for _, s := range statefulImages {
		if strings.Contains(repo, s) {
			return true
		}
	}
	return false
}
