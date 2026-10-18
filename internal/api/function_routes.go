package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

const functionRulePrefix = "function:"

// FunctionRoute is one path a function answers on, stored as a forwarder rule on the domain.
type FunctionRoute struct {
	Domain      string `json:"domain"`
	Path        string `json:"path"`
	StripPrefix bool   `json:"strip_prefix"`
}

type functionRouteRequest struct {
	Domain      string `json:"domain"`
	Path        string `json:"path"`
	StripPrefix *bool  `json:"strip_prefix,omitempty"`
}

type functionRoutesResponse struct {
	Routes []FunctionRoute `json:"routes"`
}

func functionRuleName(fn string) string { return functionRulePrefix + fn }

func normalizeFunctionPath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	p = strings.TrimRight(p, "/")
	if p == "" {
		return "", fmt.Errorf("path must name a prefix below /, for example /hello")
	}
	return p, nil
}

func (rt *Router) domainOwned(ctx context.Context, domain string) (bool, error) {
	svcs, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return false, err
	}
	for _, s := range svcs {
		for _, d := range s.Domains {
			if d == domain {
				return true, nil
			}
		}
	}
	return false, nil
}

func (rt *Router) loadForwarders(ctx context.Context, domain string) (trafficpolicy.Forwarders, error) {
	var fw trafficpolicy.Forwarders
	row, found, err := rt.domainPolicy.store.GetDomainTrafficPolicy(ctx, domain, trafficpolicy.KindForwarders)
	if err != nil || !found {
		return fw, err
	}
	if err := json.Unmarshal(row.Spec, &fw); err != nil {
		return fw, fmt.Errorf("decode forwarders for %q: %w", domain, err)
	}
	return fw, nil
}

// handleSetFunctionRoute handles PUT /api/v1/functions/{name}/route: serve the
// function at <domain><path> through a forwarder rule on that domain.
func (rt *Router) handleSetFunctionRoute(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var req functionRouteRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxPolicyBody)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	domain := strings.ToLower(strings.TrimSpace(req.Domain))
	path, err := normalizeFunctionPath(req.Path)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := rt.apps.GetDesiredService(r.Context(), name); err != nil {
		writeError(w, http.StatusNotFound, "function not found")
		return
	}
	owned, err := rt.domainOwned(r.Context(), domain)
	if err != nil {
		rt.internalError(w, "api: set function route: list apps failed", err)
		return
	}
	if !owned {
		writeError(w, http.StatusNotFound, fmt.Sprintf("domain %q is not attached to any app; add it to an app first", domain))
		return
	}
	fw, err := rt.loadForwarders(r.Context(), domain)
	if err != nil {
		rt.internalError(w, "api: set function route: load forwarders failed", err, slog.String("domain", domain))
		return
	}
	strip := req.StripPrefix == nil || *req.StripPrefix
	rule := trafficpolicy.Forwarder{
		Name:        functionRuleName(name),
		Match:       trafficpolicy.Match{Kind: trafficpolicy.MatchPrefix, Path: path},
		Action:      trafficpolicy.ActionApp,
		App:         name,
		StripPrefix: strip,
	}
	fw.Rules = upsertFunctionRule(fw.Rules, rule)
	body, err := json.Marshal(fw)
	if err != nil {
		rt.internalError(w, "api: set function route: encode failed", err)
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	rt.saveDomainPolicy(w, r, domain, trafficpolicy.KindForwarders)
}

// upsertFunctionRule replaces the rule for the same function and path, else
// puts the new one first so a longer prefix is tried before the domain's own rules.
func upsertFunctionRule(rules []trafficpolicy.Forwarder, rule trafficpolicy.Forwarder) []trafficpolicy.Forwarder {
	fnRules := []trafficpolicy.Forwarder{rule}
	var rest []trafficpolicy.Forwarder
	for _, f := range rules {
		switch {
		case f.Name == rule.Name && f.Match.Path == rule.Match.Path:
		case isFunctionRule(f):
			fnRules = append(fnRules, f)
		default:
			rest = append(rest, f)
		}
	}
	sort.SliceStable(fnRules, func(i, j int) bool { return len(fnRules[i].Match.Path) > len(fnRules[j].Match.Path) })
	return append(fnRules, rest...)
}

func isFunctionRule(f trafficpolicy.Forwarder) bool {
	return strings.HasPrefix(f.Name, functionRulePrefix)
}

// functionRoutes returns the routes of one function across every domain.
func (rt *Router) functionRoutes(ctx context.Context, name string) ([]FunctionRoute, error) {
	svcs, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	out := []FunctionRoute{}
	for _, s := range svcs {
		for _, d := range s.Domains {
			if seen[d] {
				continue
			}
			seen[d] = true
			fw, err := rt.loadForwarders(ctx, d)
			if err != nil {
				return nil, err
			}
			for _, f := range fw.Rules {
				if f.Name == functionRuleName(name) {
					out = append(out, FunctionRoute{Domain: d, Path: f.Match.Path, StripPrefix: f.StripPrefix})
				}
			}
		}
	}
	return out, nil
}

// handleListFunctionRoutes handles GET /api/v1/functions/{name}/route.
func (rt *Router) handleListFunctionRoutes(w http.ResponseWriter, r *http.Request) {
	routes, err := rt.functionRoutes(r.Context(), r.PathValue("name"))
	if err != nil {
		rt.internalError(w, "api: list function routes failed", err)
		return
	}
	writeJSON(w, http.StatusOK, functionRoutesResponse{Routes: routes})
}

// handleDeleteFunctionRoute handles DELETE /api/v1/functions/{name}/route
// with ?domain=&path= (both optional, none removes every route of the function).
func (rt *Router) handleDeleteFunctionRoute(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	wantDomain := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("domain")))
	wantPath := strings.TrimRight(strings.TrimSpace(r.URL.Query().Get("path")), "/")
	routes, err := rt.functionRoutes(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: delete function route failed", err)
		return
	}
	byDomain := map[string]bool{}
	for _, fr := range routes {
		if wantDomain == "" || fr.Domain == wantDomain {
			byDomain[fr.Domain] = true
		}
	}
	removed := 0
	for domain := range byDomain {
		fw, err := rt.loadForwarders(r.Context(), domain)
		if err != nil {
			rt.internalError(w, "api: delete function route failed", err, slog.String("domain", domain))
			return
		}
		kept := fw.Rules[:0]
		for _, f := range fw.Rules {
			if f.Name == functionRuleName(name) && (wantPath == "" || f.Match.Path == wantPath) {
				removed++
				continue
			}
			kept = append(kept, f)
		}
		if len(kept) == 0 {
			err = rt.domainPolicy.store.DeleteDomainTrafficPolicy(r.Context(), domain, trafficpolicy.KindForwarders)
		} else {
			var spec []byte
			spec, err = json.Marshal(trafficpolicy.Forwarders{Rules: kept})
			if err == nil {
				err = rt.domainPolicy.store.SetDomainTrafficPolicy(r.Context(), domain, trafficpolicy.KindForwarders, spec)
			}
		}
		if err != nil {
			rt.internalError(w, "api: delete function route failed", err, slog.String("domain", domain))
			return
		}
	}
	if removed > 0 {
		rt.nudgeReconciler()
	}
	rest, err := rt.functionRoutes(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: delete function route failed", err)
		return
	}
	writeJSON(w, http.StatusOK, functionRoutesResponse{Routes: rest})
}
