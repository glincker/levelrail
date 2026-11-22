package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/placement"
	"github.com/GLINCKER/levelrail/internal/store"
)

// poolStore is the optional pool and warm-list surface of the placement store.
type poolStore interface {
	SetNodePool(ctx context.Context, name string, selector map[string]string) error
	ListNodePools(ctx context.Context) ([]store.NodePool, error)
	GetNodePool(ctx context.Context, name string) (store.NodePool, error)
	DeleteNodePool(ctx context.Context, name string) error
	SetAppPool(ctx context.Context, app, pool string) error
	GetAppPool(ctx context.Context, app string) (string, error)
	SetAppWarm(ctx context.Context, app string, t store.WarmTargets) error
	GetAppWarm(ctx context.Context, app string) (store.WarmTargets, error)
}

func (rt *Router) pools(w http.ResponseWriter) (poolStore, bool) {
	if !rt.requireMultiNode(w) {
		return nil, false
	}
	ps, ok := rt.multiNode.store.(poolStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, "node pools are not available")
		return nil, false
	}
	return ps, true
}

// poolSelector merges an app's pool selector under its own selector, so an
// explicit key wins and a pool edit reaches every app on the next decision.
func (rt *Router) poolSelector(ctx context.Context, kind, name string, own placement.Selector) (placement.Selector, error) {
	if kind != store.NodeMoveKindApp || rt.multiNode == nil {
		return own, nil
	}
	ps, ok := rt.multiNode.store.(poolStore)
	if !ok {
		return own, nil
	}
	pool, err := ps.GetAppPool(ctx, name)
	if err != nil || pool == "" {
		return own, err
	}
	p, err := ps.GetNodePool(ctx, pool)
	if err != nil {
		return own, err
	}
	out := placement.Selector{}
	for k, v := range p.Selector {
		out[k] = v
	}
	for k, v := range own {
		out[k] = v
	}
	return out, nil
}

type poolBody struct {
	Selector map[string]string `json:"selector"`
}

// handleListNodePools handles GET /api/v1/node-pools.
func (rt *Router) handleListNodePools(w http.ResponseWriter, r *http.Request) {
	ps, ok := rt.pools(w)
	if !ok {
		return
	}
	list, err := ps.ListNodePools(r.Context())
	if err != nil {
		rt.internalError(w, "api: list node pools failed", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"pools": list})
}

// handleSetNodePool handles PUT /api/v1/node-pools/{name}.
func (rt *Router) handleSetNodePool(w http.ResponseWriter, r *http.Request) {
	ps, ok := rt.pools(w)
	if !ok {
		return
	}
	name := r.PathValue("name")
	var body poolBody
	if err := decodeJSONBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := placement.ValidateLabels(map[string]string{name: "x"}); err != nil {
		writeError(w, http.StatusBadRequest, "pool name must be a short lowercase identifier")
		return
	}
	if err := placement.ValidateLabels(body.Selector); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := ps.SetNodePool(r.Context(), name, body.Selector); err != nil {
		rt.internalError(w, "api: set node pool failed", err, slog.String("pool", name))
		return
	}
	if body.Selector == nil {
		body.Selector = map[string]string{}
	}
	writeJSON(w, http.StatusOK, store.NodePool{Name: name, Selector: body.Selector})
}

// handleDeleteNodePool handles DELETE /api/v1/node-pools/{name}.
func (rt *Router) handleDeleteNodePool(w http.ResponseWriter, r *http.Request) {
	ps, ok := rt.pools(w)
	if !ok {
		return
	}
	name := r.PathValue("name")
	switch err := ps.DeleteNodePool(r.Context(), name); {
	case errors.Is(err, store.ErrPoolNotFound):
		writeError(w, http.StatusNotFound, "pool not found")
	case errors.Is(err, store.ErrPoolInUse):
		writeError(w, http.StatusConflict, "pool is still used by an app")
	case err != nil:
		rt.internalError(w, "api: delete node pool failed", err, slog.String("pool", name))
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

type appPoolBody struct {
	Pool string `json:"pool"`
}

// handleSetAppPool handles PUT /api/v1/apps/{name}/pool; "" clears it.
func (rt *Router) handleSetAppPool(w http.ResponseWriter, r *http.Request) {
	ps, ok := rt.pools(w)
	if !ok {
		return
	}
	name := r.PathValue("name")
	var body appPoolBody
	if err := decodeJSONBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	switch err := ps.SetAppPool(r.Context(), name, body.Pool); {
	case errors.Is(err, store.ErrPoolNotFound):
		writeError(w, http.StatusBadRequest, "unknown pool")
		return
	case err != nil:
		rt.internalError(w, "api: set app pool failed", err, slog.String("name", name))
		return
	}
	view, err := rt.placementView(r.Context(), store.NodeMoveKindApp, name)
	if isResourceNotFound(err) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: set app pool: reload failed", err, slog.String("name", name))
		return
	}
	writeJSON(w, http.StatusOK, view)
}

// handleGetAppWarm handles GET /api/v1/apps/{name}/warm.
func (rt *Router) handleGetAppWarm(w http.ResponseWriter, r *http.Request) {
	ps, ok := rt.pools(w)
	if !ok {
		return
	}
	t, err := ps.GetAppWarm(r.Context(), r.PathValue("name"))
	if err != nil {
		rt.internalError(w, "api: get app warm failed", err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// handleSetAppWarm handles PUT /api/v1/apps/{name}/warm: which nodes or label
// group keep the app's image pulled. Empty body clears it.
func (rt *Router) handleSetAppWarm(w http.ResponseWriter, r *http.Request) {
	ps, ok := rt.pools(w)
	if !ok {
		return
	}
	name := r.PathValue("name")
	var body store.WarmTargets
	if err := decodeJSONBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := placement.ValidateLabels(body.Labels); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	refs := make([]resourceRef, 0, len(body.Nodes))
	for _, id := range body.Nodes {
		refs = append(refs, nodeRef(id))
	}
	if !rt.authorizeRefs(w, r, AbilityWrite, notFoundAs(http.StatusBadRequest, "unknown node"), refs...) {
		return
	}
	for _, id := range body.Nodes {
		if _, err := rt.nodes.GetNode(r.Context(), id); errors.Is(err, store.ErrNodeNotFound) {
			writeError(w, http.StatusBadRequest, "unknown node")
			return
		} else if err != nil {
			rt.internalError(w, "api: set app warm: node lookup failed", err)
			return
		}
	}
	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: set app warm: load app failed", err, slog.String("name", name))
		return
	}
	if err := ps.SetAppWarm(r.Context(), name, body); err != nil {
		rt.internalError(w, "api: set app warm failed", err, slog.String("name", name))
		return
	}
	t, err := ps.GetAppWarm(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: set app warm: reload failed", err, slog.String("name", name))
		return
	}
	writeJSON(w, http.StatusOK, t)
}
