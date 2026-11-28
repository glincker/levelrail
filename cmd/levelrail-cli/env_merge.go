package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// envMergeFromPlan turns a plan into one column-scoped request: only keys that
// really change are sent, so the batch cannot overwrite a concurrent writer's other fields.
func envMergeFromPlan(plan envImportPlan, keepExisting bool, unset []string) apiclient.EnvMergeRequest {
	req := apiclient.EnvMergeRequest{Set: map[string]string{}, Unset: unset}
	for _, k := range plan.New {
		req.Set[k] = plan.Merged[k]
	}
	if !keepExisting {
		for _, k := range plan.Changed {
			req.Set[k] = plan.Merged[k]
		}
	}
	return req
}

// mergeEnv sends one versioned apply and wraps the error with the app name, never a value.
// base is the app as read; a concurrent change to the plan's own keys fails as a conflict, other keys retry.
func mergeEnv(ctx context.Context, client *Client, prog, name string, base apiclient.AppResource, rt envRetry, req apiclient.EnvMergeRequest) (apiclient.EnvMergeResult, error) {
	if len(req.Set) == 0 && len(req.Unset) == 0 {
		return apiclient.EnvMergeResult{App: name}, nil
	}
	res, err := sendEnvPlan(ctx, client, prog, name, base, apiclient.EnvApplyRequest{Set: req.Set, Unset: req.Unset}, rt)
	if err != nil {
		return apiclient.EnvMergeResult{}, fmt.Errorf("update env for app %q: %w", name, readableNetError(err))
	}
	out := apiclient.EnvMergeResult{App: name, Changed: res.Changed, EnvDirty: res.Changed}
	keys := make([]string, 0, len(res.Plan))
	for _, c := range res.Plan {
		keys = append(keys, c.Key)
	}
	out.RebuildKeys = buildTimeEnvKeys(keys)
	return out, nil
}

// wrapEnvConflict adds the reload command to a stale-version error so the next step is obvious.
func wrapEnvConflict(prog, name string, err error) error {
	var conflict *apiclient.EnvVersionConflictError
	if !errors.As(err, &conflict) {
		return err
	}
	return fmt.Errorf("%w\nnothing was changed: the env of app %q changed since it was read; review with \"%s apps env get %s\" and run the command again", err, name, prog, name)
}
