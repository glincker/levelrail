package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

// networkPoolWarnPercent is the pool usage above which the doctor warns.
const networkPoolWarnPercent = 80

type networkPoolResource struct {
	CIDR     string `json:"cidr"`
	Enabled  bool   `json:"enabled"`
	Capacity int    `json:"capacity"`
	InUse    int    `json:"in_use"`
	Percent  int    `json:"percent"`
}

type systemNetworkResource struct {
	Name               string    `json:"name"`
	OwnerKind          string    `json:"owner_kind"`
	OwnerName          string    `json:"owner_name"`
	Subnets            []string  `json:"subnets"`
	AttachedContainers int       `json:"attached_containers"`
	Orphaned           bool      `json:"orphaned"`
	Removable          bool      `json:"removable"`
	Reason             string    `json:"reason"`
	Foreign            bool      `json:"foreign_instance"`
	CreatedAt          time.Time `json:"created_at"`
}

type systemNetworksResponse struct {
	Pool        networkPoolResource     `json:"pool"`
	Networks    []systemNetworkResource `json:"networks"`
	OrphanCount int                     `json:"orphan_count"`
}

type systemNetworksPruneRequest struct {
	DryRun bool `json:"dry_run"`
}

type systemNetworksPruneResponse struct {
	DryRun     bool     `json:"dry_run"`
	Candidates []string `json:"candidates"`
	Removed    []string `json:"removed"`
}

type networkAppSource interface {
	ListApps(ctx context.Context) ([]store.App, error)
	GetOrCreateInstanceID(ctx context.Context) (string, error)
}

func (rt *Router) networkInventory(ctx context.Context) (docker.NetworkAdmin, []application.NetworkView, networkPoolResource, error) {
	admin, ok := rt.dockerPinger.(docker.NetworkAdmin)
	if !ok {
		return nil, nil, networkPoolResource{}, fmt.Errorf("container runtime does not expose network details")
	}
	src, ok := rt.appGroups.(networkAppSource)
	if !ok {
		return nil, nil, networkPoolResource{}, fmt.Errorf("store does not expose apps")
	}
	prefix := ""
	if rt.brand != nil {
		prefix = rt.brand.ShortName
	}
	instanceID, err := src.GetOrCreateInstanceID(ctx)
	if err != nil {
		return nil, nil, networkPoolResource{}, fmt.Errorf("instance id: %w", err)
	}
	apps, err := src.ListApps(ctx)
	if err != nil {
		return nil, nil, networkPoolResource{}, fmt.Errorf("list apps: %w", err)
	}
	prefix = application.NetworkPrefixOrDefault(prefix)
	details, err := admin.DescribeNetworks(ctx, prefix+"-app-")
	if err != nil {
		return nil, nil, networkPoolResource{}, err
	}
	views := application.ClassifyNetworks(prefix, instanceID, details, apps, time.Now(), application.NetworkGraceFromEnv())
	usage, err := admin.NetworkPoolUsage(ctx)
	if err != nil {
		return nil, nil, networkPoolResource{}, err
	}
	pool := networkPoolResource{CIDR: usage.Pool, Enabled: usage.Enabled, Capacity: usage.Capacity, InUse: usage.InUse}
	if usage.Capacity > 0 {
		pool.Percent = usage.InUse * 100 / usage.Capacity
	}
	return admin, views, pool, nil
}

func (rt *Router) handleGetSystemNetworks(w http.ResponseWriter, r *http.Request) {
	_, views, pool, err := rt.networkInventory(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	res := systemNetworksResponse{Pool: pool, Networks: make([]systemNetworkResource, 0, len(views))}
	for _, v := range views {
		if v.Orphaned {
			res.OrphanCount++
		}
		subnets := v.Subnets
		if subnets == nil {
			subnets = []string{}
		}
		res.Networks = append(res.Networks, systemNetworkResource{
			Name: v.Name, OwnerKind: v.OwnerKind, OwnerName: v.OwnerName, Subnets: subnets,
			AttachedContainers: v.Attached, Orphaned: v.Orphaned, Removable: v.Removable,
			Reason: v.Reason, Foreign: v.Foreign, CreatedAt: v.Created,
		})
	}
	writeJSON(w, http.StatusOK, res)
}

// handlePruneSystemNetworks removes only networks ClassifyNetworks marks
// removable: ownerless, empty, past the grace period, not another instance's.
func (rt *Router) handlePruneSystemNetworks(w http.ResponseWriter, r *http.Request) {
	var req systemNetworksPruneRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	admin, views, _, err := rt.networkInventory(r.Context())
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	res := systemNetworksPruneResponse{DryRun: req.DryRun, Candidates: []string{}, Removed: []string{}}
	for _, v := range views {
		if !v.Removable {
			continue
		}
		res.Candidates = append(res.Candidates, v.Name)
		if req.DryRun {
			continue
		}
		ok, err := admin.RemoveNetworkIfEmpty(r.Context(), v.Name)
		if err != nil {
			rt.logger.Error("api: prune network failed", slog.String("error", err.Error()), slog.String("network", v.Name))
			continue
		}
		if ok {
			res.Removed = append(res.Removed, v.Name)
		}
	}
	writeJSON(w, http.StatusOK, res)
}

// doctorCheckNetworkPool warns when the per-app network pool is nearly full.
func (rt *Router) doctorCheckNetworkPool(ctx context.Context) doctorCheckResource {
	const code, name = "network_pool", "App network pool"
	admin, ok := rt.dockerPinger.(docker.NetworkAdmin)
	if !ok {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: "not reported by this container runtime"}
	}
	usage, err := admin.NetworkPoolUsage(ctx)
	if err != nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusWarn, Message: "could not read network usage: " + err.Error()}
	}
	if !usage.Enabled {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: "explicit subnet pool disabled, Docker's default address pools apply (about 30 networks)"}
	}
	pct := usage.InUse * 100 / usage.Capacity
	msg := fmt.Sprintf("%d of %d networks in use in %s (%d%%)", usage.InUse, usage.Capacity, usage.Pool, pct)
	if pct > networkPoolWarnPercent {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusWarn,
			Message:  msg + ": remove unused networks or widen " + docker.EnvNetworkPool,
			DocsPath: "/troubleshooting#app-network-pool"}
	}
	return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: msg}
}
