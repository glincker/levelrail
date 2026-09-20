package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// TagStore is the store surface the tag handlers need, mirroring
// FeatureFlagStore's own shape for a different child resource.
type TagStore interface {
	SaveTag(ctx context.Context, t store.Tag) error
	GetTag(ctx context.Context, id string) (store.Tag, error)
	GetTagByName(ctx context.Context, name string) (store.Tag, error)
	ListTags(ctx context.Context) ([]store.Tag, error)
	DeleteTag(ctx context.Context, id string) error
	AttachAppTag(ctx context.Context, tagID, appName string) error
	DetachAppTag(ctx context.Context, tagID, appName string) error
	ListTagsForApp(ctx context.Context, appName string) ([]store.Tag, error)
	ListTagsForApps(ctx context.Context, appNames []string) (map[string][]store.Tag, error)
	ListAppNamesByTag(ctx context.Context, tagID string) ([]string, error)
}

// tagResource is the wire shape for a tag.
type tagResource struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

func toTagResource(t store.Tag) tagResource {
	return tagResource{ID: t.ID, Name: t.Name, CreatedAt: t.CreatedAt}
}

// tagNamesFromStoreTags maps a []store.Tag to the bare name list
// appResource.Tags carries: a caller of GET /api/v1/apps or
// /api/v1/apps/{name} wants to filter and display by name, not chase an
// ID through a second lookup.
func tagNamesFromStoreTags(tags []store.Tag) []string {
	if len(tags) == 0 {
		return nil
	}
	names := make([]string, len(tags))
	for i, t := range tags {
		names[i] = t.Name
	}
	return names
}

// appTagNames is handleGetApp's single-app counterpart to
// handleListApps' batched rt.tags.ListTagsForApps call.
func (rt *Router) appTagNames(ctx context.Context, appName string) ([]string, error) {
	tags, err := rt.tags.ListTagsForApp(ctx, appName)
	if err != nil {
		return nil, err
	}
	return tagNamesFromStoreTags(tags), nil
}

// validateTagName checks the one field a tag has: non-empty, trimmed,
// capped at a reasonable display length so a chip in the UI never wraps
// unreasonably.
func validateTagName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("name is required")
	}
	if len(name) > 64 {
		return "", errors.New("name must be 64 characters or fewer")
	}
	return name, nil
}

// handleCreateTag handles POST /api/v1/tags.
func (rt *Router) handleCreateTag(w http.ResponseWriter, r *http.Request) {
	var req tagResource
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	name, err := validateTagName(req.Name)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	id, err := randomTagID()
	if err != nil {
		rt.logger.Error("api: create tag: generate id failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	tag := store.Tag{ID: id, Name: name, CreatedAt: time.Now().UTC()}
	if err := rt.tags.SaveTag(r.Context(), tag); err != nil {
		if errors.Is(err, store.ErrTagNameTaken) {
			writeError(w, http.StatusConflict, "a tag with this name already exists")
			return
		}
		rt.logger.Error("api: create tag failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, toTagResource(tag))
}

// handleListTags handles GET /api/v1/tags.
func (rt *Router) handleListTags(w http.ResponseWriter, r *http.Request) {
	tags, err := rt.tags.ListTags(r.Context())
	if err != nil {
		rt.logger.Error("api: list tags failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]tagResource, 0, len(tags))
	for _, t := range tags {
		out = append(out, toTagResource(t))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleDeleteTag handles DELETE /api/v1/tags/{id}.
func (rt *Router) handleDeleteTag(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := rt.tags.DeleteTag(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrTagNotFound) {
			writeError(w, http.StatusNotFound, "tag not found")
			return
		}
		rt.logger.Error("api: delete tag failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// tagAppResource is GET /api/v1/tags/{id}/apps' wire shape: enough for
// a filter-by-tag result to link back into the apps list, without
// duplicating every field appListResource already carries for a
// consumer that only needs to know "does this tag apply."
type tagAppResource struct {
	Name string `json:"name"`
}

// handleListAppsByTag handles GET /api/v1/tags/{id}/apps: "list
// resources filtered by tag", apps-only for this first cut (see
// migrations/0106_tags.sql's own doc comment).
func (rt *Router) handleListAppsByTag(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := rt.tags.GetTag(r.Context(), id); errors.Is(err, store.ErrTagNotFound) {
		writeError(w, http.StatusNotFound, "tag not found")
		return
	} else if err != nil {
		rt.logger.Error("api: list apps by tag: load tag failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	names, err := rt.tags.ListAppNamesByTag(r.Context(), id)
	if err != nil {
		rt.logger.Error("api: list apps by tag failed", slog.String("error", err.Error()), slog.String("id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	out := make([]tagAppResource, len(names))
	for i, name := range names {
		out[i] = tagAppResource{Name: name}
	}
	writeJSON(w, http.StatusOK, out)
}

// handleListAppTags handles GET /api/v1/apps/{name}/tags.
func (rt *Router) handleListAppTags(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.logger.Error("api: list app tags: load app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	tags, err := rt.tags.ListTagsForApp(r.Context(), name)
	if err != nil {
		rt.logger.Error("api: list app tags failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]tagResource, 0, len(tags))
	for _, t := range tags {
		out = append(out, toTagResource(t))
	}
	writeJSON(w, http.StatusOK, out)
}

// attachAppTagRequest is POST /api/v1/apps/{name}/tags' body: a tag
// name, not an ID, so a caller (CLI, UI) doesn't have to resolve a name
// to an ID first. A name that doesn't exist yet is created on the fly,
// the same "declare it, the server resolves or creates" convenience
// app.yaml's own env: { from: ... } resolution gives a deploy-time
// reference, scaled down to this one field.
type attachAppTagRequest struct {
	Name string `json:"name"`
}

// handleAttachAppTag handles POST /api/v1/apps/{name}/tags: attaches an
// existing tag by name, or creates it first if this is the first time
// it's been used.
func (rt *Router) handleAttachAppTag(w http.ResponseWriter, r *http.Request) {
	appName := r.PathValue("name")
	if _, err := rt.apps.GetDesiredService(r.Context(), appName); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.logger.Error("api: attach app tag: load app failed", slog.String("error", err.Error()), slog.String("name", appName))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	var req attachAppTagRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	name, err := validateTagName(req.Name)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	tag, err := rt.tags.GetTagByName(r.Context(), name)
	if err != nil && !errors.Is(err, store.ErrTagNotFound) {
		rt.logger.Error("api: attach app tag: load tag failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if errors.Is(err, store.ErrTagNotFound) {
		id, idErr := randomTagID()
		if idErr != nil {
			rt.logger.Error("api: attach app tag: generate id failed", slog.String("error", idErr.Error()))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		tag = store.Tag{ID: id, Name: name, CreatedAt: time.Now().UTC()}
		saveErr := rt.tags.SaveTag(r.Context(), tag)
		switch {
		case saveErr == nil:
			// created
		case errors.Is(saveErr, store.ErrTagNameTaken):
			// Lost a create race: someone else created this exact name
			// between the GetTagByName miss above and this SaveTag
			// call. Re-read it rather than failing a valid attach.
			tag, err = rt.tags.GetTagByName(r.Context(), name)
			if err != nil {
				rt.logger.Error("api: attach app tag: reload after race failed", slog.String("error", err.Error()), slog.String("name", name))
				writeError(w, http.StatusInternalServerError, "internal error")
				return
			}
		default:
			rt.logger.Error("api: attach app tag: create tag failed", slog.String("error", saveErr.Error()), slog.String("name", name))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	if err := rt.tags.AttachAppTag(r.Context(), tag.ID, appName); err != nil {
		rt.logger.Error("api: attach app tag failed", slog.String("error", err.Error()), slog.String("name", appName), slog.String("tag_id", tag.ID))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusCreated, toTagResource(tag))
}

// handleDetachAppTag handles DELETE /api/v1/apps/{name}/tags/{id}: id
// is the tag's ID (from GET .../tags above or GET /api/v1/tags), not
// its name, symmetric with GET /api/v1/tags/{id} and DELETE
// /api/v1/tags/{id}.
func (rt *Router) handleDetachAppTag(w http.ResponseWriter, r *http.Request) {
	appName := r.PathValue("name")
	id := r.PathValue("id")
	if err := rt.tags.DetachAppTag(r.Context(), id, appName); err != nil {
		if errors.Is(err, store.ErrTagNotFound) {
			writeError(w, http.StatusNotFound, "tag not attached to this app")
			return
		}
		rt.logger.Error("api: detach app tag failed", slog.String("error", err.Error()), slog.String("name", appName), slog.String("tag_id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// randomTagID mirrors randomFeatureFlagID's exact shape (9 random
// bytes, URL-safe base64, a short type prefix), duplicated rather than
// shared for the same "different resource, different ID space" reason
// randomFeatureFlagID's own doc comment gives.
func randomTagID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("api: generate tag id: %w", err)
	}
	return "tag_" + base64.RawURLEncoding.EncodeToString(buf), nil
}
