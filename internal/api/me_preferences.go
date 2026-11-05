package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	preferenceAppearanceKey   = "appearance"
	preferenceEventUpdated    = "preferences.appearance.updated"
	preferenceEventReset      = "preferences.appearance.reset"
	preferenceMaxDepth        = 4
	preferenceDefaultMaxBytes = 8192
)

// userPreferencesStore is the slice of the store the per-user preference routes need.
type userPreferencesStore interface {
	GetUserPreference(ctx context.Context, userID, key string) (string, bool, error)
	SetUserPreference(ctx context.Context, userID, key, value string, maxBytes int) error
	DeleteUserPreference(ctx context.Context, userID, key string) error
}

type mePreferenceResource struct {
	Value json.RawMessage `json:"value"`
}

func (rt *Router) userPreferencesDB() userPreferencesStore {
	for _, c := range []any{rt.policies, rt.apps, rt.tokens, rt.auditLog} {
		if s, ok := c.(userPreferencesStore); ok {
			return s
		}
	}
	return nil
}

func (rt *Router) preferencesMaxBytes(r *http.Request) int {
	if rt.runtimeSettings == nil {
		return preferenceDefaultMaxBytes
	}
	return rt.runtimeSettings.Limit(r.Context(), runtimesettings.KeyAccountPreferencesMaxBytes)
}

// validPreferenceObject accepts objects of strings, numbers, booleans and nested objects up to the depth cap.
func validPreferenceObject(v any, depth int) bool {
	obj, ok := v.(map[string]any)
	if !ok || depth > preferenceMaxDepth {
		return false
	}
	for _, e := range obj {
		switch x := e.(type) {
		case string, bool, json.Number:
		case map[string]any:
			if !validPreferenceObject(x, depth+1) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// handleGetMePreferenceAppearance handles GET /api/v1/me/preferences/appearance.
func (rt *Router) handleGetMePreferenceAppearance(w http.ResponseWriter, r *http.Request) {
	userID, _, ok := rt.accountCaller(w, r, AbilityRead)
	if !ok {
		return
	}
	db := rt.userPreferencesDB()
	if db == nil {
		writeError(w, http.StatusNotImplemented, "preferences are not available on this server")
		return
	}
	v, found, err := db.GetUserPreference(r.Context(), userID, preferenceAppearanceKey)
	if err != nil {
		rt.internalError(w, "api: get appearance preference failed", err)
		return
	}
	out := mePreferenceResource{Value: json.RawMessage("null")}
	if found {
		out.Value = json.RawMessage(v)
	}
	writeJSON(w, http.StatusOK, out)
}

// handlePutMePreferenceAppearance handles PUT /api/v1/me/preferences/appearance.
func (rt *Router) handlePutMePreferenceAppearance(w http.ResponseWriter, r *http.Request) {
	userID, viaSession, ok := rt.accountCaller(w, r, AbilityWrite)
	if !ok {
		return
	}
	db := rt.userPreferencesDB()
	if db == nil {
		writeError(w, http.StatusNotImplemented, "preferences are not available on this server")
		return
	}
	limit := rt.preferencesMaxBytes(r)
	raw, err := io.ReadAll(io.LimitReader(r.Body, int64(limit)+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(raw) > limit {
		writeError(w, http.StatusRequestEntityTooLarge, "appearance settings are too large")
		return
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var parsed any
	if err := dec.Decode(&parsed); err != nil || dec.More() || !validPreferenceObject(parsed, 1) {
		writeError(w, http.StatusBadRequest, "body must be a JSON object of strings, numbers, booleans and nested objects (up to 4 levels)")
		return
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := db.SetUserPreference(r.Context(), userID, preferenceAppearanceKey, compact.String(), limit); err != nil {
		if errors.Is(err, store.ErrUserPreferenceTooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "appearance settings are too large")
			return
		}
		rt.internalError(w, "api: save appearance preference failed", err)
		return
	}
	rt.recordAccountAction(r, userID, viaSession, preferenceEventUpdated, http.StatusOK)
	writeJSON(w, http.StatusOK, mePreferenceResource{Value: json.RawMessage(compact.Bytes())})
}

// handleDeleteMePreferenceAppearance handles DELETE /api/v1/me/preferences/appearance.
func (rt *Router) handleDeleteMePreferenceAppearance(w http.ResponseWriter, r *http.Request) {
	userID, viaSession, ok := rt.accountCaller(w, r, AbilityWrite)
	if !ok {
		return
	}
	db := rt.userPreferencesDB()
	if db == nil {
		writeError(w, http.StatusNotImplemented, "preferences are not available on this server")
		return
	}
	if err := db.DeleteUserPreference(r.Context(), userID, preferenceAppearanceKey); err != nil {
		rt.internalError(w, "api: reset appearance preference failed", err)
		return
	}
	rt.recordAccountAction(r, userID, viaSession, preferenceEventReset, http.StatusOK)
	writeJSON(w, http.StatusOK, mePreferenceResource{Value: json.RawMessage("null")})
}
