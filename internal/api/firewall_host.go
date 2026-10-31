package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/bindaddr"
)

const (
	envFirewallSSHPorts   = "APP_FIREWALL_SSH_PORTS"
	defaultFirewallSSHTCP = 22
	httpsUDPPort          = 443
)

type hostFirewallPort struct {
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
}

// hostFirewallPublishedPort is one host port an app binds directly, tcp or
// udp, with whether the host firewall lets it in right now.
type hostFirewallPublishedPort struct {
	Port        int    `json:"port"`
	Protocol    string `json:"protocol"`
	App         string `json:"app"`
	Service     string `json:"service"`
	BindAddress string `json:"bind_address"`
	Allowed     bool   `json:"allowed"`
}

type hostFirewallResource struct {
	Installed       bool                        `json:"installed"`
	Active          bool                        `json:"active"`
	DefaultIncoming string                      `json:"default_incoming,omitempty"`
	Required        []hostFirewallPort          `json:"required"`
	Commands        []string                    `json:"commands"`
	PublishedPorts  []hostFirewallPublishedPort `json:"published_ports"`
}

type hostFirewallActionRequest struct {
	DryRun bool `json:"dry_run"`
}

func execFirewallCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput() // #nosec G204 -- name is always "ufw", args are built from validated ports
}

// hostFirewallSSHPorts reads the SSH ports to keep open from APP_FIREWALL_SSH_PORTS
// (comma separated), defaulting to 22 so a stock server is never locked out.
func hostFirewallSSHPorts() []int {
	raw := strings.TrimSpace(os.Getenv(envFirewallSSHPorts))
	if raw == "" {
		return []int{defaultFirewallSSHTCP}
	}
	var ports []int
	for _, part := range strings.Split(raw, ",") {
		p, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || p < 1 || p > 65535 {
			continue
		}
		ports = append(ports, p)
	}
	if len(ports) == 0 {
		return []int{defaultFirewallSSHTCP}
	}
	return ports
}

// hostFirewallRequired lists SSH first, then the ports the control plane itself
// needs, then 443/udp for HTTP/3.
func (rt *Router) hostFirewallRequired(ctx context.Context) []hostFirewallPort {
	return rt.hostFirewallRequiredFor(ctx, rt.publishedHostPortRows(ctx))
}

func (rt *Router) hostFirewallRequiredFor(ctx context.Context, rows []hostFirewallPublishedPort) []hostFirewallPort {
	var out []hostFirewallPort
	seen := map[hostFirewallPort]bool{}
	add := func(p hostFirewallPort) {
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	for _, p := range rt.hostFirewallSSHPorts {
		add(hostFirewallPort{Port: p, Protocol: "tcp"})
	}
	ports := slices.Clone(rt.firewallRequiredPorts)
	slices.Sort(ports)
	for _, p := range ports {
		if p < 1 || p > 65535 {
			continue
		}
		add(hostFirewallPort{Port: p, Protocol: "tcp"})
	}
	add(hostFirewallPort{Port: httpsUDPPort, Protocol: "udp"})
	var published []hostFirewallPort
	for _, row := range rows {
		if row.BindAddress != bindaddr.Private {
			published = append(published, hostFirewallPort{Port: row.Port, Protocol: row.Protocol})
		}
	}
	for _, p := range rt.streamFirewallPorts(ctx) {
		published = append(published, hostFirewallPort{Port: p, Protocol: "tcp"})
	}
	slices.SortFunc(published, func(a, b hostFirewallPort) int {
		if a.Port != b.Port {
			return a.Port - b.Port
		}
		return strings.Compare(a.Protocol, b.Protocol)
	})
	for _, p := range published {
		if p.Port >= 1 && p.Port <= 65535 {
			add(p)
		}
	}
	return out
}

// publishedHostPortRows lists every host port an app binds directly: the
// pinned routed port (tcp) and each published port with its own protocol.
func (rt *Router) publishedHostPortRows(ctx context.Context) []hostFirewallPublishedPort {
	services, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		rt.logger.Warn("api: firewall: list services for published ports failed", slog.String("error", err.Error()))
		return []hostFirewallPublishedPort{}
	}
	out := []hostFirewallPublishedPort{}
	for _, svc := range services {
		row := hostFirewallPublishedPort{App: svc.AppID, Service: svc.Name, BindAddress: svc.BindAddress}
		if row.App == "" {
			row.App = svc.Name
		}
		if row.BindAddress == "" {
			row.BindAddress = bindaddr.Default
		}
		if svc.HostPort != nil {
			row.Port, row.Protocol = *svc.HostPort, "tcp"
			out = append(out, row)
		}
		for _, p := range svc.PublishedPorts {
			portRow := row
			portRow.Port, portRow.Protocol = p.HostPort, p.EffectiveProtocol()
			portRow.BindAddress = p.EffectiveBind(row.BindAddress)
			out = append(out, portRow)
		}
	}
	return slices.DeleteFunc(out, func(r hostFirewallPublishedPort) bool { return r.Port < 1 || r.Port > 65535 })
}

// streamFirewallPorts lists raw TCP stream ports, so enabling ufw never cuts
// off a running app.
func (rt *Router) streamFirewallPorts(ctx context.Context) []int {
	services, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return nil
	}
	var ports []int
	for _, svc := range services {
		streams, err := rt.apps.ListAppStreamsForService(ctx, svc.Name)
		if err != nil {
			continue
		}
		for _, st := range streams {
			ports = append(ports, st.HostPort)
		}
	}
	return ports
}

// ufwAllowed reads the ALLOW rules of `ufw status verbose`. A bare port
// rule ("53 ALLOW IN") covers both protocols; ranges use "lo:hi".
func ufwAllowed(text string, port int, proto string) bool {
	for _, line := range strings.Split(text, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 || f[1] != "ALLOW" {
			continue
		}
		ports, rp, hasProto := strings.Cut(f[0], "/")
		if hasProto && rp != proto {
			continue
		}
		for _, part := range strings.Split(ports, ",") {
			lo, hi, isRange := strings.Cut(part, ":")
			l, err := strconv.Atoi(lo)
			if err != nil {
				continue
			}
			h := l
			if isRange {
				if h, err = strconv.Atoi(hi); err != nil {
					continue
				}
			}
			if port >= l && port <= h {
				return true
			}
		}
	}
	return false
}
func allowArgs(p hostFirewallPort) []string {
	return []string{"allow", fmt.Sprintf("%d/%s", p.Port, p.Protocol)}
}

func (rt *Router) hostFirewallStatus(ctx context.Context) hostFirewallResource {
	rows := rt.publishedHostPortRows(ctx)
	res := hostFirewallResource{Required: rt.hostFirewallRequiredFor(ctx, rows), Commands: []string{}, PublishedPorts: rows}
	for _, p := range res.Required {
		res.Commands = append(res.Commands, "ufw "+strings.Join(allowArgs(p), " "))
	}
	res.Commands = append(res.Commands, "ufw --force enable")
	text := ""
	if _, err := rt.hostFirewallLookPath("ufw"); err == nil {
		res.Installed = true
		if out, err := rt.hostFirewallRun(ctx, "ufw", "status", "verbose"); err == nil {
			text = string(out)
		}
	}
	res.Active = strings.Contains(text, "Status: active")
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) > 1 && fields[0] == "Default:" {
			res.DefaultIncoming = fields[1]
		}
	}
	// Nothing filters while ufw is absent, inactive or defaulting to allow.
	open := !res.Active || res.DefaultIncoming == "allow"
	for i, row := range res.PublishedPorts {
		res.PublishedPorts[i].Allowed = open || ufwAllowed(text, row.Port, row.Protocol)
	}
	return res
}

// handleHostFirewallStatus handles GET /api/v1/firewall/host.
func (rt *Router) handleHostFirewallStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, rt.hostFirewallStatus(r.Context()))
}

func decodeHostFirewallAction(r *http.Request) (hostFirewallActionRequest, bool) {
	var req hostFirewallActionRequest
	if r.ContentLength == 0 {
		return req, true
	}
	return req, json.NewDecoder(r.Body).Decode(&req) == nil
}

// handleEnableHostFirewall handles POST /api/v1/firewall/host/enable. Every
// required port is allowed before ufw is switched on, and any failure stops
// before the enable step, so a half-applied run leaves the firewall off.
func (rt *Router) handleEnableHostFirewall(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeHostFirewallAction(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	status := rt.hostFirewallStatus(r.Context())
	if !status.Installed {
		writeError(w, http.StatusConflict, "ufw is not installed on this host")
		return
	}
	if req.DryRun {
		writeJSON(w, http.StatusOK, status)
		return
	}
	for _, p := range status.Required {
		if out, err := rt.hostFirewallRun(r.Context(), "ufw", allowArgs(p)...); err != nil {
			rt.logger.Error("api: enable host firewall: allow failed", slog.Int("port", p.Port), slog.String("output", string(out)), slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("could not allow %d/%s, the firewall was not enabled", p.Port, p.Protocol))
			return
		}
	}
	if out, err := rt.hostFirewallRun(r.Context(), "ufw", "--force", "enable"); err != nil {
		rt.logger.Error("api: enable host firewall failed", slog.String("output", string(out)), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "ufw could not be enabled")
		return
	}
	rt.nudgeReconciler()
	writeJSON(w, http.StatusOK, rt.hostFirewallStatus(r.Context()))
}

// handleDisableHostFirewall handles POST /api/v1/firewall/host/disable.
func (rt *Router) handleDisableHostFirewall(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeHostFirewallAction(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	status := rt.hostFirewallStatus(r.Context())
	if !status.Installed {
		writeError(w, http.StatusConflict, "ufw is not installed on this host")
		return
	}
	if req.DryRun {
		status.Commands = []string{"ufw disable"}
		writeJSON(w, http.StatusOK, status)
		return
	}
	if out, err := rt.hostFirewallRun(r.Context(), "ufw", "disable"); err != nil {
		rt.logger.Error("api: disable host firewall failed", slog.String("output", string(out)), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "ufw could not be disabled")
		return
	}
	writeJSON(w, http.StatusOK, rt.hostFirewallStatus(r.Context()))
}
