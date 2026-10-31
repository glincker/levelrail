package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

const maxAppCDNBody = 4 << 10

// appCDNStore is the optional slice of the app store that holds the per-app
// Cloudflare proxy override.
type appCDNStore interface {
	GetAppDNSProxied(ctx context.Context, app string) (proxied, set bool, err error)
	SetAppDNSProxied(ctx context.Context, app string, proxied *bool, actor string) error
}

type appCDNResource struct {
	App string `json:"app"`
	// Proxied is the override; null means the app follows the global setting.
	Proxied *bool `json:"proxied"`
	// GlobalProxied is the server-wide default the app falls back to.
	GlobalProxied bool `json:"global_proxied"`
	// Effective is what new DNS records for this app's domains use.
	Effective bool `json:"effective"`
}

type appCDNRequest struct {
	Proxied *bool `json:"proxied"`
}

func (rt *Router) appCDNStore() appCDNStore {
	st, _ := rt.apps.(appCDNStore)
	return st
}

// appDNSProxied resolves the proxy flag for app: its own override, else the
// global ingress setting.
func (rt *Router) appDNSProxied(ctx context.Context, app string, s store.IngressSettings) bool {
	if st := rt.appCDNStore(); st != nil && app != "" {
		if v, set, err := st.GetAppDNSProxied(ctx, app); err == nil && set {
			return v
		} else if err != nil {
			rt.logger.Warn("api: app cdn: read override failed", slog.String("error", err.Error()), slog.String("app", app))
		}
	}
	return s.DNSProxied
}

func (rt *Router) appCDNResource(ctx context.Context, app string) (appCDNResource, error) {
	s, err := rt.ingressSettings.GetIngressSettings(ctx)
	if err != nil {
		return appCDNResource{}, err
	}
	res := appCDNResource{App: app, GlobalProxied: s.DNSProxied, Effective: s.DNSProxied}
	if st := rt.appCDNStore(); st != nil {
		v, set, err := st.GetAppDNSProxied(ctx, app)
		if err != nil {
			return appCDNResource{}, err
		}
		if set {
			res.Proxied, res.Effective = &v, v
		}
	}
	return res, nil
}

// handleGetAppCDN handles GET /api/v1/apps/{name}/cdn.
func (rt *Router) handleGetAppCDN(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !rt.abuseAppExists(w, r, name) {
		return
	}
	res, err := rt.appCDNResource(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: app cdn: read failed", err, slog.String("name", name))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, res)
}

// handlePutAppCDN handles PUT /api/v1/apps/{name}/cdn. A null proxied clears
// the override. It affects records written from now on, not existing ones.
func (rt *Router) handlePutAppCDN(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	st := rt.appCDNStore()
	if st == nil {
		writeError(w, http.StatusNotImplemented, "per-app CDN settings are not available")
		return
	}
	if !rt.abuseAppExists(w, r, name) {
		return
	}
	var req appCDNRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAppCDNBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	_, _, actor, _ := rt.currentActor(r)
	if err := st.SetAppDNSProxied(r.Context(), name, req.Proxied, actor); err != nil {
		rt.internalError(w, "api: app cdn: save failed", err, slog.String("name", name))
		return
	}
	res, err := rt.appCDNResource(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: app cdn: read back failed", err, slog.String("name", name))
		return
	}
	writeJSON(w, http.StatusOK, res)
}
