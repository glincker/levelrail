package spec

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestEnvVarSecretRefForms(t *testing.T) {
	var m map[string]EnvVar
	src := "A: secret://prod/apps/web#a\nB: {secret: {conn: prod, path: apps/web, key: b, version: 2}}\nC: {secret: true}\nD: {vault: {path: p, key: k}}\n"
	if err := yaml.Unmarshal([]byte(src), &m); err != nil {
		t.Fatal(err)
	}
	if m["A"].SecretRef == nil || m["A"].SecretRef.Text == "" || m["B"].SecretRef.Key != "b" || m["B"].SecretRef.Version != 2 {
		t.Fatalf("refs = %+v %+v", m["A"], m["B"])
	}
	if !m["C"].Secret || m["C"].SecretRef != nil || m["D"].Vault == nil {
		t.Fatalf("legacy forms changed: %+v %+v", m["C"], m["D"])
	}
}

func TestEnvVarSecretRefRejects(t *testing.T) {
	for _, src := range []string{
		"A: {secret: {conn: prod, path: a}}",
		"A: {secret: {conn: prod, path: a, key: k}, from: x.y}",
	} {
		var m map[string]EnvVar
		err := yaml.Unmarshal([]byte(src), &m)
		if err == nil {
			t.Fatalf("%s must fail", src)
		}
		_ = strings.TrimSpace(err.Error())
	}
}
