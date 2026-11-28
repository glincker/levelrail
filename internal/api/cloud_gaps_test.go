package api

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/glincker/levelrail/internal/provision"
	"github.com/glincker/levelrail/internal/store"
)

const gapsCompartment = "ocid1.compartment.oc1..own"

type gapsOracleFake struct {
	*fakeInstanceProvisioner
	detail provision.InstanceDetail
	usage  provision.OracleUsage
}

func (f *gapsOracleFake) DescribeInstance(context.Context, string) (provision.InstanceDetail, error) {
	return f.detail, nil
}

func (f *gapsOracleFake) OracleUsage(context.Context, string) (provision.OracleUsage, error) {
	return f.usage, nil
}

func gapsRouter(t *testing.T, fake NodeProvisioner) (*Router, func(*http.Request)) {
	t.Helper()
	secrets := newFakeNodeProviderSecrets()
	_ = secrets.SetValue(context.Background(), store.NodeProviderSecretsKey("oracle"), store.NodeProviderTokenEnvKey, "tok")
	db := openTestDB(t)
	rt := NewRouter(slog.New(slog.NewTextHandler(discardWriter{}, nil)), testBrand(), db, WithNodeProviderSecrets(secrets),
		WithNodeProvisionerFactory(func(string, string) (NodeProvisioner, error) { return fake, nil }))
	cfg := `{"compartment_ocid":"` + gapsCompartment + `"}`
	if err := db.CreateCloudAccount(context.Background(), store.CloudAccount{ID: "oracle-default", Provider: "oracle", AuthMethod: "static", Config: []byte(cfg)}); err != nil {
		t.Fatal(err)
	}
	return rt, withCookie(loginTestSession(t, rt, db))
}

func newGapsFake(comp string) *gapsOracleFake {
	base := &fakeInstanceProvisioner{fakeNodeProvisioner: &fakeNodeProvisioner{regions: []provision.Region{{ID: "us-ashburn-1"}, {ID: "eu-frankfurt-1"}}}}
	return &gapsOracleFake{fakeInstanceProvisioner: base, detail: provision.InstanceDetail{
		Instance: provision.Instance{ID: "i-1", Region: "us-ashburn-1", Shape: "VM.Standard.A1.Flex", OCPUs: 2, MemoryGB: 12, Managed: true}, CompartmentID: comp}}
}

func TestCloudGaps_CrossCompartmentInstanceIs404(t *testing.T) {
	fake := newGapsFake("ocid1.compartment.oc1..other")
	rt, c := gapsRouter(t, fake)
	base := "/api/v1/cloud-accounts/oracle-default/instances/i-1"
	for _, call := range []struct{ method, path, body string }{
		{http.MethodDelete, base, ""},
		{http.MethodPost, base + "/actions", `{"action":"stop"}`},
		{http.MethodPut, base + "/tags", `{"tags":{"a":"b"}}`},
		{http.MethodPost, base + "/resize", `{"ocpus":2,"memory_gb":12}`},
		{http.MethodGet, base + "/events", ""},
	} {
		if code, body := teamDo(t, rt, call.method, call.path, call.body, c); code != http.StatusNotFound {
			t.Errorf("%s %s = %d %s, want 404", call.method, call.path, code, body)
		}
	}
	if fake.terminated.Load() != 0 || len(fake.actions) != 0 {
		t.Error("provider acted on a foreign instance")
	}
}

func TestCloudGaps_AllowedRegionsRejectsUnknownRegion(t *testing.T) {
	rt, c := gapsRouter(t, newGapsFake(gapsCompartment))
	url := "/api/v1/cloud-accounts/oracle-default"
	if code, body := teamDo(t, rt, http.MethodPatch, url, `{"allowed_regions":["mars-1"]}`, c); code != http.StatusBadRequest {
		t.Fatalf("unknown region = %d %s", code, body)
	}
	code, body := teamDo(t, rt, http.MethodPatch, url, `{"allowed_regions":["us-ashburn-1"]}`, c)
	if code != http.StatusOK {
		t.Fatalf("known region = %d %s", code, body)
	}
	if code, _ := teamDo(t, rt, http.MethodGet, url+"/instances?region=eu-frankfurt-1", "", c); code != http.StatusForbidden {
		t.Fatalf("region outside the allowlist = %d, want 403", code)
	}
}

func TestCloudGaps_ResizeQuotaRefusal(t *testing.T) {
	fake := newGapsFake(gapsCompartment)
	fake.usage = provision.OracleUsage{A1OCPU: 4, A1MemoryGB: 24}
	rt, c := gapsRouter(t, fake)
	t.Setenv("APP_PROVISION_ORACLE_MAX_A1_OCPU", "4")
	t.Setenv("APP_PROVISION_ORACLE_MAX_A1_MEMORY_GB", "24")
	code, body := teamDo(t, rt, http.MethodPost, "/api/v1/cloud-accounts/oracle-default/instances/i-1/resize", `{"ocpus":3,"memory_gb":18}`, c)
	if code != http.StatusUnprocessableEntity || !strings.Contains(body, "A1 OCPU limit") {
		t.Fatalf("resize over quota = %d %s", code, body)
	}
}

func TestCloudGaps_EventsRouteShape(t *testing.T) {
	rt, c := gapsRouter(t, newGapsFake(gapsCompartment))
	code, body := teamDo(t, rt, http.MethodGet, "/api/v1/cloud-accounts/oracle-default/instances/i-1/events", "", c)
	if code != http.StatusOK || !strings.Contains(body, `"state":"ok"`) || !strings.Contains(body, `"events":[]`) || !strings.Contains(body, `"since"`) {
		t.Fatalf("events = %d %s", code, body)
	}
}
