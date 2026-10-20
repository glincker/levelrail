package application

import (
	"os"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	appspec "github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	// EnvNetworkGCGrace is how old an ownerless network must be before GC removes it.
	EnvNetworkGCGrace   = "APP_NETWORK_GC_GRACE"
	defaultNetworkGrace = 10 * time.Minute
)

// Reasons a network is not removable, surfaced by the diagnostics API.
const (
	NetworkKeepOwned    = "owner_exists"
	NetworkKeepForeign  = "foreign_instance"
	NetworkKeepInUse    = "containers_attached"
	NetworkKeepGrace    = "within_grace_period"
	NetworkKeepNoApps   = "no_apps_in_store"
	NetworkRemovable    = "removable"
	networkOwnerKindApp = "app"
)

// NetworkGraceFromEnv reads APP_NETWORK_GC_GRACE, falling back to 10m.
func NetworkGraceFromEnv() time.Duration {
	if v, ok := os.LookupEnv(EnvNetworkGCGrace); ok {
		if d, err := time.ParseDuration(strings.TrimSpace(v)); err == nil && d >= 0 {
			return d
		}
	}
	return defaultNetworkGrace
}

// NetworkPrefixOrDefault applies the same fallback NetworkName uses.
func NetworkPrefixOrDefault(prefix string) string {
	if prefix == "" {
		return defaultNetworkPrefix
	}
	return prefix
}

// NetworkView is one product network with its GC verdict.
type NetworkView struct {
	ID        string
	Name      string
	OwnerKind string
	OwnerName string
	Subnets   []string
	Attached  int
	Created   time.Time
	Labeled   bool
	Foreign   bool
	Orphaned  bool
	Removable bool
	Reason    string
}

// ClassifyNetworks decides, per network, whether it is orphaned and safe
// to remove. Networks of another instance are never removable, attached
// containers always block removal, and unlabeled legacy networks are
// only removable when this store has at least one app (an empty store
// cannot tell its own leftovers from a sibling instance's idle network).
func ClassifyNetworks(prefix, instanceID string, details []docker.NetworkDetail, apps []store.App, now time.Time, grace time.Duration) []NetworkView {
	if prefix == "" {
		prefix = defaultNetworkPrefix
	}
	wanted := make(map[string]bool, len(apps))
	for _, a := range apps {
		wanted[NetworkName(prefix, a.ID)] = true
	}
	infix := prefix + "-app-"
	out := make([]NetworkView, 0, len(details))
	for _, d := range details {
		if !strings.HasPrefix(d.Name, infix) {
			continue
		}
		v := NetworkView{ID: d.ID, Name: d.Name, OwnerKind: networkOwnerKindApp, OwnerName: strings.TrimPrefix(d.Name, infix),
			Subnets: d.Subnets, Attached: d.Attached, Created: d.Created}
		owner, labeled := d.Labels[appspec.InstanceLabelKey]
		v.Labeled = labeled
		v.Foreign = instanceID != "" && labeled && owner != instanceID
		v.Orphaned = !wanted[d.Name]
		switch {
		case v.Foreign:
			v.Reason = NetworkKeepForeign
			v.Orphaned = false
		case !v.Orphaned:
			v.Reason = NetworkKeepOwned
		case d.Attached > 0:
			v.Reason = NetworkKeepInUse
		case !d.Created.IsZero() && now.Sub(d.Created) < grace:
			v.Reason = NetworkKeepGrace
		case instanceID != "" && !labeled && len(apps) == 0:
			v.Reason = NetworkKeepNoApps
		default:
			v.Reason = NetworkRemovable
			v.Removable = true
		}
		out = append(out, v)
	}
	return out
}
