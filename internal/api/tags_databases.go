package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

// DatabaseTagStore is the store surface for database tags, held in resource_tags.
type DatabaseTagStore interface {
	AttachResourceTag(ctx context.Context, tagID string, ref store.ResourceRef) error
	AttachResourceTagCapped(ctx context.Context, tagID string, ref store.ResourceRef, limit int) error
	DetachResourceTag(ctx context.Context, tagID string, ref store.ResourceRef) error
	ListTagsForResource(ctx context.Context, ref store.ResourceRef) ([]store.Tag, error)
	ListTagsForKind(ctx context.Context, kind string) (map[string][]store.Tag, error)
	ListResourceNamesByTag(ctx context.Context, tagID, kind string) ([]string, error)
}

// The handler drops databases the caller cannot read, so team-only members may call it.
func init() {
	tenantRoutes["GET /api/v1/tags/{id}/databases"] = true
}

func (rt *Router) databaseTagStore() DatabaseTagStore {
	s, _ := rt.tags.(DatabaseTagStore)
	return s
}

func databaseTagRef(name string) store.ResourceRef {
	return store.ResourceRef{Kind: store.ResourceKindDatabase, Name: name}
}

// maxTagsPerResource is the live cap for every non-app kind.
func (rt *Router) maxTagsPerResource(ctx context.Context) int {
	return rt.runtimeSettings.Limit(ctx, runtimesettings.KeyMaxTagsPerResource)
}

var errTooManyResourceTags = errors.New("resource has reached the maximum number of tags")

// databaseTagsOrNotFound loads the database first so a missing one answers 404, never an empty list.
func (rt *Router) databaseTagsOrNotFound(w http.ResponseWriter, r *http.Request, name string) (DatabaseTagStore, bool) {
	ts := rt.databaseTagStore()
	if ts == nil {
		writeError(w, http.StatusNotImplemented, "database tags are not available")
		return nil, false
	}
	if _, err := rt.databases.GetDesiredDatabase(r.Context(), name); errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusNotFound, "database not found")
		return nil, false
	} else if err != nil {
		rt.internalError(w, "api: database tags: load database failed", err, slog.String("name", name))
		return nil, false
	}
	return ts, true
}

// handleListDatabaseTags handles GET /api/v1/databases/{name}/tags.
func (rt *Router) handleListDatabaseTags(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	ts, ok := rt.databaseTagsOrNotFound(w, r, name)
	if !ok {
		return
	}
	tags, err := ts.ListTagsForResource(r.Context(), databaseTagRef(name))
	if err != nil {
		rt.internalError(w, "api: list database tags failed", err, slog.String("name", name))
		return
	}
	out := make([]tagResource, 0, len(tags))
	for _, t := range tags {
		out = append(out, toTagResource(t))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleAttachDatabaseTag handles POST /api/v1/databases/{name}/tags: get-or-create by name, then attach.
func (rt *Router) handleAttachDatabaseTag(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	ts, ok := rt.databaseTagsOrNotFound(w, r, name)
	if !ok {
		return
	}
	var req attachAppTagRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	tagName, err := validateTagName(req.Name)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	tag, err := rt.attachResourceTagByName(r.Context(), ts, databaseTagRef(name), tagName)
	if errors.Is(err, errTooManyResourceTags) {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if err != nil {
		rt.internalError(w, "api: attach database tag failed", err, slog.String("name", name))
		return
	}
	writeJSON(w, http.StatusCreated, toTagResource(tag))
}

// attachResourceTagByName enforces the cap only when the tag is not already attached.
func (rt *Router) attachResourceTagByName(ctx context.Context, ts DatabaseTagStore, ref store.ResourceRef, tagName string) (store.Tag, error) {
	existing, err := ts.ListTagsForResource(ctx, ref)
	if err != nil {
		return store.Tag{}, fmt.Errorf("list resource tags: %w", err)
	}
	for _, t := range existing {
		if t.Name == tagName {
			return t, nil
		}
	}
	limit := rt.maxTagsPerResource(ctx)
	if len(existing) >= limit {
		return store.Tag{}, errTooManyResourceTags
	}
	tag, err := rt.ensureTag(ctx, tagName)
	if err != nil {
		return store.Tag{}, err
	}
	if err := ts.AttachResourceTagCapped(ctx, tag.ID, ref, limit); errors.Is(err, store.ErrResourceTagCap) {
		return store.Tag{}, errTooManyResourceTags
	} else if err != nil {
		return store.Tag{}, fmt.Errorf("attach tag: %w", err)
	}
	return tag, nil
}

// handleDetachDatabaseTag handles DELETE /api/v1/databases/{name}/tags/{id}.
func (rt *Router) handleDetachDatabaseTag(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	id := r.PathValue("id")
	ts, ok := rt.databaseTagsOrNotFound(w, r, name)
	if !ok {
		return
	}
	if err := ts.DetachResourceTag(r.Context(), id, databaseTagRef(name)); err != nil {
		if errors.Is(err, store.ErrTagNotFound) {
			writeError(w, http.StatusNotFound, "tag not attached to this database")
			return
		}
		rt.internalError(w, "api: detach database tag failed", err, slog.String("name", name), slog.String("tag_id", id))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// tagDatabaseResource is one row of GET /api/v1/tags/{id}/databases.
type tagDatabaseResource struct {
	Name string `json:"name"`
}

// handleListDatabasesByTag handles GET /api/v1/tags/{id}/databases. Rows the caller
// cannot read are dropped, the same visibility the databases list applies.
func (rt *Router) handleListDatabasesByTag(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ts := rt.databaseTagStore()
	if ts == nil {
		writeError(w, http.StatusNotImplemented, "database tags are not available")
		return
	}
	if _, err := rt.tags.GetTag(r.Context(), id); errors.Is(err, store.ErrTagNotFound) {
		writeError(w, http.StatusNotFound, "tag not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: list databases by tag: load tag failed", err, slog.String("id", id))
		return
	}
	canSee, err := rt.databaseVisibilityFilter(r)
	if err != nil {
		rt.internalError(w, "api: list databases by tag: visibility", err)
		return
	}
	names, err := ts.ListResourceNamesByTag(r.Context(), id, store.ResourceKindDatabase)
	if err != nil {
		rt.internalError(w, "api: list databases by tag failed", err, slog.String("id", id))
		return
	}
	out := make([]tagDatabaseResource, 0, len(names))
	for _, n := range names {
		if canSee(n) {
			out = append(out, tagDatabaseResource{Name: n})
		}
	}
	writeJSON(w, http.StatusOK, pageQuery(w, r, out))
}

// stampDatabaseListTags fills each row's tags and keeps only rows carrying tagFilter when it is set.
func (rt *Router) stampDatabaseListTags(ctx context.Context, rows []databaseListResource, tagFilter string) ([]databaseListResource, error) {
	ts := rt.databaseTagStore()
	if ts == nil {
		if tagFilter != "" {
			return rows[:0], nil
		}
		return rows, nil
	}
	byName, err := ts.ListTagsForKind(ctx, store.ResourceKindDatabase)
	if err != nil {
		return nil, err
	}
	out := rows[:0]
	for _, row := range rows {
		row.Tags = tagNamesFromStoreTags(byName[row.Name])
		keep := tagFilter == ""
		for _, t := range row.Tags {
			keep = keep || t == tagFilter
		}
		if keep {
			out = append(out, row)
		}
	}
	return out, nil
}
