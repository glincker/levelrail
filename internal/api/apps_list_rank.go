package api

import (
	"context"
	"log/slog"
	"net/http"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/search"
	"github.com/GLINCKER/levelrail/internal/store"
)

// appRank orders an app list by search rank. It only narrows the DB query:
// the caller's own read filter still runs afterwards.
type appRank struct {
	names []string
	pos   map[string]int
}

func (a *appRank) sort(svcs []store.DesiredService) {
	sort.SliceStable(svcs, func(i, j int) bool { return a.pos[svcs[i].Name] < a.pos[svcs[j].Name] })
}

// listRankedApps pages in rank order over names alone, so only the page's
// service rows are loaded. canRead is the caller's own visibility check.
func listRankedApps(ctx context.Context, fs appFilterStore, f store.AppListFilter, rank *appRank, canRead func(string) bool) ([]store.DesiredService, int, error) {
	nf := f
	nf.Query, nf.Names, nf.Limit, nf.Offset = "", rank.names, 0, 0
	matching, err := fs.ListAppNamesFiltered(ctx, nf)
	if err != nil {
		return nil, 0, err
	}
	keep := make(map[string]bool, len(matching))
	for _, n := range matching {
		keep[n] = true
	}
	ordered := make([]string, 0, len(matching))
	for _, n := range rank.names {
		if keep[n] && canRead(n) {
			ordered = append(ordered, n)
		}
	}
	total := len(ordered)
	if f.Offset >= total {
		return nil, total, nil
	}
	page := ordered[f.Offset:]
	if f.Limit > 0 && f.Limit < len(page) {
		page = page[:f.Limit]
	}
	svcs, _, err := fs.ListDesiredServicesFiltered(ctx, store.AppListFilter{Names: page})
	if err != nil {
		return nil, 0, err
	}
	rank.sort(svcs)
	return svcs, total, nil
}

// rankedAppOrder returns the search-ranked app names for ?q=, followed by any
// substring matches the index missed. It returns nil (use the plain LIKE list)
// when there is no query, the index is cold, or nothing ranked.
func (rt *Router) rankedAppOrder(r *http.Request, fs appFilterStore, f store.AppListFilter) *appRank {
	if strings.TrimSpace(f.Query) == "" {
		return nil
	}
	svc := rt.searchService()
	if svc == nil {
		return nil
	}
	caller, err := rt.newSearchCaller(r)
	if err != nil {
		return nil
	}
	names, warm, err := svc.RankedNames(r.Context(), caller, f.Query, search.KindApp)
	if err != nil {
		rt.logger.Warn("api: list apps: ranked search failed, using substring match", slog.String("error", err.Error()))
		return nil
	}
	if !warm || len(names) == 0 {
		return nil
	}
	pos := make(map[string]int, len(names))
	for i, n := range names {
		pos[n] = i
	}
	likeFilter := f
	likeFilter.Limit, likeFilter.Offset = 0, 0
	if extra, err := fs.ListAppNamesFiltered(r.Context(), likeFilter); err == nil {
		for _, n := range extra {
			if _, ok := pos[n]; !ok {
				pos[n] = len(names)
				names = append(names, n)
			}
		}
	}
	return &appRank{names: names, pos: pos}
}
