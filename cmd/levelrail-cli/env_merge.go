package main

import (
	"context"
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

// mergeEnv sends one batch and wraps the error with the app name, never a value.
func mergeEnv(ctx context.Context, client *Client, name string, req apiclient.EnvMergeRequest) (apiclient.EnvMergeResult, error) {
	res, err := client.MergeAppEnv(ctx, name, req)
	if err != nil {
		return res, fmt.Errorf("update env for app %q: %w", name, err)
	}
	return res, nil
}
