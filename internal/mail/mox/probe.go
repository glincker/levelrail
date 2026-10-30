package mox

import (
	"context"
	"net/http"
	"net/netip"
	"net/url"

	"github.com/GLINCKER/levelrail/kit/netguard"
)

// AdminReachable reports whether the admin listener at base answers HTTP.
// It sends no credentials: any non-5xx reply (a login prompt included)
// proves the listener is up. Private-target rules match New.
func AdminReachable(ctx context.Context, base string) bool {
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return false
	}
	if ip, perr := netip.ParseAddr(u.Hostname()); perr == nil && !netguard.IsPrivateTargetAllowed(ip) {
		return false
	}
	hc := netguard.NewClientPrivateTarget()
	hc.Timeout = envDuration(TimeoutEnv, defaultTimeout)
	hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String()+"/admin/", nil)
	if err != nil {
		return false
	}
	resp, err := hc.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode < 500
}
