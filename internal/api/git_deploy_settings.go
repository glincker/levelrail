package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/pathfilter"
)

const maxDeployPathGlobs = 50

// deployPathFilter is the path filter a connected source applies to pushes.
func deployPathFilter(gs store.GitSource) pathfilter.Filter {
	return pathfilter.Filter{Paths: gs.DeployPaths, Ignore: gs.DeployPathsIgnore}
}

type gitDeploySettings struct {
	DeployPaths       []string `json:"deploy_paths"`
	DeployPathsIgnore []string `json:"deploy_paths_ignore"`
	ReportStatus      bool     `json:"report_status"`
	WaitForCI         bool     `json:"wait_for_ci"`
	CIRequiredChecks  []string `json:"ci_required_checks"`
	TagPattern        string   `json:"tag_pattern"`
	BranchPattern     string   `json:"branch_pattern"`
}

type setGitDeploySettingsRequest struct {
	DeployPaths       *[]string `json:"deploy_paths,omitempty"`
	DeployPathsIgnore *[]string `json:"deploy_paths_ignore,omitempty"`
	ReportStatus      *bool     `json:"report_status,omitempty"`
	WaitForCI         *bool     `json:"wait_for_ci,omitempty"`
	CIRequiredChecks  *[]string `json:"ci_required_checks,omitempty"`
	TagPattern        *string   `json:"tag_pattern,omitempty"`
	BranchPattern     *string   `json:"branch_pattern,omitempty"`
}

// AuditActionGitTriggerPatterns marks the audit row of a trigger pattern change.
const AuditActionGitTriggerPatterns = "git_source.trigger_patterns"

// triggerPatternSetter is the optional store surface for trigger patterns.
type triggerPatternSetter interface {
	SetGitSourceTriggerPatterns(ctx context.Context, serviceName, tagPattern, branchPattern string) error
}

func toGitDeploySettings(gs store.GitSource) gitDeploySettings {
	return gitDeploySettings{
		DeployPaths:       nonNilPaths(gs.DeployPaths),
		DeployPathsIgnore: nonNilPaths(gs.DeployPathsIgnore),
		ReportStatus:      gs.ReportStatus,
		WaitForCI:         gs.WaitForCI,
		CIRequiredChecks:  nonNilPaths(gs.CIRequiredChecks),
		TagPattern:        gs.TagPattern,
		BranchPattern:     gs.BranchPattern,
	}
}

func nonNilPaths(in []string) []string {
	if in == nil {
		return []string{}
	}
	return in
}

// handleSetGitDeploySettings updates the push path filters and the status
// reporting flag of an app's connected source. Omitted fields keep their
// current value.
func (rt *Router) handleSetGitDeploySettings(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	ctx := r.Context()

	var req setGitDeploySettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.DeployPaths == nil && req.DeployPathsIgnore == nil && req.ReportStatus == nil && req.WaitForCI == nil && req.CIRequiredChecks == nil && req.TagPattern == nil && req.BranchPattern == nil {
		writeError(w, http.StatusBadRequest, "deploy_paths, deploy_paths_ignore, report_status, wait_for_ci, ci_required_checks, tag_pattern or branch_pattern is required")
		return
	}
	gs, err := rt.gitSources.GetGitSource(ctx, name)
	if errors.Is(err, store.ErrGitSourceNotFound) {
		writeError(w, http.StatusNotFound, "no git source connected for this app")
		return
	}
	if err != nil {
		rt.internalError(w, "api: load git source for deploy settings failed", err, slog.String("name", name))
		return
	}
	next := toGitDeploySettings(*gs)
	if req.DeployPaths != nil {
		next.DeployPaths = *req.DeployPaths
	}
	if req.DeployPathsIgnore != nil {
		next.DeployPathsIgnore = *req.DeployPathsIgnore
	}
	if req.ReportStatus != nil {
		next.ReportStatus = *req.ReportStatus
	}
	if req.WaitForCI != nil {
		next.WaitForCI = *req.WaitForCI
	}
	if req.CIRequiredChecks != nil {
		next.CIRequiredChecks = *req.CIRequiredChecks
	}
	if req.TagPattern != nil {
		next.TagPattern = spec.NormalizeRefPatterns(*req.TagPattern)
	}
	if req.BranchPattern != nil {
		next.BranchPattern = spec.NormalizeRefPatterns(*req.BranchPattern)
	}
	if err := rt.validateTriggerPatterns(ctx, req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateDeployPaths(next); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateCIChecks(next.CIRequiredChecks); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if (req.WaitForCI != nil || req.CIRequiredChecks != nil) && !rt.saveCIGate(w, r, name, next) {
		return
	}
	if (req.TagPattern != nil || req.BranchPattern != nil) && !rt.saveTriggerPatterns(w, r, name, *gs, next) {
		return
	}
	if err := rt.gitSources.SetGitSourceDeploySettings(ctx, name, next.DeployPaths, next.DeployPathsIgnore, next.ReportStatus); err != nil {
		rt.internalError(w, "api: set git deploy settings failed", err, slog.String("name", name))
		return
	}
	rt.logger.Info("api: git deploy settings updated", slog.String("name", name), slog.Int("paths", len(next.DeployPaths)), slog.Int("paths_ignore", len(next.DeployPathsIgnore)), slog.Bool("report_status", next.ReportStatus))
	writeJSON(w, http.StatusOK, next)
}

// validateTriggerPatterns checks only the lists the request sets, so a lowered
// cap never blocks an unrelated edit of a source saved under the old cap.
func (rt *Router) validateTriggerPatterns(ctx context.Context, req setGitDeploySettingsRequest) error {
	lim := spec.RefPatternLimits{
		MaxPatterns:   rt.runtimeSettings.Limit(ctx, runtimesettings.KeyGitTriggerPatternsMax),
		MaxPatternLen: rt.runtimeSettings.Limit(ctx, runtimesettings.KeyGitTriggerPatternMaxLen),
		MaxListBytes:  rt.runtimeSettings.Limit(ctx, runtimesettings.KeyGitTriggerListMaxBytes),
	}
	if req.TagPattern != nil {
		if err := spec.ValidateRefPatterns(*req.TagPattern, lim); err != nil {
			return fmt.Errorf("tag_pattern: %w", err)
		}
	}
	if req.BranchPattern != nil {
		if err := spec.ValidateRefPatterns(*req.BranchPattern, lim); err != nil {
			return fmt.Errorf("branch_pattern: %w", err)
		}
	}
	return nil
}

func (rt *Router) saveTriggerPatterns(w http.ResponseWriter, r *http.Request, name string, prev store.GitSource, next gitDeploySettings) bool {
	setter, ok := rt.gitSources.(triggerPatternSetter)
	if !ok {
		writeError(w, http.StatusNotImplemented, "trigger patterns are not available on this control plane")
		return false
	}
	if err := setter.SetGitSourceTriggerPatterns(r.Context(), name, next.TagPattern, next.BranchPattern); err != nil {
		rt.internalError(w, "api: set git source trigger patterns failed", err, slog.String("name", name))
		return false
	}
	if prev.TagPattern != next.TagPattern || prev.BranchPattern != next.BranchPattern {
		rt.auditTriggerPatterns(r, name, next)
	}
	return true
}

// auditTriggerPatterns records the pattern text, which is not secret, so a
// reviewer can see exactly which refs were allowed to deploy and when.
func (rt *Router) auditTriggerPatterns(r *http.Request, name string, next gitDeploySettings) {
	if rt.auditLog == nil {
		return
	}
	a := rt.accessActor(r)
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: trigger patterns audit id failed", slog.String("error", err.Error()), slog.String("name", name))
		return
	}
	detail, _ := json.Marshal(map[string]string{"app": name, "tag_pattern": next.TagPattern, "branch_pattern": next.BranchPattern})
	entry := store.AuditEntry{
		ID: id, ActorType: a.kind, ActorID: a.id, ActorName: a.name,
		Ability: AbilityWriteSensitive, Method: r.Method, Path: r.URL.Path, StatusCode: http.StatusOK,
		RemoteAddr: clientIP(r), CreatedAt: store.FormatAuditTime(time.Now()),
		ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")),
		Action:     AuditActionGitTriggerPatterns, Detail: string(detail),
	}
	if err := rt.auditLog.SaveAuditEntry(context.WithoutCancel(r.Context()), entry); err != nil {
		rt.logger.Warn("api: trigger patterns audit save failed", slog.String("error", err.Error()), slog.String("name", name))
	}
}

func validateDeployPaths(s gitDeploySettings) error {
	if len(s.DeployPaths) > maxDeployPathGlobs || len(s.DeployPathsIgnore) > maxDeployPathGlobs {
		return fmt.Errorf("at most %d globs per list", maxDeployPathGlobs)
	}
	return pathfilter.Filter{Paths: s.DeployPaths, Ignore: s.DeployPathsIgnore}.Validate()
}

// skipPushForPaths applies the source's path filter to a push. It returns a
// non-empty message when the push should not deploy. Unknown changed files
// (no list in the payload, and a failed forge lookup) let the push through.
func (rt *Router) skipPushForPaths(ctx context.Context, name string, gs store.GitSource, ref, before, after string, inline []string) string {
	f := deployPathFilter(gs)
	if f.IsZero() || strings.HasPrefix(ref, "refs/tags/") {
		return ""
	}
	files := inline
	if len(files) == 0 {
		fetched, err := rt.changedFilesFn(name, changeQuery{Base: before, Head: after})(ctx)
		if err != nil {
			rt.logger.Warn("api: git push webhook: changed files unavailable, deploying unfiltered", slog.String("name", name), slog.String("error", err.Error()))
		}
		files = fetched
	}
	res := f.Apply(files)
	if res.Run {
		return ""
	}
	return fmt.Sprintf("ignored: %s\n", res.Reason)
}
