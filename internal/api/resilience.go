package api

import (
	"context"
	"net/http"
	"time"

	ingressdriver "github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/resilience"
)

// IngressResilience is the persisted ingress state the resilience check reads.
type IngressResilience struct {
	Status   func() ingressdriver.SnapshotStatus
	Verify   func() (ingressdriver.SnapshotCheck, error)
	SaveNow  func() error
	CertKeys func(ctx context.Context) (int, error)
}

// SetIngressResilience wires the resilience check to the ingress driver. Call it before the router serves requests.
func (rt *Router) SetIngressResilience(src IngressResilience) { rt.ingressResilience = &src }

func (rt *Router) buildResilience(ctx context.Context, simulate bool) (resilience.Report, error) {
	var in resilience.IngressInput
	if src := rt.ingressResilience; src != nil {
		st := src.Status()
		in.SnapshotEnabled, in.SavedAt, in.SaveError = st.Enabled, st.SavedAt, st.SaveError
		if simulate && st.Enabled {
			chk, err := src.Verify()
			in.Verified, in.VerifyErr, in.ConfigBytes = true, err, chk.ConfigBytes
			if err == nil && in.SavedAt.IsZero() {
				in.SavedAt = chk.SavedAt
			}
		}
		in.CertKeys, in.CertErr = src.CertKeys(ctx)
	}
	var nodes []resilience.NodeInput
	if rt.nodes != nil {
		list, err := rt.nodes.ListNodes(ctx)
		if err != nil {
			return resilience.Report{}, err
		}
		for _, n := range list {
			nodes = append(nodes, resilience.NodeInput{ID: n.ID, Name: n.Name, Local: rt.isLocalGuardNode(n.ID), Status: string(n.Status), LastSeen: n.LastSeenAt})
		}
	}
	return resilience.Evaluate(time.Now().UTC(), simulate, in, nodes), nil
}

// handleGetResilience handles GET /api/v1/system/resilience.
func (rt *Router) handleGetResilience(w http.ResponseWriter, r *http.Request) {
	rt.writeResilience(w, r, false)
}

// handleSimulateResilience handles POST /api/v1/system/resilience/simulate: a dry run that stops nothing.
func (rt *Router) handleSimulateResilience(w http.ResponseWriter, r *http.Request) {
	rt.writeResilience(w, r, true)
}

// handleResilienceFix handles POST /api/v1/system/resilience/snapshot: rewrites the route snapshot.
func (rt *Router) handleResilienceFix(w http.ResponseWriter, r *http.Request) {
	if rt.ingressResilience == nil {
		writeError(w, http.StatusNotImplemented, "ingress resilience is not configured")
		return
	}
	if err := rt.ingressResilience.SaveNow(); err != nil {
		writeError(w, http.StatusConflict, "could not save the route snapshot: "+err.Error())
		return
	}
	rt.writeResilience(w, r, true)
}

func (rt *Router) writeResilience(w http.ResponseWriter, r *http.Request, simulate bool) {
	rep, err := rt.buildResilience(r.Context(), simulate)
	if err != nil {
		rt.internalError(w, "api: resilience check failed", err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}
