package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/privnet"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

const privateNetworkEnforceTimeout = 30 * time.Second

var privateNetworkNameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)

// Edge fix kinds.
const (
	privateNetworkFixAllow   = "allow"
	privateNetworkFixReapply = "reapply"
)

// Observed connectivity of an edge.
const (
	privateNetworkConnected = "connected"
	privateNetworkIsolated  = "isolated"
	privateNetworkUnknown   = "unknown"
)

// PrivateNetworkStore is the store surface the private network handlers need.
type PrivateNetworkStore interface {
	privnet.Source
	SavePrivateNetwork(ctx context.Context, n store.PrivateNetwork) error
	UpdatePrivateNetwork(ctx context.Context, id string, defaultDeny bool, members []string) error
	DeletePrivateNetwork(ctx context.Context, id string) error
	SavePrivateNetworkRule(ctx context.Context, r store.PrivateNetworkRule) error
	DeletePrivateNetworkRule(ctx context.Context, id string) error
}

// PrivateNetworkConfig wires the private network feature into the router.
type PrivateNetworkConfig struct {
	Store    PrivateNetworkStore
	Enforcer reconcile.Controller
	// Containers lists local containers for observed connectivity; nil skips it.
	Containers func(ctx context.Context) ([]docker.ContainerState, error)
	Prefix     string
	// Nodes resolves an app's node ID; nil treats every app as local.
	Nodes func(ctx context.Context) (map[string]string, error)
}

// WithPrivateNetworks enables the private network routes (501 without it).
func WithPrivateNetworks(cfg PrivateNetworkConfig) Option {
	return func(rt *Router) { rt.privateNetworks = &cfg }
}

type privateNetworkResource struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	ProjectID   string   `json:"project_id,omitempty"`
	DefaultDeny bool     `json:"default_deny"`
	Members     []string `json:"members"`
	CreatedAt   string   `json:"created_at"`
}

type privateNetworkRuleResource struct {
	ID        string `json:"id"`
	From      string `json:"from"`
	To        string `json:"to"`
	Note      string `json:"note,omitempty"`
	CreatedAt string `json:"created_at"`
}

type privateNetworkListResponse struct {
	Networks []privateNetworkResource     `json:"networks"`
	Rules    []privateNetworkRuleResource `json:"rules"`
}

type privateNetworkRequest struct {
	Name        string   `json:"name"`
	ProjectID   string   `json:"project_id"`
	DefaultDeny bool     `json:"default_deny"`
	Members     []string `json:"members"`
}

type privateNetworkRuleRequest struct {
	From string `json:"from"`
	To   string `json:"to"`
	Note string `json:"note"`
}

type privateNetworkMapApp struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	ProjectID string   `json:"project_id,omitempty"`
	NodeID    string   `json:"node_id,omitempty"`
	Networks  []string `json:"networks"`
}

type privateNetworkMapEdge struct {
	A         string `json:"a"`
	B         string `json:"b"`
	Allowed   bool   `json:"allowed"`
	Via       string `json:"via,omitempty"`
	ViaName   string `json:"via_name,omitempty"`
	RuleID    string `json:"rule_id,omitempty"`
	CrossNode bool   `json:"cross_node"`
	Observed  string `json:"observed"`
	Fix       string `json:"fix,omitempty"`
}

type privateNetworkMapResponse struct {
	Apps  []privateNetworkMapApp  `json:"apps"`
	Edges []privateNetworkMapEdge `json:"edges"`
}

// SetPrivateNetworks enables the private network routes after construction.
func (rt *Router) SetPrivateNetworks(cfg PrivateNetworkConfig) { rt.privateNetworks = &cfg }

func (rt *Router) privateNetworksOrFail(w http.ResponseWriter) *PrivateNetworkConfig {
	if rt.privateNetworks == nil || rt.privateNetworks.Store == nil {
		writeError(w, http.StatusNotImplemented, "private networks are not configured on this control plane")
		return nil
	}
	return rt.privateNetworks
}

func newPrivateNetworkID() (string, error) {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (cfg *PrivateNetworkConfig) enforce(ctx context.Context, rt *Router) bool {
	if cfg.Enforcer == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), privateNetworkEnforceTimeout)
	defer cancel()
	if _, err := cfg.Enforcer.Reconcile(ctx); err != nil {
		rt.logger.Warn("api: private network enforce failed", slog.String("error", err.Error()))
		return false
	}
	return true
}

// resolveApps maps app names or IDs to IDs and rejects unknown ones.
func resolveAppRefs(apps []store.App, refs []string) ([]string, string) {
	byRef := make(map[string]string, len(apps)*2)
	for _, a := range apps {
		byRef[a.ID] = a.ID
		byRef[a.Name] = a.ID
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		id, ok := byRef[ref]
		if !ok {
			return nil, "unknown app " + ref
		}
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out, ""
}

func appNameIndex(apps []store.App) map[string]string {
	m := make(map[string]string, len(apps))
	for _, a := range apps {
		m[a.ID] = a.Name
	}
	return m
}

func (rt *Router) handleListPrivateNetworks(w http.ResponseWriter, r *http.Request) {
	cfg := rt.privateNetworksOrFail(w)
	if cfg == nil {
		return
	}
	nets, err := cfg.Store.ListPrivateNetworks(r.Context())
	if err != nil {
		rt.internalError(w, "api: list private networks", err)
		return
	}
	rules, err := cfg.Store.ListPrivateNetworkRules(r.Context())
	if err != nil {
		rt.internalError(w, "api: list private network rules", err)
		return
	}
	apps, err := cfg.Store.ListApps(r.Context())
	if err != nil {
		rt.internalError(w, "api: list apps for private networks", err)
		return
	}
	names := appNameIndex(apps)
	nameOf := func(id string) string {
		if n, ok := names[id]; ok {
			return n
		}
		return id
	}
	out := privateNetworkListResponse{Networks: []privateNetworkResource{}, Rules: []privateNetworkRuleResource{}}
	for _, n := range nets {
		members := make([]string, 0, len(n.Members))
		for _, m := range n.Members {
			members = append(members, nameOf(m))
		}
		sort.Strings(members)
		out.Networks = append(out.Networks, privateNetworkResource{ID: n.ID, Name: n.Name, ProjectID: n.ProjectID, DefaultDeny: n.DefaultDeny, Members: members, CreatedAt: n.CreatedAt})
	}
	for _, ru := range rules {
		out.Rules = append(out.Rules, privateNetworkRuleResource{ID: ru.ID, From: nameOf(ru.FromApp), To: nameOf(ru.ToApp), Note: ru.Note, CreatedAt: ru.CreatedAt})
	}
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) handleCreatePrivateNetwork(w http.ResponseWriter, r *http.Request) {
	cfg := rt.privateNetworksOrFail(w)
	if cfg == nil {
		return
	}
	var req privateNetworkRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if !privateNetworkNameRE.MatchString(req.Name) {
		writeError(w, http.StatusBadRequest, "name must be lowercase letters, digits and dashes, up to 40 characters")
		return
	}
	apps, err := cfg.Store.ListApps(r.Context())
	if err != nil {
		rt.internalError(w, "api: create private network: list apps", err)
		return
	}
	members, bad := resolveAppRefs(apps, req.Members)
	if bad != "" {
		writeError(w, http.StatusBadRequest, bad)
		return
	}
	existing, err := cfg.Store.ListPrivateNetworks(r.Context())
	if err != nil {
		rt.internalError(w, "api: create private network: list", err)
		return
	}
	for _, n := range existing {
		if n.Name == req.Name {
			writeError(w, http.StatusConflict, "a private network with this name already exists")
			return
		}
	}
	id, err := newPrivateNetworkID()
	if err != nil {
		rt.internalError(w, "api: create private network: id", err)
		return
	}
	n := store.PrivateNetwork{ID: id, Name: req.Name, ProjectID: req.ProjectID, DefaultDeny: req.DefaultDeny, Members: members, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	if err := cfg.Store.SavePrivateNetwork(r.Context(), n); err != nil {
		rt.internalError(w, "api: create private network: save", err)
		return
	}
	enforced := cfg.enforce(r.Context(), rt)
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "name": n.Name, "enforced": enforced})
}

func (rt *Router) handleUpdatePrivateNetwork(w http.ResponseWriter, r *http.Request) {
	cfg := rt.privateNetworksOrFail(w)
	if cfg == nil {
		return
	}
	var req privateNetworkRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	apps, err := cfg.Store.ListApps(r.Context())
	if err != nil {
		rt.internalError(w, "api: update private network: list apps", err)
		return
	}
	members, bad := resolveAppRefs(apps, req.Members)
	if bad != "" {
		writeError(w, http.StatusBadRequest, bad)
		return
	}
	id := r.PathValue("id")
	if err := cfg.Store.UpdatePrivateNetwork(r.Context(), id, req.DefaultDeny, members); err != nil {
		if errors.Is(err, store.ErrPrivateNetworkNotFound) {
			writeError(w, http.StatusNotFound, "private network not found")
			return
		}
		rt.internalError(w, "api: update private network", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "enforced": cfg.enforce(r.Context(), rt)})
}

func (rt *Router) handleDeletePrivateNetwork(w http.ResponseWriter, r *http.Request) {
	cfg := rt.privateNetworksOrFail(w)
	if cfg == nil {
		return
	}
	if err := cfg.Store.DeletePrivateNetwork(r.Context(), r.PathValue("id")); err != nil {
		if errors.Is(err, store.ErrPrivateNetworkNotFound) {
			writeError(w, http.StatusNotFound, "private network not found")
			return
		}
		rt.internalError(w, "api: delete private network", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true, "enforced": cfg.enforce(r.Context(), rt)})
}

func (rt *Router) handleCreatePrivateNetworkRule(w http.ResponseWriter, r *http.Request) {
	cfg := rt.privateNetworksOrFail(w)
	if cfg == nil {
		return
	}
	var req privateNetworkRuleRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	apps, err := cfg.Store.ListApps(r.Context())
	if err != nil {
		rt.internalError(w, "api: create private network rule: list apps", err)
		return
	}
	ids, bad := resolveAppRefs(apps, []string{req.From, req.To})
	if bad != "" {
		writeError(w, http.StatusBadRequest, bad)
		return
	}
	if len(ids) != 2 {
		writeError(w, http.StatusBadRequest, "a rule needs two different apps")
		return
	}
	id, err := newPrivateNetworkID()
	if err != nil {
		rt.internalError(w, "api: create private network rule: id", err)
		return
	}
	rule := store.PrivateNetworkRule{ID: id, FromApp: ids[0], ToApp: ids[1], Note: strings.TrimSpace(req.Note), CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	if err := cfg.Store.SavePrivateNetworkRule(r.Context(), rule); err != nil {
		rt.internalError(w, "api: create private network rule: save", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "enforced": cfg.enforce(r.Context(), rt)})
}

func (rt *Router) handleDeletePrivateNetworkRule(w http.ResponseWriter, r *http.Request) {
	cfg := rt.privateNetworksOrFail(w)
	if cfg == nil {
		return
	}
	if err := cfg.Store.DeletePrivateNetworkRule(r.Context(), r.PathValue("id")); err != nil {
		if errors.Is(err, store.ErrPrivateNetworkRuleNotFound) {
			writeError(w, http.StatusNotFound, "rule not found")
			return
		}
		rt.internalError(w, "api: delete private network rule", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true, "enforced": cfg.enforce(r.Context(), rt)})
}

func (rt *Router) handleApplyPrivateNetworks(w http.ResponseWriter, r *http.Request) {
	cfg := rt.privateNetworksOrFail(w)
	if cfg == nil {
		return
	}
	if cfg.Enforcer == nil {
		writeError(w, http.StatusNotImplemented, "enforcement is not available on this control plane")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"enforced": cfg.enforce(r.Context(), rt)})
}

func (rt *Router) handlePrivateNetworkMap(w http.ResponseWriter, r *http.Request) {
	cfg := rt.privateNetworksOrFail(w)
	if cfg == nil {
		return
	}
	nets, rules, apps, err := privnet.Load(r.Context(), cfg.Store)
	if err != nil {
		rt.internalError(w, "api: private network map", err)
		return
	}
	nodes := map[string]string{}
	if cfg.Nodes != nil {
		if nodes, err = cfg.Nodes(r.Context()); err != nil {
			rt.internalError(w, "api: private network map: nodes", err)
			return
		}
	}
	for id, nodeID := range nodes {
		if nodeID == rt.localNodeID {
			nodes[id] = ""
		}
	}
	memberOf := map[string][]string{}
	for _, n := range nets {
		for _, m := range n.Members {
			memberOf[m] = append(memberOf[m], n.Name)
		}
	}
	evalApps := make([]privnet.App, 0, len(apps))
	resp := privateNetworkMapResponse{Apps: []privateNetworkMapApp{}, Edges: []privateNetworkMapEdge{}}
	for _, a := range apps {
		evalApps = append(evalApps, privnet.App{ID: a.ID, Name: a.Name, ProjectID: a.ProjectID, NodeID: nodes[a.ID]})
		nn := memberOf[a.ID]
		if nn == nil {
			nn = []string{}
		}
		resp.Apps = append(resp.Apps, privateNetworkMapApp{ID: a.ID, Name: a.Name, ProjectID: a.ProjectID, NodeID: nodes[a.ID], Networks: nn})
	}

	var shared map[[2]string]bool
	hasContainers := map[string]bool{}
	if cfg.Containers != nil {
		if cts, err := cfg.Containers(r.Context()); err == nil {
			shared = privnet.SharedNetworks(cfg.Prefix, apps, cts)
			for _, app := range privnet.AppOwners(cfg.Prefix, apps, cts) {
				hasContainers[app] = true
			}
		} else {
			rt.logger.Warn("api: private network map: list containers", slog.String("error", err.Error()))
		}
	}

	for _, e := range privnet.Evaluate(evalApps, nets, rules) {
		out := privateNetworkMapEdge{A: e.A, B: e.B, Allowed: e.Allowed, Via: e.Via, ViaName: e.ViaName, RuleID: e.RuleID, CrossNode: e.CrossNode, Observed: privateNetworkUnknown}
		if shared != nil && !e.CrossNode && hasContainers[e.A] && hasContainers[e.B] {
			if shared[privnet.PairKey(e.A, e.B)] {
				out.Observed = privateNetworkConnected
			} else {
				out.Observed = privateNetworkIsolated
			}
		}
		switch {
		case !e.Allowed && out.Observed == privateNetworkConnected:
			out.Fix = privateNetworkFixReapply
		case !e.Allowed:
			out.Fix = privateNetworkFixAllow
		case out.Observed == privateNetworkIsolated:
			out.Fix = privateNetworkFixReapply
		}
		resp.Edges = append(resp.Edges, out)
	}
	writeJSON(w, http.StatusOK, resp)
}
