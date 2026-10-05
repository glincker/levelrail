package api

import (
	"fmt"
	"strings"
	"time"
)

// DoctorIngressEdge is the ingress edge policy GET /api/v1/system/doctor reports.
type DoctorIngressEdge struct {
	SocketActivation bool
	Hardening        bool
	TrustedProxies   int
	MaxBodyBytes     int64
	RetryWindow      time.Duration
}

// SetDoctorIngressEdge makes doctor report the ingress edge policy. Call it
// before the router serves requests.
func (rt *Router) SetDoctorIngressEdge(edge DoctorIngressEdge) { rt.doctorEdge = &edge }

func (rt *Router) doctorCheckIngressEdge() []doctorCheckResource {
	if rt.doctorEdge == nil {
		return nil
	}
	const code, name = "ingress_edge", "Ingress edge policy"
	e := rt.doctorEdge
	if !e.Hardening {
		return []doctorCheckResource{{Code: code, Name: name, Status: doctorStatusWarn, Message: "edge hardening is off (APP_INGRESS_HARDENING=false): no timeouts, size limits or friendly 503 page"}}
	}
	parts := []string{fmt.Sprintf("failover retry window %s", e.RetryWindow)}
	if e.MaxBodyBytes > 0 {
		parts = append(parts, fmt.Sprintf("request bodies capped at %d bytes", e.MaxBodyBytes))
	}
	if e.TrustedProxies > 0 {
		parts = append(parts, fmt.Sprintf("%d trusted proxy ranges", e.TrustedProxies))
	} else {
		parts = append(parts, "no trusted proxies (behind a CDN, set APP_INGRESS_TRUSTED_PROXIES so apps see real client IPs)")
	}
	if e.SocketActivation {
		parts = append(parts, "systemd socket activation on (restarts queue connections)")
	} else {
		parts = append(parts, "systemd socket activation off (a control plane restart refuses connections briefly)")
	}
	return []doctorCheckResource{{Code: code, Name: name, Status: doctorStatusOK, Message: strings.Join(parts, "; ")}}
}
