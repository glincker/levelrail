package api

import (
	"fmt"
	"time"
)

const (
	doctorIngressSurvivesCode = "ingress_survives_control_plane"
	doctorSocketActivationFix = "LEVELRAIL_SOCKET_ACTIVATION=1 ./install.sh upgrade"
	doctorSocketActivationDoc = "/domains-and-ingress#surviving-a-control-plane-restart"
	doctorResilienceDoc       = "/resilience#what-happens-to-traffic-when-the-control-plane-restarts"
)

func (rt *Router) doctorCheckIngressSurvives(now time.Time) []doctorCheckResource {
	if rt.doctorEdge == nil {
		return nil
	}
	return []doctorCheckResource{doctorIngressSurvives(rt.doctorEdge, now)}
}

// doctorIngressSurvives reports whether established traffic keeps flowing while
// this control plane process restarts: the listeners must outlive it and the
// route table must be servable before the first reconcile.
func doctorIngressSurvives(e *DoctorIngressEdge, now time.Time) doctorCheckResource {
	res := doctorCheckResource{Code: doctorIngressSurvivesCode, Name: "Ingress survives control plane restarts"}
	if !e.SocketActivation {
		res.Status = doctorStatusWarn
		res.Message = "not true on this install: systemd does not hold ports 80 and 443, so a control plane restart or upgrade refuses every connection until it is back (seconds). Apps keep running, only routing stops."
		res.Fix = doctorSocketActivationFix
		res.DocsPath = doctorSocketActivationDoc
		return res
	}
	res.DocsPath = doctorResilienceDoc
	if e.Snapshot == nil {
		res.Status = doctorStatusWarn
		res.Message = "listeners are held by systemd, so connections queue during a restart, but no route snapshot is kept: the first requests after a restart wait for the first reconcile pass."
		return res
	}
	st := e.Snapshot()
	switch {
	case !st.Enabled:
		res.Status = doctorStatusWarn
		res.Message = "listeners are held by systemd, but the route snapshot is off (no master key to seal it): after a restart, routing waits for the first reconcile pass."
	case st.SaveError != "":
		res.Status = doctorStatusWarn
		res.Message = fmt.Sprintf("listeners are held by systemd, but the route snapshot could not be written (%s): a restart will wait for the first reconcile pass. Check free space and permissions on the data directory.", st.SaveError)
	case st.RestoreError != "" && st.SavedAt.IsZero():
		res.Status = doctorStatusWarn
		res.Message = fmt.Sprintf("the last route snapshot could not be used at startup (%s) and none has been written since.", st.RestoreError)
	case st.SavedAt.IsZero():
		res.Status = doctorStatusUnknown
		res.Message = "listeners are held by systemd; the route snapshot is written after the first reconcile pass, which has not finished yet."
	default:
		res.Status = doctorStatusOK
		res.Message = fmt.Sprintf("true on this install: systemd holds ports 80 and 443 and the route table saved %s ago is served at startup before the first reconcile. One ingress process: losing this host still stops routing.", now.Sub(st.SavedAt).Round(time.Second))
	}
	return res
}
