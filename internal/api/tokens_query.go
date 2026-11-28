package api

import (
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Token kinds derived from how a token was minted; the store has no origin column.
const (
	tokenKindEmergency  = "emergency"
	tokenKindAgent      = "agent"
	tokenKindSystem     = "system"
	tokenKindUser       = "user"
	tokenStatusExpiring = "expiring"

	emergencyTokenName = "emergency (host)"
	tokenListMaxLimit  = 500
	tokenListDefLimit  = 100
)

// tokenFilter narrows a token list; the same shape drives bulk revoke.
type tokenFilter struct {
	Q             string     `json:"q,omitempty"`
	Status        string     `json:"status,omitempty"`
	Ability       string     `json:"ability,omitempty"`
	Name          string     `json:"name,omitempty"`
	Kind          string     `json:"kind,omitempty"`
	ExpiresBefore *time.Time `json:"expires_before,omitempty"`
}

func (f tokenFilter) empty() bool {
	return f == tokenFilter{}
}

func tokenKind(t tokenResource) string {
	switch {
	case t.Name == emergencyTokenName:
		return tokenKindEmergency
	case t.Agent != nil:
		return tokenKindAgent
	case t.OwnerUserID == "":
		return tokenKindSystem
	}
	return tokenKindUser
}

func (f tokenFilter) matches(t tokenResource, now time.Time, window time.Duration) bool {
	if f.Q != "" && !strings.Contains(strings.ToLower(t.Name), strings.ToLower(f.Q)) {
		return false
	}
	if f.Name != "" && t.Name != f.Name {
		return false
	}
	if f.Ability != "" && !slices.Contains(t.Abilities, f.Ability) {
		return false
	}
	if f.Kind != "" && tokenKind(t) != f.Kind {
		return false
	}
	if f.ExpiresBefore != nil && (t.ExpiresAt == nil || !t.ExpiresAt.Before(*f.ExpiresBefore)) {
		return false
	}
	switch f.Status {
	case "":
	case tokenStatusExpiring:
		return t.Status == tokenStatusActive && t.ExpiresAt != nil && t.ExpiresAt.Before(now.Add(window))
	default:
		return t.Status == f.Status
	}
	return true
}

// tokenListQuery is the parsed set of optional list parameters.
type tokenListQuery struct {
	Filter tokenFilter
	Sort   string
	Desc   bool
	Limit  int
	Offset int
	Groups bool
	Paged  bool
}

// parseTokenListQuery reports Paged false when no new parameter is present, which keeps the old array response.
func parseTokenListQuery(v url.Values) (tokenListQuery, bool) {
	q := tokenListQuery{Sort: "created", Desc: true, Limit: tokenListDefLimit}
	for _, k := range []string{"q", "status", "ability", "name", "kind", "expires_before", "sort", "order", "limit", "cursor", "groups"} {
		if v.Has(k) {
			q.Paged = true
		}
	}
	q.Filter = tokenFilter{Q: v.Get("q"), Status: v.Get("status"), Ability: v.Get("ability"), Name: v.Get("name"), Kind: v.Get("kind")}
	if raw := v.Get("expires_before"); raw != "" {
		ts, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return q, false
		}
		q.Filter.ExpiresBefore = &ts
	}
	if s := v.Get("sort"); s != "" {
		if !slices.Contains([]string{"name", "created", "last_used", "expires"}, s) {
			return q, false
		}
		q.Sort = s
		q.Desc = s == "created" || s == "last_used"
	}
	switch v.Get("order") {
	case "asc":
		q.Desc = false
	case "desc":
		q.Desc = true
	case "":
	default:
		return q, false
	}
	if raw := v.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			return q, false
		}
		q.Limit = min(n, tokenListMaxLimit)
	}
	if raw := v.Get("cursor"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			return q, false
		}
		q.Offset = n
	}
	q.Groups = v.Get("groups") == "name"
	if st := q.Filter.Status; st != "" && !slices.Contains([]string{tokenStatusActive, tokenStatusExpired, tokenStatusRevoked, tokenStatusExpiring}, st) {
		return q, false
	}
	return q, true
}

func sortTokenResources(items []tokenResource, by string, desc bool) {
	key := func(t tokenResource) time.Time {
		switch by {
		case "last_used":
			if t.LastUsedAt != nil {
				return *t.LastUsedAt
			}
		case "expires":
			if t.ExpiresAt != nil {
				return *t.ExpiresAt
			}
		}
		return t.CreatedAt
	}
	slices.SortStableFunc(items, func(a, b tokenResource) int {
		var c int
		if by == "name" {
			c = strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
		} else {
			c = key(a).Compare(key(b))
		}
		if desc {
			return -c
		}
		return c
	})
}

type tokenListPage struct {
	Items          []tokenResource `json:"items"`
	Total          int             `json:"total"`
	CountsByStatus map[string]int  `json:"counts_by_status"`
	NextCursor     string          `json:"next_cursor,omitempty"`
}

type tokenGroup struct {
	Name             string     `json:"name"`
	Count            int        `json:"count"`
	Active           int        `json:"active"`
	Expiring         int        `json:"expiring"`
	Expired          int        `json:"expired"`
	OldestCreated    time.Time  `json:"oldest_created"`
	NewestLastUsedAt *time.Time `json:"newest_last_used,omitempty"`
}

type tokenGroupsPage struct {
	Groups         []tokenGroup   `json:"groups"`
	Total          int            `json:"total"`
	CountsByStatus map[string]int `json:"counts_by_status"`
}

// countTokenStatuses counts every status, with expiring as a subset of active.
func countTokenStatuses(items []tokenResource, now time.Time, window time.Duration) map[string]int {
	out := map[string]int{tokenStatusActive: 0, tokenStatusExpired: 0, tokenStatusRevoked: 0, tokenStatusExpiring: 0}
	expiring := tokenFilter{Status: tokenStatusExpiring}
	for _, t := range items {
		out[t.Status]++
		if expiring.matches(t, now, window) {
			out[tokenStatusExpiring]++
		}
	}
	return out
}

func groupTokensByName(items []tokenResource, now time.Time, window time.Duration) []tokenGroup {
	idx := map[string]int{}
	var groups []tokenGroup
	expiring := tokenFilter{Status: tokenStatusExpiring}
	for _, t := range items {
		i, ok := idx[t.Name]
		if !ok {
			i = len(groups)
			idx[t.Name] = i
			groups = append(groups, tokenGroup{Name: t.Name, OldestCreated: t.CreatedAt})
		}
		g := &groups[i]
		g.Count++
		switch t.Status {
		case tokenStatusActive:
			g.Active++
		case tokenStatusExpired:
			g.Expired++
		}
		if expiring.matches(t, now, window) {
			g.Expiring++
		}
		if t.CreatedAt.Before(g.OldestCreated) {
			g.OldestCreated = t.CreatedAt
		}
		if t.LastUsedAt != nil && (g.NewestLastUsedAt == nil || t.LastUsedAt.After(*g.NewestLastUsedAt)) {
			g.NewestLastUsedAt = t.LastUsedAt
		}
	}
	slices.SortStableFunc(groups, func(a, b tokenGroup) int { return b.Count - a.Count })
	return groups
}
