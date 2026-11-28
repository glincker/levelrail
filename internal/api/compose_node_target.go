package api

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/compose"
	"github.com/GLINCKER/levelrail/internal/dockerguard"
	"github.com/GLINCKER/levelrail/internal/placement"
	"github.com/GLINCKER/levelrail/internal/store"
)

// nodeTarget is the node a deploy was pinned to. id is the placement id
// services record, so "" is the control plane host itself.
type nodeTarget struct {
	id   string
	name string
}

// nodeRefError carries the status a failed node reference answers with.
type nodeRefError struct {
	status int
	msg    string
}

func (e *nodeRefError) Error() string { return e.msg }

// resolveNodeRef turns a node name, id or key=value label selector into one
// schedulable, online node the caller may place on. A selector with several
// matches takes the least loaded allowed one, and errors name only visible nodes.
func (rt *Router) resolveNodeRef(r *http.Request, ref string) (nodeTarget, *nodeRefError) {
	ctx := r.Context()
	sel, err := compose.NodeRefSelector(ref)
	if err != nil {
		return nodeTarget{}, &nodeRefError{http.StatusBadRequest, fmt.Sprintf("node selector %q is invalid: %v", ref, err)}
	}
	cands, err := rt.placementCandidates(ctx)
	if err != nil {
		return nodeTarget{}, &nodeRefError{http.StatusInternalServerError, "could not read the node list: " + err.Error()}
	}
	cands, err = rt.allowedPlacementCandidates(r, cands)
	if err != nil {
		return nodeTarget{}, &nodeRefError{http.StatusInternalServerError, "could not check node access"}
	}
	names := make([]string, 0, len(cands))
	best := -1
	var unavailable []string
	for i, c := range cands {
		names = append(names, c.Name)
		hit := c.Name == ref || c.ID == ref
		if sel != nil {
			hit = sel.Matches(c.Labels)
		}
		if !hit {
			continue
		}
		if !c.Schedulable || !c.Online {
			unavailable = append(unavailable, c.Name)
			continue
		}
		if best < 0 || c.Load < cands[best].Load || (c.Load == cands[best].Load && c.Name < cands[best].Name) {
			best = i
		}
	}
	sort.Strings(names)
	if best >= 0 {
		id := cands[best].ID
		if id == rt.localNodeID {
			id = ""
		}
		return nodeTarget{id: id, name: cands[best].Name}, nil
	}
	sort.Strings(unavailable)
	switch {
	case len(unavailable) > 0:
		return nodeTarget{}, &nodeRefError{http.StatusConflict, fmt.Sprintf("node %s matches %q but is cordoned or offline, so nothing can be placed on it. Uncordon it, wait for it to come online, or pick another node.", strings.Join(unavailable, ", "), ref)}
	case sel != nil:
		return nodeTarget{}, &nodeRefError{http.StatusBadRequest, fmt.Sprintf("no node has the labels %q (nodes: %s). Label a node or fix the selector.", ref, strings.Join(names, ", "))}
	default:
		return nodeTarget{}, &nodeRefError{http.StatusBadRequest, fmt.Sprintf("unknown node %q (nodes: %s). List them with 'levelrail nodes list'.", ref, strings.Join(names, ", "))}
	}
}

// allowedPlacementCandidates drops nodes an IAM Deny hides from the caller. The
// control plane node is always allowed.
func (rt *Router) allowedPlacementCandidates(r *http.Request, cands []placement.Candidate) ([]placement.Candidate, error) {
	allowed, err := rt.authorizeRefPredicate(r, AbilityWrite)
	if err != nil {
		return nil, err
	}
	out := make([]placement.Candidate, 0, len(cands))
	for _, c := range cands {
		if c.ID != rt.localNodeID && c.ID != "" {
			ok, err := allowed(nodeRef(c.ID))
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
		}
		out = append(out, c)
	}
	return out, nil
}

// resolveRequestNode reads the node_id and node_selector request fields. It
// returns nil when neither is set, and writes the refusal itself on failure.
func (rt *Router) resolveRequestNode(w http.ResponseWriter, r *http.Request, nodeID, selector string) (*nodeTarget, bool) {
	if nodeID != "" && selector != "" {
		writeError(w, http.StatusBadRequest, "pass node_id or node_selector, not both")
		return nil, false
	}
	ref := nodeID
	if ref == "" {
		ref = selector
	}
	if ref == "" {
		return nil, true
	}
	t, nerr := rt.resolveNodeRef(r, ref)
	if nerr != nil {
		writeError(w, nerr.status, nerr.msg)
		return nil, false
	}
	if t.id != "" {
		if err := rt.authorizePlacementTarget(r, t.id); err != nil {
			rt.respondPlacementValidationError(w, err, t.id, "api: compose node target: validate node failed")
			return nil, false
		}
	}
	return &t, true
}

// remoteGrantRefusal names a host-level grant that only works on the control
// plane node, so a remote placement is refused instead of dropping it silently.
func remoteGrantRefusal(svc store.DesiredService, node string) string {
	_, dockerAPI := dockerguard.ProfileFromCaps(svc.GrantedCaps)
	var kind string
	switch {
	case dockerAPI:
		kind = "a docker_api grant"
	case dockerguard.HasHostNetworkGrant(svc.GrantedCaps):
		kind = "a host_network grant"
	case len(dockerguard.DevicesFromCaps(svc.GrantedCaps)) > 0:
		kind = "a devices grant"
	default:
		return ""
	}
	return fmt.Sprintf("service %s uses %s, which only works on the control plane node, and node %q is remote. Pick the control plane node, or deploy without the grant.", svc.Name, kind, node)
}

// applyComposeNodeTargets pins each service to its x-levelrail-node, else to
// the request-level node. It returns the service-to-node map to store after
// the save, or false after writing the refusal.
func (rt *Router) applyComposeNodeTargets(w http.ResponseWriter, r *http.Request, app string, file *compose.File, services []store.DesiredService, def *nodeTarget) (map[string]string, bool) {
	var pinned map[string]string
	resolved := map[string]nodeTarget{}
	for i := range services {
		key := strings.TrimPrefix(services[i].Name, app+"-")
		target := def
		if ref := file.Services[key].Node; ref != "" {
			t, ok := resolved[ref]
			if !ok {
				var nerr *nodeRefError
				if t, nerr = rt.resolveNodeRef(r, ref); nerr != nil {
					writeError(w, nerr.status, fmt.Sprintf("service %q: x-levelrail-node: %s", key, nerr.msg))
					return nil, false
				}
				if t.id != "" {
					if err := rt.authorizePlacementTarget(r, t.id); err != nil {
						rt.respondPlacementValidationError(w, err, t.id, "api: compose node target: validate node failed")
						return nil, false
					}
				}
				resolved[ref] = t
			}
			target = &t
		}
		if target == nil {
			continue
		}
		if target.id != "" {
			if msg := remoteGrantRefusal(services[i], target.name); msg != "" {
				writeError(w, http.StatusUnprocessableEntity, msg)
				return nil, false
			}
		}
		services[i].NodeID = target.id
		if pinned == nil {
			pinned = map[string]string{}
		}
		pinned[services[i].Name] = target.id
	}
	return pinned, true
}
