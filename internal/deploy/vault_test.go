package deploy

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/spec"
)

// fakeVaultConfigChecker is a hand-written fake for VaultConfigChecker,
// the same "one bool plus an optional error" shape newFakeSecretChecker
// establishes for SecretChecker.
type fakeVaultConfigChecker struct {
	configured bool
	err        error
	calls      int
}

func (f *fakeVaultConfigChecker) VaultConfigured(context.Context) (bool, error) {
	f.calls++
	if f.err != nil {
		return false, f.err
	}
	return f.configured, nil
}

// deployVaultBackedApp deploys a service whose only env var, API_KEY, is
// vault-backed via {myapp/config, api_key}, the setup every
// TestPipeline_Deploy_VaultEnv_* test in this file shares.
func deployVaultBackedApp(p *Pipeline) (string, error) {
	svc := dockerfileService()
	svc.Env = map[string]spec.EnvVar{"API_KEY": {Vault: &spec.VaultRef{Path: "myapp/config", Key: "api_key"}}}
	return p.Deploy(context.Background(), Request{ServiceName: "web", Service: svc, SourceDir: "/repo", CommitSHA: "abc1234", ImageRepo: "levelrail/web"}, nil)
}

func TestPipeline_Deploy_VaultEnv_NoCheckerConfigured_Rejected(t *testing.T) {
	builder := &fakeBuilder{result: &build.Result{Tag: "x:y"}}
	p := New(builder, &fakeServiceStore{}) // no WithVaultConfigChecker

	_, err := deployVaultBackedApp(p)
	if err == nil {
		t.Fatal("Deploy() error = nil, want a vault-backed env var with no checker configured to be rejected")
	}
	if builder.calls != 0 {
		t.Errorf("builder.Build called %d times, want 0: must fail before attempting a build", builder.calls)
	}
}

func TestPipeline_Deploy_VaultEnv_NotConfiguredOnControlPlane_Rejected(t *testing.T) {
	builder := &fakeBuilder{result: &build.Result{Tag: "x:y"}}
	checker := &fakeVaultConfigChecker{configured: false}
	p := New(builder, &fakeServiceStore{}, WithVaultConfigChecker(checker))

	_, err := deployVaultBackedApp(p)
	if err == nil {
		t.Fatal("Deploy() error = nil, want a vault-backed env var to be rejected when vault integration isn't enabled")
	}
	if checker.calls != 1 {
		t.Errorf("checker.VaultConfigured called %d times, want 1", checker.calls)
	}
	if builder.calls != 0 {
		t.Errorf("builder.Build called %d times, want 0", builder.calls)
	}
}

func TestPipeline_Deploy_VaultEnv_Configured_PassesThrough(t *testing.T) {
	builder := &fakeBuilder{result: &build.Result{Tag: "x:y"}}
	svcStore := &fakeServiceStore{}
	checker := &fakeVaultConfigChecker{configured: true}
	p := New(builder, svcStore, WithVaultConfigChecker(checker))

	_, err := deployVaultBackedApp(p)
	if err != nil {
		t.Fatalf("Deploy() error = %v, want it to pass once vault is configured", err)
	}
	if builder.calls != 1 {
		t.Errorf("builder.Build called %d times, want 1", builder.calls)
	}
	if _, exists := svcStore.saved.Env["API_KEY"]; exists {
		t.Error("saved.Env contains API_KEY, want vault-backed vars kept out of the plain Env map entirely")
	}
	got, ok := svcStore.saved.VaultEnv["API_KEY"]
	if !ok {
		t.Fatalf("saved.VaultEnv[%q] missing, want an entry", "API_KEY")
	}
	if got.Path != "myapp/config" || got.Key != "api_key" {
		t.Errorf("saved.VaultEnv[%q] = %+v, want Path=myapp/config Key=api_key", "API_KEY", got)
	}
}

func TestPipeline_Deploy_VaultEnv_CheckerErrorPropagates(t *testing.T) {
	builder := &fakeBuilder{result: &build.Result{Tag: "x:y"}}
	checker := &fakeVaultConfigChecker{err: errors.New("database unavailable")}
	p := New(builder, &fakeServiceStore{}, WithVaultConfigChecker(checker))

	_, err := deployVaultBackedApp(p)
	if err == nil {
		t.Fatal("Deploy() error = nil, want the checker's error to propagate")
	}
	if builder.calls != 0 {
		t.Errorf("builder.Build called %d times, want 0", builder.calls)
	}
}
