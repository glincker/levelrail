package preview

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
)

// WithMetaFetcher overrides the metadata tier's HTTP client, mainly for tests.
func WithMetaFetcher(f MetaFetcher) Option { return func(m *Manager) { m.fetcher = f } }

// processMeta is the metadata tier: no browser and no image pull, one bounded
// GET of the app's page, then its og:image if it has a usable one, else a card.
func (m *Manager) processMeta(ctx context.Context, settings AppSettings, j job) {
	t, err := m.deps.Resolver.ResolveMeta(ctx, j.app, j.image)
	var skip *SkipError
	switch {
	case errors.As(err, &skip):
		if skip.DeploymentID != "" {
			m.commit(ctx, captured{rec: Record{DeploymentID: skip.DeploymentID, App: j.app, Path: settings.Path, Status: StatusSkipped, Reason: skip.Reason, Detail: skip.Detail}})
		}
		return
	case err != nil:
		m.log.Warn("preview: resolve metadata target failed", slog.String("app", j.app), slog.String("error", err.Error()))
		return
	}
	if !j.force {
		if existing, gerr := m.deps.Store.GetPreviewRecord(ctx, t.DeploymentID); gerr == nil && existing != nil {
			return
		}
	}
	m.commit(ctx, m.captureMeta(ctx, settings, t))
}

func (m *Manager) captureMeta(ctx context.Context, s AppSettings, t MetaTarget) captured {
	rec := Record{DeploymentID: t.DeploymentID, App: s.App, Path: s.Path}
	fail := func(status, reason, detail string) captured {
		rec.Status, rec.Reason, rec.Detail = status, reason, detail
		return captured{rec: rec}
	}
	page, err := m.fetcher.FetchPage(ctx, t, s.Path)
	switch {
	case errors.Is(err, errOffsiteRedirect):
		return fail(StatusSkipped, ReasonRedirect, "the page redirects away from the app")
	case errors.Is(err, ErrNotHTML):
		return fail(StatusSkipped, ReasonNotHTML, "the page is not HTML")
	case err != nil:
		m.log.Warn("preview: fetch page failed", slog.String("app", s.App), slog.String("error", err.Error()))
		return fail(StatusFailed, ReasonUnreachable, "the app did not answer on its port")
	}
	rec.HTTPStatus = page.HTTPStatus
	shot := &ShotResult{HTTPStatus: page.HTTPStatus, FinalURL: page.Base.String()}
	if reason, detail := shot.Classify(s.Path); reason != "" {
		return fail(StatusSkipped, reason, detail)
	}

	card := CardMeta{Title: page.Meta.DisplayTitle(), Description: page.Meta.Description, ThemeColor: page.Meta.ThemeColor}
	metaJSON, _ := json.Marshal(card)
	rec.Meta = string(metaJSON)

	if ref := page.Meta.ImageRef(); ref != "" {
		if thumb := m.fetchSiteImage(ctx, s.App, t, page, ref); thumb != nil {
			rec.Status, rec.Source = StatusOK, SourceOGImage
			rec.Bytes, rec.Width, rec.Height = int64(len(thumb.JPEG)), thumb.Width, thumb.Height
			return captured{rec: rec, jpeg: thumb.JPEG}
		}
	}
	rec.Status, rec.Source, rec.Bytes = StatusOK, SourceCard, int64(len(rec.Meta))
	return captured{rec: rec}
}

func (m *Manager) fetchSiteImage(ctx context.Context, app string, t MetaTarget, page *PageResult, ref string) *Thumb {
	data, err := m.fetcher.FetchImage(ctx, t, page.Base, ref)
	if err != nil {
		m.log.Info("preview: site image not used", slog.String("app", app), slog.String("error", err.Error()))
		return nil
	}
	thumb, err := BuildThumbFit(data, m.cfg.ThumbWidth, m.cfg.ThumbHeight, m.cfg.MetaMinImgPx, m.cfg.Quality, m.cfg.MaxThumbKB<<10, m.cfg.BlankRatio)
	if err != nil {
		m.log.Info("preview: site image rejected", slog.String("app", app), slog.String("error", err.Error()))
		return nil
	}
	return thumb
}
