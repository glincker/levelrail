package application

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestReconcile_SecretSetAfterCreateReachesNextContainer(t *testing.T) {
	rt := newFakeRuntime(0)
	resolver := newFakeSecretResolver(map[string]string{})
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80, SecretEnv: []store.SecretEnvRef{{Name: "TOKEN"}}}
	rec := &fakeApplied{}
	c := New("web", &fakeStore{svc: desired}, rt, WithSecretResolver(resolver), WithAppliedConfigRecorder(rec))

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := rt.lastCreateEnv["TOKEN"]; ok {
		t.Fatal("unset secret must be omitted from the first container")
	}
	first := rec.last()

	resolver.values["web/TOKEN"] = "plaintext-secret-value"
	desired.RestartNonce = "n1"
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := rt.lastCreateEnv["TOKEN"]; got != "plaintext-secret-value" {
		t.Fatalf("container env TOKEN = %q after restart, want the stored secret", got)
	}
	second := rec.last()
	if second.Release == first.Release {
		t.Fatal("restart did not produce a new release snapshot")
	}
	if second.EnvHashes["TOKEN"] != store.HashEnvValue("web", "TOKEN", "plaintext-secret-value") {
		t.Fatalf("snapshot missing the secret's hash: %+v", second.EnvHashes)
	}
	raw, _ := json.Marshal(second)
	if strings.Contains(string(raw), "plaintext-secret-value") {
		t.Fatalf("snapshot leaks the secret value: %s", raw)
	}
}
