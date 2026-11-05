package iac

import (
	"context"
	"strings"
	"testing"
)

const existingSecretApp = "version: 1\nkind: App\nmetadata: {name: web}\nspec:\n  service:\n    build: {type: image, image: x}\n    port: 80\n    env:\n      TOKEN_X: {secretRef: TOKEN_X, existing: true}\n"

func TestSecretRefExistingFailsWhenMissing(t *testing.T) {
	f := newFake()
	p := planFor(t, f, existingSecretApp, Options{Secrets: map[string]string{"TOKEN_X": "must-not-be-used"}})
	if p.Summary.Error != 1 {
		t.Fatalf("summary = %+v", p.Summary)
	}
	var reason string
	for _, c := range p.Changes {
		if c.Action == ActionError {
			reason = c.Reason
		}
	}
	if !strings.Contains(reason, "TOKEN_X") || !strings.Contains(reason, "existing") || strings.Contains(reason, "must-not-be-used") {
		t.Fatalf("reason = %q", reason)
	}
}

func TestSecretRefExistingIsNeverWritten(t *testing.T) {
	f := newFake()
	plain := strings.Replace(existingSecretApp, ", existing: true", "", 1)
	applyAll(t, f, plain, Options{Secrets: map[string]string{"TOKEN_X": "v1"}})
	if !f.secrets["web"]["TOKEN_X"] {
		t.Fatal("setup did not store the secret")
	}
	before := len(f.writes)
	opts := Options{Secrets: map[string]string{"TOKEN_X": "v2"}, Prune: true}
	res := mustBuild(t, existingSecretApp, opts)
	if got := res[0].ExistingSecrets; len(got) != 1 || got[0] != "TOKEN_X" {
		t.Fatalf("existing = %v", got)
	}
	out, err := Apply(context.Background(), f, res, opts, "")
	if err != nil || !out.OK() {
		t.Fatalf("apply = %+v, %v", out, err)
	}
	for _, w := range f.writes[before:] {
		if strings.Contains(w, "/secrets/") {
			t.Fatalf("secret was touched: %s", w)
		}
	}
	if !f.secrets["web"]["TOKEN_X"] {
		t.Fatal("secret was deleted")
	}
}
