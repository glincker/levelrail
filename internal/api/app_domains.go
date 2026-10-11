package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
)

// serviceDomainsEditor is the domain-only write behind PATCH
// /apps/{name}/domains. *store.DB provides it.
type serviceDomainsEditor interface {
	EditServiceDomains(ctx context.Context, name string, edit func(current []string) ([]string, error)) ([]string, bool, error)
}

// editDomainsRequest is PATCH /api/v1/apps/{name}/domains's body: either Set
// (the whole list) or Add and Remove.
type editDomainsRequest struct {
	Set    *[]string `json:"set,omitempty"`
	Add    []string  `json:"add,omitempty"`
	Remove []string  `json:"remove,omitempty"`
	// Environment (ID, name or kind) edits that environment's own domain
	// set instead of the app's default set.
	Environment string `json:"environment,omitempty"`
	// DNS is auto (default), off or preview: whether added domains get their
	// DNS record created. Replace lets a conflicting A/AAAA/CNAME be
	// overwritten. RemoveDNS deletes the records Levelrail created for
	// removed domains. Automation overrides the go-live policy per request.
	DNS        string                    `json:"dns,omitempty"`
	Replace    bool                      `json:"replace,omitempty"`
	RemoveDNS  bool                      `json:"remove_dns,omitempty"`
	Automation *domainAutomationOverride `json:"automation,omitempty"`
}

type editDomainsResponse struct {
	App           string   `json:"app"`
	Domains       []string `json:"domains"`
	Changed       bool     `json:"changed"`
	EnvironmentID string   `json:"environment_id,omitempty"`
	// DNSResults is one automatic DNS outcome per added domain (and per
	// removed domain with remove_dns). GoLive carries the full step list.
	DNSResults []domainDNSResult `json:"dns_results,omitempty"`
	GoLive     []goLiveResult    `json:"go_live,omitempty"`
}

// errDomainNotSet marks a remove of a domain the app does not have.
var errDomainNotSet = errors.New("domain is not set")

// handleEditAppDomains handles PATCH /api/v1/apps/{name}/domains: changes only
// the domain list, atomically, so it never overwrites a concurrent edit to
// the app's other settings.
func (rt *Router) handleEditAppDomains(w http.ResponseWriter, r *http.Request) {
	editor, ok := rt.apps.(serviceDomainsEditor)
	if !ok {
		writeError(w, http.StatusNotImplemented, "editing domains is not supported by this store")
		return
	}
	name := r.PathValue("name")
	var req editDomainsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Set != nil && len(req.Add)+len(req.Remove) > 0 {
		writeError(w, http.StatusBadRequest, "set cannot be combined with add or remove")
		return
	}
	if req.Set == nil && len(req.Add)+len(req.Remove) == 0 {
		writeError(w, http.StatusBadRequest, "one of set, add or remove is required")
		return
	}
	var set []string
	if req.Set != nil {
		set = normalizeDomainList(*req.Set)
	}
	add, remove := normalizeDomainList(req.Add), normalizeDomainList(req.Remove)
	for _, d := range append(slices.Clone(set), add...) {
		if err := ingress.ValidateWildcardDomain(d); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	switch req.DNS {
	case "", dnsModeAuto, dnsModeOff, dnsModePreview:
	default:
		writeError(w, http.StatusBadRequest, "dns must be auto, off or preview")
		return
	}
	if req.Automation != nil {
		if msg := req.Automation.validate(); msg != "" {
			writeError(w, http.StatusBadRequest, msg)
			return
		}
	}

	if env := strings.TrimSpace(req.Environment); env != "" {
		rt.editAppEnvironmentDomains(w, r, name, env, req.Set, add, remove)
		return
	}

	var before []string
	next, changed, err := editor.EditServiceDomains(r.Context(), name, func(current []string) ([]string, error) {
		before = slices.Clone(current)
		if req.Set != nil {
			return set, nil
		}
		return applyDomainEdit(current, add, remove)
	})
	var taken *store.ErrDomainTaken
	switch {
	case errors.Is(err, store.ErrServiceNotFound):
		writeError(w, http.StatusNotFound, "app not found")
		return
	case errors.Is(err, errDomainNotSet):
		writeError(w, http.StatusBadRequest, err.Error())
		return
	case errors.As(err, &taken):
		writeError(w, http.StatusConflict, taken.Error())
		return
	case err != nil:
		rt.internalError(w, "api: edit app domains failed", err, slog.String("name", name))
		return
	}
	if changed {
		if ev, ok := domainChangeEvent(name, before, next); ok {
			rt.recordAppEvent(r, ev)
		}
		rt.nudgeReconciler()
	}
	resp := editDomainsResponse{App: name, Domains: next, Changed: changed}
	if changed {
		rt.automateDomainChanges(r, name, req, before, next, &resp)
	}
	writeJSON(w, http.StatusOK, resp)
}

// automateDomainChanges runs the go-live automation for every domain the
// edit added, and removes tracked DNS records for removed ones on request.
// A failure here never fails the already-applied domain edit.
func (rt *Router) automateDomainChanges(r *http.Request, name string, req editDomainsRequest, before, next []string, resp *editDomainsResponse) {
	ctx := r.Context()
	added, removed := sliceDiff(before, next)
	policy := rt.effectiveAutomation(ctx, r, req.Automation)
	for _, d := range added {
		if ingress.IsWildcardDomain(d) {
			continue
		}
		res := rt.goLive(ctx, r, name, d, policy, goLiveRequest{DNS: req.DNS, Replace: req.Replace, skipVerify: true}, goLiveModeApply)
		res.undo = append([]undoAction{{Kind: undoKindAppDomain, App: name, Domain: d}}, res.undo...)
		res.Undoable = true
		if req.DNS != dnsModePreview {
			rt.recordGoLiveRun(ctx, &res)
		}
		resp.DNSResults = append(resp.DNSResults, res.DNS...)
		resp.GoLive = append(resp.GoLive, res)
	}
	if req.RemoveDNS {
		for _, d := range removed {
			resp.DNSResults = append(resp.DNSResults, rt.removeManagedDNS(ctx, r, name, d)...)
		}
	}
}

func domainChangeEvent(name string, before, next []string) (store.AppEvent, bool) {
	added, removed := sliceDiff(before, next)
	if len(added)+len(removed) == 0 {
		return store.AppEvent{}, false
	}
	return store.AppEvent{
		AppName: name, Kind: store.AppEventConfigChange, Keys: []string{"domains"},
		Title:  "Config changed: domains",
		Detail: "domains " + joinNonEmpty(" ", prefixAll("+", added), prefixAll("-", removed)),
	}, true
}

func normalizeDomainList(in []string) []string {
	out := make([]string, 0, len(in))
	for _, d := range in {
		if d = strings.ToLower(strings.TrimSpace(d)); d != "" && !slices.Contains(out, d) {
			out = append(out, d)
		}
	}
	return out
}

// applyDomainEdit adds then removes. Removing a domain the app does not have
// is an error rather than a silent no-op.
func applyDomainEdit(current, add, remove []string) ([]string, error) {
	next := current
	for _, d := range add {
		if !slices.Contains(next, d) {
			next = append(next, d)
		}
	}
	for _, d := range remove {
		if !slices.Contains(next, d) {
			return nil, fmt.Errorf("%w: %q", errDomainNotSet, d)
		}
		next = slices.DeleteFunc(next, func(x string) bool { return x == d })
	}
	return next, nil
}
