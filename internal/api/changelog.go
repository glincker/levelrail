package api

import (
	"net/http"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/changelog"
	"github.com/GLINCKER/levelrail/internal/version"
)

// Default and max page size for GET /api/v1/changelog's ?limit=.
const (
	defaultChangelogLimit = 10
	maxChangelogLimit     = 50
)

type changelogEntryResource struct {
	Version string   `json:"version"`
	Date    string   `json:"date"`
	Bullets []string `json:"bullets"`
}

type changelogResource struct {
	CurrentVersion string                   `json:"current_version"`
	Entries        []changelogEntryResource `json:"entries"`
}

// handleGetChangelog serves GET /api/v1/changelog: the most recent
// entries parsed from the repo's own release-please CHANGELOG.md (see
// internal/changelog and WithChangelog), the dashboard's "What's new"
// panel data source. AbilityRead, the same passive-visibility tier GET
// /api/v1/updates uses. rt.changelogEntries is nil, not an error, when
// no CHANGELOG.md was found at startup, so this always answers 200 with
// however many entries are actually available.
func (rt *Router) handleGetChangelog(w http.ResponseWriter, r *http.Request) {
	limit := defaultChangelogLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > maxChangelogLimit {
		limit = maxChangelogLimit
	}

	latest := changelog.Latest(rt.changelogEntries, limit)
	out := make([]changelogEntryResource, 0, len(latest))
	for _, e := range latest {
		out = append(out, changelogEntryResource{Version: e.Version, Date: e.Date, Bullets: e.Bullets})
	}
	writeJSON(w, http.StatusOK, changelogResource{CurrentVersion: version.Version, Entries: out})
}
