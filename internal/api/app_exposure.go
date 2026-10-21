package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/bindaddr"
	"github.com/GLINCKER/levelrail/internal/diagnose"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/firewall"
)

// Exposure modes: proxied keeps the app behind the ingress (its domain, or
// the automatic hostname); direct also publishes a host port on every
// interface and opens it in the host firewall.
const (
	exposureModeProxied = "proxied"
	exposureModeDirect  = "direct"
)

const (
	exposureOutcomeUnchanged  = "unchanged"
	exposureOutcomeApplied    = "applied"
	exposureOutcomeRecreating = "recreating"
)

const exposureRuleLabelPrefix = "app-exposure:"

// hostListeningPorts reads this server's listening TCP ports; replaced in tests.
var hostListeningPorts = func() []int {
	var tables []string
	for _, p := range procNetTables {
		if b, err := os.ReadFile(p); err == nil { //nolint:gosec // fixed /proc paths
			tables = append(tables, string(b))
		}
	}
	return diagnose.ParseListeningPorts(tables...)
}

type exposureRequest struct {
	Mode          string `json:"mode,omitempty"`
	HostPort      *int   `json:"host_port,omitempty"`
	ContainerPort *int   `json:"container_port,omitempty"`
}

type exposureChange struct {
	Kind string `json:"kind"`
	From string `json:"from,omitempty"`
	To   string `json:"to,omitempty"`
}

type exposureURL struct {
	Kind string `json:"kind"`
	URL  string `json:"url"`
}

type exposurePlan struct {
	Mode          string           `json:"mode"`
	ContainerPort int              `json:"container_port"`
	HostPort      int              `json:"host_port,omitempty"`
	BindAddress   string           `json:"bind_address"`
	Changes       []exposureChange `json:"changes"`
	Warnings      []string         `json:"warnings,omitempty"`
	Recreates     bool             `json:"recreates"`
	URLs          []exposureURL    `json:"urls"`
}

type exposureResource struct {
	exposurePlan
	App         string       `json:"app"`
	Running     bool         `json:"running"`
	FirewallRow string       `json:"firewall_rule_id,omitempty"`
	PortsInUse  []portHolder `json:"ports_in_use"`
	PortRange   [2]int       `json:"port_range"`
	// Outcome is set on PUT only: unchanged, applied (no container
	// recreate needed) or recreating (queued, the reconciler republishes).
	Outcome string `json:"outcome,omitempty"`
	// LocalNode is false for an app placed on a remote node, where direct
	// exposure is unavailable (the firewall reconciler only manages this node).
	LocalNode bool `json:"local_node"`
}

type exposureError struct {
	status int
	msg    string
}

func (e *exposureError) Error() string { return e.msg }

func isDirectExposure(svc *store.DesiredService) bool {
	return svc.HostPort != nil && svc.BindAddress != "" &&
		svc.BindAddress != bindaddr.Private && svc.BindAddress != "127.0.0.1"
}

// takenHostPorts merges every source of an occupied host port on this node:
// the control plane's own listeners, other apps' pins and streams, live
// container bindings, and real listeners from the kernel socket table.
func (rt *Router) takenHostPorts(ctx context.Context, self string) map[int]portHolder {
	taken := map[int]portHolder{}
	add := func(p int, kind, holder string) {
		if p < 1 || p > 65535 {
			return
		}
		if cur, ok := taken[p]; ok && cur.Kind != holderKindListener {
			return
		}
		taken[p] = portHolder{Port: p, Kind: kind, Holder: holder}
	}
	for _, p := range hostListeningPorts() {
		add(p, holderKindListener, "")
	}
	for _, p := range []int{80, 443, 8080, 8088, 8443, 9443} {
		add(p, holderKindControl, "ingress or dashboard")
	}
	for _, cp := range rt.standaloneSafety.ControlPorts {
		add(cp.Port, holderKindControl, cp.Label)
	}
	for _, p := range rt.standaloneSafety.IngressPorts {
		add(p, holderKindControl, "ingress")
	}
	for _, p := range rt.firewallRequiredPorts {
		add(p, holderKindControl, "required by the firewall guard")
	}
	for _, p := range rt.hostFirewallSSHPorts {
		add(p, holderKindControl, "ssh")
	}
	svcs, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		rt.logger.Warn("api: exposure: list apps failed", slog.String("error", err.Error()))
		return taken
	}
	byContainer := map[string]string{}
	for _, s := range svcs {
		byContainer[application.ContainerName(s.Name, application.NameImage(s), s.RestartNonce)] = s.Name
		if s.HostPort != nil && s.Name != self {
			add(*s.HostPort, holderKindApp, s.Name)
		}
		if streams, serr := rt.apps.ListAppStreamsForService(ctx, s.Name); serr == nil && s.Name != self {
			for _, st := range streams {
				add(st.HostPort, holderKindStream, s.Name)
			}
		}
	}
	if rt.containers != nil {
		if list, lerr := rt.containers.ListByPrefix(ctx, ""); lerr == nil {
			for _, c := range list {
				owner, ok := byContainer[c.Name]
				if !c.Running || !ok || owner == self {
					continue
				}
				for _, b := range c.Ports {
					add(b.HostPort, holderKindApp, owner)
				}
			}
		}
	}
	return taken
}

// selfHostPorts are the host ports this app's own container holds, which
// its own listener entries must not count against it.
func (rt *Router) selfHostPorts(ctx context.Context, svc *store.DesiredService) []int {
	var out []int
	if svc.HostPort != nil {
		out = append(out, *svc.HostPort)
	}
	if rt.execRuntime == nil {
		return out
	}
	runtime, err := rt.execRuntime(svc.NodeID)
	if err != nil {
		return out
	}
	ictx, cancel := context.WithTimeout(ctx, dockerInspectTimeout)
	defer cancel()
	st, err := runtime.InspectByName(ictx, application.ContainerName(svc.Name, application.NameImage(*svc), svc.RestartNonce))
	if err == nil && st != nil {
		for _, b := range st.Ports {
			out = append(out, b.HostPort)
		}
	}
	return out
}

func (rt *Router) exposureRule(ctx context.Context, app string) (store.FirewallRule, bool) {
	rules, err := rt.firewallRules.ListFirewallRules(ctx)
	if err != nil {
		return store.FirewallRule{}, false
	}
	for _, r := range rules {
		if r.Label == exposureRuleLabelPrefix+app {
			return r, true
		}
	}
	return store.FirewallRule{}, false
}

// planExposure validates req against svc's current state and returns the
// exact change set; it never writes.
func (rt *Router) planExposure(ctx context.Context, svc *store.DesiredService, req exposureRequest) (exposurePlan, *exposureError) {
	plan := exposurePlan{Mode: exposureModeProxied, ContainerPort: svc.Port, BindAddress: svc.BindAddress, Changes: []exposureChange{}, URLs: []exposureURL{}}
	if isDirectExposure(svc) {
		plan.Mode = exposureModeDirect
		plan.HostPort = *svc.HostPort
	}
	curMode := plan.Mode
	if req.Mode != "" {
		if req.Mode != exposureModeProxied && req.Mode != exposureModeDirect {
			return plan, &exposureError{http.StatusBadRequest, `mode must be "proxied" or "direct"`}
		}
		plan.Mode = req.Mode
	}
	if req.ContainerPort != nil {
		if err := validateExposurePort("container_port", *req.ContainerPort); err != nil {
			return plan, &exposureError{http.StatusBadRequest, err.Error()}
		}
		if *req.ContainerPort != svc.Port {
			plan.Changes = append(plan.Changes, exposureChange{Kind: "container_port", From: strconv.Itoa(svc.Port), To: strconv.Itoa(*req.ContainerPort)})
			plan.ContainerPort = *req.ContainerPort
			plan.Recreates = true
		}
	}
	if req.HostPort != nil {
		if err := validateExposurePort("host_port", *req.HostPort); err != nil {
			return plan, &exposureError{http.StatusBadRequest, err.Error()}
		}
		if req.Mode == "" {
			plan.Mode = exposureModeDirect
		}
	}
	if plan.Mode == exposureModeDirect && svc.NodeID != "" {
		return plan, &exposureError{http.StatusConflict, "direct exposure is only available for apps on the control plane's own node"}
	}
	if plan.Mode == exposureModeDirect {
		if rerr := rt.planDirect(ctx, svc, req, curMode, &plan); rerr != nil {
			return plan, rerr
		}
	} else {
		plan.HostPort = 0
		plan.BindAddress = bindaddr.Private
		if curMode == exposureModeDirect {
			plan.Changes = append(plan.Changes,
				exposureChange{Kind: "host_port", From: strconv.Itoa(*svc.HostPort), To: "automatic"},
				exposureChange{Kind: "bind_address", From: svc.BindAddress, To: bindaddr.Private},
				exposureChange{Kind: "firewall", From: "open", To: "closed"})
			plan.Recreates = true
		}
	}
	rt.fillExposureURLs(ctx, svc, &plan)
	return plan, nil
}

func (rt *Router) planDirect(ctx context.Context, svc *store.DesiredService, req exposureRequest, curMode string, plan *exposurePlan) *exposureError {
	taken := rt.takenHostPorts(ctx, svc.Name)
	for _, p := range rt.selfHostPorts(ctx, svc) {
		if h, ok := taken[p]; ok && h.Kind == holderKindListener {
			delete(taken, p)
		}
	}
	want := 0
	switch {
	case req.HostPort != nil:
		want = *req.HostPort
	case svc.HostPort != nil:
		want = *svc.HostPort
	}
	if want != 0 {
		if h, busy := taken[want]; busy {
			return &exposureError{http.StatusConflict, fmt.Sprintf("host port %d is held by %s", want, h.describe())}
		}
	} else {
		lo, hi := exposurePortRange(osGetenv)
		p, ok := pickHostPort(svc.Name, lo, hi, taken)
		if !ok {
			return &exposureError{http.StatusConflict, fmt.Sprintf("no free host port in %d-%d", lo, hi)}
		}
		want = p
	}
	rule := firewall.Rule{Port: want, Proto: "tcp", Action: firewall.ActionAllow}
	if err := firewall.Validate(rule, rt.firewallRequiredPorts); err != nil {
		return &exposureError{http.StatusBadRequest, err.Error()}
	}
	prevPort := "automatic"
	if svc.HostPort != nil {
		prevPort = strconv.Itoa(*svc.HostPort)
	}
	plan.HostPort = want
	plan.BindAddress = bindaddr.Public
	if curMode != exposureModeDirect || prevPort != strconv.Itoa(want) {
		plan.Changes = append(plan.Changes, exposureChange{Kind: "host_port", From: prevPort, To: strconv.Itoa(want)})
		plan.Recreates = true
	}
	if curMode != exposureModeDirect {
		plan.Changes = append(plan.Changes, exposureChange{Kind: "bind_address", From: svc.BindAddress, To: bindaddr.Public})
	}
	if _, has := rt.exposureRule(ctx, svc.Name); !has || prevPort != strconv.Itoa(want) {
		plan.Changes = append(plan.Changes, exposureChange{Kind: "firewall", From: "closed", To: "allow " + strconv.Itoa(want) + "/tcp"})
	}
	plan.Warnings = append(plan.Warnings, "plain_http")
	return nil
}

func (rt *Router) fillExposureURLs(ctx context.Context, svc *store.DesiredService, plan *exposurePlan) {
	ing, err := rt.ingressSettings.GetIngressSettings(ctx)
	if err != nil {
		ing = store.IngressSettings{}
	}
	for _, d := range svc.Domains {
		plan.URLs = append(plan.URLs, exposureURL{Kind: "domain", URL: rt.ingressHTTPSURL(ing, d)})
	}
	if len(svc.Domains) == 0 && !ing.FallbackDomainsDisabled {
		if fb, ok := rt.autoHost(ctx, svc.Name); ok {
			plan.URLs = append(plan.URLs, exposureURL{Kind: "fallback", URL: rt.ingressHTTPSURL(ing, fb)})
		}
	}
	if plan.Mode == exposureModeDirect && plan.HostPort > 0 && rt.publicHost != "" {
		plan.URLs = append(plan.URLs, exposureURL{Kind: "direct", URL: "http://" + exposureHostPort(rt.publicHost, plan.HostPort)})
	}
}

func (rt *Router) exposureState(ctx context.Context, svc *store.DesiredService) exposureResource {
	plan, _ := rt.planExposure(ctx, svc, exposureRequest{})
	lo, hi := exposurePortRange(osGetenv)
	res := exposureResource{exposurePlan: plan, App: svc.Name, PortRange: [2]int{lo, hi}, LocalNode: svc.NodeID == "", PortsInUse: []portHolder{}}
	if r, ok := rt.exposureRule(ctx, svc.Name); ok {
		res.FirewallRow = r.ID
	}
	for _, h := range rt.takenHostPorts(ctx, svc.Name) {
		if h.Kind == holderKindApp || h.Kind == holderKindStream || h.Kind == holderKindControl {
			res.PortsInUse = append(res.PortsInUse, h)
		}
	}
	sort.Slice(res.PortsInUse, func(i, j int) bool { return res.PortsInUse[i].Port < res.PortsInUse[j].Port })
	return res
}

func (rt *Router) loadExposureApp(w http.ResponseWriter, r *http.Request) (*store.DesiredService, bool) {
	name := r.PathValue("name")
	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return nil, false
	}
	if err != nil {
		rt.internalError(w, "api: exposure: load app failed", err, slog.String("name", name))
		return nil, false
	}
	return svc, true
}

// handleGetAppExposure handles GET /api/v1/apps/{name}/exposure.
func (rt *Router) handleGetAppExposure(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.loadExposureApp(w, r)
	if !ok {
		return
	}
	res := rt.exposureState(r.Context(), svc)
	res.Running = rt.appsRunning(r.Context(), []string{svc.Name})[svc.Name]
	writeJSON(w, http.StatusOK, res)
}

func decodeExposureRequest(w http.ResponseWriter, r *http.Request) (exposureRequest, bool) {
	var req exposureRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return req, false
	}
	return req, true
}

// handlePreviewAppExposure handles POST /api/v1/apps/{name}/exposure/preview.
func (rt *Router) handlePreviewAppExposure(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.loadExposureApp(w, r)
	if !ok {
		return
	}
	req, ok := decodeExposureRequest(w, r)
	if !ok {
		return
	}
	plan, eerr := rt.planExposure(r.Context(), svc, req)
	if eerr != nil {
		writeError(w, eerr.status, eerr.msg)
		return
	}
	writeJSON(w, http.StatusOK, plan)
}

// handleSetAppExposure handles PUT /api/v1/apps/{name}/exposure: writes the
// desired port, bind address and tagged firewall rule; the reconcilers
// recreate the container only when a baked-in field changed.
func (rt *Router) handleSetAppExposure(w http.ResponseWriter, r *http.Request) {
	svc, ok := rt.loadExposureApp(w, r)
	if !ok {
		return
	}
	req, ok := decodeExposureRequest(w, r)
	if !ok {
		return
	}
	plan, eerr := rt.planExposure(r.Context(), svc, req)
	if eerr != nil {
		writeError(w, eerr.status, eerr.msg)
		return
	}
	if len(plan.Changes) == 0 {
		res := rt.exposureState(r.Context(), svc)
		res.Outcome = exposureOutcomeUnchanged
		writeJSON(w, http.StatusOK, res)
		return
	}
	next := *svc
	next.Port = plan.ContainerPort
	next.BindAddress = plan.BindAddress
	next.HostPort = nil
	if plan.HostPort > 0 {
		hp := plan.HostPort
		next.HostPort = &hp
	}
	if err := rt.apps.SaveDesiredService(r.Context(), next); err != nil {
		rt.internalError(w, "api: exposure: save app failed", err, slog.String("name", svc.Name))
		return
	}
	if !rt.syncExposureRule(w, r, svc.Name, plan) {
		return
	}
	rt.recordAppEvent(r, store.AppEvent{AppName: svc.Name, Kind: store.AppEventConfigChange, Title: "Exposure changed", Detail: exposureDetail(plan)})
	rt.nudgeReconciler()
	fresh, err := rt.apps.GetDesiredService(r.Context(), svc.Name)
	if err != nil {
		fresh = &next
	}
	res := rt.exposureState(r.Context(), fresh)
	res.Outcome = exposureOutcomeApplied
	if plan.Recreates {
		res.Outcome = exposureOutcomeRecreating
	}
	writeJSON(w, http.StatusOK, res)
}

func exposureDetail(p exposurePlan) string {
	out := "mode " + p.Mode
	for _, c := range p.Changes {
		out += "; " + c.Kind + " " + c.From + " -> " + c.To
	}
	return out
}

// syncExposureRule makes the tagged firewall rule match the plan: exactly
// one allow rule for the pinned port in direct mode, none otherwise.
func (rt *Router) syncExposureRule(w http.ResponseWriter, r *http.Request, app string, plan exposurePlan) bool {
	ctx := r.Context()
	cur, has := rt.exposureRule(ctx, app)
	if has && (plan.Mode != exposureModeDirect || cur.Port != plan.HostPort) {
		if err := rt.firewallRules.DeleteFirewallRule(ctx, cur.ID); err != nil && !errors.Is(err, store.ErrFirewallRuleNotFound) {
			rt.internalError(w, "api: exposure: remove firewall rule failed", err, slog.String("name", app))
			return false
		}
		has = false
	}
	if plan.Mode != exposureModeDirect || has {
		return true
	}
	id, err := randomFirewallRuleID()
	if err != nil {
		rt.internalError(w, "api: exposure: firewall rule id failed", err, slog.String("name", app))
		return false
	}
	rule := store.FirewallRule{ID: id, Port: plan.HostPort, Protocol: "tcp", Action: store.FirewallRuleActionAllow,
		Label: exposureRuleLabelPrefix + app, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	if err := rt.firewallRules.SaveFirewallRule(ctx, rule); err != nil {
		rt.internalError(w, "api: exposure: save firewall rule failed", err, slog.String("name", app))
		return false
	}
	return true
}

func exposureHostPort(host string, port int) string {
	return net.JoinHostPort(host, strconv.Itoa(port))
}
