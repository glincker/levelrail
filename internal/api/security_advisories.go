package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"sync"

	"github.com/GLINCKER/levelrail/internal/advisoryfeed"
	"github.com/GLINCKER/levelrail/internal/attention"
	"github.com/GLINCKER/levelrail/internal/store"
)

var (
	advisoryOnce sync.Once
	advisorySvc  *advisoryfeed.Service
)

// advisoryService is built from the environment on first use; with no feed
// configured it stays inert and costs nothing per request.
func advisoryService() *advisoryfeed.Service {
	advisoryOnce.Do(func() {
		advisorySvc = advisoryfeed.New(advisoryfeed.ConfigFromEnv(os.Getenv), nil)
	})
	return advisorySvc
}

type advisoryResource struct {
	advisoryfeed.Snapshot
	Matches []advisoryfeed.Match `json:"matches"`
}

// runningImages lists what each visible app runs. Digests are looked up only
// when some advisory selects by digest.
func (rt *Router) runningImages(ctx context.Context, canSee func(string) bool, withDigest bool) []advisoryfeed.Image {
	svcs, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		rt.logger.Warn("api: advisories: list apps failed", slog.String("error", err.Error()))
		return nil
	}
	var out []advisoryfeed.Image
	for _, s := range svcs {
		if s.Image == "" || !canSee(s.Name) {
			continue
		}
		img := advisoryfeed.Image{App: s.Name, Ref: s.Image}
		if withDigest && rt.deployAttempts != nil {
			img.Digest = rt.servingDigest(ctx, s)
		}
		out = append(out, img)
	}
	return out
}

func (rt *Router) servingDigest(ctx context.Context, s store.DesiredService) string {
	attempts, err := rt.deployAttempts.ListDeployAttempts(ctx, s.Name)
	if err != nil {
		return ""
	}
	for _, a := range attempts {
		if a.Status == store.DeployAttemptStatusSucceeded && a.Image == s.Image {
			if a.DigestReason == store.DigestReasonResolved || a.DigestReason == store.DigestReasonPinned {
				return a.ImageDigest
			}
			return ""
		}
	}
	return ""
}

func (rt *Router) advisoryReport(r *http.Request) (advisoryResource, error) {
	svc := advisoryService()
	if !svc.Enabled() {
		return advisoryResource{Snapshot: advisoryfeed.Snapshot{Entries: []advisoryfeed.Entry{}}, Matches: []advisoryfeed.Match{}}, nil
	}
	canSee, err := rt.appVisibilityFilter(r)
	if err != nil {
		return advisoryResource{}, fmt.Errorf("advisories: %w", err)
	}
	snap := svc.Snapshot(r.Context())
	images := rt.runningImages(r.Context(), canSee, snap.UsesDigests())
	snap, matches := svc.Match(r.Context(), images)
	if matches == nil {
		matches = []advisoryfeed.Match{}
	}
	return advisoryResource{Snapshot: snap, Matches: matches}, nil
}

// handleSecurityAdvisories handles GET /api/v1/security/advisories.
func (rt *Router) handleSecurityAdvisories(w http.ResponseWriter, r *http.Request) {
	res, err := rt.advisoryReport(r)
	if err != nil {
		rt.internalError(w, "api: security advisories failed", err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// advisoryAttentionItems is one grouped item, so a feed with many matches
// stays a single line with the per-app detail in params.
func (rt *Router) advisoryAttentionItems(r *http.Request, abilities []string) []attention.Item {
	if !advisoryService().Enabled() || !hasAbility(abilities, AbilityRead) {
		return nil
	}
	res, err := rt.advisoryReport(r)
	if err != nil || len(res.Matches) == 0 {
		return nil
	}
	return []attention.Item{advisoryItem(res)}
}

func advisoryItem(res advisoryResource) attention.Item {
	apps := map[string]bool{}
	worst := ""
	var detail string
	for i, m := range res.Matches {
		apps[m.App] = true
		if advisoryfeed.SeverityRank(m.Severity) > advisoryfeed.SeverityRank(worst) {
			worst = m.Severity
		}
		if i > 0 {
			detail += "; "
		}
		detail += fmt.Sprintf("%s: %s (%s), %s", m.App, m.AdvisoryID, m.Severity, m.Hint)
	}
	names := make([]string, 0, len(apps))
	for a := range apps {
		names = append(names, a)
	}
	sort.Strings(names)
	sev := attention.Warning
	if worst == advisoryfeed.SeverityCritical {
		sev = attention.Critical
	}
	subject := fmt.Sprintf("%d running images", len(res.Matches))
	if len(res.Matches) == 1 {
		subject = "1 running image"
	}
	return feedItem(sev, attention.KindAdvisoryMatch, subject, detail+" [feed: "+res.Signature+"]", map[string]string{
		"matches":   fmt.Sprint(len(res.Matches)),
		"apps":      fmt.Sprint(len(names)),
		"worst":     worst,
		"signature": res.Signature,
	})
}
