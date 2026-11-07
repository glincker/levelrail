package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	ingressdriver "github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

// DoctorIngressEdge is the ingress edge policy GET /api/v1/system/doctor reports.
type DoctorIngressEdge struct {
	SocketActivation bool
	Hardening        bool
	TrustedProxies   int
	MaxBodyBytes     int64
	// LiveMaxBodyBytes resolves the cap now (saved setting, dashboard overrides, env), so doctor never shows the boot figure after a change.
	LiveMaxBodyBytes func(ctx context.Context) int64
	RetryWindow      time.Duration
	// Snapshot reports the ingress route snapshot; nil means the control plane has none.
	Snapshot func() ingressdriver.SnapshotStatus
}

// SetDoctorIngressEdge makes doctor report the ingress edge policy. Call it
// before the router serves requests.
func (rt *Router) SetDoctorIngressEdge(edge DoctorIngressEdge) { rt.doctorEdge = &edge }

func (rt *Router) doctorCheckIngressEdge(ctx context.Context) []doctorCheckResource {
	if rt.doctorEdge == nil {
		return nil
	}
	const code, name = "ingress_edge", "Ingress edge policy"
	e := rt.doctorEdge
	if !e.Hardening {
		return []doctorCheckResource{{Code: code, Name: name, Status: doctorStatusWarn, Message: "edge hardening is off (APP_INGRESS_HARDENING=false): no timeouts, size limits or friendly 503 page"}}
	}
	parts := []string{fmt.Sprintf("failover retry window %s", e.RetryWindow)}
	maxBody := e.MaxBodyBytes
	// A saved value wins over the boot-time figure, which goes stale after a live change.
	if e.LiveMaxBodyBytes != nil {
		maxBody = e.LiveMaxBodyBytes(ctx)
	} else if rs := rt.runtimeSettings; rs != nil && rs.Source(ctx, runtimesettings.KeyIngressMaxBodyBytes) == runtimesettings.SourceSaved {
		maxBody = int64(rs.Limit(ctx, runtimesettings.KeyIngressMaxBodyBytes))
	}
	if maxBody > 0 {
		parts = append(parts, fmt.Sprintf("request bodies capped at %d bytes", maxBody))
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
