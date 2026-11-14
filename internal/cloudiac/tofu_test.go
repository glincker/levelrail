package cloudiac

import (
	"bytes"
	"context"
	"crypto/md5" //nolint:gosec // OCI fingerprint format in a fixture
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/cloudconn"
	"github.com/GLINCKER/levelrail/internal/iac"
	"github.com/GLINCKER/levelrail/internal/store"
)

const stateCanary = "lr-canary-state-7c4e1a9b"

// fakeRunner never execs; it returns canned output per tofu subcommand.
type fakeRunner struct {
	out      map[string]RunOutput
	fail     string
	calls    []string
	credSeen bool
	// applyState is the state the fake apply leaves behind; onApply runs first.
	applyState []byte
	onApply    func()
	// hold blocks that step until the context ends, like a hung provider call.
	hold string
}

func (f *fakeRunner) Run(ctx context.Context, spec SandboxSpec) (RunOutput, error) {
	sub := spec.Args[1]
	f.calls = append(f.calls, sub)
	if sub == f.hold {
		<-ctx.Done()
		return RunOutput{}, ctx.Err()
	}
	for _, m := range spec.Mounts {
		if m.Target != SandboxStateDir {
			continue
		}
		switch {
		case sub == "plan":
			_ = os.WriteFile(filepath.Join(m.Source, planFile), []byte("fake saved plan "+strings.Join(spec.Args, " ")), 0o600)
		case sub == "apply" && f.applyState != nil:
			if f.onApply != nil {
				f.onApply()
			}
			_ = os.WriteFile(filepath.Join(m.Source, tofuStateFile), f.applyState, 0o600)
		}
	}
	for _, m := range spec.Mounts {
		if m.Target == SandboxCredentialDir+"/credential" {
			if _, err := os.Stat(filepath.Join(m.Source, cloudconn.OracleKeyFileName)); err == nil {
				f.credSeen = true
			}
		}
	}
	if sub == f.fail {
		return f.out[sub], errStepExit
	}
	return f.out[sub], nil
}

type fakeBinary struct{ path string }

func (b fakeBinary) Ensure(context.Context) (string, error) { return b.path, nil }

type fakeCreds struct{ c cloudconn.OracleCredential }

func (f fakeCreds) ResolveOracle(context.Context, CredentialHandle) (cloudconn.OracleCredential, error) {
	return f.c, nil
}

func oracleFixture(t *testing.T) (cloudconn.OracleCredential, string) {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pemKey := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)})
	der, err := x509.MarshalPKIXPublicKey(&k.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	sum := md5.Sum(der) //nolint:gosec // fixture fingerprint
	parts := make([]string, len(sum))
	for i, b := range sum {
		parts[i] = fmt.Sprintf("%02x", b)
	}
	sec := cloudconn.NewOracleSecret("ocid1.user.oc1..aaaaaaaauserunique01", strings.Join(parts, ":"), pemKey)
	return cloudconn.OracleCredential{TenancyOCID: "ocid1.tenancy.oc1..aaaaaaaatenancyunique01",
		CompartmentOCID: "ocid1.compartment.oc1..aaaaaaaacompunique01", Region: "us-phoenix-1", Secret: sec}, string(pemKey)
}

type harness struct {
	eng    *TofuEngine
	db     *store.DB
	state  *StateStore
	work   string
	runner *fakeRunner
	pem    string
}

func newHarness(t *testing.T, prices map[string]int64, limit int64) *harness {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	db, err := store.Open(ctx, filepath.Join(dir, "levelrail.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ss, err := NewStateStore(db, fixedKeys{key: bytes.Repeat([]byte{9}, 32)}, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(dir, "work")
	bin := filepath.Join(dir, "tofu")
	if err := os.Mkdir(work, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("not a binary"), 0o600); err != nil {
		t.Fatal(err)
	}
	np, err := NewNetworkPolicy("lr-cloud-egress", []string{"10.20.0.5"})
	if err != nil {
		t.Fatal(err)
	}
	cred, pemKey := oracleFixture(t)
	r := &fakeRunner{out: map[string]RunOutput{}}
	cfg := TofuConfig{
		Runtime: "docker", Image: "cgr.example/static@sha256:" + strings.Repeat("a", 64), User: "65532:65532",
		WorkBase: work, Network: np,
		Limits:     SandboxLimits{MemoryMB: 512, CPUs: 1, Pids: 128, WorkTmpMB: 64, StateTmpMB: 16},
		Timeout:    func() time.Duration { return time.Minute },
		PlanMaxAge: func() time.Duration { return time.Hour },
		Policy:     func() Policy { return NewPolicy(1, false) },
		Cost:       func() CostPolicy { return NewCostPolicy(limit, true) },
		Prices:     StaticPrices{Date: "2026-11-01", Prices: prices},
	}
	eng := NewTofuEngine(cfg, TofuDeps{Runner: r, Binary: fakeBinary{bin}, Locks: db, State: ss, Creds: fakeCreds{cred}})
	return &harness{eng: eng, db: db, state: ss, work: work, runner: r, pem: pemKey}
}

func (h *harness) seedState(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	lock, err := h.db.AcquireCloudStateLock(ctx, "oci-test", "net-lr", "seed", time.Minute, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.state.Save(ctx, lock, 0, []byte(`{"secret":"`+stateCanary+`"}`)); err != nil {
		t.Fatal(err)
	}
	if err := h.db.ReleaseCloudStateLock(ctx, lock); err != nil {
		t.Fatal(err)
	}
}

func (h *harness) assertClean(t *testing.T) {
	t.Helper()
	ents, err := os.ReadDir(h.work)
	if err != nil || len(ents) != 0 {
		t.Fatalf("work base not empty after run: %v %v", ents, err)
	}
	lock, err := h.db.AcquireCloudStateLock(context.Background(), "oci-test", "net-lr", "probe", time.Minute, time.Now())
	if err != nil {
		t.Fatalf("lock still held after run: %v", err)
	}
	_ = h.db.ReleaseCloudStateLock(context.Background(), lock)
}

func testModule() ModuleSpec {
	return ModuleSpec{Name: "net-lr", Provider: "oci", Files: map[string][]byte{MainFile: []byte(`{}`)}}
}

const showJSON = `{"resource_changes":[
 {"address":"oci_core_vcn.lr","mode":"managed","type":"oci_core_vcn","change":{"actions":["create"],"before":null,"after":{"cidr_block":"10.30.0.0/16"},"after_unknown":{"id":true}}},
 {"address":"oci_core_subnet.lr","mode":"managed","type":"oci_core_subnet","change":{"actions":["update"],"before":{"dns_label":"a","secret_hint":"lr-canary-attr"},"after":{"dns_label":"b","secret_hint":"lr-canary-attr2"},"after_sensitive":{"secret_hint":true}}},
 {"address":"oci_core_route_table.lr","mode":"managed","type":"oci_core_route_table","change":{"actions":["delete","create"],"before":{},"after":{}}},
 {"address":"data.oci_identity_availability_domains.ad","mode":"data","type":"oci_identity_availability_domains","change":{"actions":["read"]}}
]}`

var allPrices = map[string]int64{"oci_core_vcn": 0, "oci_core_subnet": 0, "oci_core_route_table": 0}

func TestTofuPlanSuccess(t *testing.T) {
	h := newHarness(t, allPrices, 0)
	h.seedState(t)
	h.runner.out["show"] = RunOutput{Stdout: []byte(showJSON)}
	res, err := h.eng.PlanDetailed(context.Background(), testModule(), CredentialHandle{ID: "oci-test"})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if got := strings.Join(h.runner.calls, ","); got != "init,plan,show" {
		t.Fatalf("steps = %s", got)
	}
	if !h.runner.credSeen {
		t.Fatal("credential file was not present during the run")
	}
	want := map[string]string{"oci_core_vcn.lr": ActionCreate, "oci_core_subnet.lr": ActionUpdate, "oci_core_route_table.lr": ActionReplace}
	if len(res.Changes) != len(want) {
		t.Fatalf("changes = %+v", res.Changes)
	}
	for _, c := range res.Changes {
		if want[c.Address] != c.Action {
			t.Fatalf("%s action %s", c.Address, c.Action)
		}
	}
	if strings.Contains(fmt.Sprintf("%+v", res), "lr-canary") {
		t.Fatalf("sensitive value not masked: %+v", res)
	}
	h.assertClean(t)
	r, err := h.eng.Plan(context.Background(), testModule(), CredentialHandle{ID: "oci-test"})
	if err != nil || len(r.Changes) != 3 || strings.Contains(r.Log, stateCanary) {
		t.Fatalf("plan result = %+v, %v", r, err)
	}
	got, ver, err := h.state.Load(context.Background(), "oci-test", "net-lr")
	if err != nil || ver != 1 || !strings.Contains(string(got), stateCanary) {
		t.Fatalf("state changed by plan: %d %v", ver, err)
	}
}

func TestTofuPlanFailureCleansUpAndRedacts(t *testing.T) {
	h := newHarness(t, allPrices, 0)
	h.seedState(t)
	h.runner.fail = "plan"
	keyLine := strings.Split(h.pem, "\n")[2]
	h.runner.out["plan"] = RunOutput{Stderr: []byte("Error: bad config\n" + h.pem + "\nkey " + keyLine)}
	_, err := h.eng.Plan(context.Background(), testModule(), CredentialHandle{ID: "oci-test"})
	var ee *Error
	if !errors.As(err, &ee) || ee.Kind != ErrProvider {
		t.Fatalf("err = %v", err)
	}
	if msg := err.Error(); strings.Contains(msg, keyLine) || strings.Contains(msg, "PRIVATE KEY-----\nMII") {
		t.Fatalf("secret leaked in error: %s", msg)
	}
	if !strings.Contains(err.Error(), "bad config") {
		t.Fatalf("error lost the provider message: %s", err)
	}
	h.assertClean(t)
}

func TestTofuPlanLockContention(t *testing.T) {
	h := newHarness(t, allPrices, 0)
	ctx := context.Background()
	lock, err := h.db.AcquireCloudStateLock(ctx, "oci-test", "net-lr", "other", time.Minute, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	_, err = h.eng.Plan(ctx, testModule(), CredentialHandle{ID: "oci-test"})
	var ee *Error
	if !errors.Is(err, ErrStateLocked) || !errors.As(err, &ee) || ee.Kind != ErrLockHeld {
		t.Fatalf("err = %v", err)
	}
	if len(h.runner.calls) != 0 {
		t.Fatal("runner ran without the lock")
	}
	_ = h.db.ReleaseCloudStateLock(ctx, lock)
}

func TestTofuPlanCostRefusal(t *testing.T) {
	cases := map[string]struct {
		prices map[string]int64
		limit  int64
		code   string
	}{
		"unpriced":   {map[string]int64{}, 1_000_000_000, CodeCostUnpriced},
		"over limit": {map[string]int64{"oci_core_vcn": 5_000_000, "oci_core_subnet": 0, "oci_core_route_table": 0}, 1_000_000, CodeCostLimit},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t, tc.prices, tc.limit)
			h.runner.out["show"] = RunOutput{Stdout: []byte(showJSON)}
			_, err := h.eng.Plan(context.Background(), testModule(), CredentialHandle{ID: "oci-test"})
			var ee *Error
			if !errors.As(err, &ee) || ee.Kind != ErrPolicyRefused || !strings.Contains(err.Error(), tc.code) {
				t.Fatalf("err = %v", err)
			}
			h.assertClean(t)
		})
	}
}

func TestTofuUnconfiguredRefuses(t *testing.T) {
	e := NewTofuEngine(TofuConfig{}, TofuDeps{})
	if _, err := e.Apply(context.Background(), ApplyRequest{Spec: testModule(), PlanHash: "x"}); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("apply err = %v", err)
	}
	if _, err := e.Plan(context.Background(), testModule(), CredentialHandle{ID: "a"}); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("unconfigured plan err = %v", err)
	}
}

func TestInstallIntoOnlyWhenEnabled(t *testing.T) {
	t.Cleanup(func() { iac.SetCloudPlanner(nil); iac.SetCloudPolicy(nil) })
	iac.SetCloudPlanner(nil)
	iac.SetCloudPolicy(nil)
	planner := CloudPlannerAdapter{}
	policy := PolicyAdapter{Policy: func() Policy { return NewPolicy(0, false) }}
	if InstallInto(false, planner, policy) || iac.CloudPlannerInstalled() {
		t.Fatal("installed with the flag off")
	}
	if !InstallInto(true, planner, policy) || !iac.CloudPlannerInstalled() {
		t.Fatal("not installed with the flag on")
	}
}

func TestPolicyAdapterRefusesIAMAndDestroyCap(t *testing.T) {
	p := PolicyAdapter{}
	out := p.CheckCloud([]iac.CloudChange{
		{Address: "a", Type: "oci_identity_policy", Action: "create"},
		{Address: "b", Type: "oci_core_vcn", Action: "delete"},
	})
	codes := map[string]bool{}
	for _, r := range out {
		codes[r.Code] = true
	}
	if !codes[CodeIAMForbidden] || !codes[CodeDestroyCap] {
		t.Fatalf("refusals = %+v", out)
	}
}

func TestTofuPlanTimeoutAndCancelReleaseEverything(t *testing.T) {
	h := newHarness(t, allPrices, 0)
	h.seedState(t)
	h.runner.hold = "plan"
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := h.eng.PlanDetailed(ctx, testModule(), CredentialHandle{ID: "oci-test"})
	var ee *Error
	if !errors.As(err, &ee) || ee.Kind != ErrTimeout || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timed out plan err = %v, want kind %s", err, ErrTimeout)
	}
	h.assertClean(t)
	gone, stop := context.WithCancel(context.Background())
	stop()
	h.runner.calls = nil
	_, err = h.eng.PlanDetailed(gone, testModule(), CredentialHandle{ID: "oci-test"})
	if !errors.As(err, &ee) || ee.Kind != ErrCanceled || len(h.runner.calls) != 0 {
		t.Fatalf("cancelled plan err = %v calls %v, want kind %s and no step", err, h.runner.calls, ErrCanceled)
	}
	h.assertClean(t)
}
