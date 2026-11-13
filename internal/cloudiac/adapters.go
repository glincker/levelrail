package cloudiac

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/GLINCKER/levelrail/internal/iac"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

// AccountInfo is what a CloudAccount document resolves to; secrets stay behind the handle.
type AccountInfo struct {
	Region          string
	CompartmentOCID string
	Namespace       string
	Credential      CredentialHandle
}

// AccountResolver looks up a CloudAccount by name.
type AccountResolver interface {
	ResolveAccount(ctx context.Context, name string) (AccountInfo, error)
}

// DetailedPlanner is the engine surface the adapter needs; TofuEngine implements it.
type DetailedPlanner interface {
	PlanDetailed(ctx context.Context, spec ModuleSpec, cred CredentialHandle) (PlanResult, error)
}

// CloudPlannerAdapter implements iac.CloudPlanner on top of the engine, one stack per document.
type CloudPlannerAdapter struct {
	Engine   DetailedPlanner
	Accounts AccountResolver
}

var _ iac.CloudPlanner = CloudPlannerAdapter{}

// PlanCloud renders the document, plans it and folds the changes into one diff.
func (a CloudPlannerAdapter) PlanCloud(r iac.CloudResource) (iac.CloudDiff, error) {
	if r.Kind == iac.KindCloudAccount {
		return iac.CloudDiff{Action: "noop"}, nil
	}
	if a.Engine == nil || a.Accounts == nil {
		return iac.CloudDiff{}, ErrNotConfigured
	}
	ctx := context.Background()
	acct, _ := r.Spec["account"].(string)
	if acct == "" {
		return iac.CloudDiff{}, fmt.Errorf("spec.account is required")
	}
	info, err := a.Accounts.ResolveAccount(ctx, acct)
	if err != nil {
		return iac.CloudDiff{}, err
	}
	oci := OCISpec{Name: kindStack(r.Kind) + "-" + r.Name, Region: info.Region,
		CompartmentOCID: info.CompartmentOCID, Namespace: info.Namespace}
	switch r.Kind {
	case iac.KindCloudNetwork:
		cidr, _ := r.Spec["cidr"].(string)
		oci.Networks = []NetworkSpec{{Name: r.Name, CIDR: cidr, IngressPorts: ingressPorts(r.Spec["ingress"])}}
	case iac.KindCloudBucket:
		oci.Buckets = []BucketSpec{{Name: r.Name}}
	default:
		return iac.CloudDiff{}, fmt.Errorf("%s planning is not available yet", r.Kind)
	}
	mod, err := RenderOCI(oci)
	if err != nil {
		return iac.CloudDiff{}, err
	}
	pr, err := a.Engine.PlanDetailed(ctx, mod, info.Credential)
	if err != nil {
		return iac.CloudDiff{}, err
	}
	return foldDiff(pr), nil
}

func kindStack(k iac.Kind) string {
	switch k {
	case iac.KindCloudNetwork:
		return "net"
	case iac.KindCloudBucket:
		return "bucket"
	}
	return "res"
}

func ingressPorts(v any) []int {
	list, _ := v.([]any)
	var out []int
	for _, x := range list {
		m, _ := x.(map[string]any)
		switch p := m["port"].(type) {
		case int:
			out = append(out, p)
		case int64:
			out = append(out, int(p))
		case uint64:
			out = append(out, int(p)) //nolint:gosec // RenderOCI rejects out of range ports
		case float64:
			out = append(out, int(p))
		}
	}
	return out
}

// actionRank orders actions so the most destructive one names the diff.
var actionRank = map[string]int{ActionCreate: 1, ActionUpdate: 2, ActionReplace: 3, ActionDelete: 4}

// foldDiff reduces a stack plan to one diff; a replace reports as update with a lifecycle attr.
func foldDiff(pr PlanResult) iac.CloudDiff {
	if len(pr.Changes) == 0 {
		return iac.CloudDiff{Action: "noop"}
	}
	top := pr.Changes[0]
	var attrs []iac.CloudAttr
	for _, c := range pr.Changes {
		if actionRank[c.Action] > actionRank[top.Action] {
			top = c
		}
		if c.Action == ActionReplace {
			attrs = append(attrs, iac.CloudAttr{Path: c.Address + ".lifecycle", New: "replace"})
		}
		for _, at := range c.Attrs {
			attrs = append(attrs, iac.CloudAttr{Path: c.Address + "." + at.Path, Old: at.Old, New: at.New, Sensitive: at.Sensitive})
		}
	}
	sort.Slice(attrs, func(i, j int) bool { return attrs[i].Path < attrs[j].Path })
	action := top.Action
	if action == ActionReplace {
		action = "update"
	}
	return iac.CloudDiff{Action: action, Type: top.Type, Attrs: attrs}
}

// PolicyAdapter exposes Policy.Check as an iac.PolicyChecker, reading the policy per call.
type PolicyAdapter struct {
	Policy func() Policy
}

var _ iac.PolicyChecker = PolicyAdapter{}

// CheckCloud implements iac.PolicyChecker. A nil policy func stays strict.
func (p PolicyAdapter) CheckCloud(changes []iac.CloudChange) []iac.CloudRefusal {
	pol := NewPolicy(0, false)
	if p.Policy != nil {
		pol = p.Policy()
	}
	rc := make([]ResourceChange, 0, len(changes))
	for _, c := range changes {
		rc = append(rc, ResourceChange{Address: c.Address, Type: c.Type, Action: c.Action})
	}
	var out []iac.CloudRefusal
	for _, r := range pol.Check(rc) {
		out = append(out, iac.CloudRefusal{Code: r.Code, Address: r.Address, Message: r.Message})
	}
	return out
}

// InstallInto wires the planner and policy into iac only when the cloud flag is on.
// With it off nothing is installed and cloud documents keep failing loudly.
func InstallInto(enabled bool, planner iac.CloudPlanner, policy iac.PolicyChecker) bool {
	if !enabled || planner == nil || policy == nil {
		return false
	}
	iac.SetCloudPlanner(planner)
	iac.SetCloudPolicy(policy)
	return true
}

// SettingsReader is the runtime settings surface the engine reads per run.
type SettingsReader interface {
	Int(ctx context.Context, key string) int
	Bool(ctx context.Context, key string) bool
}

// SettingsConfig fills the live knobs of a TofuConfig from runtime settings.
// A nil reader keeps every guard at its strictest value.
func SettingsConfig(cfg TofuConfig, get func() SettingsReader) TofuConfig {
	ctx := context.Background()
	read := func() SettingsReader {
		if get == nil {
			return nil
		}
		return get()
	}
	cfg.Timeout = func() time.Duration {
		if s := read(); s != nil {
			return time.Duration(s.Int(ctx, runtimesettings.KeyCloudEngineTimeoutSeconds)) * time.Second
		}
		return 0
	}
	cfg.Policy = func() Policy {
		if s := read(); s != nil {
			return NewPolicy(s.Int(ctx, runtimesettings.KeyCloudMaxDestroys), s.Bool(ctx, runtimesettings.KeyCloudAllowProtectedDelete))
		}
		return NewPolicy(0, false)
	}
	cfg.Cost = func() CostPolicy {
		if s := read(); s != nil {
			limit := int64(s.Int(ctx, runtimesettings.KeyCloudDefaultMonthlyCostLim)) * 1_000_000
			return NewCostPolicy(limit, s.Bool(ctx, runtimesettings.KeyCloudRequireFullEstimate))
		}
		return NewCostPolicy(0, true)
	}
	if cfg.Prices == nil {
		cfg.Prices = StaticPrices{Date: "none"}
	}
	return cfg
}
