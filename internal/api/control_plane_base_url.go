package api

import (
	"context"
	"errors"
	"fmt"
)

// errNoPrimaryDomain is controlPlaneBaseURL's sentinel for
// "ingress_settings.primary_domain is unset", so callers can distinguish
// that specific, actionable case from a genuine store error.
var errNoPrimaryDomain = errors.New("api: primary domain is not configured")

// controlPlaneBaseURL derives this control plane's own public, reachable
// origin from store.IngressSettings.PrimaryDomain (PUT
// /api/v1/settings/ingress), the same field
// internal/reconcile/ingress's WithDashboardDial route already treats
// as "the control plane's own hostname" when one is configured. Always
// https: PrimaryDomain is meant to be served by embedded Caddy with a
// real certificate, so there is no legitimate plain-http case here.
//
// Shared by every git provider's connect flow (GitHub App manifest
// callback, GitLab/Bitbucket OAuth redirect) since all of them need the
// same real, reachable callback URL. Returns errNoPrimaryDomain if
// PrimaryDomain is empty: there is no meaningful fallback (not
// "localhost", which none of these providers can reach in a real
// deployment either) to substitute for an operator explicitly
// configuring one first.
func (rt *Router) controlPlaneBaseURL(ctx context.Context) (string, error) {
	settings, err := rt.ingressSettings.GetIngressSettings(ctx)
	if err != nil {
		return "", fmt.Errorf("api: get ingress settings: %w", err)
	}
	if settings.PrimaryDomain == "" {
		return "", errNoPrimaryDomain
	}
	return "https://" + settings.PrimaryDomain, nil
}
