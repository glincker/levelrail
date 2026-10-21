package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	envRerunChainMaxDepth     = "APP_RERUN_CHAIN_MAX_DEPTH"
	defaultRerunChainMaxDepth = 50
)

const rerunOfInvalidMsg = "rerun_of: no such deploy attempt for this app"

type rerunOfKey struct{}

// withRerunOf carries a validated rerun source down to the attempt row
// minted deeper in the call chain, like the commit metadata does.
func withRerunOf(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, rerunOfKey{}, id)
}

func rerunOfFrom(ctx context.Context) string {
	id, _ := ctx.Value(rerunOfKey{}).(string)
	return id
}

// validateRerunOf checks that a caller-supplied rerun_of names an attempt
// of the same app. It returns an HTTP status and message when it does not.
// Both failures share one message so a caller cannot probe whether an id
// exists under another app.
func (rt *Router) validateRerunOf(ctx context.Context, app, id string) (int, string) {
	if id == "" {
		return 0, ""
	}
	a, err := rt.deployAttempts.GetDeployAttempt(ctx, id)
	switch {
	case errors.Is(err, store.ErrDeployAttemptNotFound):
		return http.StatusBadRequest, rerunOfInvalidMsg
	case err != nil:
		rt.logger.Error("api: validate rerun_of failed", slog.String("error", err.Error()), slog.String("attempt_id", id))
		return http.StatusInternalServerError, "internal error"
	case a.ServiceName != app:
		return http.StatusBadRequest, rerunOfInvalidMsg
	}
	return 0, ""
}

// rerunChainPositions returns a lookup from attempt id to its 1-based
// position in its rerun chain, computed from the app's own attempts.
func rerunChainPositions(attempts []store.DeployAttempt) func(id string) int {
	parents := make(map[string]string, len(attempts))
	for _, a := range attempts {
		parents[a.ID] = a.RerunOf
	}
	maxDepth := envIntDefault(envRerunChainMaxDepth, defaultRerunChainMaxDepth)
	return func(id string) int {
		return store.RerunChainPosition(id, func(cur string) string { return parents[cur] }, maxDepth)
	}
}
