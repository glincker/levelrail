package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/exposure"
	"github.com/GLINCKER/levelrail/internal/store"
)

type exposureRestrictRequest struct {
	Node            string   `json:"node,omitempty"`
	Port            int      `json:"port,omitempty"`
	Protocol        string   `json:"protocol,omitempty"`
	Allow           []string `json:"allow"`
	LocalContainers bool     `json:"local_containers,omitempty"`
	// Confirm must be true to apply; the preview text states what is dropped.
	Confirm bool `json:"confirm,omitempty"`
}

type exposurePlanResource struct {
	exposure.Plan
	Port     int      `json:"port"`
	Protocol string   `json:"protocol"`
	Allow    []string `json:"allow"`
	Warnings []string `json:"warnings"`
	Applied  bool     `json:"applied"`
}

func (rt *Router) exposureRestriction(req exposureRestrictRequest, port int, proto string) exposure.Restriction {
	allow := append([]string(nil), req.Allow...)
	if req.LocalContainers {
		allow = append(allow, localContainerSource)
	}
	return exposure.Restriction{Port: port, Protocol: proto, Allow: allow}
}

// exposureTarget decodes the request and enforces that the target is this host.
func (rt *Router) exposureTarget(w http.ResponseWriter, r *http.Request) (exposureRestrictRequest, int, string, bool) {
	var req exposureRestrictRequest
	if rt.exposure == nil || rt.exposureStore == nil {
		writeError(w, http.StatusNotImplemented, "exposure restrictions are not configured on this control plane")
		return req, 0, "", false
	}
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return req, 0, "", false
		}
	}
	port, proto := req.Port, req.Protocol
	if p := r.PathValue("port"); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil {
			writeError(w, http.StatusBadRequest, "port must be a number")
			return req, 0, "", false
		}
		port, proto = n, r.PathValue("protocol")
	}
	if proto == "" {
		proto = "tcp"
	}
	if req.Node != "" && req.Node != localNodeLabel && !rt.isLocalNode(req.Node) {
		writeError(w, http.StatusConflict, "rules can only be applied on the control plane's own host in this version; run the dry-run commands on that node yourself")
		return req, 0, "", false
	}
	return req, port, proto, true
}

// exposureWarnings names known nodes the allow-list would cut off.
func (rt *Router) exposureWarnings(r *http.Request, allow []string) []string {
	var out []string
	if rt.nodes == nil {
		return out
	}
	nodes, err := rt.nodes.ListNodes(r.Context())
	if err != nil {
		return out
	}
	for _, n := range nodes {
		if rt.isLocalNode(n.ID) {
			continue
		}
		for _, a := range []string{n.Address, n.MeshAddress} {
			host := strings.TrimSpace(a)
			if h, _, err := net.SplitHostPort(host); err == nil {
				host = h
			}
			ip, err := netip.ParseAddr(host)
			if err != nil || allowCovers(allow, ip) {
				continue
			}
			out = append(out, fmt.Sprintf("Node %s (%s) is not in the allow-list, so apps there will lose access to this port.", n.Name, host))
		}
	}
	return out
}

func allowCovers(allow []string, ip netip.Addr) bool {
	for _, a := range allow {
		if p, err := netip.ParsePrefix(a); err == nil && p.Contains(ip) {
			return true
		}
	}
	return false
}

// handleExposurePreview handles POST /api/v1/firewall/exposure/preview: the
// exact rules and what they drop, changing nothing.
func (rt *Router) handleExposurePreview(w http.ResponseWriter, r *http.Request) {
	req, port, proto, ok := rt.exposureTarget(w, r)
	if !ok {
		return
	}
	want, err := rt.exposure.Normalize(rt.exposureRestriction(req, port, proto))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	plan, err := rt.exposure.Plan(want)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, exposurePlanResource{
		Plan: plan, Port: want.Port, Protocol: want.Protocol, Allow: want.Allow, Warnings: rt.exposureWarnings(r, want.Allow),
	})
}

// handleExposureRestrict handles PUT /api/v1/firewall/exposure/restrictions/{protocol}/{port}.
// AbilityRoot. The port is in the path so the audit log records which one.
func (rt *Router) handleExposureRestrict(w http.ResponseWriter, r *http.Request) {
	req, port, proto, ok := rt.exposureTarget(w, r)
	if !ok {
		return
	}
	want, err := rt.exposure.Normalize(rt.exposureRestriction(req, port, proto))
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, exposure.ErrLockout) {
			status = http.StatusConflict
		}
		writeError(w, status, err.Error())
		return
	}
	plan, err := rt.exposure.Plan(want)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !req.Confirm {
		writeError(w, http.StatusBadRequest, "confirmation required: "+plan.Drops)
		return
	}
	if _, err := rt.exposure.Apply(r.Context(), want); err != nil {
		rt.logger.Error("api: exposure restrict failed", slog.Int("port", port), slog.String("protocol", proto), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "could not apply the rule: "+err.Error())
		return
	}
	row := store.ExposureRestriction{Port: want.Port, Protocol: want.Protocol, Allow: want.Allow, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	if err := rt.exposureStore.SaveExposureRestriction(r.Context(), row); err != nil {
		_ = rt.exposure.Remove(r.Context(), want.Port, want.Protocol)
		rt.internalError(w, "api: exposure restrict: save failed, rule rolled back", err, slog.Int("port", port))
		return
	}
	rt.logger.Info("api: exposure restriction applied", slog.Int("port", want.Port), slog.String("protocol", want.Protocol), slog.Int("sources", len(want.Allow)))
	rt.nudgeReconciler()
	writeJSON(w, http.StatusOK, exposurePlanResource{
		Plan: plan, Port: want.Port, Protocol: want.Protocol, Allow: want.Allow, Warnings: rt.exposureWarnings(r, want.Allow), Applied: true,
	})
}

// handleExposureUnrestrict handles DELETE /api/v1/firewall/exposure/restrictions/{protocol}/{port}.
func (rt *Router) handleExposureUnrestrict(w http.ResponseWriter, r *http.Request) {
	_, port, proto, ok := rt.exposureTarget(w, r)
	if !ok {
		return
	}
	if err := rt.exposureStore.DeleteExposureRestriction(r.Context(), port, proto); err != nil {
		rt.internalError(w, "api: exposure unrestrict: delete failed", err, slog.Int("port", port))
		return
	}
	if err := rt.exposure.Remove(r.Context(), port, proto); err != nil {
		rt.logger.Warn("api: exposure unrestrict: rule removal failed, the reconciler will retry", slog.Int("port", port), slog.String("error", err.Error()))
	}
	rt.logger.Info("api: exposure restriction removed", slog.Int("port", port), slog.String("protocol", proto))
	w.WriteHeader(http.StatusNoContent)
}
