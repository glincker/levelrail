package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strings"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Upstream proxy kinds reported by detection.
const (
	proxyKindNone       = "none"
	proxyKindCloudflare = "cloudflare"
	proxyKindCoolify    = "coolify"
	proxyKindTraefik    = "traefik"
	proxyKindGeneric    = "proxy"
)

const (
	maxEdgeBodyBytes   = int64(1) << 40
	maxEdgeHeaderBytes = int64(16) << 20
	maxEdgeTimeoutSecs = int64(86400)
	edgeSourceEnv      = "env"
	edgeSourceSetting  = "setting"
	edgeSourceDefault  = "default"
)

// IngressEdgeStore is the optional store surface for edge limits; *store.DB satisfies it.
type IngressEdgeStore interface {
	GetIngressEdgeSettings(ctx context.Context) (store.IngressEdgeSettings, error)
	UpdateIngressEdgeSettings(ctx context.Context, s store.IngressEdgeSettings) error
	GetAppIngressLimit(ctx context.Context, app string) (store.AppIngressLimit, error)
	SetAppIngressLimit(ctx context.Context, l store.AppIngressLimit) error
	DeleteAppIngressLimit(ctx context.Context, app string) error
}

type edgeDetection struct {
	Proxy string `json:"proxy"`
	// Evidence lists the request headers the verdict rests on, never their values.
	Evidence []string `json:"evidence,omitempty"`
	PeerAddr string   `json:"peer_addr,omitempty"`
	// Suggested is the value the one-click fix would store; empty when no safe guess exists.
	Suggested string `json:"suggested,omitempty"`
	// RangesRotate warns that the proxy's own address list changes over time.
	RangesRotate bool `json:"ranges_rotate,omitempty"`
}

type edgeLimitValue struct {
	Value  int64  `json:"value"`
	Source string `json:"source"`
}

type edgeSettingsResource struct {
	TrustedProxies       string          `json:"trusted_proxies"`
	TrustedProxiesSource string          `json:"trusted_proxies_source"`
	TrustedProxiesActive bool            `json:"trusted_proxies_active"`
	MaxHeaderBytes       *int64          `json:"max_header_bytes"`
	MaxBodyBytes         *int64          `json:"max_body_bytes"`
	ReadHeaderTimeout    *int64          `json:"read_header_timeout_secs"`
	IdleTimeout          *int64          `json:"idle_timeout_secs"`
	Effective            edgeEffective   `json:"effective"`
	Detected             edgeDetection   `json:"detected"`
	NeedsTrustedProxies  bool            `json:"needs_trusted_proxies"`
	Defaults             edgeEffective   `json:"defaults"`
	EnvLocked            map[string]bool `json:"env_locked"`
}

type edgeEffective struct {
	MaxHeaderBytes    edgeLimitValue `json:"max_header_bytes"`
	MaxBodyBytes      edgeLimitValue `json:"max_body_bytes"`
	ReadHeaderTimeout edgeLimitValue `json:"read_header_timeout_secs"`
	IdleTimeout       edgeLimitValue `json:"idle_timeout_secs"`
}

type edgeSettingsUpdate struct {
	TrustedProxies    string `json:"trusted_proxies"`
	MaxHeaderBytes    *int64 `json:"max_header_bytes"`
	MaxBodyBytes      *int64 `json:"max_body_bytes"`
	ReadHeaderTimeout *int64 `json:"read_header_timeout_secs"`
	IdleTimeout       *int64 `json:"idle_timeout_secs"`
}

type appIngressLimitsResource struct {
	MaxBodyBytes *int64 `json:"max_body_bytes"`
	// InstanceMaxBodyBytes is what applies when the app has no override.
	InstanceMaxBodyBytes int64 `json:"instance_max_body_bytes"`
}

func (rt *Router) edgeStore() IngressEdgeStore {
	st, _ := rt.ingressSettings.(IngressEdgeStore)
	return st
}

// detectUpstreamProxy reads the forwarding headers of a request that reached
// the dashboard to name the proxy in front of this instance.
func detectUpstreamProxy(r *http.Request) edgeDetection {
	d := edgeDetection{Proxy: proxyKindNone}
	var peer netip.Addr
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		peer, _ = netip.ParseAddr(host)
	} else {
		peer, _ = netip.ParseAddr(r.RemoteAddr)
	}
	peer = peer.Unmap()
	if peer.IsValid() {
		d.PeerAddr = peer.String()
	}
	has := func(names ...string) []string {
		var found []string
		for _, n := range names {
			if r.Header.Get(n) != "" {
				found = append(found, n)
			}
		}
		return found
	}
	if cf := has("CF-Ray", "CF-Connecting-IP"); len(cf) > 0 {
		d.Proxy, d.Evidence = proxyKindCloudflare, cf
	} else if tr := has("X-Forwarded-Server"); len(tr) > 0 {
		d.Proxy, d.Evidence = proxyKindTraefik, tr
		if strings.Contains(strings.ToLower(r.Header.Get("X-Forwarded-Server")), "coolify") {
			d.Proxy = proxyKindCoolify
		}
	} else if g := has("X-Forwarded-For", "X-Forwarded-Proto", "X-Forwarded-Host", "X-Real-Ip", "Forwarded", "Via"); len(g) > 0 {
		d.Proxy, d.Evidence = proxyKindGeneric, g
	}
	if d.Proxy == proxyKindNone || !peer.IsValid() {
		return d
	}
	switch {
	case peer.IsLoopback() || peer.IsPrivate():
		d.Suggested = "private_ranges"
	case d.Proxy == proxyKindCloudflare:
		d.RangesRotate = true
	default:
		d.Suggested = peer.String()
	}
	return d
}

func edgeSource(envName string, getenv func(string) string, set bool) string {
	switch {
	case ingress.EnvSet(getenv, envName):
		return edgeSourceEnv
	case set:
		return edgeSourceSetting
	default:
		return edgeSourceDefault
	}
}

func int64Ptr(v int64) *int64 { return &v }

func (rt *Router) edgeResource(r *http.Request, st IngressEdgeStore) (edgeSettingsResource, error) {
	s, err := st.GetIngressEdgeSettings(r.Context())
	if err != nil {
		return edgeSettingsResource{}, err
	}
	getenv := rt.domainPolicy.env
	def := ingress.DefaultHardening()
	base, err := ingress.HardeningFromEnv(getenv)
	if err != nil {
		base = def
	}
	h, err := base.WithOverrides(ingress.EdgeOverrides{
		MaxHeaderBytes: s.MaxHeaderBytes, MaxBodyBytes: s.MaxBodyBytes,
		ReadHeaderTimeoutSecs: s.ReadHeaderTimeoutSecs, IdleTimeoutSecs: s.IdleTimeoutSecs,
	}, getenv)
	if err != nil {
		return edgeSettingsResource{}, err
	}
	eff := func(env string, v int64, set bool) edgeLimitValue {
		return edgeLimitValue{Value: v, Source: edgeSource(env, getenv, set)}
	}
	trustedEnv := strings.TrimSpace(getenv(ingress.EnvTrustedProxies))
	active := trustedEnv != "" || strings.TrimSpace(s.TrustedProxies) != ""
	det := detectUpstreamProxy(r)
	rt.observeUpstreamProxy(r.Context(), det)
	res := edgeSettingsResource{
		TrustedProxies:       s.TrustedProxies,
		TrustedProxiesSource: edgeSource(ingress.EnvTrustedProxies, getenv, strings.TrimSpace(s.TrustedProxies) != ""),
		TrustedProxiesActive: active,
		MaxHeaderBytes:       s.MaxHeaderBytes, MaxBodyBytes: s.MaxBodyBytes,
		ReadHeaderTimeout: s.ReadHeaderTimeoutSecs, IdleTimeout: s.IdleTimeoutSecs,
		Effective: edgeEffective{
			MaxHeaderBytes:    eff(ingress.EnvMaxHeaderBytes, int64(h.MaxHeaderBytes), s.MaxHeaderBytes != nil),
			MaxBodyBytes:      eff(ingress.EnvMaxBodyBytes, h.MaxBodyBytes, s.MaxBodyBytes != nil),
			ReadHeaderTimeout: eff(ingress.EnvReadHeaderTimeout, int64(h.ReadHeaderTimeout.Seconds()), s.ReadHeaderTimeoutSecs != nil),
			IdleTimeout:       eff(ingress.EnvIdleTimeout, int64(h.IdleTimeout.Seconds()), s.IdleTimeoutSecs != nil),
		},
		Defaults: edgeEffective{
			MaxHeaderBytes:    edgeLimitValue{Value: int64(def.MaxHeaderBytes), Source: edgeSourceDefault},
			MaxBodyBytes:      edgeLimitValue{Value: def.MaxBodyBytes, Source: edgeSourceDefault},
			ReadHeaderTimeout: edgeLimitValue{Value: int64(def.ReadHeaderTimeout.Seconds()), Source: edgeSourceDefault},
			IdleTimeout:       edgeLimitValue{Value: int64(def.IdleTimeout.Seconds()), Source: edgeSourceDefault},
		},
		Detected: det,
		EnvLocked: map[string]bool{
			"trusted_proxies":          ingress.EnvSet(getenv, ingress.EnvTrustedProxies),
			"max_header_bytes":         ingress.EnvSet(getenv, ingress.EnvMaxHeaderBytes),
			"max_body_bytes":           ingress.EnvSet(getenv, ingress.EnvMaxBodyBytes),
			"read_header_timeout_secs": ingress.EnvSet(getenv, ingress.EnvReadHeaderTimeout),
			"idle_timeout_secs":        ingress.EnvSet(getenv, ingress.EnvIdleTimeout),
		},
	}
	res.NeedsTrustedProxies = det.Proxy != proxyKindNone && !active
	return res, nil
}

func validateEdgeLimit(name string, v *int64, limit int64) string {
	if v == nil {
		return ""
	}
	if *v < 0 || *v > limit {
		return name + " is out of range"
	}
	return ""
}

func (rt *Router) handleGetIngressEdge(w http.ResponseWriter, r *http.Request) {
	st := rt.edgeStore()
	if st == nil {
		writeError(w, http.StatusNotImplemented, "edge settings are not available")
		return
	}
	res, err := rt.edgeResource(r, st)
	if err != nil {
		rt.internalError(w, "api: get ingress edge settings", err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (rt *Router) handlePutIngressEdge(w http.ResponseWriter, r *http.Request) {
	st := rt.edgeStore()
	if st == nil {
		writeError(w, http.StatusNotImplemented, "edge settings are not available")
		return
	}
	var req edgeSettingsUpdate
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.TrustedProxies = strings.TrimSpace(req.TrustedProxies)
	if req.TrustedProxies != "" {
		if err := ingress.ValidateTrustedProxies(req.TrustedProxies); err != nil {
			writeError(w, http.StatusBadRequest, "trusted_proxies must list IP addresses, CIDR ranges or private_ranges")
			return
		}
	}
	for _, msg := range []string{
		validateEdgeLimit("max_header_bytes", req.MaxHeaderBytes, maxEdgeHeaderBytes),
		validateEdgeLimit("max_body_bytes", req.MaxBodyBytes, maxEdgeBodyBytes),
		validateEdgeLimit("read_header_timeout_secs", req.ReadHeaderTimeout, maxEdgeTimeoutSecs),
		validateEdgeLimit("idle_timeout_secs", req.IdleTimeout, maxEdgeTimeoutSecs),
	} {
		if msg != "" {
			writeError(w, http.StatusBadRequest, msg)
			return
		}
	}
	err := st.UpdateIngressEdgeSettings(r.Context(), store.IngressEdgeSettings{
		TrustedProxies: req.TrustedProxies, MaxHeaderBytes: req.MaxHeaderBytes, MaxBodyBytes: req.MaxBodyBytes,
		ReadHeaderTimeoutSecs: req.ReadHeaderTimeout, IdleTimeoutSecs: req.IdleTimeout,
	})
	if err != nil {
		rt.internalError(w, "api: update ingress edge settings", err)
		return
	}
	rt.nudgeReconciler()
	res, err := rt.edgeResource(r, st)
	if err != nil {
		rt.internalError(w, "api: update ingress edge settings: reload", err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handleApplyDetectedProxy is the one-click fix: it stores the trusted proxy
// value detection suggests for this very request, so the client never names it.
func (rt *Router) handleApplyDetectedProxy(w http.ResponseWriter, r *http.Request) {
	st := rt.edgeStore()
	if st == nil {
		writeError(w, http.StatusNotImplemented, "edge settings are not available")
		return
	}
	if ingress.EnvSet(rt.domainPolicy.env, ingress.EnvTrustedProxies) {
		writeError(w, http.StatusConflict, "the trusted proxy list is set by an environment variable and cannot be changed here")
		return
	}
	det := detectUpstreamProxy(r)
	if det.Suggested == "" {
		writeError(w, http.StatusConflict, "no upstream proxy was detected with an address that is safe to trust automatically; enter the proxy addresses by hand")
		return
	}
	cur, err := st.GetIngressEdgeSettings(r.Context())
	if err != nil {
		rt.internalError(w, "api: apply detected proxy: load", err)
		return
	}
	cur.TrustedProxies = det.Suggested
	if err := st.UpdateIngressEdgeSettings(r.Context(), cur); err != nil {
		rt.internalError(w, "api: apply detected proxy", err)
		return
	}
	rt.logger.Info("ingress: trusted proxies set from detection", slog.String("proxy", det.Proxy), slog.String("value", det.Suggested))
	rt.nudgeReconciler()
	res, err := rt.edgeResource(r, st)
	if err != nil {
		rt.internalError(w, "api: apply detected proxy: reload", err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (rt *Router) appLimitsResource(ctx context.Context, st IngressEdgeStore, r *http.Request, app string) (appIngressLimitsResource, error) {
	res := appIngressLimitsResource{}
	l, err := st.GetAppIngressLimit(ctx, app)
	switch {
	case err == nil:
		res.MaxBodyBytes = int64Ptr(l.MaxBodyBytes)
	case !errors.Is(err, store.ErrAppIngressLimitNotFound):
		return res, err
	}
	edge, err := rt.edgeResource(r, st)
	if err != nil {
		return res, err
	}
	res.InstanceMaxBodyBytes = edge.Effective.MaxBodyBytes.Value
	return res, nil
}

func (rt *Router) handleGetAppIngressLimits(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	st := rt.edgeStore()
	if st == nil {
		writeError(w, http.StatusNotImplemented, "edge settings are not available")
		return
	}
	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: get app ingress limits: load app", err, slog.String("name", name))
		return
	}
	res, err := rt.appLimitsResource(r.Context(), st, r, name)
	if err != nil {
		rt.internalError(w, "api: get app ingress limits", err, slog.String("name", name))
		return
	}
	writeJSON(w, http.StatusOK, res)
}

type appIngressLimitsRequest struct {
	MaxBodyBytes *int64 `json:"max_body_bytes"`
}

func (rt *Router) handlePutAppIngressLimits(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	st := rt.edgeStore()
	if st == nil {
		writeError(w, http.StatusNotImplemented, "edge settings are not available")
		return
	}
	var req appIngressLimitsRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.MaxBodyBytes != nil && (*req.MaxBodyBytes <= 0 || *req.MaxBodyBytes > maxEdgeBodyBytes) {
		writeError(w, http.StatusBadRequest, "max_body_bytes must be a positive size, or null to use the instance value")
		return
	}
	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: set app ingress limits: load app", err, slog.String("name", name))
		return
	}
	var err error
	if req.MaxBodyBytes == nil {
		err = st.DeleteAppIngressLimit(r.Context(), name)
		if errors.Is(err, store.ErrAppIngressLimitNotFound) {
			err = nil
		}
	} else {
		err = st.SetAppIngressLimit(r.Context(), store.AppIngressLimit{App: name, MaxBodyBytes: *req.MaxBodyBytes})
	}
	if err != nil {
		rt.internalError(w, "api: set app ingress limits", err, slog.String("name", name))
		return
	}
	rt.nudgeReconciler()
	res, err := rt.appLimitsResource(r.Context(), st, r, name)
	if err != nil {
		rt.internalError(w, "api: set app ingress limits: reload", err, slog.String("name", name))
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// edgeTrustedProxiesSet reports whether the dashboard setting holds a trusted
// proxy list, for callers that only knew the environment.
func (rt *Router) edgeTrustedProxiesSet(ctx context.Context) bool {
	st := rt.edgeStore()
	if st == nil {
		return false
	}
	s, err := st.GetIngressEdgeSettings(ctx)
	return err == nil && strings.TrimSpace(s.TrustedProxies) != ""
}
