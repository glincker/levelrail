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

type hostFirewallResource struct {
	Installed       bool               `json:"installed"`
	Active          bool               `json:"active"`
	DefaultIncoming string             `json:"default_incoming,omitempty"`
	Required        []hostFirewallPort `json:"required"`
	Commands        []string           `json:"commands"`
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
func (rt *Router) hostFirewallRequired() []hostFirewallPort {
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
	return out
}

func allowArgs(p hostFirewallPort) []string {
	return []string{"allow", fmt.Sprintf("%d/%s", p.Port, p.Protocol)}
}

func (rt *Router) hostFirewallStatus(ctx context.Context) hostFirewallResource {
	res := hostFirewallResource{Required: rt.hostFirewallRequired(), Commands: []string{}}
	for _, p := range res.Required {
		res.Commands = append(res.Commands, "ufw "+strings.Join(allowArgs(p), " "))
	}
	res.Commands = append(res.Commands, "ufw --force enable")
	if _, err := rt.hostFirewallLookPath("ufw"); err != nil {
		return res
	}
	res.Installed = true
	out, err := rt.hostFirewallRun(ctx, "ufw", "status", "verbose")
	if err != nil {
		return res
	}
	text := string(out)
	res.Active = strings.Contains(text, "Status: active")
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) > 1 && fields[0] == "Default:" {
			res.DefaultIncoming = fields[1]
		}
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
