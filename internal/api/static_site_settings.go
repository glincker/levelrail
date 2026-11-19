package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/staticroutes"
	"github.com/GLINCKER/levelrail/internal/store"
)

// staticSiteSettingsStore is implemented by *store.DB; a fake store without it
// answers 501 so the routes degrade like the other optional static features.
type staticSiteSettingsStore interface {
	GetStaticSiteSettings(ctx context.Context, name string) (store.StaticSiteSettings, store.StaticUIRules, error)
	SaveStaticSiteSettings(ctx context.Context, name string, s store.StaticSiteSettings) error
	SaveStaticSiteUIRules(ctx context.Context, name string, r store.StaticUIRules) error
}

// staticSettingsResource is the stored overrides plus what the live site serves.
// A null bool or empty string means "not overridden, app.yaml decides".
type staticSettingsResource struct {
	Name          string `json:"name"`
	BuildCommand  string `json:"build_command"`
	OutputDir     string `json:"output_dir"`
	SPAFallback   *bool  `json:"spa_fallback"`
	CleanURLs     *bool  `json:"clean_urls"`
	TrailingSlash string `json:"trailing_slash"`
	NotFoundPage  string `json:"not_found_page"`
	// Effective is what the live deploy serves; empty before the first publish.
	Effective staticroutes.Options `json:"effective"`
}

// staticSettingsPatch changes only the fields present. Reset lists override
// names to clear back to the app.yaml value.
type staticSettingsPatch struct {
	BuildCommand  *string  `json:"build_command"`
	OutputDir     *string  `json:"output_dir"`
	SPAFallback   *bool    `json:"spa_fallback"`
	CleanURLs     *bool    `json:"clean_urls"`
	TrailingSlash *string  `json:"trailing_slash"`
	NotFoundPage  *string  `json:"not_found_page"`
	Reset         []string `json:"reset"`
}

type staticUIRulesBody struct {
	Redirects []staticroutes.UIRedirect `json:"redirects"`
	Headers   []staticroutes.UIHeader   `json:"headers"`
}

type staticRuleRow struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Status int    `json:"status"`
	Force  bool   `json:"force"`
	// Source is ui, app.yaml or file; Origin is the exact label such as "_redirects:3".
	Source string `json:"source"`
	Origin string `json:"origin"`
}

type staticHeaderRow struct {
	Path   string            `json:"path"`
	Set    map[string]string `json:"set"`
	Remove []string          `json:"remove"`
	Source string            `json:"source"`
	Origin string            `json:"origin"`
}

type staticEffectiveRulesResource struct {
	Redirects []staticRuleRow        `json:"redirects"`
	Headers   []staticHeaderRow      `json:"headers"`
	Options   staticroutes.Options   `json:"options"`
	Problems  []staticroutes.Problem `json:"problems"`
	Published bool                   `json:"published"`
	// UI is the editable part, in the shape PUT accepts.
	UI staticUIRulesBody `json:"ui"`
}

type staticRulesLintFailure struct {
	Error    string                     `json:"error"`
	Problems []staticroutes.LineProblem `json:"problems"`
}

func (rt *Router) staticSettingsDeps(w http.ResponseWriter, r *http.Request) (*store.StaticSite, staticSiteSettingsStore, bool) {
	st, ok := rt.staticSites.(staticSiteSettingsStore)
	getter, ok2 := rt.staticSites.(staticSiteGetter)
	if !ok || !ok2 {
		writeError(w, http.StatusNotImplemented, "static site settings are not configured on this control plane")
		return nil, nil, false
	}
	name := r.PathValue("name")
	site, err := getter.GetStaticSite(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		// Settings are keyed by name, so a Pages app that has not published yet can store them.
		if rt.isUnpublishedStaticApp(r, name) {
			return &store.StaticSite{Name: name}, st, true
		}
		writeError(w, http.StatusNotFound, "static site not found")
		return nil, nil, false
	}
	if err != nil {
		rt.internalError(w, "api: static site settings: lookup", err, slog.String("name", name))
		return nil, nil, false
	}
	return site, st, true
}

// isUnpublishedStaticApp is true for an app created as a Pages site or
// connected to a git source with the static build type.
func (rt *Router) isUnpublishedStaticApp(r *http.Request, name string) bool {
	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if err != nil || svc == nil {
		return false
	}
	if svc.Image == staticSitePlaceholderImage(name) {
		return true
	}
	if rt.gitSources == nil {
		return false
	}
	gs, err := rt.gitSources.GetGitSource(r.Context(), name)
	return err == nil && gs != nil && gs.BuildType == spec.BuildStatic
}

func (rt *Router) staticSettingsResource(site *store.StaticSite, s store.StaticSiteSettings) (staticSettingsResource, error) {
	out := staticSettingsResource{Name: site.Name, BuildCommand: s.BuildCommand, OutputDir: s.OutputDir, SPAFallback: s.SPAFallback,
		CleanURLs: s.CleanURLs, TrailingSlash: s.TrailingSlash, NotFoundPage: s.NotFoundPage}
	if site.RootDir == "" {
		return out, nil
	}
	rs, err := staticroutes.LoadSidecar(site.RootDir)
	if err != nil {
		return out, err
	}
	if rs != nil {
		out.Effective = rs.Options
	}
	return out, nil
}

// handleGetStaticSiteSettings handles GET /api/v1/static-sites/{name}/settings.
func (rt *Router) handleGetStaticSiteSettings(w http.ResponseWriter, r *http.Request) {
	site, st, ok := rt.staticSettingsDeps(w, r)
	if !ok {
		return
	}
	s, _, err := st.GetStaticSiteSettings(r.Context(), site.Name)
	if err != nil {
		rt.internalError(w, "api: static site settings: read", err, slog.String("name", site.Name))
		return
	}
	out, err := rt.staticSettingsResource(site, s)
	if err != nil {
		rt.internalError(w, "api: static site settings: live rules", err, slog.String("name", site.Name))
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// applyStaticSettingsPatch validates p and applies it to s; the message is user facing.
func applyStaticSettingsPatch(s *store.StaticSiteSettings, p staticSettingsPatch) string {
	for _, f := range p.Reset {
		switch f {
		case "build_command":
			s.BuildCommand = ""
		case "output_dir":
			s.OutputDir = ""
		case "spa_fallback":
			s.SPAFallback = nil
		case "clean_urls":
			s.CleanURLs = nil
		case "trailing_slash":
			s.TrailingSlash = ""
		case "not_found_page":
			s.NotFoundPage = ""
		default:
			return "reset: unknown setting " + f
		}
	}
	bc, od := s.BuildCommand, s.OutputDir
	if p.BuildCommand != nil {
		bc = *p.BuildCommand
	}
	if p.OutputDir != nil {
		od = *p.OutputDir
	}
	if err := spec.ValidateStaticOverrides(bc, od, spec.StaticBuildCommandMaxFromEnv(os.LookupEnv)); err != nil {
		return err.Error()
	}
	od, nerr := spec.NormalizeRepoPath("output_dir", od)
	if nerr != nil {
		return nerr.Error()
	}
	s.BuildCommand, s.OutputDir = bc, od
	if p.SPAFallback != nil {
		s.SPAFallback = p.SPAFallback
	}
	if p.CleanURLs != nil {
		s.CleanURLs = p.CleanURLs
	}
	if p.TrailingSlash != nil {
		s.TrailingSlash = *p.TrailingSlash
	}
	if p.NotFoundPage != nil {
		s.NotFoundPage = *p.NotFoundPage
	}
	o := staticroutes.Options{TrailingSlash: s.TrailingSlash, NotFound: s.NotFoundPage}
	if err := staticroutes.ValidateOptions(&o); err != nil {
		return strings.Replace(err.Error(), "static.", "", 1)
	}
	return ""
}

// handlePatchStaticSiteSettings handles PATCH /api/v1/static-sites/{name}/settings.
func (rt *Router) handlePatchStaticSiteSettings(w http.ResponseWriter, r *http.Request) {
	site, st, ok := rt.staticSettingsDeps(w, r)
	if !ok {
		return
	}
	var p staticSettingsPatch
	if err := decodeJSONBodyLimit(r, &p, maxStaticRoutesBody); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	s, ui, err := st.GetStaticSiteSettings(r.Context(), site.Name)
	if err != nil {
		rt.internalError(w, "api: static site settings: read", err, slog.String("name", site.Name))
		return
	}
	if msg := applyStaticSettingsPatch(&s, p); msg != "" {
		writeError(w, http.StatusUnprocessableEntity, msg)
		return
	}
	if err := st.SaveStaticSiteSettings(r.Context(), site.Name, s); err != nil {
		rt.internalError(w, "api: static site settings: save", err, slog.String("name", site.Name))
		return
	}
	if perr := rt.publishStaticUI(site, s, ui); perr != nil {
		rt.internalError(w, "api: static site settings: apply to live site", perr, slog.String("name", site.Name))
		return
	}
	out, err := rt.staticSettingsResource(site, s)
	if err != nil {
		rt.internalError(w, "api: static site settings: live rules", err, slog.String("name", site.Name))
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func uiToCompile(ui store.StaticUIRules) ([]staticroutes.UIRedirect, []staticroutes.UIHeader) {
	rd := make([]staticroutes.UIRedirect, 0, len(ui.Redirects))
	for _, r := range ui.Redirects {
		rd = append(rd, staticroutes.UIRedirect(r))
	}
	hd := make([]staticroutes.UIHeader, 0, len(ui.Headers))
	for _, h := range ui.Headers {
		hd = append(hd, staticroutes.UIHeader(h))
	}
	return rd, hd
}

// publishStaticUI rewrites the live tree's sidecar with the stored overrides and
// wakes the ingress controller, so the edit serves without a redeploy. A site
// not yet published has no sidecar to update; the next deploy picks them up.
func (rt *Router) publishStaticUI(site *store.StaticSite, s store.StaticSiteSettings, ui store.StaticUIRules) error {
	if site.RootDir == "" {
		return nil
	}
	rs, err := staticroutes.LoadSidecar(site.RootDir)
	if err != nil || rs == nil {
		return err
	}
	rd, hd := uiToCompile(ui)
	rules, blocks, probs := staticroutes.CompileUI(rd, hd, staticroutes.LimitsFromEnv(os.LookupEnv))
	if len(probs) > 0 {
		return errors.New(probs[0].Message)
	}
	if err := rs.ApplyUI(rules, blocks, staticroutes.OptionOverrides{SPA: s.SPAFallback, CleanURLs: s.CleanURLs, TrailingSlash: s.TrailingSlash, NotFound: s.NotFoundPage}); err != nil {
		return err
	}
	if err := staticroutes.WriteSidecar(site.RootDir, rs); err != nil {
		return err
	}
	if rt.reconcileNudger != nil {
		rt.reconcileNudger.Nudge()
	}
	return nil
}

func effectiveRules(rs *staticroutes.Ruleset, ui store.StaticUIRules, published bool) staticEffectiveRulesResource {
	out := staticEffectiveRulesResource{Redirects: []staticRuleRow{}, Headers: []staticHeaderRow{}, Options: rs.Options, Problems: rs.Problems, Published: published}
	for _, r := range rs.Rules {
		out.Redirects = append(out.Redirects, staticRuleRow{From: r.From, To: r.To, Status: r.Status, Force: r.Force, Source: staticroutes.SourceKind(r.Source), Origin: r.Source})
	}
	for _, b := range rs.Headers {
		set := map[string]string{}
		for _, kv := range b.Set {
			set[kv.Name] = kv.Value
		}
		rm := b.Remove
		if rm == nil {
			rm = []string{}
		}
		out.Headers = append(out.Headers, staticHeaderRow{Path: b.Path, Set: set, Remove: rm, Source: staticroutes.SourceKind(b.Source), Origin: b.Source})
	}
	rd, hd := uiToCompile(ui)
	out.UI = staticUIRulesBody{Redirects: rd, Headers: hd}
	return out
}

// handleGetStaticSiteRules handles GET /api/v1/static-sites/{name}/rules.
func (rt *Router) handleGetStaticSiteRules(w http.ResponseWriter, r *http.Request) {
	site, st, ok := rt.staticSettingsDeps(w, r)
	if !ok {
		return
	}
	_, ui, err := st.GetStaticSiteSettings(r.Context(), site.Name)
	if err != nil {
		rt.internalError(w, "api: static site rules: read", err, slog.String("name", site.Name))
		return
	}
	rs, err := staticroutes.LoadSidecar(site.RootDir)
	if err != nil {
		rt.internalError(w, "api: static site rules: load live rules", err, slog.String("name", site.Name))
		return
	}
	published := rs != nil
	if rs == nil {
		rs, err = staticroutes.Compile(staticroutes.Input{}, staticroutes.DefaultLimits())
		if err != nil {
			rt.internalError(w, "api: static site rules: empty rules", err)
			return
		}
	}
	writeJSON(w, http.StatusOK, effectiveRules(rs, ui, published))
}

// handlePutStaticSiteRules handles PUT /api/v1/static-sites/{name}/rules: it
// replaces the dashboard rules, answering 422 with every failing entry.
func (rt *Router) handlePutStaticSiteRules(w http.ResponseWriter, r *http.Request) {
	site, st, ok := rt.staticSettingsDeps(w, r)
	if !ok {
		return
	}
	var body staticUIRulesBody
	if err := decodeJSONBodyLimit(r, &body, maxStaticRoutesBody); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if _, _, probs := staticroutes.CompileUI(body.Redirects, body.Headers, staticroutes.LimitsFromEnv(os.LookupEnv)); len(probs) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, staticRulesLintFailure{Error: "routing rules are invalid", Problems: probs})
		return
	}
	ui := store.StaticUIRules{Redirects: make([]store.StaticUIRedirect, 0, len(body.Redirects)), Headers: make([]store.StaticUIHeader, 0, len(body.Headers))}
	for _, rd := range body.Redirects {
		ui.Redirects = append(ui.Redirects, store.StaticUIRedirect(rd))
	}
	for _, h := range body.Headers {
		ui.Headers = append(ui.Headers, store.StaticUIHeader(h))
	}
	s, _, err := st.GetStaticSiteSettings(r.Context(), site.Name)
	if err != nil {
		rt.internalError(w, "api: static site rules: read settings", err, slog.String("name", site.Name))
		return
	}
	if err := st.SaveStaticSiteUIRules(r.Context(), site.Name, ui); err != nil {
		rt.internalError(w, "api: static site rules: save", err, slog.String("name", site.Name))
		return
	}
	if err := rt.publishStaticUI(site, s, ui); err != nil {
		rt.internalError(w, "api: static site rules: apply to live site", err, slog.String("name", site.Name))
		return
	}
	rs, err := staticroutes.LoadSidecar(site.RootDir)
	published := err == nil && rs != nil
	if !published {
		rs, _ = staticroutes.Compile(staticroutes.Input{}, staticroutes.DefaultLimits())
	}
	writeJSON(w, http.StatusOK, effectiveRules(rs, ui, published))
}
