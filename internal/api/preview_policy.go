package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/webhook"
)

// PreviewLimits caps simultaneous live preview environments. Zero means
// unlimited for that scope.
type PreviewLimits struct {
	MaxPerApp int
	MaxTotal  int
}

// WithPreviewLimits sets the per-app and global preview caps
// (cmd/levelrail reads APP_PREVIEW_ENV_MAX_PER_APP and APP_PREVIEW_ENV_MAX_TOTAL).
func WithPreviewLimits(l PreviewLimits) Option {
	return func(rt *Router) { rt.previewLimits = l }
}

type previewEviction struct {
	victim store.PreviewEnvironment
	reason string
}

// selectEvictions returns the oldest live previews to remove so one more
// preview for appName fits under l. selfID is excluded from the counts.
func selectEvictions(all []store.PreviewEnvironment, appName, selfID string, l PreviewLimits) []previewEviction {
	live := make([]store.PreviewEnvironment, 0, len(all))
	for _, p := range all {
		if p.ID != selfID && p.Occupies() {
			live = append(live, p)
		}
	}
	sort.SliceStable(live, func(i, j int) bool {
		if live[i].CreatedAt != live[j].CreatedAt {
			return live[i].CreatedAt < live[j].CreatedAt
		}
		return live[i].ID < live[j].ID
	})

	var out []previewEviction
	evicted := map[string]bool{}
	if l.MaxPerApp > 0 {
		var mine []store.PreviewEnvironment
		for _, p := range live {
			if p.AppName == appName {
				mine = append(mine, p)
			}
		}
		for len(mine) >= l.MaxPerApp {
			out = append(out, previewEviction{victim: mine[0], reason: fmt.Sprintf("this app already has %d previews (limit %d)", len(mine), l.MaxPerApp)})
			evicted[mine[0].ID] = true
			mine = mine[1:]
		}
	}
	if l.MaxTotal > 0 {
		var rest []store.PreviewEnvironment
		for _, p := range live {
			if !evicted[p.ID] {
				rest = append(rest, p)
			}
		}
		for len(rest) >= l.MaxTotal {
			out = append(out, previewEviction{victim: rest[0], reason: fmt.Sprintf("the platform already has %d previews (limit %d)", len(rest), l.MaxTotal)})
			rest = rest[1:]
		}
	}
	return out
}

// admitPreview makes room for one more live preview. It returns a reason
// when the app's policy is reject and the cap is full; otherwise it evicts
// the oldest previews and returns "". Callers hold previewAdmitMu.
func (rt *Router) admitPreview(ctx context.Context, appName string, existing *store.PreviewEnvironment, settings store.PreviewAppSettings) (string, error) {
	if existing != nil && existing.Occupies() {
		return "", nil
	}
	if rt.previewLimits.MaxPerApp <= 0 && rt.previewLimits.MaxTotal <= 0 {
		return "", nil
	}
	all, err := rt.previewEnvironments.ListPreviewEnvironments(ctx)
	if err != nil {
		return "", fmt.Errorf("list previews for limit check: %w", err)
	}
	selfID := ""
	if existing != nil {
		selfID = existing.ID
	}
	plan := selectEvictions(all, appName, selfID, rt.previewLimits)
	if len(plan) == 0 {
		return "", nil
	}
	if settings.OnLimit == store.PreviewOnLimitReject {
		return "Preview limit reached: " + plan[0].reason + ".", nil
	}
	for _, ev := range plan {
		status, msg := rt.teardownPreviewRecordReason(ctx, ev.victim, "Evicted to make room for a newer preview: "+ev.reason+".")
		rt.logger.Info("api: evicted oldest preview at the limit",
			slog.String("app_name", ev.victim.AppName), slog.Int("pr_number", ev.victim.PRNumber),
			slog.Int("teardown_status", status), slog.String("teardown_message", msg))
	}
	return "", nil
}

func (rt *Router) previewSettings(ctx context.Context, appName string) store.PreviewAppSettings {
	s, err := rt.previewEnvironments.GetPreviewAppSettings(ctx, appName)
	if err != nil {
		rt.logger.Error("api: load preview settings failed, using defaults", slog.String("error", err.Error()), slog.String("app_name", appName))
		return store.DefaultPreviewAppSettings(appName)
	}
	return s
}

// previewTTLFor is appName's effective TTL: its own override in hours, else
// the control plane default.
func (rt *Router) previewTTLFor(s store.PreviewAppSettings) time.Duration {
	if s.TTLHours > 0 {
		return time.Duration(s.TTLHours) * time.Hour
	}
	return rt.effectivePreviewTTL()
}

// upsertPreviewRow saves next as a new row, or merges it into existing.
func (rt *Router) upsertPreviewRow(ctx context.Context, existing *store.PreviewEnvironment, next store.PreviewEnvironment) (*store.PreviewEnvironment, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if existing == nil {
		id, err := store.NewPreviewEnvironmentID()
		if err != nil {
			return nil, fmt.Errorf("mint preview id: %w", err)
		}
		next.ID, next.CreatedAt, next.UpdatedAt = id, now, now
		if err := rt.previewEnvironments.SavePreviewEnvironment(ctx, next); err != nil {
			return nil, fmt.Errorf("save preview: %w", err)
		}
		return &next, nil
	}
	merged := *existing
	merged.Branch, merged.HeadSHA, merged.Status, merged.StatusReason = next.Branch, next.HeadSHA, next.Status, next.StatusReason
	merged.IsFork, merged.HeadRepo, merged.UpdatedAt = next.IsFork, next.HeadRepo, now
	if err := rt.previewEnvironments.UpdatePreviewEnvironment(ctx, merged); err != nil {
		return nil, fmt.Errorf("update preview: %w", err)
	}
	return &merged, nil
}

// beginPreview runs the gates that decide whether a pull request event may
// deploy: fork approval, then the concurrency cap. On success it returns the
// row already marked deploying. When gated it returns done=true with the
// webhook response to send.
func (rt *Router) beginPreview(ctx context.Context, appName string, gs store.GitSource, ev webhook.PullRequestEvent, approved bool) (preview *store.PreviewEnvironment, done bool, status int, message string) {
	rt.previewAdmitMu.Lock()
	defer rt.previewAdmitMu.Unlock()

	previewName := previewAppName(appName, ev.Number)
	existing, err := rt.previewEnvironments.GetPreviewEnvironmentByAppAndPR(ctx, appName, ev.Number)
	if errors.Is(err, store.ErrPreviewEnvironmentNotFound) {
		existing = nil
	} else if err != nil {
		rt.logger.Error("api: pull request webhook: load preview failed", slog.String("error", err.Error()), slog.String("app_name", appName), slog.Int("pr_number", ev.Number))
		return nil, true, http.StatusInternalServerError, "internal error\n"
	}

	settings := rt.previewSettings(ctx, appName)
	base := store.PreviewEnvironment{
		AppName: appName, PRNumber: ev.Number, PreviewAppID: previewName, Branch: ev.HeadRef, HeadSHA: ev.HeadSHA,
		Status: store.PreviewStatusDeploying, IsFork: ev.IsFork(), HeadRepo: ev.HeadRepoFullName,
	}

	if base.IsFork && !approved && !settings.AllowForkPreviews {
		held, status, message := rt.holdForkPreview(ctx, gs, existing, base)
		return held, true, status, message
	}

	reason, err := rt.admitPreview(ctx, appName, existing, settings)
	if err != nil {
		rt.logger.Error("api: pull request webhook: preview limit check failed", slog.String("error", err.Error()), slog.String("app_name", appName))
		return nil, true, http.StatusInternalServerError, "internal error\n"
	}
	if reason != "" {
		base.Status, base.StatusReason = store.PreviewStatusLimitReached, reason
		held, saveErr := rt.upsertPreviewRow(ctx, existing, base)
		if saveErr != nil {
			rt.logger.Error("api: pull request webhook: record rejected preview failed", slog.String("error", saveErr.Error()), slog.String("app_name", appName))
			return nil, true, http.StatusInternalServerError, "internal error\n"
		}
		rt.upsertPreviewComment(ctx, gs, held, previewCommentLimitReached, reason)
		return held, true, http.StatusOK, fmt.Sprintf("ignored: %s\n", reason)
	}

	row, err := rt.upsertPreviewRow(ctx, existing, base)
	if err != nil {
		rt.logger.Error("api: pull request webhook: save preview failed", slog.String("error", err.Error()), slog.String("app_name", appName))
		return nil, true, http.StatusInternalServerError, "internal error\n"
	}
	return row, false, 0, ""
}

const forkHoldDetail = "This pull request comes from a fork, so no preview was deployed and no secrets or environment variables were shared with it. " +
	"A maintainer can review the changes and approve a one-time preview from the app's Previews page in the dashboard or with the CLI."

// holdForkPreview records a fork pull request as awaiting approval instead
// of deploying it, and removes anything an earlier approval had deployed.
func (rt *Router) holdForkPreview(ctx context.Context, gs store.GitSource, existing *store.PreviewEnvironment, base store.PreviewEnvironment) (*store.PreviewEnvironment, int, string) {
	base.Status = store.PreviewStatusAwaitingApproval
	base.StatusReason = "Fork pull request from " + base.HeadRepo + " needs operator approval before it can deploy."
	if base.HeadRepo == "" {
		base.StatusReason = "Pull request from an unknown source repository needs operator approval before it can deploy."
	}

	if existing != nil && existing.Status != store.PreviewStatusAwaitingApproval && existing.Status != store.PreviewStatusLimitReached {
		if failed := rt.teardownPreviewApp(ctx, existing.PreviewAppID); len(failed) > 0 {
			rt.logger.Error("api: fork preview hold: remove earlier deployment failed", slog.Any("failed_resources", failed), slog.String("app_name", base.AppName))
		}
		failed := rt.teardownPreviewEphemeralDatabases(ctx, existing.ID)
		if len(failed) > 0 {
			rt.logger.Error("api: fork preview hold: remove earlier databases failed", slog.Any("failed_resources", failed), slog.String("app_name", base.AppName))
		}
	}

	held, err := rt.upsertPreviewRow(ctx, existing, base)
	if err != nil {
		rt.logger.Error("api: fork preview hold: record failed", slog.String("error", err.Error()), slog.String("app_name", base.AppName))
		return nil, http.StatusInternalServerError, "internal error\n"
	}
	rt.upsertPreviewComment(ctx, gs, held, previewCommentAwaitingApproval, forkHoldDetail)
	return held, http.StatusOK, fmt.Sprintf("ignored: fork pull request #%d needs operator approval\n", base.PRNumber)
}
