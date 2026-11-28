package configrev

import (
	"strings"
	"testing"

	"github.com/glincker/levelrail/internal/store"
)

func TestSecretNeverPlaintext(t *testing.T) {
	svc := store.DesiredService{Env: map[string]string{"MODE": "prod", "API_TOKEN": "s3cr3t-value-123"}, SecretEnv: []store.SecretEnvRef{{Name: "DB_PASS"}}}
	raw, err := Marshal(Capture(svc))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, "s3cr3t-value-123") {
		t.Fatalf("snapshot leaked a secret value: %s", raw)
	}
	next := svc
	next.Env = map[string]string{"MODE": "dev", "API_TOKEN": "other-secret-456"}
	for _, c := range Diff(Capture(svc), Capture(next)) {
		if strings.Contains(c.From+c.To, "secret-") || strings.Contains(c.From+c.To, "s3cr3t") {
			t.Fatalf("diff leaked: %+v", c)
		}
	}
}

func TestRecordThenRevertEnv(t *testing.T) {
	before := store.DesiredService{Env: map[string]string{"MODE": "prod"}, Replicas: 1, Domains: []string{"a.example.com"}}
	after := before
	after.Env = map[string]string{"MODE": "broken", "EXTRA": "1"}
	after.Replicas = 3
	snap := Capture(before)
	if Equal(snap, Capture(after)) {
		t.Fatal("change not detected")
	}
	plan := Apply(after, snap)
	if !Equal(Capture(plan.Service), snap) {
		t.Fatalf("revert did not restore: %+v", plan.Service)
	}
	if plan.Service.Env["MODE"] != "prod" || len(plan.Service.Env) != 1 {
		t.Fatalf("env = %v", plan.Service.Env)
	}
}

func TestRevertSkipsMissingMaskedValue(t *testing.T) {
	old := store.DesiredService{Env: map[string]string{"API_TOKEN": "s3cr3t-value-123"}}
	snap := Capture(old)
	plan := Apply(store.DesiredService{Env: map[string]string{}}, snap)
	if len(plan.Skipped) != 1 || plan.Skipped[0] != "API_TOKEN" {
		t.Fatalf("skipped = %v", plan.Skipped)
	}
	if _, ok := plan.Service.Env["API_TOKEN"]; ok {
		t.Fatal("secret value must not reappear")
	}
}
