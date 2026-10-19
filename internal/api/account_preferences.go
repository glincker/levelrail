package api

import (
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

var (
	languagePattern    = regexp.MustCompile(`^[a-z]{2,3}(-[A-Za-z0-9]{2,8})?$`)
	landingPagePattern = regexp.MustCompile(`^/[A-Za-z0-9/_-]{0,100}$`)
)

var validThemes = map[string]bool{"": true, "light": true, "dark": true, "system": true}

type preferencesResource struct {
	Timezone    string `json:"timezone"`
	Language    string `json:"language"`
	Theme       string `json:"theme"`
	LandingPage string `json:"landing_page"`
	Gravatar    bool   `json:"gravatar"`
}

func toPreferencesResource(p store.UserPreferences) preferencesResource {
	return preferencesResource{Timezone: p.Timezone, Language: p.Language, Theme: p.Theme, LandingPage: p.LandingPage, Gravatar: p.Gravatar}
}

// preferencesProblem explains the first invalid field, empty when all are valid.
func preferencesProblem(p preferencesResource) string {
	if p.Timezone != "" {
		if len(p.Timezone) > 64 {
			return "timezone is not a valid IANA zone name"
		}
		if _, err := time.LoadLocation(p.Timezone); err != nil {
			return "timezone is not a valid IANA zone name"
		}
	}
	if p.Language != "" && !languagePattern.MatchString(p.Language) {
		return "language must be a locale code such as en or pt-BR"
	}
	if !validThemes[p.Theme] {
		return "theme must be light, dark or system"
	}
	if p.LandingPage != "" && (!landingPagePattern.MatchString(p.LandingPage) || strings.HasPrefix(p.LandingPage, "//")) {
		return "landing_page must be a path inside the dashboard such as /apps"
	}
	return ""
}

// handleGetPreferences handles GET /api/v1/auth/preferences: the caller's own settings.
func (rt *Router) handleGetPreferences(w http.ResponseWriter, r *http.Request) {
	userID, _, ok := rt.accountCaller(w, r, AbilityRead)
	if !ok {
		return
	}
	db := rt.accountDB()
	if db == nil {
		writeError(w, http.StatusNotImplemented, "preferences are not available on this server")
		return
	}
	p, err := db.GetUserPreferences(r.Context(), userID)
	if err != nil {
		rt.internalError(w, "api: get preferences failed", err)
		return
	}
	writeJSON(w, http.StatusOK, toPreferencesResource(p))
}

// handlePutPreferences handles PUT /api/v1/auth/preferences: replaces the caller's own settings.
func (rt *Router) handlePutPreferences(w http.ResponseWriter, r *http.Request) {
	userID, viaSession, ok := rt.accountCaller(w, r, AbilityWriteSensitive)
	if !ok {
		return
	}
	db := rt.accountDB()
	if db == nil {
		writeError(w, http.StatusNotImplemented, "preferences are not available on this server")
		return
	}
	var req preferencesResource
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if msg := preferencesProblem(req); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	err := db.SetUserPreferences(r.Context(), store.UserPreferences{
		UserID: userID, Timezone: req.Timezone, Language: req.Language, Theme: req.Theme,
		LandingPage: req.LandingPage, Gravatar: req.Gravatar,
	})
	if err != nil {
		rt.internalError(w, "api: set preferences failed", err)
		return
	}
	rt.recordAccountAction(r, userID, viaSession, accountEventPrefsUpdated, http.StatusOK)
	writeJSON(w, http.StatusOK, req)
}
