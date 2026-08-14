package network

// This file: TASKS.md 3.4's internal DNS, the half that decides what a
// name means. The half that speaks the DNS wire protocol is in
// dns_server.go.
//
// What this exists to make true, in the words of Phase 3's own exit
// criterion: "you add a second server from the UI, move an app to it, and
// the app's internal database connection keeps working across machines."
// A connection string is a literal string sitting in an environment
// variable in a running container. Nothing re-writes it when the database
// moves to another machine, and nothing can: the container is not
// restarted, the env var is not re-injected, and even if it were, every
// other service holding that string would still have the old one. So the
// string has to have been a name from the start, and the name has to
// resolve to wherever the service is now.
//
// That is the entire design constraint, and it is why records are keyed
// by service name and resolve to the *node's* mesh address rather than to
// anything about the container. A container's identity changes on every
// deploy (new ID, new bridge IP, possibly a new host port); the node's
// mesh address does not change until the service is moved, and moving it
// is exactly the event that should change the answer.

import (
	"fmt"
	"net/netip"
	"sort"
	"strings"
	"sync"
)

// DefaultZoneSuffix is the parent of every internal name. ".internal" is
// reserved by RFC 6762/8375 convention for exactly this (names that are
// only meaningful inside one network and must never resolve publicly),
// which matters more than it sounds: a suffix that could be a real TLD
// means a lookup that misses the mesh resolver silently escapes to the
// public internet instead of failing.
const DefaultZoneSuffix = "internal"

// Zone returns the internal DNS zone for a brand short name, e.g. a
// short name of "Acme" gives "acme.internal".
//
// Derived from a caller-supplied short name rather than a constant
// because CLAUDE.md section 3 forbids the product name appearing in
// source, and because the zone is genuinely user-visible: it appears in
// every connection string a user writes, so it has to follow a rebrand
// rather than being frozen at whatever the name was when this was
// written. Callers pass brand.Brand.ShortName.
//
// A short name with nothing usable in it (empty, or entirely
// punctuation) falls back to "mesh", which is descriptive rather than
// branded and so is safe to hardcode.
func Zone(shortName string) string {
	clean := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			return r
		case r >= 'A' && r <= 'Z':
			return r + ('a' - 'A')
		default:
			return -1
		}
	}, shortName)
	clean = strings.Trim(clean, "-")
	if clean == "" {
		clean = "mesh"
	}
	return clean + "." + DefaultZoneSuffix
}

// Placement is one service's current node assignment, as
// desired_services.node_id / desired_databases.node_id record it
// (migrations/0009_node_placement.sql).
type Placement struct {
	// Service is the name the service is addressed by. This is the
	// user-facing name from app.yaml, which is what makes it usable in a
	// connection string.
	Service string

	// NodeID is the node the service currently runs on. The empty string
	// means the control plane's own node, exactly as 0009's schema
	// defines it, which is why BuildRecords needs to be told which node
	// ID that is rather than being able to infer it.
	NodeID string
}

// Record is one resolvable name.
type Record struct {
	// Name is the fully qualified name, lowercase, with no trailing dot.
	Name    string
	Address netip.Addr

	// Service and NodeID are carried for logging and for the API surface
	// that will show an operator where a name currently points. They are
	// not part of resolution.
	Service string
	NodeID  string
}

// Unresolved is one placement that could not be turned into a record,
// and why.
//
// Returned rather than silently dropped because a missing record is not a
// missing row in a list, it is a connection string that will fail at
// runtime with NXDOMAIN, and the operator needs to be able to see that
// before a container does. This mirrors dynamicSource's own "log and skip
// the broken resource, do not fail the whole pass" handling of an
// unreachable node (cmd/levelrail/main.go), one layer up: the pass still
// produces every record it can.
type Unresolved struct {
	Service string
	NodeID  string
	Reason  string
}

// RecordSet is one complete, level-triggered snapshot of what every
// internal name resolves to.
type RecordSet struct {
	Zone       string
	Records    []Record
	Unresolved []Unresolved
}

// BuildRecords turns placements plus the node inventory into the complete
// record set for zone.
//
// selfID is the control plane's own node ID, needed to resolve the empty
// NodeID that 0009's schema uses for "local". Passing it explicitly
// rather than having this function guess (say, "the first node in the
// list") keeps the one piece of ambient context this needs visible at the
// call site.
//
// Two name families are produced:
//
//   - <service>.<zone> for every placement. This is the one that matters:
//     it is what goes in a connection string.
//   - <node-name>.node.<zone> for every ready node. Under a .node label so
//     a node called "db" can never collide with a service called "db",
//     which is otherwise a genuinely likely name for both.
//
// A duplicate service name is an error rather than a skipped record:
// two services answering to one name means whichever record won would
// silently take the other's traffic, and unlike an unresolved placement
// there is no correct partial answer to give.
func BuildRecords(zone, selfID string, placements []Placement, nodes []NodeInfo) (RecordSet, error) {
	if zone == "" {
		return RecordSet{}, fmt.Errorf("network: build dns records: zone is empty")
	}

	byID := make(map[string]NodeInfo, len(nodes))
	for _, n := range nodes {
		byID[n.ID] = n
	}

	set := RecordSet{Zone: zone}
	seen := make(map[string]string, len(placements))

	for _, p := range placements {
		if p.Service == "" {
			return RecordSet{}, fmt.Errorf("network: build dns records: placement with an empty service name")
		}
		name := strings.ToLower(p.Service) + "." + zone
		if other, dup := seen[name]; dup {
			return RecordSet{}, fmt.Errorf("network: build dns records: %q and %q both resolve to %s",
				other, p.Service, name)
		}
		seen[name] = p.Service

		nodeID := p.NodeID
		if nodeID == "" {
			nodeID = selfID
		}

		node, known := byID[nodeID]
		switch {
		case !known:
			set.Unresolved = append(set.Unresolved, Unresolved{
				Service: p.Service, NodeID: nodeID,
				Reason: "placed on a node that is not in the mesh inventory",
			})
			continue
		case !node.Address.IsValid():
			set.Unresolved = append(set.Unresolved, Unresolved{
				Service: p.Service, NodeID: nodeID,
				Reason: "placed on a node that has no mesh address assigned yet",
			})
			continue
		}

		set.Records = append(set.Records, Record{
			Name: name, Address: node.Address, Service: p.Service, NodeID: nodeID,
		})
	}

	for _, n := range nodes {
		if n.Name == "" || !n.Address.IsValid() {
			continue
		}
		set.Records = append(set.Records, Record{
			Name:    strings.ToLower(n.Name) + ".node." + zone,
			Address: n.Address,
			NodeID:  n.ID,
		})
	}

	// Deterministic order so two builds from the same inputs are equal,
	// which is what lets a caller skip re-publishing an unchanged set.
	sort.Slice(set.Records, func(i, j int) bool { return set.Records[i].Name < set.Records[j].Name })
	sort.Slice(set.Unresolved, func(i, j int) bool { return set.Unresolved[i].Service < set.Unresolved[j].Service })
	return set, nil
}

// Resolver answers name lookups from a record set. Safe for concurrent
// use: SetRecords swaps the whole table under a write lock while lookups
// take a read lock, so a lookup never observes a half-updated table (an
// in-place update would let a service briefly resolve to nothing during
// an ordinary reconcile pass).
type Resolver struct {
	mu      sync.RWMutex
	zone    string
	byName  map[string]Record
	records []Record
}

// NewResolver builds an empty Resolver for zone. An empty Resolver
// answers every lookup with "not found", which is the correct behavior
// before the first reconcile pass has run: NXDOMAIN is a clear failure,
// where a stale or guessed answer would be a silent misroute.
func NewResolver(zone string) *Resolver {
	return &Resolver{zone: zone, byName: map[string]Record{}}
}

// Zone reports the zone this resolver is authoritative for.
func (r *Resolver) Zone() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.zone
}

// SetRecords replaces the entire table. Level-triggered by construction:
// there is no AddRecord/RemoveRecord pair, for the same reason Mesh.Apply
// has no delta form.
func (r *Resolver) SetRecords(set RecordSet) {
	byName := make(map[string]Record, len(set.Records))
	for _, rec := range set.Records {
		byName[normalizeName(rec.Name)] = rec
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if set.Zone != "" {
		r.zone = set.Zone
	}
	r.byName = byName
	r.records = append([]Record(nil), set.Records...)
}

// Lookup resolves name to a mesh address. Accepts the name with or
// without a trailing dot and in any case, since a DNS query arrives
// fully qualified with a trailing dot while a human types neither.
func (r *Resolver) Lookup(name string) (netip.Addr, bool) {
	key := normalizeName(name)
	r.mu.RLock()
	defer r.mu.RUnlock()
	rec, ok := r.byName[key]
	if !ok {
		return netip.Addr{}, false
	}
	return rec.Address, true
}

// Records returns a copy of the current table, for the status API and
// for tests. A copy, not the live slice: handing out the internal slice
// would let a caller mutate resolution state without the lock.
func (r *Resolver) Records() []Record {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]Record(nil), r.records...)
}

// Authoritative reports whether name falls inside this resolver's zone.
// The DNS server uses it to decide between answering NXDOMAIN (a name in
// our zone that does not exist) and REFUSED (a name we have no business
// answering for at all), a distinction that matters to a resolver
// deciding whether to try another upstream.
func (r *Resolver) Authoritative(name string) bool {
	r.mu.RLock()
	zone := r.zone
	r.mu.RUnlock()
	if zone == "" {
		return false
	}
	n := normalizeName(name)
	return n == zone || strings.HasSuffix(n, "."+zone)
}

// normalizeName lowercases a name and strips any trailing dot, so the
// three forms a name can arrive in ("Web.Acme.Internal.", "web.acme.internal",
// "WEB.acme.internal") are one key.
func normalizeName(name string) string {
	return strings.ToLower(strings.TrimSuffix(name, "."))
}
