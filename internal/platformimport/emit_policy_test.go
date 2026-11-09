package platformimport

import (
	"strings"
	"testing"
)

func TestRenderNotesDomainPolicy(t *testing.T) {
	apps := []EmitApp{{Name: "blog", Policy: DomainPolicy{ForceHTTPS: true, WebSocket: true, RedirectTo: "https://x.example", BasicAuthUser: "ops"}}}
	out := renderNotes("caprover", EmitOptions{Stack: "s"}, apps, nil)
	for _, want := range []string{"redirect to HTTPS: on", "websocket upgrade: on", "redirect to https://x.example", "basic auth user ops (set a new password)"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in %s", want, out)
		}
	}
	if strings.Contains(renderNotes("caprover", EmitOptions{Stack: "s"}, []EmitApp{{Name: "a"}}, nil), "## App a") {
		t.Error("empty policy must add no section")
	}
}

func TestCapRoverPolicyKeepsUserOnly(t *testing.T) {
	a := mapCapRoverApp(caproverDefinition{AppName: "a", ForceSSL: true, HTTPAuth: &struct {
		User string `json:"user"`
	}{User: "ops"}}, "")
	if !a.Policy.ForceHTTPS || a.Policy.BasicAuthUser != "ops" {
		t.Errorf("policy: %+v", a.Policy)
	}
}
