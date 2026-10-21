package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/gitprovider"
	"github.com/GLINCKER/levelrail/internal/repolist"
)

// gitRepoRateLimitResource tells the picker when listing may work again.
type gitRepoRateLimitResource struct {
	NextCheckAt       time.Time `json:"next_check_at"`
	RetryAfterSeconds int       `json:"retry_after_seconds"`
}

// gitRepoListResource is GET /api/v1/git/repos: one filtered page of the
// repositories of one provider or of every connected account, plus how
// complete and fresh the underlying lists are.
type gitRepoListResource struct {
	Provider string          `json:"provider"`
	Items    []repolist.Repo `json:"items"`
	// Total counts matches for the current filters; Loaded counts every
	// repository known before filtering.
	Total      int  `json:"total"`
	Loaded     int  `json:"loaded"`
	Limit      int  `json:"limit"`
	Offset     int  `json:"offset"`
	NextOffset *int `json:"next_offset"`
	HasMore    bool `json:"has_more"`
	// Truncated is true when the account has more repositories than the
	// cache cap (Cap), so some can only be reached by pasting a URL.
	Truncated  bool                           `json:"truncated"`
	Cap        int                            `json:"cap"`
	CachedAt   time.Time                      `json:"cached_at"`
	Stale      bool                           `json:"stale"`
	Refreshing bool                           `json:"refreshing"`
	Owners     []repolist.OwnerCount          `json:"owners"`
	Errors     []gitHubAppRepoListErrResource `json:"errors,omitempty"`
	// Connections lists every account the merged list was built from.
	Connections []gitRepoConnectionFacet  `json:"connections"`
	RateLimit   *gitRepoRateLimitResource `json:"rate_limit,omitempty"`
}

// gitRepoRateLimitedError is the 429 body: the provider's rate limit with a
// concrete next-check time, instead of a generic failure.
type gitRepoRateLimitedError struct {
	Error             string    `json:"error"`
	Code              string    `json:"code"`
	NextCheckAt       time.Time `json:"next_check_at"`
	RetryAfterSeconds int       `json:"retry_after_seconds"`
}

const codeRateLimited = "rate_limited"

func writeGitRateLimited(w http.ResponseWriter, provider string, rl *gitprovider.RateLimitError) {
	secs := max(int(time.Until(rl.NextCheck).Seconds())+1, 1)
	w.Header().Set("Retry-After", strconv.Itoa(secs))
	writeJSON(w, http.StatusTooManyRequests, gitRepoRateLimitedError{
		Error:             provider + " is rate limiting this control plane; the list will load again after the next check time",
		Code:              codeRateLimited,
		NextCheckAt:       rl.NextCheck.UTC(),
		RetryAfterSeconds: secs,
	})
}

func (rt *Router) gitProviderSecretsMissing(provider string) (string, bool) {
	if provider == gitProviderGitHub {
		return "the github app connection requires a master key to be configured on this control plane", rt.githubAppSecrets == nil
	}
	return errGitMasterKeyRequired, rt.gitConnSecrets == nil
}

// writeGitRepoListError maps a failed listing to a response: connection
// problems to their provider's own 409s, a rate limit to a typed 429, and
// anything else to a 502.
func (rt *Router) writeGitRepoListError(w http.ResponseWriter, provider string, err error) {
	var conn *repoListConnError
	if errors.As(err, &conn) {
		logMsg := "api: git repo list connection check failed for " + provider
		if provider == gitProviderGitHub {
			rt.writeGitHubAppTokenError(w, logMsg, conn.err)
		} else {
			rt.writeGitConnTokenError(w, logMsg, conn.err)
		}
		return
	}
	if rl, ok := gitprovider.AsRateLimit(err); ok {
		writeGitRateLimited(w, provider, rl)
		return
	}
	if rt.writeGitConnError(w, err) {
		return
	}
	rt.logger.Error("api: list git repos failed", slog.String("provider", provider), slog.String("error", err.Error()))
	writeError(w, http.StatusBadGateway, "failed to list repositories from "+provider)
}

// flatRepoList merges per-source results into one list plus freshness facts.
type flatRepoList struct {
	Repos      []repolist.Repo
	Errors     []gitHubAppRepoListErrResource
	Truncated  bool
	Stale      bool
	Refreshing bool
	CachedAt   time.Time
	RateLimit  *gitprovider.RateLimitError
	FirstErr   error
	Succeeded  int
}

func flattenRepoAccounts(accts []repoListAccount) flatRepoList {
	var f flatRepoList
	for _, a := range accts {
		if a.Err != nil {
			f.Errors = append(f.Errors, gitHubAppRepoListErrResource{AccountLogin: a.Account, Provider: a.Provider, ConnectionID: a.ConnectionID, ConnectionLabel: a.ConnectionLabel, Error: a.Err.Error()})
			if f.FirstErr == nil {
				f.FirstErr = a.Err
			}
			continue
		}
		f.Succeeded++
		for _, r := range a.Result.Repos {
			r.Provider, r.ConnectionID, r.ConnectionLabel, r.Account = a.Provider, a.ConnectionID, a.ConnectionLabel, a.Account
			f.Repos = append(f.Repos, r)
		}
		f.Truncated = f.Truncated || a.Result.Truncated
		f.Stale = f.Stale || a.Result.Stale
		f.Refreshing = f.Refreshing || a.Result.Refreshing
		if f.CachedAt.IsZero() || a.Result.FetchedAt.Before(f.CachedAt) {
			f.CachedAt = a.Result.FetchedAt
		}
		if rl := a.Result.RateLimit; rl != nil && (f.RateLimit == nil || rl.NextCheck.After(f.RateLimit.NextCheck)) {
			f.RateLimit = rl
		}
	}
	return f
}

func parseRepoQuery(r *http.Request) repolist.Query {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	return repolist.Query{
		Search: q.Get("search"), Owner: q.Get("owner"),
		Archived: repolist.ParseMode(q.Get("archived")), Forks: repolist.ParseMode(q.Get("forks")),
		Limit: limit, Offset: offset,
	}
}

func isTruthy(v string) bool { return v == "1" || v == "true" }

// handleListGitRepos handles GET /api/v1/git/repos: repositories searched,
// filtered and paged on the server from the per-connection caches. Without
// provider it merges every connected account of every provider; connection
// narrows it to one. AbilityReadSensitive.
func (rt *Router) handleListGitRepos(w http.ResponseWriter, r *http.Request) {
	provider := r.URL.Query().Get("provider")
	providers := append([]string{gitProviderGitHub}, connectionGitProviders...)
	switch {
	case provider == "":
	case provider == gitProviderGitHub || validGitProvider(provider):
		providers = []string{provider}
	default:
		writeError(w, http.StatusBadRequest, "provider must be one of github, "+strings.Join(connectionGitProviders, ", "))
		return
	}
	refresh := isTruthy(r.URL.Query().Get("refresh"))

	var accts []repoListAccount
	for _, p := range providers {
		if msg, missing := rt.gitProviderSecretsMissing(p); missing {
			if provider != "" {
				writeError(w, http.StatusNotImplemented, msg)
				return
			}
			continue
		}
		got, err := rt.loadGitRepos(r.Context(), p, refresh)
		if err != nil {
			var connErr *repoListConnError
			if provider == "" && errors.As(err, &connErr) {
				continue
			}
			rt.writeGitRepoListError(w, p, err)
			return
		}
		accts = append(accts, got...)
	}
	if conn := r.URL.Query().Get("connection"); conn != "" {
		kept := accts[:0]
		for _, a := range accts {
			if a.ConnectionID == conn {
				kept = append(kept, a)
			}
		}
		accts = kept
	}
	flat := flattenRepoAccounts(accts)
	if provider != "" && flat.Succeeded == 0 && flat.FirstErr != nil {
		rt.writeGitRepoListError(w, provider, flat.FirstErr)
		return
	}

	page := repolist.Apply(flat.Repos, parseRepoQuery(r))
	res := gitRepoListResource{
		Provider: provider, Items: page.Items, Total: page.Total, Loaded: len(flat.Repos),
		Limit: page.Limit, Offset: page.Offset, HasMore: page.NextOffset >= 0,
		Truncated: flat.Truncated, Cap: rt.repoCache.Config().MaxItems,
		CachedAt: flat.CachedAt.UTC(), Stale: flat.Stale, Refreshing: flat.Refreshing,
		Owners: page.Owners, Errors: flat.Errors, Connections: connectionFacets(accts),
	}
	if page.NextOffset >= 0 {
		next := page.NextOffset
		res.NextOffset = &next
	}
	if rl := flat.RateLimit; rl != nil {
		res.RateLimit = &gitRepoRateLimitResource{NextCheckAt: rl.NextCheck.UTC(), RetryAfterSeconds: max(int(time.Until(rl.NextCheck).Seconds())+1, 1)}
	}
	writeJSON(w, http.StatusOK, res)
}

// gitRepoConnectionFacet is one account the list was built from, for the picker's account filter.
type gitRepoConnectionFacet struct {
	ConnectionID string `json:"connection_id"`
	Provider     string `json:"provider"`
	Label        string `json:"label"`
	Account      string `json:"account,omitempty"`
	Loaded       int    `json:"loaded"`
	Failed       bool   `json:"failed"`
}

func connectionFacets(accts []repoListAccount) []gitRepoConnectionFacet {
	out := make([]gitRepoConnectionFacet, 0, len(accts))
	for _, a := range accts {
		out = append(out, gitRepoConnectionFacet{
			ConnectionID: a.ConnectionID, Provider: a.Provider, Label: a.ConnectionLabel, Account: a.Account,
			Loaded: len(a.Result.Repos), Failed: a.Err != nil,
		})
	}
	return out
}
