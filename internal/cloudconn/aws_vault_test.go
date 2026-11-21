package cloudconn

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/secretsource"
	"github.com/GLINCKER/levelrail/internal/vault"
)

func vaultAWSTarget(addr, token string) Target {
	return Target{
		Connection: Connection{Name: "prod", Provider: ProviderAWS, AuthMethod: AuthVaultAWS, Config: Config{
			Region: "eu-west-1", AccountID: "123456789012", VaultAddress: addr, VaultRole: "lr", VaultTokenRef: "secret://vault/lr#token"}},
		Credential: secretsource.NewValue([]byte(token), ""),
	}
}

func TestAWSCheckVaultAWSViaFakeVaultAndSTS(t *testing.T) {
	awsTestEnv(t)
	sts := httptest.NewServer(&fakeSTSServer{account: "123456789012"})
	defer sts.Close()
	var revoked []string
	v := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Vault-Token") != "tok" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if r.URL.Path == "/v1/sys/leases/revoke" {
			var b struct {
				LeaseID string `json:"lease_id"`
			}
			_ = json.NewDecoder(r.Body).Decode(&b)
			revoked = append(revoked, b.LeaseID)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"lease_id": "l1", "lease_duration": 900,
			"data": map[string]any{"access_key": "ASIAV", "secret_key": "sk", "security_token": "st"}})
	}))
	defer v.Close()
	broker := vault.NewAWSBroker(func(context.Context) vault.Tunables {
		return vault.Tunables{Timeout: 5 * time.Second, AllowPrivate: true}
	})
	p := NewAWSProvider(AWSOptions{BaseEndpoint: sts.URL, VaultBroker: broker})
	ctx := context.Background()

	if class, err := p.Check(ctx, vaultAWSTarget(v.URL, "tok")); class != ClassOK || err != nil {
		t.Fatalf("check = %q, %v", class, err)
	}
	if class, err := p.Check(ctx, vaultAWSTarget(v.URL, "bad")); class != ClassAuthFailed || err == nil {
		t.Fatalf("bad token = %q, %v", class, err)
	} else if strings.Contains(err.Error(), "bad") {
		t.Fatal("error leaks the token")
	}
	if class, _ := p.Check(ctx, vaultAWSTarget(v.URL, "")); class != ClassCredentialUnresolved {
		t.Fatalf("empty token class = %q", class)
	}
	if err := p.RevokeVault(ctx, vaultAWSTarget(v.URL, "tok")); err != nil || len(revoked) != 1 || revoked[0] != "l1" {
		t.Fatalf("revoke = %v, %v", revoked, err)
	}
}

func TestVaultAWSValidationAndKit(t *testing.T) {
	p := NewAWSProvider(AWSOptions{})
	ok := Config{Region: "eu-west-1", VaultAddress: "https://vault.example.com", VaultRole: "lr", VaultMount: "aws"}
	if err := p.ValidateConfig(ok, AuthVaultAWS); err != nil {
		t.Fatalf("valid config refused: %v", err)
	}
	for _, bad := range []Config{{Region: "eu-west-1", VaultRole: "lr"}, {Region: "eu-west-1", VaultAddress: "ftp://x", VaultRole: "lr"},
		{Region: "eu-west-1", VaultAddress: "https://v", VaultRole: "a b"}, {Region: "eu-west-1", VaultAddress: "https://v", VaultRole: "lr", VaultMount: "../x"}} {
		if p.ValidateConfig(bad, AuthVaultAWS) == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
	kit, err := BuildVaultKit(VaultKitInput{AccountID: "123456789012", PrincipalARN: "arn:aws:iam::999999999999:user/vault", Capabilities: []string{CapInventory}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"secrets enable -path=aws aws", "credential_type=assumed_role", "role_arns=arn:aws:iam::123456789012:role/LevelrailAccess", "sys/leases/revoke"} {
		if !strings.Contains(kit.VaultCommands, want) {
			t.Errorf("commands miss %q", want)
		}
	}
	if !strings.Contains(kit.TrustPolicyJSON, "sts:AssumeRole") || !strings.Contains(kit.PolicyJSON, "ec2:Describe") {
		t.Fatalf("kit documents incomplete: %s", kit.TrustPolicyJSON)
	}
}
