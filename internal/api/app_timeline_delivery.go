package api

import (
	"context"
	"log/slog"
	"strings"

	"github.com/GLINCKER/levelrail/internal/pipeline"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	timelineKindCommitStatus = "commit_status"
	timelineKindBinding      = "connection_bound"
	timelineBindingPrefix    = "cbind_"
	timelineShortSHA         = 7
)

type forgeStatusLister interface {
	ListForgeStatusReports(ctx context.Context, app string, cursor *store.AppEventCursor, limit int) ([]store.ForgeStatusReport, error)
}

type appBindingLister interface {
	ListConnectionBindingsByTarget(ctx context.Context, targetType, targetID string) ([]store.ConnectionBinding, error)
}

// deliveryTimelineItems adds forge commit statuses (pipeline runs and MR or PR
// previews) and connection bindings to the app timeline. A storage failure
// drops these rows instead of failing the whole timeline.
func (rt *Router) deliveryTimelineItems(ctx context.Context, app string, cursor *store.AppEventCursor, limit int) []timelineItem {
	var items []timelineItem
	if s, ok := rt.apps.(forgeStatusLister); ok {
		list, err := s.ListForgeStatusReports(ctx, app, cursor, limit)
		if err != nil {
			rt.logger.Warn("api: timeline: list forge statuses failed", slog.String("error", err.Error()), slog.String("app_name", app))
		}
		for _, r := range list {
			items = append(items, forgeStatusTimelineItem(r))
		}
	}
	if s, ok := rt.apps.(appBindingLister); ok {
		list, err := s.ListConnectionBindingsByTarget(ctx, "app", app)
		if err != nil {
			rt.logger.Warn("api: timeline: list bindings failed", slog.String("error", err.Error()), slog.String("app_name", app))
		}
		for _, b := range list {
			it := bindingTimelineItem(b)
			if timelineBefore(it.At, it.ID, cursor) {
				items = append(items, it)
			}
		}
	}
	return items
}

func forgeStatusTimelineItem(r store.ForgeStatusReport) timelineItem {
	status := timelineInfo
	switch {
	case r.Error != "":
		status = timelineFailed
	case r.State == pipeline.ReportPending:
		status = timelineInProgress
	case r.State == pipeline.ReportSuccess:
		status = timelineSucceeded
	case r.State == pipeline.ReportFailure, r.State == pipeline.ReportError:
		status = timelineFailed
	}
	title := r.Context
	if title == "" {
		title = "commit status"
	}
	title += " " + r.State + " on " + r.SHA[:min(len(r.SHA), timelineShortSHA)]
	if r.Error != "" {
		title = r.Context + " status post failed on " + r.SHA[:min(len(r.SHA), timelineShortSHA)]
	}
	var parts []string
	if r.Provider != "" {
		parts = append(parts, r.Provider)
	}
	if r.Description != "" {
		parts = append(parts, r.Description)
	}
	if r.Error != "" {
		parts = append(parts, r.Error)
	}
	actor := "pipeline"
	if r.Source == store.ForgeStatusSourcePreview {
		actor = "preview"
	}
	return timelineItem{
		ID: r.ID, At: r.CreatedAt.UTC(), Kind: timelineKindCommitStatus, Status: status, Actor: actor,
		Title: strings.TrimSpace(title), Detail: strings.Join(parts, "; "), URL: r.TargetURL,
		Ref: &timelineRef{Type: "commit_status", ID: r.ID},
	}
}

func bindingTimelineItem(b store.ConnectionBinding) timelineItem {
	return timelineItem{
		ID: timelineBindingPrefix + b.ID, At: b.CreatedAt.UTC(), Kind: timelineKindBinding, Status: timelineInfo,
		Actor: b.CreatedBy, Title: "Connection " + b.Ref + " bound as " + b.Role,
		Ref: &timelineRef{Type: "connection_binding", ID: b.ID},
	}
}
