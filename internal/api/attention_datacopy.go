package api

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/attention"
	"github.com/GLINCKER/levelrail/internal/datamigrate"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	envAttentionCopyFailureDays = "APP_ATTENTION_COPY_FAILURE_DAYS" //nolint:gosec // env var name
	defaultCopyFailureDays      = 14
	copyItemKeyParts            = 3
)

func copyFailureWindow() time.Duration {
	if n, err := strconv.Atoi(os.Getenv(envAttentionCopyFailureDays)); err == nil && n > 0 {
		return time.Duration(n) * 24 * time.Hour
	}
	return defaultCopyFailureDays * 24 * time.Hour
}

// copyAttemptKey is the stable id of one copy attempt, so a dismissal or
// snooze applies to that attempt only and a retry that fails again reappears.
func copyAttemptKey(name string, imp store.DatabaseDataImport) string {
	return attention.KindDataCopy + ":" + name + ":" + strconv.FormatInt(imp.StartedAt.Unix(), 10)
}

// copyAttemptEnd is when the attempt stopped, falling back to its start.
func copyAttemptEnd(imp store.DatabaseDataImport) time.Time {
	if !imp.FinishedAt.IsZero() {
		return imp.FinishedAt
	}
	return imp.StartedAt
}

// A failure stops counting once a later restore succeeded or it is older than maxAge; a running retry is not an incident.
func dataCopyAttentionItem(name string, imp store.DatabaseDataImport, now time.Time, maxAge time.Duration, lastRestore time.Time) (attention.Item, bool) {
	switch {
	case imp.Status == store.DataImportFailed:
		end := copyAttemptEnd(imp)
		if !end.IsZero() {
			if now.Sub(end) > maxAge || lastRestore.After(end) {
				return attention.Item{}, false
			}
		}
		info := datamigrate.Humanize(imp.Reason, imp.SourceHost, imp.SourcePort)
		detail := info.Summary + " " + info.Hint
		it := feedItem(attention.Warning, attention.KindDataCopy, name, detail, map[string]string{
			"state":     store.DataImportFailed,
			"reason":    info.Summary,
			"hint":      info.Hint,
			"code":      info.Code,
			"technical": info.Technical,
			"attempt":   strconv.FormatInt(imp.StartedAt.Unix(), 10),
		})
		it.ID = copyAttemptKey(name, imp)
		return it, true
	case imp.Status == store.DataImportCopying && !imp.StartedAt.IsZero() && now.Sub(imp.StartedAt) > copyTimeout():
		detail := "a data copy has been running since " + imp.StartedAt.UTC().Format(time.RFC3339) + " and looks stalled"
		it := feedItem(attention.Warning, attention.KindDataCopy, name, detail, map[string]string{"state": "stalled"})
		it.ID = copyAttemptKey(name, imp)
		return it, true
	}
	return attention.Item{}, false
}

// latestSuccessfulRestore returns when the newest successful restore into
// name finished, or the zero time when there is none.
func (rt *Router) latestSuccessfulRestore(ctx context.Context, name string) time.Time {
	if rt.restoreHistory == nil {
		return time.Time{}
	}
	rows, err := rt.restoreHistory.ListRestoreHistory(ctx, name)
	if err != nil {
		rt.logger.Warn("api: attention feed: list restore history failed", slog.String("database", name), slog.String("error", err.Error()))
		return time.Time{}
	}
	var latest time.Time
	for _, row := range rows {
		if row.Status != store.BackupStatusSucceeded {
			continue
		}
		at, err := time.Parse(time.RFC3339, row.FinishedAt)
		if err == nil && at.After(latest) {
			latest = at
		}
	}
	return latest
}

// attentionDismissals returns the item keys the session user dismissed.
func (rt *Router) attentionDismissals(r *http.Request) map[string]bool {
	userID, ok := rt.currentSessionUserID(r)
	if !ok {
		return nil
	}
	l, ok := rt.auditLog.(dismissalLister)
	if !ok {
		return nil
	}
	out, err := l.ListAttentionDismissals(r.Context(), userID)
	if err != nil {
		rt.logger.Warn("api: attention feed: list dismissals failed", slog.String("error", err.Error()))
		return nil
	}
	return out
}

// dismissDataCopyItem clears one failed copy attempt for the caller. It
// refuses a key that is not the database's current failed attempt.
func (rt *Router) dismissDataCopyItem(w http.ResponseWriter, r *http.Request, userID, key string) {
	parts := strings.Split(key, deviceLoginItemKeySep)
	if len(parts) != copyItemKeyParts {
		writeError(w, http.StatusBadRequest, "item_key is not a dismissible item")
		return
	}
	name := parts[1]
	canSee, err := rt.databaseVisibilityFilter(r)
	if err != nil {
		rt.internalError(w, "api: dismiss data copy: database visibility failed", err)
		return
	}
	if !canSee(name) {
		writeError(w, http.StatusNotFound, "no such item")
		return
	}
	imp, found, err := rt.dataImports.GetDatabaseDataImport(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: dismiss data copy: load copy failed", err, slog.String("database", name))
		return
	}
	if !found || copyAttemptKey(name, imp) != key {
		writeError(w, http.StatusNotFound, "no such item")
		return
	}
	if imp.Status == store.DataImportCopying {
		writeError(w, http.StatusConflict, "item is not in a dismissible state")
		return
	}
	d, ok := rt.auditLog.(interface {
		DismissAttentionItem(ctx context.Context, userID, itemKey string, at time.Time, entry *store.AuditEntry) (bool, error)
	})
	if !ok {
		writeError(w, http.StatusNotImplemented, "dismissal is not available")
		return
	}
	if _, err := d.DismissAttentionItem(r.Context(), userID, key, time.Now(), nil); err != nil {
		rt.internalError(w, "api: dismiss data copy failed", err, slog.String("database", name))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
