package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/attention"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	envAttentionTokenWindow = "APP_ATTENTION_TOKEN_WINDOW" //nolint:gosec // env var name
	envAttentionBackupGrace = "APP_ATTENTION_BACKUP_GRACE" //nolint:gosec // env var name

	defaultAttentionTokenWindow = 7 * 24 * time.Hour

	attentionBackupLookback = 20
)

type attentionFeedResponse struct {
	Items []attention.Item `json:"items"`
}

// handleAttentionFeed handles GET /api/v1/attention/feed: items from sources
// that need a store query, limited to what the caller's abilities and
// resource scope may read. Read-only and free of codes and secrets.
func (rt *Router) handleAttentionFeed(w http.ResponseWriter, r *http.Request) {
	abilities, err := rt.callerAbilities(r)
	if err != nil {
		rt.internalError(w, "api: attention feed: resolve caller abilities failed", err)
		return
	}
	now := time.Now()
	items := []attention.Item{}
	items = append(items, rt.tokenAttentionItems(r, abilities, now)...)
	items = append(items, rt.databaseAttentionItems(r, abilities, now)...)
	items = append(items, rt.inviteAttentionItems(r, abilities, now)...)
	items = append(items, rt.signInAttentionItems(r, now)...)
	items = append(items, rt.loginAnomalyItems(r, abilities, now)...)
	items = append(items, rt.tokenHygieneItems(r, abilities, now)...)
	items = append(items, rt.flaggedAccountItems(r, abilities)...)
	writeJSON(w, http.StatusOK, attentionFeedResponse{Items: items})
}

func feedItem(severity, kind, subject, detail string, params map[string]string) attention.Item {
	it := attention.NewItem(severity, kind, subject, detail)
	it.Params = params
	return it
}

func (rt *Router) tokenAttentionItems(r *http.Request, abilities []string, now time.Time) []attention.Item {
	admin := hasAbility(abilities, AbilityRoot)
	callerID, isSession := rt.currentSessionUserID(r)
	if !isSession && !admin {
		return nil
	}
	tokens, err := rt.libraryListTokens(r.Context(), callerID, admin)
	if err != nil {
		rt.logger.Warn("api: attention feed: list tokens failed", slog.String("error", err.Error()))
		return nil
	}
	window := envDuration(envAttentionTokenWindow, defaultAttentionTokenWindow)
	var items []attention.Item
	for _, t := range tokens {
		if t.RevokedAt != nil || t.ExpiresAt == nil {
			continue
		}
		left := t.ExpiresAt.Sub(now)
		params := map[string]string{"name": t.Name, "at": t.ExpiresAt.UTC().Format(time.RFC3339)}
		switch {
		case left <= 0 && -left <= window:
			it := feedItem(attention.Warning, attention.KindTokenExpired, t.Name, "expired "+t.ExpiresAt.UTC().Format("2006-01-02")+", requests using it are rejected", params)
			it.ID += ":" + t.ID
			items = append(items, it)
		case left > 0 && left <= window:
			it := feedItem(attention.Warning, attention.KindTokenExpiring, t.Name, "expires "+t.ExpiresAt.UTC().Format("2006-01-02")+", anything using it will stop working", params)
			it.ID += ":" + t.ID
			items = append(items, it)
		}
	}
	return items
}

func (rt *Router) databaseAttentionItems(r *http.Request, abilities []string, now time.Time) []attention.Item {
	if !hasAbility(abilities, AbilityRead) {
		return nil
	}
	ctx := r.Context()
	dbs, err := rt.databases.ListDesiredDatabases(ctx)
	if err != nil {
		rt.logger.Warn("api: attention feed: list databases failed", slog.String("error", err.Error()))
		return nil
	}
	canSee, err := rt.databaseVisibilityFilter(r)
	if err != nil {
		rt.logger.Warn("api: attention feed: database visibility failed", slog.String("error", err.Error()))
		return nil
	}
	imports := rt.dataImportsByName(ctx)
	grace := envDuration(envAttentionBackupGrace, alerting.DefaultBackupMissingGracePeriod)
	var items []attention.Item
	for _, d := range dbs {
		if !canSee(d.Name) {
			continue
		}
		if imp, ok := imports[d.Name]; ok {
			if it, ok := dataCopyItem(d.Name, imp, now); ok {
				items = append(items, it)
			}
		}
		if it, ok := rt.backupOverdueItem(ctx, d, grace, now); ok {
			items = append(items, it)
		}
	}
	return items
}

func (rt *Router) dataImportsByName(ctx context.Context) map[string]store.DatabaseDataImport {
	rows, err := rt.dataImports.ListDatabaseDataImports(ctx)
	if err != nil {
		rt.logger.Warn("api: attention feed: list data copies failed", slog.String("error", err.Error()))
		return nil
	}
	out := make(map[string]store.DatabaseDataImport, len(rows))
	for _, row := range rows {
		out[row.DatabaseName] = row
	}
	return out
}

func dataCopyItem(name string, imp store.DatabaseDataImport, now time.Time) (attention.Item, bool) {
	switch {
	case imp.Status == store.DataImportFailed:
		detail := "the last copy of live data failed"
		if imp.Reason != "" {
			detail += ": " + imp.Reason
		}
		return feedItem(attention.Warning, attention.KindDataCopy, name, detail, map[string]string{"state": store.DataImportFailed, "reason": imp.Reason}), true
	case imp.Status == store.DataImportCopying && !imp.StartedAt.IsZero() && now.Sub(imp.StartedAt) > copyTimeout():
		detail := "a data copy has been running since " + imp.StartedAt.UTC().Format(time.RFC3339) + " and looks stalled"
		return feedItem(attention.Warning, attention.KindDataCopy, name, detail, map[string]string{"state": "stalled"}), true
	}
	return attention.Item{}, false
}

func (rt *Router) backupOverdueItem(ctx context.Context, d store.DesiredDatabase, grace time.Duration, now time.Time) (attention.Item, bool) {
	if d.BackupSchedule == "" || d.Suspended {
		return attention.Item{}, false
	}
	history, err := rt.backupHistory.ListBackupHistory(ctx, d.Name, attentionBackupLookback, nil)
	if err != nil || len(history) == 0 {
		return attention.Item{}, false
	}
	overdue, anchor, ok, err := alerting.BackupOverdue(history, d.BackupSchedule, grace, now)
	if err != nil || !overdue {
		return attention.Item{}, false
	}
	detail := "no successful backup since " + anchor.UTC().Format(time.RFC3339) + ", later than its schedule expects"
	if !ok {
		detail = "every recent backup attempt failed, the oldest was " + anchor.UTC().Format(time.RFC3339)
	}
	return feedItem(attention.Warning, attention.KindBackupOverdue, d.Name, detail, map[string]string{"since": anchor.UTC().Format(time.RFC3339)}), true
}

func (rt *Router) inviteAttentionItems(r *http.Request, abilities []string, now time.Time) []attention.Item {
	if !hasAbility(abilities, AbilityRead) {
		return nil
	}
	invites, err := rt.invites.ListPendingInvites(r.Context())
	if err != nil {
		rt.logger.Warn("api: attention feed: list invites failed", slog.String("error", err.Error()))
		return nil
	}
	callerID, _ := rt.currentSessionUserID(r)
	admin := hasAbility(abilities, AbilityRoot)
	pending, expired := 0, 0
	for _, inv := range invites {
		if !admin && inv.CreatedBy != callerID {
			continue
		}
		if now.After(inv.ExpiresAt) {
			expired++
		} else {
			pending++
		}
	}
	if pending+expired == 0 {
		return nil
	}
	severity := attention.Info
	if expired > 0 {
		severity = attention.Warning
	}
	detail := fmt.Sprintf("%d waiting to be accepted, %d expired without being accepted", pending, expired)
	params := map[string]string{"pending": strconv.Itoa(pending), "expired": strconv.Itoa(expired)}
	return []attention.Item{feedItem(severity, attention.KindInvites, "invitations", detail, params)}
}
