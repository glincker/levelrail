package iac

import (
	"context"
	"strings"
	"testing"
)

func credDoc(cred string) string {
	line := ""
	if cred != "" {
		line = ", registryCredential: " + cred
	}
	return "version: 1\nkind: App\nmetadata: {name: web}\nspec:\n  service:\n    build: {type: image, image: \"registry.example.com/acme/web:1.0.0\"" + line + "}\n    port: 3000\n"
}

func TestRegistryCredentialMissingIsCodedPlanIssue(t *testing.T) {
	f := newFake()
	p := planFor(t, f, credDoc("nope"), Options{})
	if len(p.Changes) != 1 || p.Changes[0].Code != CodeRegistryCredentialNotFound {
		t.Fatalf("changes = %+v", p.Changes)
	}
}

func TestRegistryCredentialCreateUpdateClearAndExport(t *testing.T) {
	f := newFake()
	f.creds = []string{"a-pull", "b-pull"}
	if out := applyAll(t, f, credDoc("a-pull"), Options{}); !out.OK() {
		t.Fatalf("create: %+v", out)
	}
	if f.apps["web"]["registry_credential"] != "a-pull" {
		t.Fatalf("create body: %v", f.apps["web"])
	}
	if p := planFor(t, f, credDoc("a-pull"), Options{}); p.Pending() {
		t.Fatalf("not idempotent: %+v", p.Changes)
	}
	p := planFor(t, f, credDoc("b-pull"), Options{})
	if !p.Pending() || p.Changes[0].Fields[0].Path != "registryCredential" {
		t.Fatalf("drift not shown: %+v", p.Changes)
	}
	if out := applyAll(t, f, credDoc("b-pull"), Options{}); !out.OK() || f.apps["web"]["registry_credential"] != "b-pull" {
		t.Fatalf("update: %+v %v", out, f.apps["web"])
	}
	ex, err := Export(context.Background(), f, ExportOptions{})
	if err != nil || !strings.Contains(ex.Join(), "registryCredential: b-pull") {
		t.Fatalf("export: %v\n%s", err, ex.Join())
	}
	if out := applyAll(t, f, credDoc(""), Options{}); !out.OK() {
		t.Fatalf("clear: %+v", out)
	}
	if v := f.apps["web"]["registry_credential"]; v != nil {
		t.Fatalf("not cleared: %v", v)
	}
}
