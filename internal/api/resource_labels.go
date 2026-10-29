package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/placement"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	envLabelsMaxPerResource     = "APP_LABELS_MAX_PER_RESOURCE"
	defaultLabelsMaxPerResource = 32
	labelQueryParam             = "label"
)

// resourceLabelStore is the label table behind apps, databases and stacks;
// *store.DB satisfies it and other stores simply do not offer labels.
type resourceLabelStore interface {
	ListResourceLabels(ctx context.Context, kind string) (map[string]map[string]string, error)
	GetResourceLabels(ctx context.Context, kind, name string) (map[string]string, error)
	SetResourceLabels(ctx context.Context, kind, name string, labels map[string]string) error
}

func (rt *Router) resourceLabels() resourceLabelStore {
	s, _ := rt.apps.(resourceLabelStore)
	return s
}

func labelsMaxPerResource() int {
	if n, err := strconv.Atoi(os.Getenv(envLabelsMaxPerResource)); err == nil && n > 0 {
		return n
	}
	return defaultLabelsMaxPerResource
}

// validateResourceLabels checks every pair like a DNS label and the count against the env limit.
func validateResourceLabels(labels map[string]string) error {
	if limit := labelsMaxPerResource(); len(labels) > limit {
		return fmt.Errorf("at most %d labels are allowed, got %d", limit, len(labels))
	}
	for k, v := range labels {
		if err := placement.ValidateLabels(map[string]string{k: v}); err != nil {
			return err
		}
	}
	return nil
}

// labelPatch is the PATCH body: set adds or overwrites, remove drops keys.
type labelPatch struct {
	Set    map[string]string `json:"set"`
	Remove []string          `json:"remove"`
}

// labelPut distinguishes an omitted labels field (keep) from an explicit {} (clear).
type labelPut struct {
	Labels *map[string]string `json:"labels"`
}

// labelFilter returns nil when the request carries no label selector, else a name matcher for kind.
func (rt *Router) labelFilter(r *http.Request, kind string) (func(string) bool, error) {
	raw := r.URL.Query().Get(labelQueryParam)
	if raw == "" {
		return nil, nil
	}
	sel, err := parseV2Selector(raw)
	if err != nil {
		return nil, err
	}
	ls := rt.resourceLabels()
	if ls == nil {
		return func(string) bool { return len(sel) == 0 }, nil
	}
	all, err := ls.ListResourceLabels(r.Context(), kind)
	if err != nil {
		return nil, err
	}
	return func(name string) bool { return sel.matches(all[name]) }, nil
}

// mergeLabels overlays operator labels on the base set the resource already carries.
func mergeLabels(base, extra map[string]string) map[string]string {
	if len(extra) == 0 {
		return base
	}
	out := make(map[string]string, len(base)+len(extra))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

// labelKindExists writes the 404 itself and reports whether the resource is there.
func (rt *Router) labelKindExists(w http.ResponseWriter, r *http.Request, kind, name string) bool {
	var err error
	switch kind {
	case store.LabelKindApp:
		_, err = rt.apps.GetDesiredService(r.Context(), name)
		if errors.Is(err, store.ErrServiceNotFound) {
			writeError(w, http.StatusNotFound, "app not found")
			return false
		}
	case store.LabelKindDatabase:
		_, err = rt.databases.GetDesiredDatabase(r.Context(), name)
		if errors.Is(err, store.ErrDatabaseNotFound) {
			writeError(w, http.StatusNotFound, "database not found")
			return false
		}
	case store.LabelKindStack:
		ss, ok := rt.stackStoreOr501(w)
		if !ok {
			return false
		}
		_, ok = rt.loadStack(w, r, ss)
		return ok
	}
	if err != nil {
		rt.internalError(w, "api: labels: look up "+kind, err, slog.String("name", name))
		return false
	}
	return true
}

func (rt *Router) labelsHandler(kind string, apply func(current map[string]string, r *http.Request) (map[string]string, bool, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ls := rt.resourceLabels()
		if ls == nil {
			writeError(w, http.StatusNotImplemented, "labels are not available on this store")
			return
		}
		name := r.PathValue("name")
		if !rt.labelKindExists(w, r, kind, name) {
			return
		}
		current, err := ls.GetResourceLabels(r.Context(), kind, name)
		if err != nil {
			rt.internalError(w, "api: labels: read "+kind, err, slog.String("name", name))
			return
		}
		if apply != nil {
			next, changed, err := apply(current, r)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			if changed {
				if err := ls.SetResourceLabels(r.Context(), kind, name, next); err != nil {
					rt.internalError(w, "api: labels: write "+kind, err, slog.String("name", name))
					return
				}
			}
			current = next
		}
		writeJSON(w, http.StatusOK, labelsBody{Labels: nonNilLabels(current)})
	}
}

func nonNilLabels(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

func applyLabelPut(current map[string]string, r *http.Request) (map[string]string, bool, error) {
	var body labelPut
	if err := decodeJSONBody(r, &body); err != nil {
		return nil, false, errors.New("invalid request body")
	}
	if body.Labels == nil {
		return current, false, nil
	}
	if err := validateResourceLabels(*body.Labels); err != nil {
		return nil, false, err
	}
	return *body.Labels, true, nil
}

func applyLabelPatch(current map[string]string, r *http.Request) (map[string]string, bool, error) {
	var body labelPatch
	if err := decodeJSONBody(r, &body); err != nil {
		return nil, false, errors.New("invalid request body")
	}
	next := mergeLabels(nil, current)
	if next == nil {
		next = map[string]string{}
	}
	for _, k := range body.Remove {
		delete(next, k)
	}
	for k, v := range body.Set {
		next[k] = v
	}
	if err := validateResourceLabels(next); err != nil {
		return nil, false, err
	}
	return next, true, nil
}

// registerResourceLabelRoutes wires GET, PUT (replace) and PATCH (merge) for each labelled kind.
func (rt *Router) registerResourceLabelRoutes(mux *http.ServeMux) {
	kinds := []struct {
		path, kind string
		resource   func(*http.Request) string
	}{
		{"apps", store.LabelKindApp, appResourceFromPath},
		{"databases", store.LabelKindDatabase, databaseResourceFromPath},
		{"stacks", store.LabelKindStack, stackResourceFromPath},
	}
	for _, k := range kinds {
		base := "/api/v1/" + k.path + "/{name}/labels"
		mux.HandleFunc("GET "+base, rt.requireAbilityForResource(AbilityRead, k.resource, rt.labelsHandler(k.kind, nil)))
		mux.HandleFunc("PUT "+base, rt.requireAbilityForResource(AbilityWrite, k.resource, rt.labelsHandler(k.kind, applyLabelPut)))
		mux.HandleFunc("PATCH "+base, rt.requireAbilityForResource(AbilityWrite, k.resource, rt.labelsHandler(k.kind, applyLabelPatch)))
	}
}

// filterNodesByLabels keeps nodes whose stored labels satisfy sel.
func (rt *Router) filterNodesByLabels(ctx context.Context, nodes []store.Node, sel v2Selector) []store.Node {
	var all map[string]map[string]string
	if rt.multiNode != nil {
		all, _ = rt.multiNode.store.ListNodeLabels(ctx)
	}
	out := nodes[:0:0]
	for _, n := range nodes {
		if sel.matches(all[n.ID]) {
			out = append(out, n)
		}
	}
	return out
}
