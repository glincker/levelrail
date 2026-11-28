package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/glincker/levelrail/internal/awsx/awsxtest"
	"github.com/glincker/levelrail/internal/runtimesettings"
	"github.com/glincker/levelrail/internal/store"
	"github.com/glincker/levelrail/internal/vault"
)

func TestVaultAWSAccountWithoutTokenRefLogsInAndLeases(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/auth/aws/login":
			_ = json.NewEncoder(w).Encode(map[string]any{"auth": map[string]any{"client_token": "tok", "lease_duration": 600}})
		case "/v1/sys/health":
			_ = json.NewEncoder(w).Encode(map[string]any{"initialized": true})
		case "/v1/aws/roles/lr":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{}})
		case "/v1/aws/creds/lr":
			if r.Header.Get("X-Vault-Token") != "tok" {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"lease_id": "l1", "lease_duration": 900,
				"data": map[string]any{"access_key": "ASIAEXAMPLE", "secret_key": "sk", "security_token": "st"}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	rt, db := awsServicesRouter(t, func(awsxtest.Call, int) awsxtest.Reply {
		return awsxtest.Reply{Body: `<GetCallerIdentityResponse/>`}
	})
	ctx := context.Background()
	for k, v := range map[string]string{runtimesettings.KeyCloudVaultAuth: `"aws"`, runtimesettings.KeyCloudVaultAuthRole: `"levelrail"`} {
		if _, err := rt.runtimeSettings.Set(ctx, k, json.RawMessage(v), "test"); err != nil {
			t.Fatal(err)
		}
	}
	prev := vaultAmbientKeys
	vaultAmbientKeys = func(context.Context) (vault.AWSCredentials, error) {
		return vault.AWSCredentials{AccessKeyID: "AKIDEXAMPLE", SecretAccessKey: "secret", SessionToken: "s"}, nil
	}
	t.Cleanup(func() { vaultAmbientKeys = prev })
	cfg, _ := json.Marshal(map[string]string{"region": "us-east-1", "vault_address": srv.URL, "vault_role": "lr"})
	a := store.CloudAccount{ID: "vault-acct", Provider: "aws", AuthMethod: "vault_aws", Config: cfg}
	if err := db.CreateCloudAccount(ctx, a); err != nil {
		t.Fatal(err)
	}

	raw, err := rt.vaultProvisionCredentials(ctx, a)
	if err != nil || !strings.Contains(raw, "ASIAEXAMPLE") {
		t.Fatalf("provision credentials = %v, err %v", strings.Contains(raw, "ASIA"), err)
	}
	steps := rt.doctorLiveVault(ctx, a, "tok")
	if len(steps) != 2 || steps[0].Status != doctorOK || steps[1].Status != doctorOK {
		t.Fatalf("doctor steps = %+v", steps)
	}
	if body, _ := json.Marshal(steps); strings.Contains(string(body), "ASIAEXAMPLE") {
		t.Fatal("doctor output leaked key material")
	}
}
