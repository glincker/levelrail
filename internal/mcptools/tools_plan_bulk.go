package mcptools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// planBulk asks the API for its own dry run (dry_run is always forced true),
// so the target list comes from the same selection code the real call uses.
func planBulk(action string) planner {
	return func(ctx context.Context, c *apiclient.Client, raw json.RawMessage) (PlanResult, error) {
		req := apiclient.BulkAppsRequest{Action: action, DryRun: true}
		if action == "delete" {
			var in bulkDeleteAppsInput
			if err := decodeArgs(raw, &in); err != nil {
				return PlanResult{}, err
			}
			req.Names, req.Tag, req.Environment = in.Names, in.Tag, in.Environment
		} else {
			var in bulkAppsInput
			if err := decodeArgs(raw, &in); err != nil {
				return PlanResult{}, err
			}
			if in.Action == "delete" {
				return PlanResult{}, fmt.Errorf("use bulk_delete_apps to delete")
			}
			req.Action, req.Names, req.Tag, req.Environment, req.Value = in.Action, in.Names, in.Tag, in.Environment, in.Value
		}
		resp, err := c.BulkApps(ctx, req)
		if err != nil {
			return PlanResult{}, fmt.Errorf("bulk %s dry run: %w", req.Action, err)
		}
		detail, err := asDetail(resp)
		if err != nil {
			return PlanResult{}, fmt.Errorf("encode bulk dry run: %w", err)
		}
		return PlanResult{Summary: fmt.Sprintf("bulk %s would target %d apps (see detail for each)", req.Action, len(resp.Results)), Detail: detail}, nil
	}
}
