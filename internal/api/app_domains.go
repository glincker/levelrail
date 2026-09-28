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
}

type editDomainsResponse struct {
	App     string   `json:"app"`
	Domains []string `json:"domains"`
	Changed bool     `json:"changed"`
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
	writeJSON(w, http.StatusOK, editDomainsResponse{App: name, Domains: next, Changed: changed})
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
