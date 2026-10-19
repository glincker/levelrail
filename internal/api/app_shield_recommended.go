package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

const (
	shieldSkipConfigured = "already_configured"
	shieldBlockedProxy   = "behind_proxy"
)

type shieldRecommendedRequest struct {
	DryRun    bool `json:"dry_run,omitempty"`
	BlockBots bool `json:"block_bots,omitempty"`
}

type shieldRecommendedSkip struct {
	Domain string `json:"domain"`
	Reason string `json:"reason"`
}

type shieldRecommendedResource struct {
	DryRun        bool                    `json:"dry_run"`
	Blocked       string                  `json:"blocked,omitempty"`
	RatePerMinute int                     `json:"rate_per_minute"`
	Burst         int                     `json:"burst"`
	MaxConcurrent int                     `json:"max_concurrent"`
	BlockBots     bool                    `json:"block_bots"`
	Applied       []string                `json:"applied"`
	Skipped       []shieldRecommendedSkip `json:"skipped"`
}

// handleApplyRecommendedShield handles POST /api/v1/apps/{name}/shield/recommended: it turns on
// sensible per-client limits for each of the app's domains that has no shield yet. A domain with
// any shield row, even a disabled one, is left alone, and nothing runs while a proxy hides client IPs.
func (rt *Router) handleApplyRecommendedShield(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var req shieldRecommendedRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxPolicyBody)).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: recommended shield: load app failed", err, slog.String("name", name))
		return
	}
	l := rt.domainPolicy.limits()
	rec := trafficpolicy.Shield{
		Enabled: true, RatePerMinute: l.ShieldRecommendedRate, Burst: l.ShieldRecommendedBurst,
		MaxConcurrent: l.ShieldRecommendedConcurrent, BlockBots: req.BlockBots,
	}
	if err := rec.Validate(l); err != nil {
		writePolicyError(w, err)
		return
	}
	out := shieldRecommendedResource{
		DryRun: req.DryRun, RatePerMinute: rec.RatePerMinute, Burst: rec.Burst,
		MaxConcurrent: rec.MaxConcurrent, BlockBots: rec.BlockBots,
		Applied: []string{}, Skipped: []shieldRecommendedSkip{},
	}
	if rt.clientIPHidden(r) {
		out.Blocked = shieldBlockedProxy
		writeJSON(w, http.StatusOK, out)
		return
	}
	raw, err := json.Marshal(rec)
	if err != nil {
		rt.internalError(w, "api: recommended shield: encode failed", err)
		return
	}
	for _, d := range svc.Domains {
		domain := strings.ToLower(strings.TrimSpace(d))
		_, found, err := rt.domainPolicy.store.GetDomainTrafficPolicy(r.Context(), domain, trafficpolicy.KindShield)
		if err != nil {
			rt.internalError(w, "api: recommended shield: read failed", err, slog.String("domain", domain))
			return
		}
		if found {
			out.Skipped = append(out.Skipped, shieldRecommendedSkip{Domain: domain, Reason: shieldSkipConfigured})
			continue
		}
		if !req.DryRun {
			if err := rt.domainPolicy.store.SetDomainTrafficPolicy(r.Context(), domain, trafficpolicy.KindShield, raw); err != nil {
				rt.internalError(w, "api: recommended shield: save failed", err, slog.String("domain", domain))
				return
			}
		}
		out.Applied = append(out.Applied, domain)
	}
	if len(out.Applied) > 0 && !req.DryRun {
		rt.nudgeReconciler()
	}
	writeJSON(w, http.StatusOK, out)
}

// clientIPHidden is true when a proxy owns TLS or DNS in front of the ingress and no trusted
// proxy list is set, so every request would look like one client and a rate limit would hit everyone.
func (rt *Router) clientIPHidden(r *http.Request) bool {
	s, err := rt.ingressSettings.GetIngressSettings(r.Context())
	if err != nil {
		return true
	}
	if !s.TLSTerminatedUpstream && !s.DNSProxied {
		return false
	}
	return strings.TrimSpace(rt.domainPolicy.env(ingress.EnvTrustedProxies)) == "" && !rt.edgeTrustedProxiesSet(r.Context())
}
