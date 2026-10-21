// Package repolist caches and queries the repository lists of connected git
// providers: one cached snapshot per connection, filtered and paged server
// side so a huge account never reaches the browser in one response.
package repolist

import (
	"sort"
	"strings"
)

// Repo is one repository in provider-neutral form.
type Repo struct {
	Key           string `json:"key"`
	FullName      string `json:"full_name"`
	Name          string `json:"name"`
	Owner         string `json:"owner"`
	Private       bool   `json:"private"`
	Archived      bool   `json:"archived"`
	Fork          bool   `json:"fork"`
	DefaultBranch string `json:"default_branch"`
	CloneURL      string `json:"clone_url"`
	WebURL        string `json:"web_url,omitempty"`
	Language      string `json:"language,omitempty"`
	PushedAt      string `json:"pushed_at,omitempty"`
	ProjectID     int64  `json:"project_id,omitempty"`
	AccountType   string `json:"account_type,omitempty"`
	// Provider, ConnectionID, ConnectionLabel and Account say which
	// connection a row came from; they are stamped when lists merge and are
	// never cached.
	Provider        string `json:"provider,omitempty"`
	ConnectionID    string `json:"connection_id,omitempty"`
	ConnectionLabel string `json:"connection_label,omitempty"`
	Account         string `json:"account,omitempty"`
}

// Mode selects how a boolean attribute filters the list.
type Mode string

// Filter modes for the archived and forks attributes.
const (
	Include Mode = "include"
	Exclude Mode = "exclude"
	Only    Mode = "only"
)

// ParseMode maps a query value to a Mode, defaulting to Include.
func ParseMode(v string) Mode {
	switch Mode(strings.ToLower(strings.TrimSpace(v))) {
	case Exclude:
		return Exclude
	case Only:
		return Only
	default:
		return Include
	}
}

func (m Mode) allows(v bool) bool {
	switch m {
	case Exclude:
		return !v
	case Only:
		return v
	default:
		return true
	}
}

// Query narrows and pages a repository list.
type Query struct {
	Search   string
	Owner    string
	Archived Mode
	Forks    Mode
	Limit    int
	Offset   int
}

// DefaultLimit and MaxLimit bound one page of results.
const (
	DefaultLimit = 100
	MaxLimit     = 500
	maxOwners    = 50
)

// OwnerCount is one owner facet: how many repos an owner/org has under the
// archived and forks filters.
type OwnerCount struct {
	Login string `json:"login"`
	Count int    `json:"count"`
}

// Page is one filtered, ordered slice of a repository list.
type Page struct {
	Items  []Repo
	Total  int
	Limit  int
	Offset int
	// NextOffset is the offset of the following page, or -1 on the last page.
	NextOffset int
	Owners     []OwnerCount
}

// Apply filters repos by q and returns the requested page, ordered by full
// name so offsets stay stable between requests.
func Apply(repos []Repo, q Query) Page {
	limit := q.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}
	limit = min(limit, MaxLimit)
	offset := max(q.Offset, 0)
	terms := strings.Fields(strings.ToLower(q.Search))
	owner := strings.ToLower(strings.TrimSpace(q.Owner))

	owners := map[string]*OwnerCount{}
	matched := make([]Repo, 0, len(repos))
	for _, r := range repos {
		if !q.Archived.allows(r.Archived) || !q.Forks.allows(r.Fork) {
			continue
		}
		if c, ok := owners[strings.ToLower(r.Owner)]; ok {
			c.Count++
		} else {
			owners[strings.ToLower(r.Owner)] = &OwnerCount{Login: r.Owner, Count: 1}
		}
		if owner != "" && strings.ToLower(r.Owner) != owner {
			continue
		}
		if !matchesAll(r, terms) {
			continue
		}
		matched = append(matched, r)
	}
	sort.SliceStable(matched, func(i, j int) bool {
		return strings.ToLower(matched[i].FullName) < strings.ToLower(matched[j].FullName)
	})

	page := Page{Total: len(matched), Limit: limit, Offset: offset, NextOffset: -1, Items: []Repo{}}
	if offset < len(matched) {
		end := min(offset+limit, len(matched))
		page.Items = matched[offset:end]
		if end < len(matched) {
			page.NextOffset = end
		}
	}
	page.Owners = topOwners(owners)
	return page
}

func matchesAll(r Repo, terms []string) bool {
	if len(terms) == 0 {
		return true
	}
	hay := strings.ToLower(r.FullName)
	for _, t := range terms {
		if !strings.Contains(hay, t) {
			return false
		}
	}
	return true
}

func topOwners(m map[string]*OwnerCount) []OwnerCount {
	out := make([]OwnerCount, 0, len(m))
	for _, c := range m {
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return strings.ToLower(out[i].Login) < strings.ToLower(out[j].Login)
	})
	if len(out) > maxOwners {
		out = out[:maxOwners]
	}
	return out
}
