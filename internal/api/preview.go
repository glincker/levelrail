package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/preview"
	"github.com/GLINCKER/levelrail/internal/store"
)

const previewBodyLimit = 4 << 10

// PreviewService is the deploy preview surface; *preview.Manager satisfies it.
type PreviewService interface {
	Config() preview.Config
	Settings(ctx context.Context, app string) (preview.AppSettings, error)
	SaveSettings(ctx context.Context, app string, patch preview.SettingsPatch) (preview.AppSettings, error)
	List(ctx context.Context, app string) ([]preview.Record, error)
	OpenImage(ctx context.Context, app, deploymentID string) (string, preview.Record, error)
	Capture(ctx context.Context, app string) error
	Prune(ctx context.Context, app string, all bool) (preview.PruneResult, error)
	Status(ctx context.Context, app string) (preview.Summary, error)
	DeleteApp(ctx context.Context, app string) error
}

// SetPreview enables the deploy preview routes.
func (rt *Router) SetPreview(p PreviewService) { rt.preview = p }

type previewRecordResource struct {
	DeploymentID string            `json:"deployment_id"`
	Status       string            `json:"status"`
	Reason       string            `json:"reason,omitempty"`
	Detail       string            `json:"detail,omitempty"`
	HTTPStatus   int               `json:"http_status,omitempty"`
	Path         string            `json:"path"`
	Source       string            `json:"source"`
	Meta         *preview.CardMeta `json:"meta,omitempty"`
	Width        int               `json:"width,omitempty"`
	Height       int               `json:"height,omitempty"`
	Bytes        int64             `json:"bytes"`
	CapturedAt   time.Time         `json:"captured_at"`
	ImageURL     string            `json:"image_url,omitempty"`
}

type previewBrowserImage struct {
	Ref        string     `json:"ref"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

type previewStorage struct {
	AppBytes   int64 `json:"app_bytes"`
	AppCount   int   `json:"app_count"`
	TotalBytes int64 `json:"total_bytes"`
	TotalCount int   `json:"total_count"`
}

type previewStatusResource struct {
	App           string                 `json:"app"`
	Enabled       bool                   `json:"enabled"`
	Mode          string                 `json:"mode"`
	DefaultMode   string                 `json:"default_mode"`
	Path          string                 `json:"path"`
	WaitMS        int                    `json:"wait_ms"`
	ServerEnabled bool                   `json:"server_enabled"`
	Capturing     bool                   `json:"capturing"`
	Image         string                 `json:"image"`
	BrowserImage  *previewBrowserImage   `json:"browser_image,omitempty"`
	KeepPerApp    int                    `json:"keep_per_app"`
	TTLDays       int                    `json:"ttl_days"`
	MaxTotalMB    int64                  `json:"max_total_mb"`
	Storage       previewStorage         `json:"storage"`
	Latest        *previewRecordResource `json:"latest,omitempty"`
}

func previewImageURL(app string, rec preview.Record) string {
	if !rec.HasFile() {
		return ""
	}
	return "/api/v1/apps/" + url.PathEscape(app) + "/deployments/" + url.PathEscape(rec.DeploymentID) + "/preview?v=" + strconv.FormatInt(rec.CapturedAt.Unix(), 10)
}

func toPreviewRecordResource(app string, rec preview.Record) previewRecordResource {
	out := previewRecordResource{
		DeploymentID: rec.DeploymentID, Status: rec.Status, Reason: rec.Reason, Detail: rec.Detail,
		HTTPStatus: rec.HTTPStatus, Path: rec.Path, Source: rec.Source, Width: rec.Width, Height: rec.Height, Bytes: rec.Bytes,
		CapturedAt: rec.CapturedAt.UTC(), ImageURL: previewImageURL(app, rec),
	}
	if out.Source == "" {
		out.Source = preview.SourceScreenshot
	}
	if rec.Meta != "" {
		var meta preview.CardMeta
		if err := json.Unmarshal([]byte(rec.Meta), &meta); err == nil {
			out.Meta = &meta
		}
	}
	return out
}

func toPreviewStatusResource(app string, cfg preview.Config, sum preview.Summary) previewStatusResource {
	out := previewStatusResource{
		App: app, Enabled: sum.Settings.Enabled, Mode: string(sum.Settings.Mode), DefaultMode: string(cfg.DefaultMode), Path: sum.Settings.Path, WaitMS: sum.Settings.WaitMS,
		ServerEnabled: sum.GlobalOn, Capturing: sum.Capturing, Image: sum.Image,
		KeepPerApp: cfg.KeepPerApp, TTLDays: int(cfg.TTL / (24 * time.Hour)), MaxTotalMB: cfg.MaxTotalBytes >> 20,
		Storage: previewStorage{AppBytes: sum.App.Bytes, AppCount: sum.App.Count, TotalBytes: sum.Total.Bytes, TotalCount: sum.Total.Count},
	}
	if sum.BrowserImage.ID != "" {
		bi := &previewBrowserImage{Ref: sum.BrowserImage.Ref}
		if !sum.BrowserImage.LastUsed.IsZero() {
			t := sum.BrowserImage.LastUsed.UTC()
			bi.LastUsedAt = &t
		}
		out.BrowserImage = bi
	}
	if sum.Latest != nil {
		r := toPreviewRecordResource(app, *sum.Latest)
		out.Latest = &r
	}
	return out
}

func (rt *Router) previewReady(w http.ResponseWriter, r *http.Request) bool {
	if rt.preview == nil {
		writeError(w, http.StatusNotImplemented, "deploy previews are not configured on this control plane")
		return false
	}
	return rt.requireApp(w, r)
}

func (rt *Router) writePreviewStatus(w http.ResponseWriter, r *http.Request, status int) {
	app := r.PathValue("name")
	sum, err := rt.preview.Status(r.Context(), app)
	if err != nil {
		rt.internalError(w, "api: preview status failed", err, slog.String("name", app))
		return
	}
	writeJSON(w, status, toPreviewStatusResource(app, rt.preview.Config(), sum))
}

// handleGetPreview handles GET /api/v1/apps/{name}/preview.
func (rt *Router) handleGetPreview(w http.ResponseWriter, r *http.Request) {
	if !rt.previewReady(w, r) {
		return
	}
	rt.writePreviewStatus(w, r, http.StatusOK)
}

type previewSettingsRequest struct {
	Mode    *string `json:"mode"`
	Enabled *bool   `json:"enabled"`
	Path    *string `json:"path"`
	WaitMS  *int    `json:"wait_ms"`
}

// handlePutPreview handles PUT /api/v1/apps/{name}/preview.
func (rt *Router) handlePutPreview(w http.ResponseWriter, r *http.Request) {
	if !rt.previewReady(w, r) {
		return
	}
	var req previewSettingsRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, previewBodyLimit)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	app := r.PathValue("name")
	_, err := rt.preview.SaveSettings(r.Context(), app, preview.SettingsPatch{Mode: req.Mode, Enabled: req.Enabled, Path: req.Path, WaitMS: req.WaitMS})
	if errors.Is(err, preview.ErrInvalid) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err != nil {
		rt.internalError(w, "api: save preview settings failed", err, slog.String("name", app))
		return
	}
	rt.writePreviewStatus(w, r, http.StatusOK)
}

// handleCapturePreview handles POST /api/v1/apps/{name}/preview/capture.
func (rt *Router) handleCapturePreview(w http.ResponseWriter, r *http.Request) {
	if !rt.previewReady(w, r) {
		return
	}
	app := r.PathValue("name")
	err := rt.preview.Capture(r.Context(), app)
	if errors.Is(err, preview.ErrDisabled) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		rt.internalError(w, "api: queue preview capture failed", err, slog.String("name", app))
		return
	}
	rt.writePreviewStatus(w, r, http.StatusAccepted)
}

type previewPruneRequest struct {
	All bool `json:"all"`
}

type previewPruneResponse struct {
	Removed    int   `json:"removed"`
	FreedBytes int64 `json:"freed_bytes"`
}

// handlePrunePreview handles POST /api/v1/apps/{name}/preview/prune.
func (rt *Router) handlePrunePreview(w http.ResponseWriter, r *http.Request) {
	if !rt.previewReady(w, r) {
		return
	}
	var req previewPruneRequest
	if r.ContentLength != 0 {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, previewBodyLimit)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	app := r.PathValue("name")
	res, err := rt.preview.Prune(r.Context(), app, req.All)
	if err != nil {
		rt.internalError(w, "api: prune previews failed", err, slog.String("name", app))
		return
	}
	writeJSON(w, http.StatusOK, previewPruneResponse{Removed: res.Removed, FreedBytes: res.FreedBytes})
}

// handleListPreviews handles GET /api/v1/apps/{name}/preview/history.
func (rt *Router) handleListPreviews(w http.ResponseWriter, r *http.Request) {
	if !rt.previewReady(w, r) {
		return
	}
	app := r.PathValue("name")
	recs, err := rt.preview.List(r.Context(), app)
	if err != nil {
		rt.internalError(w, "api: list previews failed", err, slog.String("name", app))
		return
	}
	out := make([]previewRecordResource, 0, len(recs))
	for _, rec := range recs {
		out = append(out, toPreviewRecordResource(app, rec))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleGetPreviewImage handles GET /api/v1/apps/{name}/deployments/{id}/preview.
func (rt *Router) handleGetPreviewImage(w http.ResponseWriter, r *http.Request) {
	if !rt.previewReady(w, r) {
		return
	}
	app, id := r.PathValue("name"), r.PathValue("id")
	path, _, err := rt.preview.OpenImage(r.Context(), app, id)
	if errors.Is(err, preview.ErrNotFound) {
		writeError(w, http.StatusNotFound, "no preview for this deployment")
		return
	}
	if err != nil {
		rt.internalError(w, "api: open preview image failed", err, slog.String("name", app), slog.String("deployment_id", id))
		return
	}
	servePreviewImage(w, r, path)
}

const (
	previewCacheVersioned = "private, max-age=31536000, immutable"
	previewCacheDefault   = "private, max-age=86400, stale-while-revalidate=604800"
)

// servePreviewImage sends the thumbnail with a strong ETag, 304 and Range
// support. The ?v= URL changes with every capture, so a versioned request is
// immutable; the route is authenticated, hence always private.
func servePreviewImage(w http.ResponseWriter, r *http.Request, path string) {
	f, err := os.Open(path) //nolint:gosec // path comes from the preview FileStore, which validates app and deployment names
	if err != nil {
		writeError(w, http.StatusNotFound, "no preview for this deployment")
		return
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil || info.IsDir() {
		writeError(w, http.StatusNotFound, "no preview for this deployment")
		return
	}
	h := w.Header()
	h.Set("Content-Type", "image/jpeg")
	h.Set("ETag", `"`+strconv.FormatInt(info.Size(), 16)+"-"+strconv.FormatInt(info.ModTime().UnixNano(), 16)+`"`)
	if r.URL.Query().Get("v") != "" {
		h.Set("Cache-Control", previewCacheVersioned)
	} else {
		h.Set("Cache-Control", previewCacheDefault)
	}
	http.ServeContent(w, r, "", info.ModTime(), f)
}

// RenderableApp reports whether an app's kind has a page worth rendering:
// web and static only. API, worker, cron and database cards are type specific.
func RenderableApp(svc store.DesiredService) bool {
	kind, _ := classifyApp(kindInputFor(svc, false))
	return kind == appKindWeb || kind == appKindStatic
}

// previewCardResource is the list-entry thumbnail: an image URL when a file
// exists, card metadata otherwise, and State while a real render is pending.
type previewCardResource struct {
	ImageURL string            `json:"image_url,omitempty"`
	Card     *preview.CardMeta `json:"card,omitempty"`
	State    string            `json:"state,omitempty"`
	Reason   string            `json:"reason,omitempty"`
}

// previewLatestLister is optional on PreviewService so one list request can
// carry every card thumbnail without a query per app.
type previewLatestLister interface {
	LatestRenderable(ctx context.Context, apps []string) (map[string]preview.Record, error)
}

// previewCardStater is optional too: it adds the render state and queues a
// background render for an app that has none, never waiting for it.
type previewCardStater interface {
	CardStates(ctx context.Context, apps []string) (map[string]preview.CardState, error)
}

// listPreviewCards returns the latest thumbnail per app, empty when the
// service is absent or fails (cards then fall back to the placeholder). The
// render state is reported only for web and static apps.
func (rt *Router) listPreviewCards(ctx context.Context, svcs []store.DesiredService) map[string]previewCardResource {
	if rt.preview == nil || len(svcs) == 0 {
		return nil
	}
	apps := make([]string, 0, len(svcs))
	renderable := make(map[string]bool, len(svcs))
	for _, s := range svcs {
		apps = append(apps, s.Name)
		renderable[s.Name] = RenderableApp(s)
	}
	if cs, ok := rt.preview.(previewCardStater); ok {
		return rt.cardsFromStates(ctx, cs, apps, renderable)
	}
	l, ok := rt.preview.(previewLatestLister)
	if !ok {
		return nil
	}
	recs, err := l.LatestRenderable(ctx, apps)
	if err != nil {
		rt.logger.Warn("api: latest previews for app list failed", slog.String("error", err.Error()))
		return nil
	}
	out := make(map[string]previewCardResource, len(recs))
	for app, rec := range recs {
		res := toPreviewRecordResource(app, rec)
		if res.ImageURL == "" && res.Meta == nil {
			continue
		}
		out[app] = previewCardResource{ImageURL: res.ImageURL, Card: res.Meta}
	}
	return out
}

func (rt *Router) cardsFromStates(ctx context.Context, cs previewCardStater, apps []string, renderable map[string]bool) map[string]previewCardResource {
	// Only web and static apps are asked for: asking also queues a render.
	ask := make([]string, 0, len(apps))
	for _, a := range apps {
		if renderable[a] {
			ask = append(ask, a)
		}
	}
	states, err := cs.CardStates(ctx, ask)
	if err != nil {
		rt.logger.Warn("api: latest previews for app list failed", slog.String("error", err.Error()))
		return nil
	}
	out := make(map[string]previewCardResource, len(states))
	for app, st := range states {
		card := previewCardResource{State: st.State, Reason: st.Reason}
		if st.Record != nil {
			res := toPreviewRecordResource(app, *st.Record)
			card.ImageURL, card.Card = res.ImageURL, res.Meta
		}
		if card.ImageURL == "" && card.Card == nil && card.State == preview.StateNone {
			continue
		}
		out[app] = card
	}
	return out
}

// previewImageURLs maps deployment IDs of app to their thumbnail URL. An
// unconfigured or failing preview service yields an empty map.
func (rt *Router) previewImageURLs(ctx context.Context, app string) map[string]string {
	if rt.preview == nil {
		return nil
	}
	recs, err := rt.preview.List(ctx, app)
	if err != nil {
		rt.logger.Warn("api: list previews for deploy history failed", slog.String("error", err.Error()), slog.String("name", app))
		return nil
	}
	out := make(map[string]string, len(recs))
	for _, rec := range recs {
		if u := previewImageURL(app, rec); u != "" {
			out[rec.DeploymentID] = u
		}
	}
	return out
}
