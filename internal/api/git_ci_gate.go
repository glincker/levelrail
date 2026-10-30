package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/cigate"
	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/githubapp"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	maxCIRequiredChecks = 50
	maxCICheckNameLen   = 200
	zeroGitSHA          = "0000000000000000000000000000000000000000"
)

// ciGateSetter is what a git source store needs to persist the CI gate;
// *store.DB satisfies it, test fakes need not.
type ciGateSetter interface {
	SetGitSourceCIGate(ctx context.Context, serviceName string, wait bool, checks []string) error
}

func validateCIChecks(names []string) error {
	if len(names) > maxCIRequiredChecks {
		return fmt.Errorf("at most %d required checks", maxCIRequiredChecks)
	}
	for _, n := range names {
		if strings.TrimSpace(n) == "" || len(n) > maxCICheckNameLen {
			return fmt.Errorf("check names must be 1 to %d characters", maxCICheckNameLen)
		}
	}
	return nil
}

func (rt *Router) saveCIGate(w http.ResponseWriter, r *http.Request, name string, next gitDeploySettings) bool {
	setter, ok := rt.gitSources.(ciGateSetter)
	if !ok {
		writeError(w, http.StatusNotImplemented, "CI gate is not available on this control plane")
		return false
	}
	if err := setter.SetGitSourceCIGate(r.Context(), name, next.WaitForCI, next.CIRequiredChecks); err != nil {
		rt.internalError(w, "api: set git source ci gate failed", err, slog.String("name", name))
		return false
	}
	return true
}

// gateOnCI holds a push deploy until the commit's CI checks conclude. It
// returns a non-empty skip message when the deploy must not start. Sources
// without the gate, and forges it cannot read, deploy as before.
func (rt *Router) gateOnCI(ctx context.Context, name string, gs store.GitSource, sha string) string {
	if !gs.WaitForCI || sha == "" || sha == zeroGitSHA {
		return ""
	}
	f, err := rt.resolveForge(ctx, gs.RepoURL, gs.ConnectionID)
	if err != nil || f.kind != forgeGitHub {
		rt.logger.Warn("api: ci gate: provider cannot be read, deploying without the gate", slog.String("name", name))
		return ""
	}
	owner, repo := f.ownerName()
	cfg := cigate.ConfigFromEnv()
	ghc := githubapp.NewClient()
	reader := &cigate.GitHub{
		HTTP:    ghc.HTTP,
		BaseURL: ghc.APIBaseURL(f.instanceURL),
		Token:   f.token,
		Owner:   owner,
		Repo:    repo,
	}
	pol := cigate.Policy{Required: gs.CIRequiredChecks}
	if rt.brand != nil && rt.brand.Name != "" {
		pol.IgnorePrefixes = []string{rt.brand.Name + " deploy"}
	}
	// A webhook client that hangs up must not abort the wait and let the deploy through.
	res, err := cigate.Wait(context.WithoutCancel(ctx), reader, sha, pol, cfg)
	if err == nil && res.Outcome == cigate.OutcomePass {
		return ""
	}
	reason := "CI gate interrupted"
	if err == nil {
		reason = res.Reason()
	}
	rt.logger.Info("api: ci gate: deploy skipped", slog.String("name", name), slog.String("commit", sha), slog.String("reason", reason))
	rt.recordCIGateSkip(ctx, name, sha, reason)
	return "not deployed: " + reason + "\n"
}

// recordCIGateSkip adds a failed attempt to the deploy history, which also
// fires the app's deploy notification channels.
func (rt *Router) recordCIGateSkip(ctx context.Context, name, sha, reason string) {
	existing, err := rt.apps.GetDesiredService(ctx, name)
	if err != nil {
		rt.logger.Warn("api: ci gate: record skip failed", slog.String("name", name), slog.String("error", err.Error()))
		return
	}
	_, _, finish, _ := rt.beginBuildDeployAttempt(ctx, deploy.Request{
		ServiceName: name,
		CommitSHA:   sha,
		ImageRepo:   defaultImageRepo(name, existing.Image),
	}, *existing, store.DeployAttemptSourceWebhook, "")
	finish(errors.New("skipped by CI gate: " + reason))
}
