package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// ResourceConsoleStore is the tag and saved-view surface the resource console needs beyond TagStore.
type ResourceConsoleStore interface {
	AttachResourceTag(ctx context.Context, tagID string, ref store.ResourceRef) error
	DetachResourceTag(ctx context.Context, tagID string, ref store.ResourceRef) error
	ListAllResourceTags(ctx context.Context) (map[string][]store.Tag, error)
	SaveResourceView(ctx context.Context, v store.ResourceView) error
	ListResourceViews(ctx context.Context, owner string) ([]store.ResourceView, error)
	DeleteResourceView(ctx context.Context, owner, id string) error
}

func (rt *Router) consoleStore() ResourceConsoleStore {
	s, _ := rt.tags.(ResourceConsoleStore)
	return s
}

const (
	bulkTagAdd    = "add"
	bulkTagRemove = "remove"
)

type consoleResource struct {
	ID     string   `json:"id,omitempty"`
	Kind   string   `json:"kind"`
	Name   string   `json:"name"`
	Detail string   `json:"detail,omitempty"`
	Status string   `json:"status,omitempty"`
	Tags   []string `json:"tags"`
	tagRef string
}

func (c consoleResource) ref() store.ResourceRef {
	if c.tagRef != "" {
		return store.ResourceRef{Kind: c.Kind, Name: c.tagRef}
	}
	return store.ResourceRef{Kind: c.Kind, Name: c.Name}
}

type consoleListResponse struct {
	Items  []consoleResource `json:"items"`
	Total  int               `json:"total"`
	Counts map[string]int    `json:"counts"`
}

// consoleInventory lists every resource the caller may act with the given ability on.
func (rt *Router) consoleInventory(r *http.Request, ability string) ([]consoleResource, error) {
	ctx := r.Context()
	abilities, policies, scope, err := rt.callerScope(r)
	if err != nil {
		return nil, fmt.Errorf("resolve scope: %w", err)
	}
	allowed := func(prefix, name string) bool { return scope.authorize(abilities, policies, ability, prefix, name) }
	instanceWide := !scope.restricted()
	out := []consoleResource{}
	appVisible := map[string]bool{}
	svcs, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return nil, fmt.Errorf("list apps: %w", err)
	}
	for _, s := range svcs {
		if allowed(resourcePrefixApp, s.Name) {
			appVisible[s.Name] = true
			out = append(out, consoleResource{Kind: store.ResourceKindApp, Name: s.Name, Detail: s.Image})
		}
	}
	dbs, err := rt.databases.ListDesiredDatabases(ctx)
	if err != nil {
		return nil, fmt.Errorf("list databases: %w", err)
	}
	for _, d := range dbs {
		if allowed(resourcePrefixDatabase, d.Name) {
			out = append(out, consoleResource{Kind: store.ResourceKindDatabase, Name: d.Name, Detail: strings.TrimSpace(d.Engine + " " + d.Version)})
		}
	}
	if instanceWide && rt.nodes != nil {
		nodes, err := rt.nodes.ListNodes(ctx)
		if err != nil {
			return nil, fmt.Errorf("list nodes: %w", err)
		}
		for _, n := range nodes {
			out = append(out, consoleResource{ID: n.ID, Kind: store.ResourceKindNode, Name: n.Name, Detail: n.Address, Status: string(n.Status), tagRef: n.ID})
		}
	}
	if rt.domains != nil {
		doms, err := rt.domains.ListServiceDomains(ctx)
		if err != nil {
			return nil, fmt.Errorf("list domains: %w", err)
		}
		for _, d := range doms {
			if appVisible[d.ServiceName] {
				out = append(out, consoleResource{Kind: store.ResourceKindDomain, Name: d.Domain, Detail: d.ServiceName})
			}
		}
	}
	if st := rt.standaloneStore(); st != nil && instanceWide {
		rows, err := st.ListStandaloneDomains(ctx)
		if err != nil {
			return nil, fmt.Errorf("list standalone domains: %w", err)
		}
		for _, d := range rows {
			out = append(out, consoleResource{Kind: store.ResourceKindDomain, Name: d.Domain, Detail: d.Target})
		}
	}
	if instanceWide {
		more, err := rt.consoleNetworkInventory(ctx)
		if err != nil {
			return nil, err
		}
		out = append(out, more...)
	}
	return out, nil
}

// consoleNetworkInventory lists storage buckets and private networks, each skipped when unwired.
func (rt *Router) consoleNetworkInventory(ctx context.Context) ([]consoleResource, error) {
	var out []consoleResource
	if rt.storage != nil && rt.storage.Buckets != nil {
		buckets, err := rt.storage.Buckets.ListStorageBuckets(ctx)
		if err != nil {
			return nil, fmt.Errorf("list buckets: %w", err)
		}
		for _, b := range buckets {
			detail := "private"
			if b.Public {
				detail = "public"
			}
			out = append(out, consoleResource{ID: b.ID, Kind: store.ResourceKindBucket, Name: b.Name, Detail: detail, tagRef: b.ID})
		}
	}
	if rt.privateNetworks != nil && rt.privateNetworks.Store != nil {
		nets, err := rt.privateNetworks.Store.ListPrivateNetworks(ctx)
		if err != nil {
			return nil, fmt.Errorf("list private networks: %w", err)
		}
		for _, n := range nets {
			out = append(out, consoleResource{ID: n.ID, Kind: store.ResourceKindNetwork, Name: n.Name, Detail: strconv.Itoa(len(n.Members)) + " apps", tagRef: n.ID})
		}
	}
	return out, nil
}

func attachConsoleTags(items []consoleResource, tags map[string][]store.Tag) {
	for i := range items {
		names := tagNamesFromStoreTags(tags[items[i].ref().Key()])
		if names == nil {
			names = []string{}
		}
		items[i].Tags = names
	}
}

func matchesConsoleQuery(it consoleResource, q string) bool {
	if q == "" {
		return true
	}
	hay := strings.ToLower(it.Kind + " " + it.Name + " " + it.Detail + " " + strings.Join(it.Tags, " "))
	for _, term := range strings.Fields(q) {
		if !strings.Contains(hay, term) {
			return false
		}
	}
	return true
}

func hasAllTags(have, want []string) bool {
	set := map[string]bool{}
	for _, t := range have {
		set[t] = true
	}
	for _, w := range want {
		if !set[w] {
			return false
		}
	}
	return true
}

// handleListResources handles GET /api/v1/resources: one searchable list of every resource.
func (rt *Router) handleListResources(w http.ResponseWriter, r *http.Request) {
	cs := rt.consoleStore()
	if cs == nil {
		writeError(w, http.StatusNotImplemented, "resource console is not configured")
		return
	}
	items, err := rt.consoleInventory(r, AbilityRead)
	if err != nil {
		rt.internalError(w, "api: list resources: inventory", err)
		return
	}
	tags, err := cs.ListAllResourceTags(r.Context())
	if err != nil {
		rt.internalError(w, "api: list resources: tags", err)
		return
	}
	attachConsoleTags(items, tags)
	q := r.URL.Query()
	text := strings.ToLower(strings.TrimSpace(q.Get("q")))
	kind := q.Get("kind")
	wantTags := q["tag"]
	counts := map[string]int{}
	filtered := items[:0:0]
	for _, it := range items {
		if !matchesConsoleQuery(it, text) || !hasAllTags(it.Tags, wantTags) {
			continue
		}
		counts[it.Kind]++
		if kind == "" || kind == it.Kind {
			filtered = append(filtered, it)
		}
	}
	sort.Slice(filtered, func(a, b int) bool {
		if filtered[a].Kind != filtered[b].Kind {
			return filtered[a].Kind < filtered[b].Kind
		}
		return filtered[a].Name < filtered[b].Name
	})
	total := len(filtered)
	limit := envIntDefault("APP_RESOURCE_EXPLORER_MAX", 5000)
	if n, err := strconv.Atoi(q.Get("limit")); err == nil && n > 0 && n < limit {
		limit = n
	}
	offset, _ := strconv.Atoi(q.Get("offset"))
	if offset < 0 || offset > total {
		offset = total
	}
	end := min(total, offset+limit)
	writeJSON(w, http.StatusOK, consoleListResponse{Items: filtered[offset:end], Total: total, Counts: counts})
}

type consoleRef struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

type consoleBulkTagRequest struct {
	Action    string       `json:"action"`
	Tag       string       `json:"tag"`
	Resources []consoleRef `json:"resources"`
	DryRun    bool         `json:"dry_run,omitempty"`
}

type consoleBulkResult struct {
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type consoleBulkTagResponse struct {
	Action  string              `json:"action"`
	Tag     string              `json:"tag"`
	DryRun  bool                `json:"dry_run"`
	Results []consoleBulkResult `json:"results"`
	Counts  map[string]int      `json:"counts"`
}

func (rt *Router) ensureTag(ctx context.Context, name string) (store.Tag, error) {
	tag, err := rt.tags.GetTagByName(ctx, name)
	if err == nil {
		return tag, nil
	}
	if !errors.Is(err, store.ErrTagNotFound) {
		return store.Tag{}, fmt.Errorf("load tag: %w", err)
	}
	id, err := randomTagID()
	if err != nil {
		return store.Tag{}, fmt.Errorf("generate tag id: %w", err)
	}
	tag = store.Tag{ID: id, Name: name, CreatedAt: time.Now().UTC()}
	if saveErr := rt.tags.SaveTag(ctx, tag); saveErr != nil {
		if !errors.Is(saveErr, store.ErrTagNameTaken) {
			return store.Tag{}, fmt.Errorf("create tag: %w", saveErr)
		}
		return rt.tags.GetTagByName(ctx, name)
	}
	return tag, nil
}

// handleBulkResourceTags handles POST /api/v1/resources/tags/bulk. Each resource is authorized on
// its own, a dry run reports what would change and writes nothing.
func (rt *Router) handleBulkResourceTags(w http.ResponseWriter, r *http.Request) {
	cs := rt.consoleStore()
	if cs == nil {
		writeError(w, http.StatusNotImplemented, "resource console is not configured")
		return
	}
	var req consoleBulkTagRequest
	if !decodeReq(w, r, &req) {
		return
	}
	if req.Action != bulkTagAdd && req.Action != bulkTagRemove {
		writeError(w, http.StatusBadRequest, "action must be add or remove")
		return
	}
	name, err := validateTagName(req.Tag)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(req.Resources) == 0 {
		writeError(w, http.StatusBadRequest, "resources is required")
		return
	}
	if maxN := envIntDefault("APP_BULK_MAX_RESOURCES", 500); len(req.Resources) > maxN {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("too many resources (%d, max %d)", len(req.Resources), maxN))
		return
	}
	ctx := r.Context()
	writable, err := rt.consoleInventory(r, AbilityWrite)
	if err != nil {
		rt.internalError(w, "api: bulk resource tags: inventory", err)
		return
	}
	all, err := rt.consoleInventory(r, AbilityRead)
	if err != nil {
		rt.internalError(w, "api: bulk resource tags: inventory", err)
		return
	}
	canWrite, exists, refs := map[string]bool{}, map[string]bool{}, map[string]store.ResourceRef{}
	for _, it := range writable {
		canWrite[store.ResourceRef{Kind: it.Kind, Name: it.Name}.Key()] = true
	}
	for _, it := range all {
		k := store.ResourceRef{Kind: it.Kind, Name: it.Name}.Key()
		exists[k] = true
		refs[k] = it.ref()
	}
	current, err := cs.ListAllResourceTags(ctx)
	if err != nil {
		rt.internalError(w, "api: bulk resource tags: tags", err)
		return
	}
	var tag store.Tag
	if existing, getErr := rt.tags.GetTagByName(ctx, name); getErr == nil {
		tag = existing
	} else if !errors.Is(getErr, store.ErrTagNotFound) {
		rt.internalError(w, "api: bulk resource tags: load tag", getErr)
		return
	}
	resp := consoleBulkTagResponse{Action: req.Action, Tag: name, DryRun: req.DryRun, Results: []consoleBulkResult{}, Counts: map[string]int{}}
	for _, ref := range req.Resources {
		display := store.ResourceRef{Kind: ref.Kind, Name: ref.Name}.Key()
		tagged, ok := refs[display]
		if !ok {
			tagged = store.ResourceRef{Kind: ref.Kind, Name: ref.Name}
		}
		res := rt.applyBulkTag(ctx, cs, req, name, &tag, tagged, display, canWrite, exists, current)
		res.Kind, res.Name = ref.Kind, ref.Name
		resp.Results = append(resp.Results, res)
		resp.Counts[res.Status]++
	}
	writeJSON(w, http.StatusMultiStatus, resp)
}

func (rt *Router) applyBulkTag(ctx context.Context, cs ResourceConsoleStore, req consoleBulkTagRequest, name string, tag *store.Tag, ref store.ResourceRef, display string, canWrite, exists map[string]bool, current map[string][]store.Tag) consoleBulkResult {
	res := consoleBulkResult{Kind: ref.Kind, Name: ref.Name}
	key := ref.Key()
	switch {
	case !exists[display]:
		res.Status = bulkStatusNotFound
		return res
	case !canWrite[display]:
		res.Status = bulkStatusDenied
		return res
	}
	has := false
	for _, t := range current[key] {
		has = has || t.Name == name
	}
	if (req.Action == bulkTagAdd && has) || (req.Action == bulkTagRemove && !has) {
		res.Status = bulkStatusSkipped
		return res
	}
	if req.DryRun {
		res.Status = bulkStatusWouldRun
		return res
	}
	var err error
	if req.Action == bulkTagAdd {
		if tag.ID == "" {
			if *tag, err = rt.ensureTag(ctx, name); err != nil {
				return failedBulk(res, err)
			}
		}
		if ref.Kind == store.ResourceKindApp {
			_, err = rt.attachTagByName(ctx, ref.Name, name)
		} else {
			err = cs.AttachResourceTag(ctx, tag.ID, ref)
		}
	} else {
		err = cs.DetachResourceTag(ctx, tag.ID, ref)
	}
	if err != nil {
		return failedBulk(res, err)
	}
	res.Status = bulkStatusOK
	return res
}

func failedBulk(res consoleBulkResult, err error) consoleBulkResult {
	res.Status = bulkStatusError
	res.Message = err.Error()
	return res
}

type resourceViewResource struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Query     string    `json:"query"`
	CreatedAt time.Time `json:"created_at"`
}

func (rt *Router) viewOwner(r *http.Request) (string, bool) {
	actorType, actorID, _, ok := rt.currentActor(r)
	if !ok {
		return "", false
	}
	return actorType + ":" + actorID, true
}

// handleListResourceViews handles GET /api/v1/resources/views.
func (rt *Router) handleListResourceViews(w http.ResponseWriter, r *http.Request) {
	cs := rt.consoleStore()
	owner, ok := rt.viewOwner(r)
	if cs == nil || !ok {
		writeJSON(w, http.StatusOK, []resourceViewResource{})
		return
	}
	views, err := cs.ListResourceViews(r.Context(), owner)
	if err != nil {
		rt.internalError(w, "api: list resource views", err)
		return
	}
	out := make([]resourceViewResource, 0, len(views))
	for _, v := range views {
		out = append(out, resourceViewResource{ID: v.ID, Name: v.Name, Query: v.Query, CreatedAt: v.CreatedAt})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleSaveResourceView handles POST /api/v1/resources/views.
func (rt *Router) handleSaveResourceView(w http.ResponseWriter, r *http.Request) {
	cs := rt.consoleStore()
	owner, ok := rt.viewOwner(r)
	if cs == nil || !ok {
		writeError(w, http.StatusNotImplemented, "saved views need a signed-in user or a user token")
		return
	}
	var req resourceViewResource
	if !decodeReq(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len(req.Name) > 64 {
		writeError(w, http.StatusBadRequest, "name is required, 64 characters at most")
		return
	}
	if len(req.Query) > 2048 || !json.Valid([]byte(req.Query)) {
		writeError(w, http.StatusBadRequest, "query must be a JSON object of 2048 bytes at most")
		return
	}
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		rt.internalError(w, "api: save resource view: id", err)
		return
	}
	v := store.ResourceView{ID: hex.EncodeToString(buf), Owner: owner, Name: req.Name, Query: req.Query, CreatedAt: time.Now().UTC()}
	if err := cs.SaveResourceView(r.Context(), v); err != nil {
		rt.internalError(w, "api: save resource view", err)
		return
	}
	saved := resourceViewResource{ID: v.ID, Name: v.Name, Query: v.Query, CreatedAt: v.CreatedAt}
	if views, err := cs.ListResourceViews(r.Context(), owner); err == nil {
		for _, sv := range views {
			if sv.Name == v.Name {
				saved = resourceViewResource{ID: sv.ID, Name: sv.Name, Query: sv.Query, CreatedAt: sv.CreatedAt}
			}
		}
	}
	writeJSON(w, http.StatusCreated, saved)
}

// handleDeleteResourceView handles DELETE /api/v1/resources/views/{id}.
func (rt *Router) handleDeleteResourceView(w http.ResponseWriter, r *http.Request) {
	cs := rt.consoleStore()
	owner, ok := rt.viewOwner(r)
	if cs == nil || !ok {
		writeError(w, http.StatusNotFound, "view not found")
		return
	}
	if err := cs.DeleteResourceView(r.Context(), owner, r.PathValue("id")); err != nil {
		if errors.Is(err, store.ErrResourceViewNotFound) {
			writeError(w, http.StatusNotFound, "view not found")
			return
		}
		rt.internalError(w, "api: delete resource view", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (rt *Router) tagNamesByResource(ctx context.Context) map[string][]string {
	out := map[string][]string{}
	cs := rt.consoleStore()
	if cs == nil {
		return out
	}
	tags, err := cs.ListAllResourceTags(ctx)
	if err != nil {
		rt.logger.Warn("api: usage report: tags skipped: " + err.Error())
		return out
	}
	for k, v := range tags {
		out[k] = tagNamesFromStoreTags(v)
	}
	return out
}
