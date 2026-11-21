package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/cloudconn"
	"github.com/GLINCKER/levelrail/internal/cloudguard"
)

func TestDoctorIdentityStepWarnsOnRoot(t *testing.T) {
	s := doctorIdentityStep("arn:aws:iam::123456789012:root", true)
	if s.Status != doctorWarn || !strings.Contains(s.Fix, "root hides permission gaps") {
		t.Fatalf("root step = %+v", s)
	}
	if got := doctorIdentityStep("arn:aws:iam::123456789012:user/ops", true); got.Status != doctorOK {
		t.Fatalf("user step = %+v", got)
	}
	if got := doctorIdentityStep("", false); got.Status != doctorFail || got.Fix == "" {
		t.Fatalf("missing identity = %+v", got)
	}
}

func TestDoctorCapabilityStepMissingActionFails(t *testing.T) {
	s := doctorCapabilityStep(cloudconn.CapabilityResult{Name: "deploy", Verified: true, Missing: []string{"ec2:RunInstances"}})
	if s.Status != doctorFail || !strings.Contains(s.Detail, "ec2:RunInstances") || s.Fix == "" {
		t.Fatalf("step = %+v", s)
	}
	ok := doctorCapabilityStep(cloudconn.CapabilityResult{Name: "inventory", OK: true, Verified: true})
	if ok.Status != doctorOK {
		t.Fatalf("ok step = %+v", ok)
	}
}

func TestDoctorGuardAndPolicySteps(t *testing.T) {
	if s := doctorGuardStep(cloudguard.Config{}); s.Status != doctorWarn || s.Fix == "" {
		t.Fatalf("empty guard = %+v", s)
	}
	full := cloudguard.Config{MonthlyCapUSD: 50, PerCreateMaxUSD: 10, InstanceTypes: []string{"t3.small"}}
	if s := doctorGuardStep(full); s.Status != doctorOK {
		t.Fatalf("full guard = %+v", s)
	}
	arn := "arn:aws:iam::123456789012:policy/budget-guard"
	if s := doctorPolicyStep("", nil); s.Status != doctorSkip {
		t.Fatalf("unset policy = %+v", s)
	}
	if s := doctorPolicyStep(arn, map[string]json.RawMessage{"guard_policy_arn": json.RawMessage(`"` + arn + `"`)}); s.Status != doctorOK {
		t.Fatalf("attached policy = %+v", s)
	}
	if s := doctorPolicyStep(arn, nil); s.Status != doctorWarn {
		t.Fatalf("unrecorded policy = %+v", s)
	}
}

func TestLinkKitDefaultsGuardPolicyFromSetting(t *testing.T) {
	db := openTestDB(t)
	rt := NewRouter(slog.New(slog.NewTextHandler(discardWriter{}, nil)), testBrand(), db)
	cookie := loginTestSession(t, rt, db)
	arn := "arn:aws:iam::123456789012:policy/budget-guard"
	setSetting(t, rt, "cloud.guard.policy_arn", `"`+arn+`"`)
	body := `{"trusted_principal_arn":"arn:aws:iam::123456789012:role/levelrail","capabilities":["inventory"]}`
	rec := provReq(rt, cookie, t, http.MethodPost, "/api/v1/cloud-accounts/aws/link-kit", body)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"guard_policy_arn":"`+arn+`"`) {
		t.Fatalf("link kit = %d %s", rec.Code, rec.Body.String())
	}
}

func TestCloudDoctorEndpointsAuthzAndNoSecrets(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/instances") {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`provider says secret-body`))
	}))
	defer srv.Close()
	db := openTestDB(t)
	rt := NewRouter(slog.New(slog.NewTextHandler(discardWriter{}, nil)), testBrand(), db,
		WithNodeProviderSecrets(newFakeNodeProviderSecrets()), WithAgentCAFingerprint("test-ca-fingerprint"))
	rt.oracleTestBase = srv.URL
	cookie := loginTestSession(t, rt, db)
	pemText, fp := oracleTestKey(t)
	add, _ := json.Marshal(map[string]any{
		"id": "ora-doc", "provider": "oracle", "tenancy_ocid": oTenancy, "user_ocid": oUser, "fingerprint": fp,
		"private_key_pem": pemText, "region": "us-phoenix-1", "compartment_ocid": oComp,
	})
	if rec := provReq(rt, cookie, t, http.MethodPost, "/api/v1/cloud-accounts", string(add)); rec.Code != http.StatusCreated {
		t.Fatalf("add = %d %s", rec.Code, rec.Body.String())
	}
	rec := provReq(rt, cookie, t, http.MethodGet, "/api/v1/cloud-accounts/ora-doc/doctor", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("doctor = %d %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, banned := range []string{"PRIVATE KEY", "secret-body", fp} {
		if strings.Contains(body, banned) {
			t.Fatalf("doctor body leaked %q: %s", banned, body)
		}
	}
	var rep doctorReport
	_ = json.Unmarshal(rec.Body.Bytes(), &rep)
	if rep.Ready || len(rep.Steps) < 5 {
		t.Fatalf("report = %+v", rep)
	}
	for _, s := range rep.Steps {
		if s.Status == doctorFail && s.Fix == "" {
			t.Fatalf("failing step %s has no fix", s.ID)
		}
	}
	if rec := provReq(rt, cookie, t, http.MethodGet, "/api/v1/cloud-accounts/doctor/preflight", ""); rec.Code != http.StatusOK {
		t.Fatalf("preflight = %d %s", rec.Code, rec.Body.String())
	}
	anon := httptest.NewRecorder()
	rt.Handler().ServeHTTP(anon, httptest.NewRequest(http.MethodGet, "/api/v1/cloud-accounts/ora-doc/doctor", nil))
	if anon.Code != http.StatusUnauthorized && anon.Code != http.StatusForbidden {
		t.Fatalf("anonymous doctor = %d", anon.Code)
	}
}
