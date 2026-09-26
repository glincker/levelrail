package api

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/gitprovider"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Preview comment states, rendered into the single status comment kept on a
// pull request.
const (
	previewCommentBuilding         = "building"
	previewCommentReady            = "ready"
	previewCommentFailed           = "failed"
	previewCommentRemoved          = "removed"
	previewCommentAwaitingApproval = "awaiting approval"
	previewCommentLimitReached     = "not deployed"
)

const previewCommentDetailMax = 200

// prCommenter is the provider-neutral comment surface the upsert needs.
type prCommenter struct {
	list   func(ctx context.Context) ([]gitprovider.Comment, error)
	create func(ctx context.Context, body string) (int64, error)
	update func(ctx context.Context, id int64, body string) error
}

func previewCommentMarker(previewAppID string) string {
	return fmt.Sprintf("<!-- preview-env:%s -->", previewAppID)
}

// shortPreviewDetail flattens detail to one bounded line without backticks
// or live @mentions, since it can carry build error text.
func shortPreviewDetail(detail string) string {
	detail = strings.Join(strings.Fields(detail), " ")
	detail = strings.ReplaceAll(detail, "`", "'")
	detail = strings.ReplaceAll(detail, "@", "@\u200b")
	if len(detail) > previewCommentDetailMax {
		detail = detail[:previewCommentDetailMax-3] + "..."
	}
	return detail
}

func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// renderPreviewComment builds the comment body. The hidden marker lets a
// later call find and edit the same comment.
func renderPreviewComment(p store.PreviewEnvironment, state, detail string, now time.Time) string {
	var sb strings.Builder
	sb.WriteString(previewCommentMarker(p.PreviewAppID))
	sb.WriteString("\n### Preview environment: ")
	sb.WriteString(state)
	sb.WriteString("\n\n- **Status:** ")
	sb.WriteString(state)
	if state == previewCommentReady && p.Domain != "" {
		fmt.Fprintf(&sb, "\n- **URL:** https://%s", p.Domain)
	}
	if p.HeadSHA != "" {
		fmt.Fprintf(&sb, "\n- **Commit:** `%s`", shortSHA(p.HeadSHA))
	}
	fmt.Fprintf(&sb, "\n- **Last updated:** %s", now.UTC().Format("2006-01-02 15:04:05 UTC"))
	if detail = shortPreviewDetail(detail); detail != "" {
		sb.WriteString("\n\n")
		sb.WriteString(detail)
	}
	sb.WriteString("\n")
	return sb.String()
}

// upsertPreviewComment creates or edits the pull request's single status
// comment. It never returns an error: a failed comment must not fail a deploy.
func (rt *Router) upsertPreviewComment(ctx context.Context, gs store.GitSource, p *store.PreviewEnvironment, state, detail string) {
	c := rt.previewCommenter(ctx, gs, *p)
	if c == nil {
		return
	}
	body := renderPreviewComment(*p, state, detail, time.Now())
	marker := previewCommentMarker(p.PreviewAppID)
	log := rt.logger.With(slog.String("app_name", p.AppName), slog.Int("pr_number", p.PRNumber), slog.String("preview_id", p.ID))

	if p.CommentID != 0 {
		err := c.update(ctx, p.CommentID, body)
		if err == nil {
			return
		}
		if !gitprovider.IsNotFound(err) {
			log.Warn("api: update preview pr comment failed", slog.String("error", err.Error()))
			return
		}
	}

	existing, err := c.list(ctx)
	if err != nil {
		log.Warn("api: list pr comments for preview failed", slog.String("error", err.Error()))
		return
	}
	id := int64(0)
	for _, cm := range existing {
		if strings.Contains(cm.Body, marker) {
			id = cm.ID
			break
		}
	}
	if id != 0 {
		if err := c.update(ctx, id, body); err != nil {
			log.Warn("api: update preview pr comment failed", slog.String("error", err.Error()))
			return
		}
	} else if id, err = c.create(ctx, body); err != nil {
		log.Warn("api: create preview pr comment failed", slog.String("error", err.Error()))
		return
	}

	p.CommentID = id
	if err := rt.previewEnvironments.SetPreviewEnvironmentCommentID(ctx, p.ID, id); err != nil {
		log.Warn("api: remember preview pr comment id failed", slog.String("error", err.Error()))
	}
}
