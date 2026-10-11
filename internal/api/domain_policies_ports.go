package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/firewall"
)

// portRestrictLabelPrefix marks firewall rules owned by the "open this
// port only to these IPs" helper, so it can replace exactly its own rules.
const portRestrictLabelPrefix = "domain-ports:"

const maxRestrictSources = 64

type portHolderRow struct {
	Container string `json:"container"`
	Image     string `json:"image"`
}

type portStreamRow struct {
	ID             string          `json:"id"`
	Protocol       string          `json:"protocol"`
	HostPort       int             `json:"host_port"`
	ContainerPort  int             `json:"container_port"`
	Target         string          `json:"target"`
	OpenToAll      bool            `json:"open_to_all"`
	AllowedSources []string        `json:"allowed_sources"`
	Conflicts      []string        `json:"conflicts"`
	Holders        []portHolderRow `json:"holders"`
}

type domainPortsResource struct {
	App             string          `json:"app"`
	Domain          string          `json:"domain"`
	PublicHTTPSPort int             `json:"public_https_port"`
	Streams         []portStreamRow `json:"streams"`
	DetectionNote   string          `json:"detection_note,omitempty"`
}

func portRestrictLabel(port int, proto string) string {
	return portRestrictLabelPrefix + strconv.Itoa(port) + "/" + proto
}

// containersPublishing maps a host port to the running containers that
// publish it, the same detection the reverse proxy guide uses for 80/443.
func (rt *Router) containersPublishing(ctx context.Context) (map[int][]portHolderRow, bool) {
	if rt.execRuntime == nil {
		return nil, false
	}
	runtime, err := rt.execRuntime("")
	if err != nil {
		return nil, false
	}
	lister, ok := runtime.(LocalContainerLister)
	if !ok {
		return nil, false
	}
	all, err := lister.ListLocalContainers(ctx)
	if err != nil {
		return nil, false
	}
	out := map[int][]portHolderRow{}
	for _, c := range all {
		if !c.Running {
			continue
		}
		for _, p := range c.Published {
			out[p] = append(out[p], portHolderRow{Container: c.Name, Image: c.Image})
		}
	}
	return out, true
}

func (rt *Router) handleGetDomainPorts(w http.ResponseWriter, r *http.Request) {
	domain, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	app := r.PathValue("name")
	streams, err := rt.appStreams.ListAppStreamsForService(ctx, app)
	if err != nil {
		rt.internalError(w, "api: domain ports list streams failed", err, slog.String("app", app))
		return
	}
	all, err := rt.appStreams.ListAllAppStreams(ctx)
	if err != nil {
		rt.internalError(w, "api: domain ports list all streams failed", err)
		return
	}
	rules, err := rt.firewallRules.ListFirewallRules(ctx)
	if err != nil {
		rt.internalError(w, "api: domain ports list firewall failed", err)
		return
	}
	settings, err := rt.ingressSettings.GetIngressSettings(ctx)
	if err != nil {
		rt.internalError(w, "api: domain ports settings failed", err)
		return
	}
	holders, detected := rt.containersPublishing(ctx)
	out := domainPortsResource{App: app, Domain: domain, PublicHTTPSPort: settings.PublicLinkPort(), Streams: []portStreamRow{}}
	if !detected {
		out.DetectionNote = "container port detection is unavailable on this instance, so ports held by other containers are not shown"
	}
	for _, s := range streams {
		row := portStreamRow{
			ID: s.ID, Protocol: s.Protocol, HostPort: s.HostPort, ContainerPort: s.ContainerPort,
			Target: fmt.Sprintf("%s:%d", app, s.ContainerPort), OpenToAll: true,
			AllowedSources: []string{}, Conflicts: []string{}, Holders: holders[s.HostPort],
		}
		for _, other := range all {
			if other.ID != s.ID && other.HostPort == s.HostPort && other.Protocol == s.Protocol {
				row.Conflicts = append(row.Conflicts, fmt.Sprintf("stream %s of app %s uses the same port", other.ID, other.ServiceName))
			}
		}
		if s.HostPort == 80 || s.HostPort == 443 || s.HostPort == settings.PublicLinkPort() {
			row.Conflicts = append(row.Conflicts, "the HTTP ingress listens on this port")
		}
		for _, h := range row.Holders {
			row.Conflicts = append(row.Conflicts, fmt.Sprintf("container %s already publishes this port", h.Container))
		}
		if row.Holders == nil {
			row.Holders = []portHolderRow{}
		}
		for _, rule := range rules {
			if rule.Port != s.HostPort || rule.Protocol != s.Protocol {
				continue
			}
			switch {
			case rule.Action == store.FirewallRuleActionAllow && rule.SourceCIDR != "":
				row.AllowedSources = append(row.AllowedSources, rule.SourceCIDR)
			case rule.Action == store.FirewallRuleActionDeny && rule.SourceCIDR == "":
				row.OpenToAll = false
			}
		}
		out.Streams = append(out.Streams, row)
	}
	writeJSON(w, http.StatusOK, out)
}

type restrictPortRequest struct {
	Sources []string `json:"sources"`
}

// streamForPort finds app's stream on the path's {port}, writing a 404 when
// the app has none: the helper only manages ports the app itself opened.
func (rt *Router) streamForPort(w http.ResponseWriter, r *http.Request) (store.AppStream, bool) {
	port, err := strconv.Atoi(r.PathValue("port"))
	if err != nil || port < 1 || port > 65535 {
		writeError(w, http.StatusBadRequest, "port must be between 1 and 65535")
		return store.AppStream{}, false
	}
	streams, err := rt.appStreams.ListAppStreamsForService(r.Context(), r.PathValue("name"))
	if err != nil {
		rt.internalError(w, "api: restrict port list streams failed", err)
		return store.AppStream{}, false
	}
	for _, s := range streams {
		if s.HostPort == port {
			return s, true
		}
	}
	writeError(w, http.StatusNotFound, fmt.Sprintf("app %s has no stream on port %d; create one with apps streams add first", r.PathValue("name"), port))
	return store.AppStream{}, false
}

// clearRestriction deletes the helper's own rules for a port.
func (rt *Router) clearRestriction(ctx context.Context, label string) error {
	rules, err := rt.firewallRules.ListFirewallRules(ctx)
	if err != nil {
		return err
	}
	for _, rule := range rules {
		if rule.Label == label {
			if err := rt.firewallRules.DeleteFirewallRule(ctx, rule.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

// handleRestrictDomainPort handles PUT .../ports/{port}/restrict: replaces
// the helper's rules with one allow per source, then a deny from anywhere.
func (rt *Router) handleRestrictDomainPort(w http.ResponseWriter, r *http.Request) {
	if _, ok := rt.requireOwnedDomain(w, r); !ok {
		return
	}
	stream, ok := rt.streamForPort(w, r)
	if !ok {
		return
	}
	var req restrictPortRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Sources) == 0 || len(req.Sources) > maxRestrictSources {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("list 1 to %d sources (IP addresses or CIDR ranges)", maxRestrictSources))
		return
	}
	label := portRestrictLabel(stream.HostPort, stream.Protocol)
	var planned []store.FirewallRule
	for _, src := range req.Sources {
		cidr := src
		if ip := net.ParseIP(src); ip != nil {
			cidr = src + "/32"
			if ip.To4() == nil {
				cidr = src + "/128"
			}
		}
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("%q is not an IP address or CIDR range", src))
			return
		}
		planned = append(planned, store.FirewallRule{Port: stream.HostPort, Protocol: stream.Protocol, SourceCIDR: cidr, Action: store.FirewallRuleActionAllow, Label: label})
	}
	planned = append(planned, store.FirewallRule{Port: stream.HostPort, Protocol: stream.Protocol, Action: store.FirewallRuleActionDeny, Label: label})
	for _, rule := range planned {
		check := firewall.Rule{Port: rule.Port, Proto: rule.Protocol, SourceCIDR: rule.SourceCIDR, Action: firewall.Action(rule.Action)}
		if err := firewall.Validate(check, rt.firewallRequiredPorts); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	ctx := r.Context()
	if err := rt.clearRestriction(ctx, label); err != nil {
		rt.internalError(w, "api: restrict port clear failed", err)
		return
	}
	for _, rule := range planned {
		id, err := randomFirewallRuleID()
		if err != nil {
			rt.internalError(w, "api: restrict port id failed", err)
			return
		}
		rule.ID = id
		rule.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		if err := rt.firewallRules.SaveFirewallRule(ctx, rule); err != nil {
			rt.internalError(w, "api: restrict port save failed", err)
			return
		}
	}
	rt.nudgeReconciler()
	rt.handleGetDomainPorts(w, r)
}

func (rt *Router) handleUnrestrictDomainPort(w http.ResponseWriter, r *http.Request) {
	if _, ok := rt.requireOwnedDomain(w, r); !ok {
		return
	}
	stream, ok := rt.streamForPort(w, r)
	if !ok {
		return
	}
	if err := rt.clearRestriction(r.Context(), portRestrictLabel(stream.HostPort, stream.Protocol)); err != nil {
		rt.internalError(w, "api: unrestrict port failed", err)
		return
	}
	rt.nudgeReconciler()
	rt.handleGetDomainPorts(w, r)
}
