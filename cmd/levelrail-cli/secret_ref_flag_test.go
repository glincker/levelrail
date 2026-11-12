package main

import "testing"

func TestSecretRefFlag(t *testing.T) {
	m := secretRefFlag{}
	if err := m.Set("API=secret://prod/apps/web#key"); err != nil {
		t.Fatal(err)
	}
	if v := m["API"]; v.Conn != "prod" || v.Path != "apps/web" || v.Key != "key" {
		t.Fatalf("parsed = %+v", v)
	}
	for _, bad := range []string{"API=literalvalue", "=secret://a/b#c", "API", "API=secret://Prod/b#c"} {
		if err := (secretRefFlag{}).Set(bad); err == nil {
			t.Fatalf("%q must be rejected", bad)
		}
	}
}
