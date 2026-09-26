package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// databaseVisibilityFilter reports whether the caller can read a database.
func (rt *Router) databaseVisibilityFilter(r *http.Request) (func(name string) bool, error) {
	principalType, principalID, abilities, err := rt.callerPrincipal(r)
	if err != nil {
		return nil, fmt.Errorf("resolve caller: %w", err)
	}
	policies, err := rt.policies.ListPoliciesForPrincipal(r.Context(), principalType, principalID)
	if err != nil {
		return nil, fmt.Errorf("list caller policies: %w", err)
	}
	return func(name string) bool {
		return authorizeResource(abilities, policies, AbilityRead, "database:"+name)
	}, nil
}

// backupVisible hides a backup only when it belongs to an app or database
// the caller cannot read.
func backupVisible(h store.BackupHistory, canSeeApp, canSeeDB func(string) bool) bool {
	if h.ServiceName != "" && !canSeeApp(h.ServiceName) {
		return false
	}
	return h.DatabaseName == "" || canSeeDB(h.DatabaseName)
}

// domainOwners maps each domain to the apps that serve it.
func (rt *Router) domainOwners(ctx context.Context) (map[string][]string, error) {
	owners := map[string][]string{}
	key := strings.ToLower
	domains, err := rt.domains.ListServiceDomains(ctx)
	if err != nil {
		return nil, fmt.Errorf("list service domains: %w", err)
	}
	for _, d := range domains {
		owners[key(d.Domain)] = append(owners[key(d.Domain)], d.ServiceName)
	}
	sites, err := rt.staticSites.ListStaticSites(ctx)
	if err != nil {
		return nil, fmt.Errorf("list static sites: %w", err)
	}
	for _, s := range sites {
		for _, d := range s.Domains {
			owners[key(d)] = append(owners[key(d)], s.Name)
		}
	}
	return owners, nil
}

// pageBackups fills up to limit visible backups. It re-reads the same cursor
// with a doubled window until the page is full or history is exhausted, so
// hidden rows never truncate a page and same-second rows never straddle a
// batch boundary.
func (rt *Router) pageBackups(ctx context.Context, limit int, before *time.Time, canSeeApp, canSeeDB func(string) bool) ([]store.BackupHistory, error) {
	for window := limit; ; window *= 2 {
		rows, err := rt.backupHistory.ListAllBackupHistory(ctx, window, before)
		if err != nil {
			return nil, fmt.Errorf("list backup history: %w", err)
		}
		var out []store.BackupHistory
		for _, h := range rows {
			if backupVisible(h, canSeeApp, canSeeDB) {
				out = append(out, h)
				if len(out) == limit {
					return out, nil
				}
			}
		}
		if len(rows) < window {
			return out, nil
		}
	}
}

// certVisible hides a certificate only when its domain or a SAN belongs to
// an app the caller cannot read; certificates matching no app stay visible.
func certVisible(domain string, sans []string, owners map[string][]string, canSee func(string) bool) bool {
	for _, d := range append([]string{domain}, sans...) {
		for _, app := range owners[strings.ToLower(d)] {
			if !canSee(app) {
				return false
			}
		}
	}
	return true
}
