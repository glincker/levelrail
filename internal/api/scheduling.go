package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/placement"
)

// createPlacement carries one create request's placement through
// resolveCreateNodePlacement and back to the handler that records it.
type createPlacement struct {
	Kind     string
	Name     string
	Needs    placement.Resources
	decision *placement.Decision
}

// autoPlaceNode picks a node for a new app or database whose create request
// omitted node_id: the eligible node with the most headroom, spread by
// load on a tie. It returns "" (local) when auto-placement is off or no
// node is registered or usable, and an error when nodes exist but none has
// room, so a full cluster fails the create instead of overloading a node.
// preferLocalIngress favors the control plane's own node when it fits,
// since ingress has no mesh path to a worker node yet (CrossNodeIngress).
func (rt *Router) autoPlaceNode(ctx context.Context, preferLocalIngress bool, needs placement.Resources) (string, *placement.Decision, error) {
	if !rt.autoPlacementEnabled {
		return "", nil, nil
	}
	inv, err := rt.loadPlacementInventory(ctx)
	if err != nil {
		return "", nil, err
	}
	if len(inv.nodes) == 0 {
		return "", nil, nil
	}
	cands, err := rt.candidatesFrom(ctx, inv)
	if err != nil {
		return "", nil, err
	}
	req := placement.Request{Needs: needs, Policy: rt.placementPolicy}
	if preferLocalIngress {
		req.Prefer = rt.localNodeID
	}
	d, err := placement.Spread(cands, req)
	switch {
	case err == nil:
		return d.NodeID, &d, nil
	case d.CapacityBlocked():
		return "", &d, err
	}
	return "", nil, nil
}

// nodeIDKeyPresent reports whether body's top-level JSON object includes
// a "node_id" key at all (any value, including null or ""), the
// distinction handleCreateApp/handleCreateDatabase need between an
// explicit placement override and a genuinely omitted field: appResource/
// databaseResource's own NodeID field is a plain string, which on its
// own can't tell "the caller sent an empty string" apart from "the
// caller sent nothing." A malformed body reports false, matching the
// caller's own primary json.Unmarshal, which will have already rejected
// it by the time this is consulted.
func nodeIDKeyPresent(body []byte) bool {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(body, &probe); err != nil {
		return false
	}
	_, ok := probe["node_id"]
	return ok
}

// resolveCreateNodePlacement is handleCreateApp/handleCreateDatabase's
// shared node_id resolution: an explicit node_id in the body (even "")
// is validated as a placement override and left untouched in *nodeID/
// *autoPlaced; node_id omitted entirely lets autoPlaceNode pick one by
// headroom, writing its result into both. When nothing fits it answers 409
// with the shortfall per node. cp.decision is set for the caller to
// record once the create is saved. ok is false once the full HTTP response
// has been written; the caller should return immediately.
func (rt *Router) resolveCreateNodePlacement(w http.ResponseWriter, r *http.Request, body []byte, nodeID *string, autoPlaced *bool, hasDomains bool, cp *createPlacement, logContext string) (ok bool) {
	if nodeIDKeyPresent(body) {
		if err := rt.authorizePlacementTarget(r, *nodeID); err != nil {
			rt.respondPlacementValidationError(w, err, *nodeID, logContext+": validate node failed")
			return false
		}
		if *nodeID != "" {
			if cands, err := rt.placementCandidates(r.Context()); err == nil {
				d := placement.Pinned(cands, placement.Request{Needs: cp.Needs, Policy: rt.placementPolicy}, *nodeID)
				cp.decision = &d
			}
		}
		return true
	}

	placed, d, err := rt.autoPlaceNode(r.Context(), hasDomains, cp.Needs)
	if errors.Is(err, placement.ErrNoEligibleNode) {
		writeError(w, http.StatusConflict, strings.TrimPrefix(err.Error(), "placement: ")+". Free capacity, add a node, or pass node_id to place it on a node anyway.")
		return false
	}
	if err != nil {
		rt.internalError(w, logContext+": auto-place node failed", err)
		return false
	}
	*nodeID = placed
	*autoPlaced = placed != ""
	cp.decision = d
	return true
}

// recordCreatePlacement stores the decision once the resource exists, and
// only when the node it was made for is the node the resource ended on.
func (rt *Router) recordCreatePlacement(ctx context.Context, cp *createPlacement, finalNodeID string) {
	if cp.decision == nil || cp.decision.NodeID != finalNodeID {
		return
	}
	rt.recordPlacement(ctx, cp.Kind, cp.Name, placementTriggerCreate, *cp.decision)
}
