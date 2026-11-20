package docker

import "testing"

func TestRegistryAuthConfigAndPullKey(t *testing.T) {
	basic := &RegistryAuth{Username: "u", Password: "p"}
	if c := basic.authConfig(); c.Username != "u" || c.Password != "p" || c.RegistryToken != "" {
		t.Fatalf("basic auth changed: %+v", c)
	}
	if pullKey("r", basic) != "r\x00u\x00p" || pullKey("r", nil) != "r\x00" {
		t.Fatal("htpasswd pull keys must stay byte for byte the same")
	}
	bearer := &RegistryAuth{Bearer: "tok"}
	if c := bearer.authConfig(); c.RegistryToken != "tok" || c.Username != "" || c.Password != "" {
		t.Fatalf("bearer must use the token field only: %+v", c)
	}
	if pullKey("r", bearer) == pullKey("r", basic) {
		t.Fatal("bearer and basic pulls must not share a key")
	}
}
