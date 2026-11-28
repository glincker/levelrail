package api

import (
	"context"
	"math"
	"net/http"
	"sort"
	"strings"

	"github.com/glincker/levelrail/internal/store"
	"github.com/glincker/levelrail/internal/telemetry"
	"github.com/glincker/levelrail/internal/usagecost"
)

// Group kinds for the deploy target picker.
const (
	deployTargetKindLocal     = "local"
	deployTargetKindAccount   = "account"
	deployTargetKindUnmanaged = "unmanaged"
)

type deployTargetNode struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Status      string            `json:"status"`
	Health      string            `json:"health"`
	Schedulable bool              `json:"schedulable"`
	Region      string            `json:"region,omitempty"`
	Tags        map[string]string `json:"tags,omitempty"`
	IsLocal     bool              `json:"is_local,omitempty"`
	// MonthlyCostEstimate is the configured node bill; omitted without one.
	MonthlyCostEstimate *float64 `json:"monthly_cost_estimate,omitempty"`
	CostCurrency        string   `json:"cost_currency,omitempty"`
	// FreeCapacity is omitted when no metric is known for the node.
	FreeCapacity *deployTargetFreeCapacity `json:"free_capacity,omitempty"`
}

// deployTargetFreeCapacity holds only the figures that could be derived.
type deployTargetFreeCapacity struct {
	CPUCores    *float64 `json:"cpu_cores,omitempty"`
	MemoryBytes *float64 `json:"memory_bytes,omitempty"`
}

type deployTargetGroup struct {
	Kind      string             `json:"kind"`
	AccountID string             `json:"account_id,omitempty"`
	Provider  string             `json:"provider,omitempty"`
	Label     string             `json:"label"`
	Nodes     []deployTargetNode `json:"nodes"`
}

// deployTargetHealth folds status and cordon state into one badge value.
func deployTargetHealth(n store.Node) string {
	switch {
	case n.Status == store.NodeStatusOffline:
		return "offline"
	case !n.Schedulable:
		return "cordoned"
	case n.Status == store.NodeStatusPending:
		return "pending"
	default:
		return "healthy"
	}
}

// handleListDeployTargets handles GET /api/v1/deploy-targets: nodes grouped by
// the cloud account that provisioned them, plus local and unmanaged groups.
func (rt *Router) handleListDeployTargets(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	nodes, err := rt.nodes.ListNodes(ctx)
	if err != nil {
		rt.internalError(w, "api: list deploy targets failed", err)
		return
	}
	var labels map[string]map[string]string
	if rt.multiNode != nil {
		labels, _ = rt.multiNode.store.ListNodeLabels(ctx)
	}
	accountOf := map[string]string{}
	if rt.nodeProvisions != nil {
		if provs, perr := rt.nodeProvisions.ListNodeProvisions(ctx); perr == nil {
			for _, p := range provs {
				if p.NodeID != "" && p.AccountID != "" {
					accountOf[p.NodeID] = p.AccountID
				}
			}
		}
	}
	accounts := map[string]store.CloudAccount{}
	if st, ok := rt.cloudAcctStore(); ok {
		if all, aerr := st.ListCloudAccounts(ctx, ""); aerr == nil {
			if vis, verr := rt.cloudAccountVisible(r, st, all); verr == nil {
				for _, a := range vis {
					accounts[a.ID] = a
				}
			}
		}
	}

	prices := rt.deployTargetPrices(ctx)
	currency := ""
	if st := rt.usageStore(); st != nil && len(prices) > 0 {
		if cfg, cerr := st.GetUsageCostSettings(ctx); cerr == nil {
			currency = cfg.Currency
		}
	}
	free := rt.deployTargetFreeCapacity(ctx, prices)
	localID := rt.localNodeID

	local := deployTargetGroup{Kind: deployTargetKindLocal, Label: "local", Nodes: []deployTargetNode{
		{ID: "", Name: "local", Status: string(store.NodeStatusOnline), Health: "healthy", Schedulable: true, IsLocal: true,
			MonthlyCostEstimate: priceOf(prices, localID), CostCurrency: currencyIf(prices, localID, currency), FreeCapacity: free[localID]},
	}}
	byAccount := map[string]*deployTargetGroup{}
	unmanaged := deployTargetGroup{Kind: deployTargetKindUnmanaged, Label: "unmanaged", Nodes: []deployTargetNode{}}
	for _, n := range nodes {
		item := deployTargetNode{
			ID: n.ID, Name: n.Name, Status: string(n.Status), Health: deployTargetHealth(n),
			Schedulable: n.Schedulable, Region: n.Region, Tags: labels[n.ID], IsLocal: n.ID == rt.localNodeID && n.ID != "",
			MonthlyCostEstimate: priceOf(prices, n.ID), CostCurrency: currencyIf(prices, n.ID, currency), FreeCapacity: free[n.ID],
		}
		if item.IsLocal {
			local.Nodes = append(local.Nodes, item)
			continue
		}
		acct, ok := accounts[accountOf[n.ID]]
		if !ok {
			unmanaged.Nodes = append(unmanaged.Nodes, item)
			continue
		}
		g := byAccount[acct.ID]
		if g == nil {
			g = &deployTargetGroup{Kind: deployTargetKindAccount, AccountID: acct.ID, Provider: acct.Provider, Label: acct.DisplayName, Nodes: []deployTargetNode{}}
			byAccount[acct.ID] = g
		}
		g.Nodes = append(g.Nodes, item)
	}
	out := []deployTargetGroup{local}
	ids := make([]string, 0, len(byAccount))
	for id := range byAccount {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		out = append(out, *byAccount[id])
	}
	if len(unmanaged.Nodes) > 0 {
		out = append(out, unmanaged)
	}
	writeJSON(w, http.StatusOK, out)
}

func priceOf(prices map[string]usagecost.NodePrice, id string) *float64 {
	p, ok := prices[id]
	if !ok || id == "" || p.MonthlyPrice <= 0 {
		return nil
	}
	v := p.MonthlyPrice
	return &v
}

func currencyIf(prices map[string]usagecost.NodePrice, id, currency string) string {
	if priceOf(prices, id) == nil {
		return ""
	}
	return currency
}

// deployTargetPrices returns configured node bills; empty when none are set.
func (rt *Router) deployTargetPrices(ctx context.Context) map[string]usagecost.NodePrice {
	st := rt.usageStore()
	if st == nil {
		return nil
	}
	prices, err := st.ListUsageCostNodes(ctx)
	if err != nil {
		return nil
	}
	return prices
}

// deployTargetFreeCapacity derives free memory from host reports and free
// cores from configured vCPUs minus placed CPU use; unknown parts stay nil.
func (rt *Router) deployTargetFreeCapacity(ctx context.Context, prices map[string]usagecost.NodePrice) map[string]*deployTargetFreeCapacity {
	out := map[string]*deployTargetFreeCapacity{}
	if rt.telemetry == nil {
		return out
	}
	entry := func(id string) *deployTargetFreeCapacity {
		if out[id] == nil {
			out[id] = &deployTargetFreeCapacity{}
		}
		return out[id]
	}
	if samples, err := rt.latestByMetric(ctx, telemetry.MetricMemoryAvailableBytes); err == nil {
		for _, s := range samples {
			if id, ok := strings.CutPrefix(s.ResourceID, "node:"); ok {
				v := s.Value
				entry(id).MemoryBytes = &v
			}
		}
	}
	if rt.apps != nil {
		services, serr := rt.apps.ListDesiredServices(ctx)
		samples, cerr := rt.latestByMetric(ctx, "cpu_percent")
		if serr == nil && cerr == nil {
			nodeOf := map[string]string{}
			for _, svc := range services {
				id := svc.NodeID
				if id == "" {
					id = rt.localNodeID
				}
				nodeOf[svc.Name] = id
			}
			used := map[string]float64{}
			for _, s := range samples {
				if name, ok := strings.CutPrefix(s.ResourceID, "service:"); ok {
					if id, tracked := nodeOf[name]; tracked {
						used[id] += s.Value
					}
				}
			}
			for id, pct := range used {
				if p, ok := prices[id]; ok && p.VCPUs > 0 {
					v := math.Max(p.VCPUs-pct/100, 0)
					entry(id).CPUCores = &v
				}
			}
		}
	}
	return out
}
