package cloudiac

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
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

// CloudPlannerAdapter implements iac.CloudStackPlanner: every cloud document of one Levelrail
// stack and account is rendered into one module with one sealed state.
type CloudPlannerAdapter struct {
	Engine   DetailedPlanner
	Accounts AccountResolver
}

// SavedPlanRunner runs an approved saved plan; TofuEngine implements it.
type SavedPlanRunner interface {
	Apply(ctx context.Context, req ApplyRequest) (Result, error)
	Destroy(ctx context.Context, req ApplyRequest) (Result, error)
}

var (
	_ iac.CloudPlanner      = CloudPlannerAdapter{}
	_ iac.CloudStackPlanner = CloudPlannerAdapter{}
	_ iac.CloudReadiness    = CloudPlannerAdapter{}
	_ iac.CloudApplier      = CloudPlannerAdapter{}
)

// CloudReady reports false until both the engine and the account resolver are set, so
// even record-only kinds fail as unavailable instead of reporting unchanged.
func (a CloudPlannerAdapter) CloudReady() bool { return a.Engine != nil && a.Accounts != nil }

// ApplyCloud re-renders the account group of the target exactly as planned and runs the
// saved plan the approval names; the engine refuses any other hash.
func (a CloudPlannerAdapter) ApplyCloud(ctx context.Context, req iac.CloudApplyRequest) (iac.CloudApplyResult, error) {
	runner, ok := a.Engine.(SavedPlanRunner)
	if !ok || a.Accounts == nil {
		return iac.CloudApplyResult{}, ErrNotConfigured
	}
	acct, _ := req.Target.Spec["account"].(string)
	var idx []int
	for i, r := range req.Group {
		if g, _ := r.Spec["account"].(string); r.Kind != iac.KindCloudAccount && g == acct {
			idx = append(idx, i)
		}
	}
	if acct == "" || len(idx) == 0 {
		return iac.CloudApplyResult{}, fmt.Errorf("no cloud documents for account %q", acct)
	}
	mod, info, _, err := a.renderGroup(ctx, StackName(req.Stack), acct, req.Group, idx)
	if err != nil {
		return iac.CloudApplyResult{}, err
	}
	ar := ApplyRequest{Spec: mod, Cred: info.Credential, PlanHash: req.PlanHash, ConfirmDestroy: req.Destroy}
	run := runner.Apply
	if req.Destroy {
		run = runner.Destroy
	}
	res, err := run(ctx, ar)
	out := iac.CloudApplyResult{Diverged: res.Diverged}
	if q := (*StateQuarantine)(nil); errors.As(err, &q) {
		out.Diverged, out.QuarantineID = true, q.ID
	}
	return out, err
}

// DefaultStack names the module when an apply source carries no name.
const DefaultStack = "default"

// PlanCloud plans one document as a stack of its own; apply uses PlanCloudStack.
func (a CloudPlannerAdapter) PlanCloud(r iac.CloudResource) (iac.CloudDiff, error) {
	out := a.PlanCloudStack(DefaultStack, []iac.CloudResource{r})
	return out[0].Diff, out[0].Err
}

// PlanCloudStack groups the documents by account, plans one module per group and splits
// the changes back to the documents by resource address.
func (a CloudPlannerAdapter) PlanCloudStack(stack string, rs []iac.CloudResource) []iac.CloudOutcome {
	out := make([]iac.CloudOutcome, len(rs))
	groups := map[string][]int{}
	var order []string
	for i, r := range rs {
		if r.Kind == iac.KindCloudAccount {
			// An account is a reference, so it stays a no-op once its connection resolves.
			if a.Accounts == nil {
				out[i].Err = ErrNotConfigured
			} else if _, err := a.Accounts.ResolveAccount(context.Background(), r.Name); err != nil {
				out[i].Err = err
			} else {
				out[i].Diff = iac.CloudDiff{Action: "noop"}
			}
			continue
		}
		acct, _ := r.Spec["account"].(string)
		if acct == "" {
			out[i].Err = fmt.Errorf("spec.account is required")
			continue
		}
		if _, ok := groups[acct]; !ok {
			order = append(order, acct)
		}
		groups[acct] = append(groups[acct], i)
	}
	for _, acct := range order {
		idx := groups[acct]
		diffs, err := a.planGroup(StackName(stack), acct, rs, idx)
		for n, i := range idx {
			if err != nil {
				out[i].Err = err
				continue
			}
			out[i].Diff = diffs[n]
		}
	}
	return out
}

// StackName maps an apply source to a module name; the state key is the account plus this name.
func StackName(source string) string {
	b := []byte(strings.ToLower(source))
	for i, c := range b {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') {
			b[i] = '-'
		}
	}
	n := strings.Trim(string(b), "-")
	if n == "" {
		return DefaultStack
	}
	if n[0] < 'a' {
		n = "s-" + n
	}
	if len(n) > 63 {
		n = strings.TrimRight(n[:63], "-")
	}
	return n
}

func (a CloudPlannerAdapter) planGroup(stack, acct string, rs []iac.CloudResource, idx []int) ([]iac.CloudDiff, error) {
	if a.Engine == nil || a.Accounts == nil {
		return nil, ErrNotConfigured
	}
	ctx := context.Background()
	mod, info, owner, err := a.renderGroup(ctx, stack, acct, rs, idx)
	if err != nil {
		return nil, err
	}
	pr, err := a.Engine.PlanDetailed(ctx, mod, info.Credential)
	if err != nil {
		return nil, err
	}
	split := make([]PlanResult, len(idx))
	for _, c := range pr.Changes {
		n, ok := owner[addressOwner(c)]
		if !ok {
			// Removing a resource from a stack is a destroy; it never rides along with an apply.
			return nil, fmt.Errorf("the plan changes %s, which no document in stack %q declares; use destroy to remove it", c.Address, stack)
		}
		split[n].Changes = append(split[n].Changes, c)
	}
	diffs := make([]iac.CloudDiff, len(idx))
	for n := range idx {
		diffs[n] = foldDiff(split[n])
		diffs[n].PlanHash = pr.Hash
	}
	return diffs, nil
}

// renderGroup renders one account group into a module; plan and apply share it so the
// module an approved plan runs against is the one it was made from.
func (a CloudPlannerAdapter) renderGroup(ctx context.Context, stack, acct string, rs []iac.CloudResource, idx []int) (ModuleSpec, AccountInfo, map[string]int, error) {
	info, err := a.Accounts.ResolveAccount(ctx, acct)
	if err != nil {
		return ModuleSpec{}, info, nil, err
	}
	oci := OCISpec{Name: stack, Region: info.Region, CompartmentOCID: info.CompartmentOCID}
	owner := map[string]int{}
	for n, i := range idx {
		r := rs[i]
		switch r.Kind {
		case iac.KindCloudNetwork:
			cidr, _ := r.Spec["cidr"].(string)
			oci.Networks = append(oci.Networks, NetworkSpec{Name: r.Name, CIDR: cidr, IngressPorts: ingressPorts(r.Spec["ingress"])})
			owner["network/"+tfName(r.Name)] = n
		case iac.KindCloudBucket:
			ns, _ := r.Spec["namespace"].(string)
			if ns == "" {
				return ModuleSpec{}, info, nil, fmt.Errorf("CloudBucket %q needs spec.namespace, the object storage namespace of the account", r.Name)
			}
			oci.Buckets = append(oci.Buckets, BucketSpec{Name: r.Name, Namespace: ns})
			owner["bucket/"+tfName(r.Name)] = n
		case iac.KindCloudInstance:
			inst, err := instanceSpec(r)
			if err != nil {
				return ModuleSpec{}, info, nil, err
			}
			oci.Instances = append(oci.Instances, inst)
			owner["instance/"+tfName(r.Name)] = n
		default:
			return ModuleSpec{}, info, nil, fmt.Errorf("%s planning is not available yet", r.Kind)
		}
	}
	for _, inst := range oci.Instances {
		if _, ok := owner["network/"+tfName(inst.Network)]; !ok {
			return ModuleSpec{}, info, nil, fmt.Errorf("CloudInstance %q: CloudNetwork %q must be declared in the same stack and account", inst.Name, inst.Network)
		}
	}
	mod, err := RenderOCI(oci)
	return mod, info, owner, err
}

// addressOwner maps a module address to the document kind and name that rendered it.
func addressOwner(c PlanChange) string {
	_, name, _ := strings.Cut(c.Address, ".")
	switch c.Type {
	case "oci_core_vcn", "oci_core_internet_gateway", "oci_core_route_table", "oci_core_security_list", "oci_core_subnet":
		return "network/" + name
	case "oci_objectstorage_bucket":
		return "bucket/" + name
	case "oci_core_instance":
		return "instance/" + name
	}
	return ""
}

// Default flexible shapes per architecture; spec.shape.name overrides them.
const (
	shapeARM = "VM.Standard.A1.Flex"
	shapeX86 = "VM.Standard.E5.Flex"
)

func instanceSpec(r iac.CloudResource) (InstanceSpec, error) {
	netName, _ := r.Spec["network"].(string)
	if netName == "" {
		return InstanceSpec{}, fmt.Errorf("CloudInstance %q: spec.network is required", r.Name)
	}
	shape, _ := r.Spec["shape"].(map[string]any)
	cpu, _ := anyInt(shape["cpu"])
	mem, _ := anyInt(shape["memoryGB"])
	name, _ := shape["name"].(string)
	if name == "" {
		name = shapeX86
		if arch, _ := shape["arch"].(string); arch == "arm64" {
			name = shapeARM
		}
	}
	ad, _ := r.Spec["availabilityDomain"].(string)
	img, _ := r.Spec["image"].(string)
	return InstanceSpec{Name: r.Name, Network: netName, Shape: name, OCPUs: cpu, MemoryGB: mem,
		AvailabilityDomain: ad, ImageOCID: img}, nil
}

func ingressPorts(v any) []int {
	list, _ := v.([]any)
	var out []int
	for _, x := range list {
		m, _ := x.(map[string]any)
		if p, ok := anyInt(m["port"]); ok {
			out = append(out, p)
		}
	}
	return out
}

// anyInt reads a decoded YAML or JSON number; RenderOCI range checks the result.
func anyInt(v any) (int, bool) {
	switch p := v.(type) {
	case int:
		return p, true
	case int64:
		return int(p), true
	case uint64:
		return int(p), true //nolint:gosec // range checked by RenderOCI
	case float64:
		return int(p), true
	}
	return 0, false
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
	cfg.PlanMaxAge = func() time.Duration {
		if s := read(); s != nil {
			return time.Duration(s.Int(ctx, runtimesettings.KeyCloudPlanMaxAgeMinutes)) * time.Minute
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
