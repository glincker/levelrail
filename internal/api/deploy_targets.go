package api

import (
	"net/http"
	"sort"

	"github.com/GLINCKER/levelrail/internal/store"
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

	local := deployTargetGroup{Kind: deployTargetKindLocal, Label: "local", Nodes: []deployTargetNode{
		{ID: "", Name: "local", Status: string(store.NodeStatusOnline), Health: "healthy", Schedulable: true, IsLocal: true},
	}}
	byAccount := map[string]*deployTargetGroup{}
	unmanaged := deployTargetGroup{Kind: deployTargetKindUnmanaged, Label: "unmanaged", Nodes: []deployTargetNode{}}
	for _, n := range nodes {
		item := deployTargetNode{
			ID: n.ID, Name: n.Name, Status: string(n.Status), Health: deployTargetHealth(n),
			Schedulable: n.Schedulable, Region: n.Region, Tags: labels[n.ID], IsLocal: n.ID == rt.localNodeID && n.ID != "",
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
