package cloudiac

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/cloudconn"
	"github.com/GLINCKER/levelrail/internal/iac"
)

type connGetter map[string]cloudconn.Connection

func (g connGetter) Get(_ context.Context, name string) (cloudconn.Connection, error) {
	c, ok := g[name]
	if !ok {
		return cloudconn.Connection{}, cloudconn.ErrNotFound
	}
	return c, nil
}

type noopEngine struct{}

func (noopEngine) PlanDetailed(context.Context, ModuleSpec, CredentialHandle) (PlanResult, error) {
	return PlanResult{}, nil
}

func TestCloudAdapterMissingConnectionIsUnavailable(t *testing.T) {
	a := CloudPlannerAdapter{Engine: noopEngine{}, Accounts: ConnectionAccounts{Conns: connGetter{}}}
	out := a.PlanCloudStack(context.Background(), "s", []iac.CloudResource{
		{Kind: iac.KindCloudAccount, Name: "oci-test"},
		{Kind: iac.KindCloudBucket, Name: "b", Spec: map[string]any{"account": "oci-test", "namespace": "ns"}},
	})
	for i, o := range out {
		if !errors.Is(o.Err, iac.ErrCloudUnavailable) || !strings.Contains(o.Err.Error(), `no stored cloud connection named "oci-test"`) {
			t.Fatalf("outcome %d err = %v", i, o.Err)
		}
	}
}

func TestCloudAdapterPresentConnectionKeepsAccountNoop(t *testing.T) {
	g := connGetter{"oci-test": {Name: "oci-test", Provider: cloudconn.ProviderOracle}}
	a := CloudPlannerAdapter{Engine: noopEngine{}, Accounts: ConnectionAccounts{Conns: g}}
	out := a.PlanCloudStack(context.Background(), "s", []iac.CloudResource{{Kind: iac.KindCloudAccount, Name: "oci-test"}})
	if out[0].Err != nil || out[0].Diff.Action != "noop" {
		t.Fatalf("outcome %+v", out[0])
	}
}

func TestCloudAdapterSettingErrorsAreUnavailable(t *testing.T) {
	for _, err := range []error{ErrNotConfigured, ErrFetchConfig} {
		if !errors.Is(err, iac.ErrCloudUnavailable) {
			t.Fatalf("%v is not unavailable", err)
		}
	}
	if errors.Is(ErrStateQuarantined, iac.ErrCloudUnavailable) {
		t.Fatal("quarantine must keep its own code")
	}
}
