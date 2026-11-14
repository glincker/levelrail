package cloudiac

import (
	"context"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/iac"
)

var updateGolden = flag.Bool("update-golden", false, "rewrite cloudiac golden files")

const stderrCanary = "lr-canary-stderr-5d1f7b3e" //nolint:gosec // leak canary, not a secret

// sharingRunner writes the saved plan on the plan step and requires it on the show step.
type sharingRunner struct {
	fakeRunner
	sawPlan bool
	modRW   bool
	failAt  string
	stderr  string
}

func mountSource(spec SandboxSpec, target string) (Mount, bool) {
	for _, m := range spec.Mounts {
		if m.Target == target {
			return m, true
		}
	}
	return Mount{}, false
}

func (s *sharingRunner) Run(ctx context.Context, spec SandboxSpec) (RunOutput, error) {
	st, ok := mountSource(spec, SandboxStateDir)
	if !ok || !st.ReadWrite {
		return RunOutput{}, errors.New("state dir is not a read-write run dir mount")
	}
	if m, ok := mountSource(spec, SandboxModuleDir); !ok || m.ReadWrite {
		s.modRW = true
	}
	if c, ok := mountSource(spec, SandboxCredentialDir+"/credential"); !ok || c.ReadWrite {
		s.modRW = true
	}
	plan := filepath.Join(st.Source, "plan.tfplan")
	switch spec.Args[1] {
	case "plan":
		if err := os.WriteFile(plan, []byte("saved plan"), 0o600); err != nil {
			return RunOutput{}, err
		}
	case "show":
		if _, err := os.Stat(plan); err == nil {
			s.sawPlan = true
		}
	}
	if spec.Args[1] == s.failAt {
		return RunOutput{Stderr: []byte(s.stderr)}, errStepExit
	}
	return s.fakeRunner.Run(ctx, spec)
}

func withRunner(h *harness, r Runner) { h.eng.deps.Runner = r }

func assertEmptyDir(t *testing.T, dir string) {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ents) != 0 {
		t.Fatalf("run dir left behind: %v", ents)
	}
}

func TestTofuStepsShareRunDir(t *testing.T) {
	h := newHarness(t, nil, 0)
	r := &sharingRunner{fakeRunner: fakeRunner{out: map[string]RunOutput{"show": {Stdout: []byte(`{"resource_changes":[]}`)}}}}
	withRunner(h, r)
	if _, err := h.eng.PlanDetailed(context.Background(), testModule(), CredentialHandle{ID: "oci-test"}); err != nil {
		t.Fatal(err)
	}
	if !r.sawPlan {
		t.Fatal("show step did not see the plan file written by the plan step")
	}
	if r.modRW {
		t.Fatal("module or credential mount was writable")
	}
	assertEmptyDir(t, h.work)
}

func TestTofuRunDirCleanedOnFailure(t *testing.T) {
	h := newHarness(t, nil, 0)
	r := &sharingRunner{fakeRunner: fakeRunner{out: map[string]RunOutput{}}, failAt: "plan", stderr: "boom"}
	withRunner(h, r)
	if _, err := h.eng.PlanDetailed(context.Background(), testModule(), CredentialHandle{ID: "oci-test"}); err == nil {
		t.Fatal("want failure")
	}
	assertEmptyDir(t, h.work)
}

func TestTofuNotConfiguredMessages(t *testing.T) {
	cases := map[string]struct {
		mut  func(*harness)
		want string
	}{
		"image":   {func(h *harness) { h.eng.cfg.Image = "" }, "cloud.engine_image"},
		"network": {func(h *harness) { h.eng.cfg.Network = NetworkPolicy{} }, "cloud.engine_network"},
		"runtime": {func(h *harness) { h.eng.cfg.Runtime = "" }, "docker or podman"},
		"binary":  {func(h *harness) { h.eng.deps.Binary = nil }, "OpenTofu binary"},
		"creds":   {func(h *harness) { h.eng.deps.Creds = nil }, "credential resolver"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, nil, 0)
			tc.mut(h)
			_, err := h.eng.PlanDetailed(context.Background(), testModule(), CredentialHandle{ID: "oci-test"})
			if !errors.Is(err, ErrNotConfigured) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want not configured naming %q, got %v", tc.want, err)
			}
		})
	}
}

func TestTofuStderrCanaryRedacted(t *testing.T) {
	h := newHarness(t, nil, 0)
	ctx := context.Background()
	lock, err := h.db.AcquireCloudStateLock(ctx, "oci-test", "net-lr", "seed", time.Minute, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	state := `{"version":4,"resources":[{"instances":[{"attributes":{"id":"ocid1.vcn","admin_password":"` + stderrCanary +
		`"},"sensitive_attributes":[[{"type":"get_attr","value":"admin_password"}]]}]}]}`
	if _, err := h.state.Save(ctx, lock, 0, []byte(state)); err != nil {
		t.Fatal(err)
	}
	if err := h.db.ReleaseCloudStateLock(ctx, lock); err != nil {
		t.Fatal(err)
	}
	noise := strings.Repeat("x", 3*stderrTailSize)
	r := &sharingRunner{fakeRunner: fakeRunner{out: map[string]RunOutput{}}, failAt: "plan",
		stderr: noise + "\nError: value \"" + stderrCanary + "\" rejected\n" + h.pem}
	withRunner(h, r)
	_, err = h.eng.PlanDetailed(ctx, testModule(), CredentialHandle{ID: "oci-test"})
	if err == nil {
		t.Fatal("want failure")
	}
	msg := err.Error()
	if strings.Contains(msg, stderrCanary) || strings.Contains(msg, "PRIVATE KEY") {
		t.Fatalf("sensitive value leaked: %s", msg)
	}
	if !strings.Contains(msg, redacted) || len(msg) > stderrTailSize+200 {
		t.Fatalf("want a redacted, capped tail, got %d bytes", len(msg))
	}
}

func TestSensitiveValuesFromPlan(t *testing.T) {
	plan := `{"resource_changes":[{"change":{"after":{"pw":"` + stderrCanary + `","name":"web"},"after_sensitive":{"pw":true}}}]}`
	got := SensitiveValues([]byte(plan))
	if len(got) != 1 || got[0] != stderrCanary {
		t.Fatalf("got %q", got)
	}
}

// capturePlanner records the module the adapter renders.
type capturePlanner struct {
	mod ModuleSpec
	res PlanResult
}

func (c *capturePlanner) PlanDetailed(_ context.Context, spec ModuleSpec, _ CredentialHandle) (PlanResult, error) {
	c.mod = spec
	return c.res, nil
}

type staticAccounts struct{}

func (staticAccounts) ResolveAccount(context.Context, string) (AccountInfo, error) {
	return AccountInfo{Region: "us-phoenix-1", CompartmentOCID: "ocid1.compartment.oc1..aaaaaaaacompunique01",
		Credential: CredentialHandle{ID: "oci-test"}}, nil
}

func TestStackPlannedAsOneModuleGolden(t *testing.T) {
	cp := &capturePlanner{}
	a := CloudPlannerAdapter{Engine: cp, Accounts: staticAccounts{}}
	inst := iac.CloudResource{Kind: iac.KindCloudInstance, Name: "worker-1", Spec: map[string]any{
		"account": "oci-test", "network": "lr-net", "availabilityDomain": "AD-1", "image": "ocid1.image.oc1..aaaaaaaaimageunique01",
		"shape": map[string]any{"cpu": 1, "memoryGB": 6, "arch": "arm64"},
	}}
	net := iac.CloudResource{Kind: iac.KindCloudNetwork, Name: "lr-net", Spec: map[string]any{
		"account": "oci-test", "cidr": "10.30.0.0/16", "ingress": []any{map[string]any{"port": 443}},
	}}
	bucket := iac.CloudResource{Kind: iac.KindCloudBucket, Name: "backups", Spec: map[string]any{"account": "oci-test"}}
	if out := a.PlanCloudStack(context.Background(), "prod", []iac.CloudResource{inst}); out[0].Err == nil || !strings.Contains(out[0].Err.Error(), "lr-net") {
		t.Fatalf("instance without its network must fail clearly, got %v", out[0].Err)
	}
	if out := a.PlanCloudStack(context.Background(), "prod", []iac.CloudResource{bucket}); out[0].Err == nil || !strings.Contains(out[0].Err.Error(), "spec.namespace") {
		t.Fatalf("bucket without namespace must name the field, got %v", out[0].Err)
	}
	bucket.Spec["namespace"] = "lrns"
	acct := iac.CloudResource{Kind: iac.KindCloudAccount, Name: "oci-test"}
	out := a.PlanCloudStack(context.Background(), "Prod Stack", []iac.CloudResource{inst, acct, net, bucket})
	for i, o := range out {
		if o.Err != nil || o.Diff.Action != "noop" {
			t.Fatalf("outcome %d = %+v", i, o)
		}
	}
	if cp.mod.Name != "prod-stack" {
		t.Fatalf("stack name %q", cp.mod.Name)
	}
	got := cp.mod.Files[MainFile]
	if !strings.Contains(string(got), "${oci_core_subnet.lr_net.id}") {
		t.Fatal("instance does not reference its network inside the module")
	}
	golden := filepath.Join("testdata", "stack_module.golden.json")
	if *updateGolden {
		if err := os.WriteFile(golden, got, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden) //nolint:gosec // fixed testdata path
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != string(got) {
		t.Fatalf("module drifted from golden\n got: %s", got)
	}
}

func TestStackPlanSplitsChangesAndRefusesOrphans(t *testing.T) {
	cp := &capturePlanner{res: PlanResult{Changes: []PlanChange{
		{Address: "oci_core_vcn.lr_net", Type: "oci_core_vcn", Action: ActionCreate},
		{Address: "oci_objectstorage_bucket.backups", Type: "oci_objectstorage_bucket", Action: ActionUpdate},
	}}}
	a := CloudPlannerAdapter{Engine: cp, Accounts: staticAccounts{}}
	net := iac.CloudResource{Kind: iac.KindCloudNetwork, Name: "lr-net", Spec: map[string]any{"account": "oci-test", "cidr": "10.30.0.0/16"}}
	bucket := iac.CloudResource{Kind: iac.KindCloudBucket, Name: "backups", Spec: map[string]any{"account": "oci-test", "namespace": "lrns"}}
	out := a.PlanCloudStack(context.Background(), "prod", []iac.CloudResource{net, bucket})
	if out[0].Diff.Action != ActionCreate || out[1].Diff.Action != ActionUpdate {
		t.Fatalf("outcomes = %+v", out)
	}
	out = a.PlanCloudStack(context.Background(), "prod", []iac.CloudResource{net})
	if out[0].Err == nil || !strings.Contains(out[0].Err.Error(), "destroy") {
		t.Fatalf("orphan change must refuse, got %+v", out[0])
	}
}
