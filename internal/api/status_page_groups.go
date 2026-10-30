package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/statuspage"
	"github.com/GLINCKER/levelrail/internal/store"
)

type statusGroupResource struct {
	ID       string `json:"id,omitempty"`
	Name     string `json:"name"`
	Position int    `json:"position"`
}

func (rt *Router) handleListStatusGroups(w http.ResponseWriter, r *http.Request) {
	if !rt.statusPageReady(w) {
		return
	}
	list, err := rt.statusPage.ListStatusGroups(r.Context())
	if err != nil {
		rt.internalError(w, "api: list status groups failed", err)
		return
	}
	out := make([]statusGroupResource, 0, len(list))
	for _, g := range list {
		out = append(out, statusGroupResource{ID: g.ID, Name: g.Name, Position: g.Position})
	}
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) saveStatusGroup(w http.ResponseWriter, r *http.Request, id string, status int) {
	var req statusGroupResource
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len(req.Name) > statuspage.TuningFromEnv().GroupNameMax {
		writeError(w, http.StatusBadRequest, "name is required and must fit the group name limit")
		return
	}
	g := store.StatusGroup{ID: id, Name: req.Name, Position: req.Position}
	if err := rt.statusPage.SaveStatusGroup(r.Context(), g); err != nil {
		rt.internalError(w, "api: save status group failed", err)
		return
	}
	rt.statusView.Invalidate()
	writeJSON(w, status, statusGroupResource{ID: g.ID, Name: g.Name, Position: g.Position})
}

func (rt *Router) handleCreateStatusGroup(w http.ResponseWriter, r *http.Request) {
	if !rt.statusPageReady(w) {
		return
	}
	existing, err := rt.statusPage.ListStatusGroups(r.Context())
	if err != nil {
		rt.internalError(w, "api: create status group: list failed", err)
		return
	}
	if len(existing) >= statuspage.TuningFromEnv().GroupsMax {
		writeError(w, http.StatusBadRequest, "too many groups")
		return
	}
	id, err := store.NewStatusID("sg_")
	if err != nil {
		rt.internalError(w, "api: create status group: mint id failed", err)
		return
	}
	rt.saveStatusGroup(w, r, id, http.StatusCreated)
}

func (rt *Router) handleUpdateStatusGroup(w http.ResponseWriter, r *http.Request) {
	if !rt.statusPageReady(w) {
		return
	}
	id := r.PathValue("id")
	if _, err := rt.statusPage.GetStatusGroup(r.Context(), id); errors.Is(err, store.ErrStatusGroupNotFound) {
		writeError(w, http.StatusNotFound, "group not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: update status group: load failed", err)
		return
	}
	rt.saveStatusGroup(w, r, id, http.StatusOK)
}

func (rt *Router) handleDeleteStatusGroup(w http.ResponseWriter, r *http.Request) {
	if !rt.statusPageReady(w) {
		return
	}
	if err := rt.statusPage.DeleteStatusGroup(r.Context(), r.PathValue("id")); err != nil {
		rt.internalError(w, "api: delete status group failed", err)
		return
	}
	rt.statusView.Invalidate()
	w.WriteHeader(http.StatusNoContent)
}
