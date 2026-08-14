package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// appResource is the wire shape for an app: store.DesiredService plus
// its name, marshaled and unmarshaled directly rather than through a
// parallel domain type, since TASKS.md 1.9 doesn't ask this endpoint to
// represent anything store.DesiredService can't already hold. Strategy
// and Replicas got a home in store.DesiredService (migration
// 0017_deploy_strategy.sql), so this resource now carries them too;
// domains closed once TASKS.md 1.6 added the column, the same way.
type appResource struct {
	Name      string                  `json:"name"`
	Image     string                  `json:"image"`
	Port      int                     `json:"port"`
	Domains   []string                `json:"domains,omitempty"`
	Env       map[string]string       `json:"env,omitempty"`
	Resources *store.ServiceResources `json:"resources,omitempty"`
	Health    *store.ServiceHealth    `json:"health,omitempty"`
	// Strategy and Replicas deliberately have no `omitempty`: a real
	// store.DesiredService read back from the store is documented to
	// always carry the *resolved* value (never "" / 0, see that
	// struct's own doc comment), matching how Image/Port (also always-
	// resolved, required fields) have no omitempty either. On the way
	// in, an empty Strategy or zero Replicas is not an error, it just
	// falls through to store.SaveDesiredService's own default-filling
	// (DefaultDeployStrategy/DefaultReplicas), the same behavior a
	// service saved without these fields has always had.
	Strategy string `json:"strategy"`
	Replicas int    `json:"replicas"`
	// NodeID is TASKS.md 3.3's placement (empty means this control
	// plane's own local node). Response-only: toDesiredService below
	// never reads it, the same "shown but not settable through this
	// endpoint" boundary ruleResource's own evaluation-state fields
	// already establish for a different resource. Set it via
	// PUT /api/v1/apps/{name}/node (handleSetAppNode) instead.
	NodeID string `json:"node_id,omitempty"`
}

func toAppResource(svc store.DesiredService) appResource {
	return appResource{
		Name:      svc.Name,
		Image:     svc.Image,
		Port:      svc.Port,
		Domains:   svc.Domains,
		Env:       svc.Env,
		Resources: svc.Resources,
		Health:    svc.Health,
		Strategy:  svc.Strategy,
		Replicas:  svc.Replicas,
		NodeID:    svc.NodeID,
	}
}

func (a appResource) toDesiredService() store.DesiredService {
	return store.DesiredService{
		Name:      a.Name,
		Image:     a.Image,
		Port:      a.Port,
		Domains:   a.Domains,
		Env:       a.Env,
		Resources: a.Resources,
		Health:    a.Health,
		Strategy:  a.Strategy,
		Replicas:  a.Replicas,
	}
}

// validKnownStrategies mirrors internal/spec's own
// StrategyRolling/StrategyRecreate/StrategyBlueGreen exactly. "rolling"
// stays in this set on purpose: it's a real, named app.yaml value (a
// service saved via the deploy pipeline, e.g. a hand-edited app.yaml,
// can already carry it, internal/reconcile/application's controller
// test seeds it directly through the store), it is only permanently
// unsupported at *reconcile* time, where it fails loudly with a
// documented condition rather than silently running as something else.
// Rejecting it here would make the API stricter than the store and the
// reconciler already are, for no safety gain, since a bad reconcile
// outcome is already visible via the app's conditions. The frontend
// picker is a separate, narrower decision: it only offers recreate/
// blue-green as new choices so a user can't pick a state that's
// guaranteed to fail on next reconcile without ever seeing why, but
// that's a UI affordance choice, not a wire-validation one.
var validKnownStrategies = map[string]bool{
	spec.StrategyRolling:   true,
	spec.StrategyRecreate:  true,
	spec.StrategyBlueGreen: true,
}

func validateAppResource(a appResource) error {
	if a.Name == "" {
		return errors.New("name is required")
	}
	if a.Image == "" {
		return errors.New("image is required")
	}
	if a.Port <= 0 {
		return errors.New("port must be a positive integer")
	}
	// Empty is valid (falls through to store.DefaultDeployStrategy), but
	// a non-empty value that isn't one of the three known spec constants
	// is almost certainly a typo, and worth rejecting loudly here rather
	// than saving it and letting the reconciler discover it later.
	if a.Strategy != "" && !validKnownStrategies[a.Strategy] {
		return fmt.Errorf("strategy must be one of %q, %q, %q", spec.StrategyRolling, spec.StrategyRecreate, spec.StrategyBlueGreen)
	}
	// Zero is valid (falls through to store.DefaultReplicas), only a
	// negative count is nonsensical.
	if a.Replicas < 0 {
		return errors.New("replicas must not be negative")
	}
	return nil
}

// handleListApps handles GET /api/v1/apps.
func (rt *Router) handleListApps(w http.ResponseWriter, r *http.Request) {
	svcs, err := rt.apps.ListDesiredServices(r.Context())
	if err != nil {
		rt.logger.Error("api: list apps failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]appResource, 0, len(svcs))
	for _, s := range svcs {
		out = append(out, toAppResource(s))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleCreateApp handles POST /api/v1/apps. Rejects a name that already
// exists rather than silently overwriting it: that's what PUT
// (handleUpdateApp) is for. A domain conflict (store.ErrDomainTaken,
// enforced by SaveDesiredService itself, see that error's own doc
// comment for why) surfaces as 409 with the real conflicting domain
// named in the message, the same shape a duplicate name gets, not the
// generic 500 it fell through to before this was wired up.
func (rt *Router) handleCreateApp(w http.ResponseWriter, r *http.Request) {
	var req appResource
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validateAppResource(req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	_, err := rt.apps.GetDesiredService(r.Context(), req.Name)
	if err == nil {
		writeError(w, http.StatusConflict, "an app with this name already exists")
		return
	}
	if !errors.Is(err, store.ErrServiceNotFound) {
		rt.logger.Error("api: create app: check existing failed", slog.String("error", err.Error()), slog.String("name", req.Name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := rt.apps.SaveDesiredService(r.Context(), req.toDesiredService()); err != nil {
		var domainTaken *store.ErrDomainTaken
		if errors.As(err, &domainTaken) {
			writeError(w, http.StatusConflict, domainTaken.Error())
			return
		}
		rt.logger.Error("api: create app failed", slog.String("error", err.Error()), slog.String("name", req.Name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, req)
}

// handleGetApp handles GET /api/v1/apps/{name}.
func (rt *Router) handleGetApp(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: get app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toAppResource(*svc))
}

// handleUpdateApp handles PUT /api/v1/apps/{name}. Full replace, same as
// SaveDesiredService itself: there is no partial-update semantics here,
// matching how store.DesiredService is already documented to work.
func (rt *Router) handleUpdateApp(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var req appResource
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Name = name // the path is authoritative, not whatever the body claims

	if err := validateAppResource(req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	existing, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: update app: check existing failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := rt.apps.SaveDesiredService(r.Context(), req.toDesiredService()); err != nil {
		var domainTaken *store.ErrDomainTaken
		if errors.As(err, &domainTaken) {
			writeError(w, http.StatusConflict, domainTaken.Error())
			return
		}
		rt.logger.Error("api: update app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	// SaveDesiredService never touches node_id (its own doc comment
	// explains why), so the response reflects existing's placement, not
	// req's: req.NodeID is always its zero value here anyway (the
	// client never has a way to set it on this endpoint), but spelling
	// out "carry the real, unchanged value forward" is clearer than
	// relying on that zero value happening to look right.
	req.NodeID = existing.NodeID
	writeJSON(w, http.StatusOK, req)
}

// setAppNodeRequest is PUT /api/v1/apps/{name}/node's body.
type setAppNodeRequest struct {
	// NodeID is which node to place the app on; empty string moves it
	// back to this control plane's own local node, the same convention
	// store.DesiredService.NodeID's own doc comment establishes.
	NodeID string `json:"node_id"`
}

// handleSetAppNode handles PUT /api/v1/apps/{name}/node (TASKS.md 3.3):
// the only way an app's placement actually changes, see appResource's
// own NodeID field doc comment. A non-empty node_id is checked against
// the real node registry first, so a typo'd or already-removed node ID
// fails loudly here with a clear 400 rather than silently parking the
// service on a node that will never reconcile it (resolveNodeTransport,
// cmd/levelrail/main.go, would otherwise just skip it forever with
// nothing surfaced beyond a log line).
func (rt *Router) handleSetAppNode(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var req setAppNodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := rt.validatePlacementTarget(r.Context(), req.NodeID); err != nil {
		switch {
		case errors.Is(err, store.ErrNodeNotFound):
			writeError(w, http.StatusBadRequest, "unknown node_id")
		case errors.Is(err, errNodeCordoned):
			// TASKS.md 3.7: cordon means "unschedulable for new
			// placements", and this is a new placement even when the
			// service already exists, since it's actively choosing to
			// move it here.
			writeError(w, http.StatusBadRequest, "node is cordoned and not accepting new placements")
		default:
			rt.logger.Error("api: set app node: look up node failed", slog.String("error", err.Error()), slog.String("node_id", req.NodeID))
			writeError(w, http.StatusInternalServerError, "internal error")
		}
		return
	}

	if err := rt.apps.UpdateServiceNode(r.Context(), name, req.NodeID); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.logger.Error("api: set app node failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if err != nil {
		rt.logger.Error("api: set app node: reload after update failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toAppResource(*svc))
}

// handleDeleteApp handles DELETE /api/v1/apps/{name}. See
// store.DeleteDesiredService's doc comment for the known gap: this
// removes desired state, it does not itself stop the running container.
func (rt *Router) handleDeleteApp(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	err := rt.apps.DeleteDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: delete app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
