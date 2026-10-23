package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/repolayout"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	releaseGroupBreaking = "breaking"
	releaseGroupOther    = "other"
)

// releaseGroupOrder is the display order; unknown types fall into "other".
var releaseGroupOrder = []struct{ Key, Title string }{
	{releaseGroupBreaking, "Breaking changes"},
	{"feat", "Features"},
	{"fix", "Fixes"},
	{"perf", "Performance"},
	{"refactor", "Refactors"},
	{"docs", "Documentation"},
	{"test", "Tests"},
	{releaseGroupOther, "Other changes"},
}

type releaseNoteEntry struct {
	SHA     string `json:"sha"`
	Scope   string `json:"scope,omitempty"`
	Summary string `json:"summary"`
	Author  string `json:"author,omitempty"`
	URL     string `json:"url,omitempty"`
}

type releaseNoteGroup struct {
	Key     string             `json:"key"`
	Title   string             `json:"title"`
	Entries []releaseNoteEntry `json:"entries"`
}

// releaseNotesResource is GET .../deploys/release-notes' response shape.
type releaseNotesResource struct {
	ServiceName string              `json:"service_name"`
	From        deployCompareSide   `json:"from"`
	To          deployCompareSide   `json:"to"`
	Commits     []repolayout.Commit `json:"commits"`
	Groups      []releaseNoteGroup  `json:"groups"`
	Markdown    string              `json:"markdown"`
	Note        string              `json:"note,omitempty"`
}

const (
	releaseNotesNoCommits = "Release notes need two deploys that recorded a git commit on a connected repository."
	releaseNotesOutside   = "One of the commits is outside the recent commit window the provider returns, so the range could not be listed."
	releaseNotesNoSource  = "This app has no connected git source, so commits cannot be listed."
)

// parseConventionalSubject splits "type(scope)!: summary". A subject that is
// not conventional returns an empty type and the whole subject as summary.
func parseConventionalSubject(subject string) (typ, scope, summary string, breaking bool) {
	head, rest, found := strings.Cut(subject, ": ")
	if !found || head == "" || strings.ContainsAny(head, " \t") {
		return "", "", strings.TrimSpace(subject), false
	}
	if strings.HasSuffix(head, "!") {
		breaking = true
		head = strings.TrimSuffix(head, "!")
	}
	if open := strings.Index(head, "("); open > 0 && strings.HasSuffix(head, ")") {
		scope = head[open+1 : len(head)-1]
		head = head[:open]
	}
	return strings.ToLower(head), scope, strings.TrimSpace(rest), breaking
}

// groupReleaseNotes buckets commits by conventional-commit type, newest
// first inside each group, dropping empty groups.
func groupReleaseNotes(commits []repolayout.Commit) []releaseNoteGroup {
	buckets := map[string][]releaseNoteEntry{}
	for _, c := range commits {
		subject, body, _ := strings.Cut(strings.TrimSpace(c.Message), "\n")
		typ, scope, summary, breaking := parseConventionalSubject(strings.TrimSpace(subject))
		key := typ
		switch {
		case breaking || strings.Contains(body, "BREAKING CHANGE"):
			key = releaseGroupBreaking
		case !isKnownReleaseGroup(typ):
			key = releaseGroupOther
		}
		buckets[key] = append(buckets[key], releaseNoteEntry{
			SHA: c.SHA, Scope: scope, Summary: summary, Author: c.Author, URL: c.URL,
		})
	}
	out := []releaseNoteGroup{}
	for _, g := range releaseGroupOrder {
		if entries := buckets[g.Key]; len(entries) > 0 {
			out = append(out, releaseNoteGroup{Key: g.Key, Title: g.Title, Entries: entries})
		}
	}
	return out
}

func isKnownReleaseGroup(typ string) bool {
	for _, g := range releaseGroupOrder {
		if g.Key == typ && typ != releaseGroupBreaking && typ != releaseGroupOther {
			return true
		}
	}
	return false
}

func renderReleaseNotesMarkdown(app, fromLabel, toLabel string, groups []releaseNoteGroup) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## %s: %s to %s\n", app, fromLabel, toLabel)
	if len(groups) == 0 {
		b.WriteString("\nNo commits between these deploys.\n")
		return b.String()
	}
	for _, g := range groups {
		fmt.Fprintf(&b, "\n### %s\n\n", g.Title)
		for _, e := range g.Entries {
			line := e.Summary
			if e.Scope != "" {
				line = "**" + e.Scope + "**: " + line
			}
			fmt.Fprintf(&b, "- %s (%s)\n", line, shortSHA(e.SHA))
		}
	}
	return b.String()
}

func shaMatches(a, b string) bool {
	if len(a) < 7 || len(b) < 7 {
		return false
	}
	return strings.HasPrefix(a, b) || strings.HasPrefix(b, a)
}

func indexOfSHA(commits []repolayout.Commit, sha string) int {
	for i, c := range commits {
		if shaMatches(c.SHA, sha) {
			return i
		}
	}
	return -1
}

// commitsBetween returns the commits after fromSHA up to and including
// toSHA from a newest-first list. ok is false when either SHA is outside it.
func commitsBetween(commits []repolayout.Commit, fromSHA, toSHA string) (out []repolayout.Commit, ok bool) {
	to, from := indexOfSHA(commits, toSHA), indexOfSHA(commits, fromSHA)
	if to < 0 || from < 0 {
		return nil, false
	}
	if to >= from {
		return []repolayout.Commit{}, true
	}
	return commits[to:from], true
}

// commitsForRange lists the commits between two deploy SHAs through the
// app's connected git source; note explains an empty result.
func (rt *Router) commitsForRange(ctx context.Context, app, fromSHA, toSHA string) (commits []repolayout.Commit, note string) {
	if fromSHA == "" || toSHA == "" {
		return []repolayout.Commit{}, releaseNotesNoCommits
	}
	gs, err := rt.gitSources.GetGitSource(ctx, app)
	if err != nil {
		if !errors.Is(err, store.ErrGitSourceNotFound) {
			rt.logger.Error("api: release notes: load git source failed", slog.String("error", err.Error()), slog.String("name", app))
		}
		return []repolayout.Commit{}, releaseNotesNoSource
	}
	if requireHTTPOrHTTPSScheme(gs.RepoURL) != nil {
		return []repolayout.Commit{}, releaseNotesNoSource
	}
	key := app + "\x00" + gs.RepoURL + "\x00" + gs.Branch
	bundle, hit := rt.gitCommits.get(key)
	if !hit {
		token := rt.tokenForRepo(ctx, app, gs.RepoURL, true)
		bundle, err = rt.loadGitCommitsBundle(ctx, gs, gs.Branch, token)
		if err != nil {
			rt.logger.Warn("api: release notes: load commits failed", slog.String("error", err.Error()), slog.String("name", app))
			return []repolayout.Commit{}, releaseNotesOutside
		}
		rt.gitCommits.put(key, bundle)
	}
	between, ok := commitsBetween(bundle.Commits, fromSHA, toSHA)
	if !ok {
		return []repolayout.Commit{}, releaseNotesOutside
	}
	return between, ""
}

// handleReleaseNotes handles
// GET /api/v1/apps/{name}/deploys/release-notes?from={deployId}&to={deployId}.
// to defaults to the newest deploy that recorded a commit.
func (rt *Router) handleReleaseNotes(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	fromID := r.URL.Query().Get("from")
	if fromID == "" {
		writeError(w, http.StatusBadRequest, "from is required")
		return
	}
	fromSide, ok := rt.loadDeployCompareSide(w, r, name, fromID)
	if !ok {
		return
	}
	toID := r.URL.Query().Get("to")
	if toID == "" {
		attempts, err := rt.deployAttempts.ListDeployAttempts(r.Context(), name)
		if err != nil {
			rt.internalError(w, "api: release notes: list attempts failed", err, slog.String("name", name))
			return
		}
		toID = newestCommitAttemptID(attempts)
		if toID == "" {
			writeError(w, http.StatusNotFound, "no deploy with a recorded commit found")
			return
		}
	}
	toSide, ok := rt.loadDeployCompareSide(w, r, name, toID)
	if !ok {
		return
	}

	commits, note := rt.commitsForRange(r.Context(), name, fromSide.CommitSHA, toSide.CommitSHA)
	groups := groupReleaseNotes(commits)
	writeJSON(w, http.StatusOK, releaseNotesResource{
		ServiceName: name,
		From:        maskDeploySideEnv(fromSide),
		To:          maskDeploySideEnv(toSide),
		Commits:     commits,
		Groups:      groups,
		Markdown:    renderReleaseNotesMarkdown(name, releaseSideLabel(fromSide), releaseSideLabel(toSide), groups),
		Note:        note,
	})
}

func releaseSideLabel(s deployCompareSide) string {
	if s.CommitSHA != "" {
		return shortSHA(s.CommitSHA)
	}
	return s.Image
}

func newestCommitAttemptID(attempts []store.DeployAttempt) string {
	sorted := append([]store.DeployAttempt(nil), attempts...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].StartedAt.After(sorted[j].StartedAt) })
	for _, a := range sorted {
		if a.CommitSHA != "" {
			return a.ID
		}
	}
	return ""
}

// envReleaseNotesTimeoutSeconds bounds notes generation for a deploy notification.
const envReleaseNotesTimeoutSeconds = "RELEASE_NOTES_TIMEOUT_SECONDS"

// releaseNotesForDeploy returns markdown release notes for a deploy that just
// succeeded, or "" when the app is not in a production environment and the
// deploy is not a promotion, or when anything goes wrong. It never fails the caller.
func (rt *Router) releaseNotesForDeploy(ctx context.Context, app, source string) (notes string) {
	defer func() {
		if p := recover(); p != nil {
			rt.logger.Error("api: deploy release notes panicked", slog.String("name", app))
			notes = ""
		}
	}()
	if rt.apps == nil || rt.deployAttempts == nil || rt.gitSources == nil {
		return ""
	}
	timeout := time.Duration(envIntDefault(envReleaseNotesTimeoutSeconds, 5)) * time.Second
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if source != store.DeployAttemptSourcePromote && !rt.isProductionApp(ctx, app) {
		return ""
	}
	attempts, err := rt.deployAttempts.ListDeployAttempts(ctx, app)
	if err != nil {
		rt.logger.Warn("api: deploy release notes: list attempts failed", slog.String("error", err.Error()), slog.String("name", app))
		return ""
	}
	sorted := append([]store.DeployAttempt(nil), attempts...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].StartedAt.After(sorted[j].StartedAt) })
	var to, from *store.DeployAttempt
	for i := range sorted {
		a := &sorted[i]
		if a.CommitSHA == "" || a.Status != store.DeployAttemptStatusSucceeded {
			continue
		}
		if to == nil {
			to = a
		} else if a.CommitSHA != to.CommitSHA {
			from = a
			break
		}
	}
	if to == nil || from == nil {
		return ""
	}
	list := rt.commitsForRange
	if rt.releaseCommits != nil {
		list = rt.releaseCommits
	}
	commits, note := list(ctx, app, from.CommitSHA, to.CommitSHA)
	if note != "" || len(commits) == 0 {
		return ""
	}
	return renderReleaseNotesMarkdown(app, shortSHA(from.CommitSHA), shortSHA(to.CommitSHA), groupReleaseNotes(commits))
}

func (rt *Router) isProductionApp(ctx context.Context, app string) bool {
	svc, err := rt.apps.GetDesiredService(ctx, app)
	if err != nil || svc.EnvironmentID == "" || rt.environments == nil {
		return false
	}
	env, err := rt.environments.GetEnvironment(ctx, svc.EnvironmentID)
	return err == nil && env.Kind == store.EnvironmentKindProduction
}
