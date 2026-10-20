package mcptools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// PlanChange is one field a planned call would change. Values for secrets
// and env are never included, only whether they are set.
type PlanChange struct {
	Field  string `json:"field"`
	Before string `json:"before"`
	After  string `json:"after"`
}

// PlanResult is what plan_change returns: what a mutating tool call would do,
// computed from reads only. Executed is always false.
type PlanResult struct {
	Tool      string       `json:"tool"`
	DryRun    bool         `json:"dry_run"`
	Executed  bool         `json:"executed"`
	Plannable bool         `json:"plannable"`
	Summary   string       `json:"summary"`
	Changes   []PlanChange `json:"changes,omitempty"`
	// Blocked is true when a freeze window or a required approval would stop
	// the call from taking effect now.
	Blocked  bool     `json:"blocked"`
	Blockers []string `json:"blockers,omitempty"`
	Note     string   `json:"note,omitempty"`
	// Detail carries a richer plan when one already exists (a resource plan,
	// a promotion preview).
	Detail map[string]any `json:"detail,omitempty"`
}

type planChangeInput struct {
	Tool      string         `json:"tool" jsonschema:"the mutating tool to plan, e.g. deploy_app, set_app_env, set_app_domains"`
	Arguments map[string]any `json:"arguments,omitempty" jsonschema:"the arguments you would pass to that tool"`
}

// planner computes a PlanResult for one mutating tool from reads only.
type planner func(ctx context.Context, c *apiclient.Client, args json.RawMessage) (PlanResult, error)

// planners maps a mutating tool name to its planner. A mutating tool with no
// entry reports itself as not plannable instead of being executed.
var planners = map[string]planner{
	"set_app_env":      planSetAppEnv,
	"unset_app_env":    planUnsetAppEnv,
	"set_app_domains":  planSetAppDomains,
	"deploy_app":       planDeployApp,
	"rollback_app":     planDeployApp,
	"restart_app":      planRestartApp,
	"promote_app":      planPromoteApp,
	"apply_resources":  planApplyResources,
	"cancel_deploy":    planCancelDeploy,
	"bulk_apps":        planBulk(""),
	"bulk_delete_apps": planBulk("delete"),
}

// PlannableTools returns the sorted names of mutating tools plan_change can plan.
func PlannableTools() []string {
	names := make([]string, 0, len(planners))
	for n := range planners {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func registerPlanTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "plan_change",
		Description: "Dry run of a mutating tool call: pass tool and the arguments you would use, get what would change (before and after), whether a freeze window or approval would block it, and the resulting image. Reads only, never executes. Tools that cannot be planned say so and are not run.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in planChangeInput) (*mcp.CallToolResult, PlanResult, error) {
		res, err := planCall(ctx, client, in.Tool, in.Arguments)
		if err != nil {
			return nil, PlanResult{}, err
		}
		return nil, res, nil
	})
}

func planCall(ctx context.Context, c *apiclient.Client, tool string, arguments map[string]any) (PlanResult, error) {
	meta, ok := toolTable[tool]
	if !ok {
		return PlanResult{}, fmt.Errorf("unknown tool %q", tool)
	}
	base := PlanResult{Tool: tool, DryRun: true}
	if meta.Class == ClassRead {
		base.Summary = tool + " is read-only, nothing to plan: call it directly."
		return base, nil
	}
	p, ok := planners[tool]
	if !ok {
		base.Summary = tool + " cannot be planned. It was not run."
		base.Note = "No dry run exists for this tool. Confirm with the user before calling it for real."
		return base, nil
	}
	raw, err := json.Marshal(arguments)
	if err != nil {
		return PlanResult{}, fmt.Errorf("encode arguments for %s: %w", tool, err)
	}
	res, err := p(ctx, c, raw)
	if err != nil {
		return PlanResult{}, fmt.Errorf("plan %s: %w", tool, err)
	}
	res.Tool, res.DryRun, res.Executed, res.Plannable = tool, true, false, true
	res.Blocked = len(res.Blockers) > 0
	return res, nil
}

// asDetail converts a plan value into the generic object PlanResult.Detail
// carries; the shapes differ per tool, so a typed field is not possible.
func asDetail(v any) (map[string]any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func decodeArgs(raw json.RawMessage, into any) error {
	if err := json.Unmarshal(raw, into); err != nil {
		return fmt.Errorf("invalid arguments: %w", err)
	}
	return nil
}

func envState(set bool) string {
	if set {
		return "set (value hidden)"
	}
	return "unset"
}

func planSetAppEnv(ctx context.Context, c *apiclient.Client, raw json.RawMessage) (PlanResult, error) {
	var in setAppEnvInput
	if err := decodeArgs(raw, &in); err != nil {
		return PlanResult{}, err
	}
	if err := validateEnvTarget(in.Name, in.Key); err != nil {
		return PlanResult{}, err
	}
	app, err := c.GetApp(ctx, in.Name)
	if err != nil {
		return PlanResult{}, fmt.Errorf("get app %q: %w", in.Name, err)
	}
	var before bool
	var unchanged bool
	if in.Secret {
		before = slices.Contains(app.SecretEnv, in.Key)
	} else {
		cur, exists := app.Env[in.Key]
		before, unchanged = exists, exists && cur == in.Value
	}
	res := PlanResult{Summary: fmt.Sprintf("set %s on %s (saved to desired state, redeploy needed to apply)", in.Key, in.Name)}
	field := "env." + in.Key
	if in.Secret {
		field = "secret." + in.Key
	}
	if unchanged {
		res.Summary = fmt.Sprintf("%s already has this value on %s: no change", in.Key, in.Name)
		return res, nil
	}
	after := "set (new value hidden)"
	res.Changes = []PlanChange{{Field: field, Before: envState(before), After: after}}
	return res, nil
}

func planUnsetAppEnv(ctx context.Context, c *apiclient.Client, raw json.RawMessage) (PlanResult, error) {
	var in unsetAppEnvInput
	if err := decodeArgs(raw, &in); err != nil {
		return PlanResult{}, err
	}
	if err := validateEnvTarget(in.Name, in.Key); err != nil {
		return PlanResult{}, err
	}
	app, err := c.GetApp(ctx, in.Name)
	if err != nil {
		return PlanResult{}, fmt.Errorf("get app %q: %w", in.Name, err)
	}
	field, exists := "env."+in.Key, false
	if in.Secret {
		field, exists = "secret."+in.Key, slices.Contains(app.SecretEnv, in.Key)
	} else {
		_, exists = app.Env[in.Key]
	}
	if !exists {
		return PlanResult{Summary: fmt.Sprintf("%s is not set on %s: no change", in.Key, in.Name)}, nil
	}
	return PlanResult{
		Summary: fmt.Sprintf("remove %s from %s (saved to desired state, redeploy needed to apply)", in.Key, in.Name),
		Changes: []PlanChange{{Field: field, Before: envState(true), After: envState(false)}},
	}, nil
}

func planSetAppDomains(ctx context.Context, c *apiclient.Client, raw json.RawMessage) (PlanResult, error) {
	var in setAppDomainsInput
	if err := decodeArgs(raw, &in); err != nil {
		return PlanResult{}, err
	}
	app, err := c.GetApp(ctx, in.Name)
	if err != nil {
		return PlanResult{}, fmt.Errorf("get app %q: %w", in.Name, err)
	}
	if slices.Equal(sortedCopy(app.Domains), sortedCopy(in.Domains)) {
		return PlanResult{Summary: in.Name + " already has exactly these domains: no change"}, nil
	}
	return PlanResult{
		Summary: fmt.Sprintf("replace the domain list of %s", in.Name),
		Changes: []PlanChange{{Field: "domains", Before: strings.Join(app.Domains, ","), After: strings.Join(in.Domains, ",")}},
		Note:    "A domain already used by another app is refused when the call runs.",
	}, nil
}

func sortedCopy(in []string) []string {
	out := slices.Clone(in)
	sort.Strings(out)
	return out
}

func planDeployApp(ctx context.Context, c *apiclient.Client, raw json.RawMessage) (PlanResult, error) {
	var in deployAppInput
	if err := decodeArgs(raw, &in); err != nil {
		return PlanResult{}, err
	}
	if in.Name == "" || in.Image == "" {
		return PlanResult{}, errors.New("name and image are required")
	}
	app, err := c.GetApp(ctx, in.Name)
	if err != nil {
		return PlanResult{}, fmt.Errorf("get app %q: %w", in.Name, err)
	}
	res := PlanResult{Summary: fmt.Sprintf("deploy %s to image %s", in.Name, in.Image)}
	digest := "resolved when the deploy runs"
	if _, d, ok := strings.Cut(in.Image, "@"); ok {
		digest = d
	}
	res.Changes = []PlanChange{
		{Field: "image", Before: app.Image, After: in.Image},
		{Field: "image_digest", Before: emptyAs(app.ImageDigest, "unknown"), After: digest},
	}
	if freeze, ferr := c.GetDeployFreeze(ctx, in.Name); ferr == nil && freeze.Status.Frozen {
		msg := "a deploy freeze window is active"
		if freeze.Status.Reason != "" {
			msg += ": " + freeze.Status.Reason
		}
		if freeze.Status.Until != nil {
			msg += " until " + freeze.Status.Until.Format("2006-01-02T15:04:05Z07:00")
		}
		res.Blockers = append(res.Blockers, msg+" (the deploy would be held)")
	}
	if app.EnvironmentID != "" {
		res.Note = "If the app's environment is protected, the call needs confirm true and may wait for a deploy approval."
	}
	return res, nil
}

func emptyAs(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

func planRestartApp(ctx context.Context, c *apiclient.Client, raw json.RawMessage) (PlanResult, error) {
	var in appNameInput
	if err := decodeArgs(raw, &in); err != nil {
		return PlanResult{}, err
	}
	app, err := c.GetApp(ctx, in.Name)
	if err != nil {
		return PlanResult{}, fmt.Errorf("get app %q: %w", in.Name, err)
	}
	res := PlanResult{Summary: fmt.Sprintf("recreate the running container of %s on the same image", in.Name)}
	res.Changes = []PlanChange{{Field: "container", Before: "running " + app.Image, After: "recreated from " + app.Image}}
	if app.EnvDirty {
		res.Note = "Saved env changes not yet applied would take effect."
	}
	return res, nil
}

func planPromoteApp(ctx context.Context, c *apiclient.Client, raw json.RawMessage) (PlanResult, error) {
	var in promoteAppInput
	if err := decodeArgs(raw, &in); err != nil {
		return PlanResult{}, err
	}
	prev, err := c.PreviewPromotion(ctx, in.Name, in.To, in.Target)
	if err != nil {
		return PlanResult{}, fmt.Errorf("preview promotion of %q: %w", in.Name, err)
	}
	detail, err := asDetail(prev)
	if err != nil {
		return PlanResult{}, fmt.Errorf("encode promotion preview: %w", err)
	}
	res := PlanResult{
		Summary:  fmt.Sprintf("promote %s to %s", prev.SourceApp, prev.TargetApp),
		Blockers: append([]string(nil), prev.Blockers...),
		Detail:   detail,
	}
	for _, ch := range prev.Changes {
		res.Changes = append(res.Changes, PlanChange{Field: ch.Field, Before: ch.From, After: ch.To})
	}
	if prev.NeedsConfirmation {
		res.Note = "The destination environment is protected: the call needs confirm true and may wait for approval."
	}
	return res, nil
}

func planApplyResources(ctx context.Context, c *apiclient.Client, raw json.RawMessage) (PlanResult, error) {
	var in planApplyInput
	if err := decodeArgs(raw, &in); err != nil {
		return PlanResult{}, err
	}
	plan, err := c.PlanIaC(ctx, in.request())
	if err != nil {
		return PlanResult{}, fmt.Errorf("plan resource files: %w", err)
	}
	detail, err := asDetail(plan)
	if err != nil {
		return PlanResult{}, fmt.Errorf("encode plan: %w", err)
	}
	return PlanResult{Summary: "apply the resource files (see detail for the per resource plan)", Detail: detail,
		Note: "Pass the plan's hash as expected_plan_hash when applying."}, nil
}

func planCancelDeploy(ctx context.Context, c *apiclient.Client, raw json.RawMessage) (PlanResult, error) {
	var in cancelDeployInput
	if err := decodeArgs(raw, &in); err != nil {
		return PlanResult{}, err
	}
	d, err := c.GetDeploy(ctx, in.Name, in.DeployID)
	if err != nil {
		return PlanResult{}, fmt.Errorf("get deploy %q of %q: %w", in.DeployID, in.Name, err)
	}
	switch d.Status {
	case "queued", "running", "held":
		return PlanResult{Summary: fmt.Sprintf("cancel deploy %s of %s before it cuts traffic", d.ID, in.Name),
			Changes: []PlanChange{{Field: "deploy.status", Before: d.Status, After: "canceled"}}}, nil
	default:
		return PlanResult{Summary: fmt.Sprintf("deploy %s is already %s: nothing to cancel", d.ID, d.Status),
			Note: "A deploy that already cut over needs rollback_app instead."}, nil
	}
}
