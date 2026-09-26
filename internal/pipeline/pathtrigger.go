package pipeline

import (
	"context"
	"log/slog"
	"slices"

	"github.com/GLINCKER/levelrail/internal/pathfilter"
)

// Pull request actions a `types` filter may name.
const (
	prActionOpened      = "opened"
	prActionReopened    = "reopened"
	prActionSynchronize = "synchronize"
)

// PRTypes lists the values on.pull_request.types accepts.
var PRTypes = []string{prActionOpened, prActionReopened, prActionSynchronize}

// acceptsAction reports whether the trigger runs for a pull request action.
// An empty action (a manual re-run) always passes.
func (t *PRTrigger) acceptsAction(action string) bool {
	if len(t.Types) == 0 || action == "" {
		return true
	}
	if slices.Contains(t.Types, action) {
		return true
	}
	return action == prActionOpened && slices.Contains(t.Types, prActionReopened)
}

// pathFilter returns the path filter configured for ev's trigger kind.
func (t Triggers) pathFilter(kind string) pathfilter.Filter {
	switch kind {
	case TriggerPush:
		if t.Push != nil {
			return pathfilter.Filter{Paths: t.Push.Paths, Ignore: t.Push.PathsIgnore}
		}
	case TriggerPullRequest:
		if t.PullRequest != nil {
			return pathfilter.Filter{Paths: t.PullRequest.Paths, Ignore: t.PullRequest.PathsIgnore}
		}
	}
	return pathfilter.Filter{}
}

// changedFiles returns the files ev touched, fetching them once from the
// forge when the payload carried none. A fetch failure yields nil so path
// filters fail open.
func (e *Engine) changedFiles(ctx context.Context, ev *Event, app string) []string {
	if len(ev.Changed) == 0 && ev.ChangedFn != nil {
		files, err := ev.ChangedFn(ctx)
		if err != nil {
			e.cfg.Logger.Warn("pipeline: changed files unavailable, running unfiltered", slog.String("app", app), slog.String("error", err.Error()))
		}
		ev.Changed, ev.ChangedFn = files, nil
	}
	return ev.Changed
}
