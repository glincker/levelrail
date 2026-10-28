package api

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/GLINCKER/levelrail/internal/store"
)

// v2ChangeStore is the change log the v2 view reads; *store.DB satisfies it.
type v2ChangeStore interface {
	CurrentResourceVersion(ctx context.Context) (int64, error)
	LatestResourceVersions(ctx context.Context, kind string) (map[string]int64, error)
	ResourceChangesSince(ctx context.Context, kind string, after int64, limit int) ([]store.ResourceChange, error)
	BackfillAppResourceChanges(ctx context.Context) (int, error)
}

// v2Server holds the v2 routes' shared state without growing Router.
type v2Server struct {
	rt       *Router
	changes  v2ChangeStore
	cfg      v2WatchConfig
	backfill sync.Once
	streams  chan struct{}
}

func newV2Server(rt *Router) *v2Server {
	cfg := v2WatchConfigFromEnv()
	s := &v2Server{rt: rt, cfg: cfg, streams: make(chan struct{}, cfg.MaxStreams)}
	s.changes, _ = rt.apps.(v2ChangeStore)
	return s
}

func (s *v2Server) apiVersion() string {
	if s.rt.brand == nil {
		return v2APIVersion("")
	}
	return v2APIVersion(s.rt.brand.Domain)
}

// ensureBackfill seeds the change log once so a watch from 0 sees every existing app.
func (s *v2Server) ensureBackfill(ctx context.Context) {
	s.backfill.Do(func() {
		if _, err := s.changes.BackfillAppResourceChanges(context.WithoutCancel(ctx)); err != nil {
			s.rt.logger.Error("api: v2 backfill app changes failed", slog.String("error", err.Error()))
		}
	})
}

// v2Selector is an equality-based label selector: k=v, k==v, k!=v, k, !k.
type v2Selector []func(map[string]string) bool

func parseV2Selector(raw string) (v2Selector, error) {
	var sel v2Selector
	for _, term := range strings.Split(raw, ",") {
		term = strings.TrimSpace(term)
		switch {
		case term == "":
			continue
		case strings.Contains(term, "!="):
			k, v, _ := strings.Cut(term, "!=")
			k, v = strings.TrimSpace(k), strings.TrimSpace(v)
			sel = append(sel, func(l map[string]string) bool { return l[k] != v })
		case strings.Contains(term, "="):
			k, v, _ := strings.Cut(strings.Replace(term, "==", "=", 1), "=")
			k, v = strings.TrimSpace(k), strings.TrimSpace(v)
			sel = append(sel, func(l map[string]string) bool { got, ok := l[k]; return ok && got == v })
		case strings.HasPrefix(term, "!"):
			k := strings.TrimSpace(term[1:])
			sel = append(sel, func(l map[string]string) bool { _, ok := l[k]; return !ok })
		default:
			if strings.ContainsAny(term, " ()") {
				return nil, fmt.Errorf("unsupported selector term %q", term)
			}
			k := term
			sel = append(sel, func(l map[string]string) bool { _, ok := l[k]; return ok })
		}
	}
	return sel, nil
}

func (sel v2Selector) matches(labels map[string]string) bool {
	for _, m := range sel {
		if !m(labels) {
			return false
		}
	}
	return true
}

// v2Page applies limit and an opaque continue token (the last name returned) to name-sorted items.
func v2Page(items []v2Object, r *http.Request) ([]v2Object, string, error) {
	sort.Slice(items, func(i, j int) bool { return items[i].Metadata.Name < items[j].Metadata.Name })
	q := r.URL.Query()
	if tok := q.Get("continue"); tok != "" {
		after, err := base64.RawURLEncoding.DecodeString(tok)
		if err != nil {
			return nil, "", errors.New("invalid continue token")
		}
		i := sort.Search(len(items), func(i int) bool { return items[i].Metadata.Name > string(after) })
		items = items[i:]
	}
	limit := 0
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			return nil, "", errors.New("limit must be a non-negative integer")
		}
		limit = n
	}
	if limit == 0 || limit >= len(items) {
		return items, "", nil
	}
	return items[:limit], base64.RawURLEncoding.EncodeToString([]byte(items[limit-1].Metadata.Name)), nil
}

func (s *v2Server) handleList(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.changes == nil {
			writeError(w, http.StatusNotImplemented, "resource view is not available on this store")
			return
		}
		if isV2Watch(r) {
			s.handleWatch(kind, w, r)
			return
		}
		sel, err := parseV2Selector(r.URL.Query().Get("labelSelector"))
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if kind == v2PathApps {
			s.ensureBackfill(r.Context())
		}
		rv, err := s.changes.CurrentResourceVersion(r.Context())
		if err != nil {
			s.rt.internalError(w, "api: v2 list: current version", err)
			return
		}
		items, err := s.listObjects(r, kind, "")
		if err != nil {
			s.rt.internalError(w, "api: v2 list "+kind, err)
			return
		}
		matched := items[:0:0]
		for _, o := range items {
			if sel.matches(o.Metadata.Labels) {
				matched = append(matched, o)
			}
		}
		page, next, err := v2Page(matched, r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, v2List{
			APIVersion: s.apiVersion(),
			Kind:       v2KindFor(kind) + "List",
			Metadata:   v2ListMeta{ResourceVersion: strconv.FormatInt(rv, 10), Continue: next},
			Items:      page,
		})
	}
}

func (s *v2Server) handleGet(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.changes == nil {
			writeError(w, http.StatusNotImplemented, "resource view is not available on this store")
			return
		}
		name := r.PathValue("name")
		items, err := s.listObjects(r, kind, name)
		if err != nil {
			s.rt.internalError(w, "api: v2 get "+kind, err)
			return
		}
		if len(items) == 0 {
			writeError(w, http.StatusNotFound, strings.ToLower(v2KindFor(kind))+" not found")
			return
		}
		writeJSON(w, http.StatusOK, items[0])
	}
}

func v2KindFor(path string) string {
	switch path {
	case v2PathNodes:
		return v2KindNode
	case v2PathDatabases:
		return v2KindDatabase
	default:
		return v2KindApp
	}
}

func v2StoreKind(path string) string {
	switch path {
	case v2PathNodes:
		return store.ResourceKindNode
	case v2PathDatabases:
		return store.ResourceKindDatabase
	default:
		return store.ResourceKindApp
	}
}

// listObjects returns every object of kind the caller can read; only is a
// single-name filter for get. Visibility uses the same per-object check as v1.
func (s *v2Server) listObjects(r *http.Request, kind, only string) ([]v2Object, error) {
	ctx := r.Context()
	versions, err := s.changes.LatestResourceVersions(ctx, v2StoreKind(kind))
	if err != nil {
		return nil, err
	}
	switch kind {
	case v2PathNodes:
		return s.listNodes(ctx, only, versions)
	case v2PathDatabases:
		return s.listDatabases(r, only, versions)
	default:
		return s.listApps(r, only, versions)
	}
}

func (s *v2Server) listApps(r *http.Request, only string, versions map[string]int64) ([]v2Object, error) {
	ctx := r.Context()
	canRead, _, err := s.rt.callerAppVisibility(r)
	if err != nil {
		return nil, err
	}
	var svcs []store.DesiredService
	if only != "" {
		svc, err := s.rt.apps.GetDesiredService(ctx, only)
		if errors.Is(err, store.ErrServiceNotFound) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		svcs = []store.DesiredService{*svc}
	} else if svcs, err = s.rt.apps.ListDesiredServices(ctx); err != nil {
		return nil, err
	}
	visible := svcs[:0:0]
	for _, svc := range svcs {
		if canRead(svc.Name) {
			visible = append(visible, svc)
		}
	}
	return s.appObjects(ctx, visible, versions, true)
}

// appObjects maps services with v1 redaction; live=false masks every env value,
// which snapshots need since they never carried values.
func (s *v2Server) appObjects(ctx context.Context, svcs []store.DesiredService, versions map[string]int64, live bool) ([]v2Object, error) {
	names := make([]string, len(svcs))
	ctrls := make([]string, len(svcs))
	for i, svc := range svcs {
		names[i], ctrls[i] = svc.Name, applicationControllerName(svc.Name)
	}
	conds, err := s.rt.deploys.GetConditionsForControllers(ctx, ctrls)
	if err != nil {
		return nil, err
	}
	tags, err := s.rt.tags.ListTagsForApps(ctx, names)
	if err != nil {
		return nil, err
	}
	var (
		settings   map[string]store.AppEnvSettings
		settingsOK bool
	)
	if live {
		settings, settingsOK = s.rt.appOverridesForList(ctx)
	}
	out := make([]v2Object, 0, len(svcs))
	for _, svc := range svcs {
		res := toAppResourceRaw(svc)
		redactListedApp(&res, svc, settings, settingsOK)
		status := summarizeAppConditions(conds[applicationControllerName(svc.Name)])
		out = append(out, v2AppObject(s.apiVersion(), res, svc, tagNamesFromStoreTags(tags[svc.Name]), &status, versions[svc.Name]))
	}
	return out, nil
}

func (s *v2Server) listNodes(ctx context.Context, only string, versions map[string]int64) ([]v2Object, error) {
	nodes, err := s.rt.nodes.ListNodes(ctx)
	if err != nil {
		return nil, err
	}
	var out []v2Object
	for _, n := range nodes {
		if only != "" && n.Name != only && n.ID != only {
			continue
		}
		res := toNodeResource(n)
		res.IsLocal = n.ID == s.rt.localNodeID
		out = append(out, v2NodeObject(s.apiVersion(), res, versions[n.Name]))
	}
	return out, nil
}

func (s *v2Server) listDatabases(r *http.Request, only string, versions map[string]int64) ([]v2Object, error) {
	ctx := r.Context()
	canSee, err := s.rt.databaseVisibilityFilter(r)
	if err != nil {
		return nil, err
	}
	dbs, err := s.rt.databases.ListDesiredDatabases(ctx)
	if err != nil {
		return nil, err
	}
	var out []v2Object
	for _, d := range dbs {
		if (only != "" && d.Name != only) || !canSee(d.Name) {
			continue
		}
		out = append(out, v2DatabaseObject(s.apiVersion(), s.rt.toDatabaseResourceWithStatus(ctx, d), versions[d.Name]))
	}
	return out, nil
}
