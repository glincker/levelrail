package main

import (
	"strings"
	"testing"
)

func TestSecretsProviderUsageListsOnePassword(t *testing.T) {
	u := secretsProviderUsage("levelrail-cli")
	for _, want := range []string{"--type onepassword", "--base-url CONNECT_URL", "op://NAME/"} {
		if !strings.Contains(u, want) {
			t.Fatalf("usage missing %q", want)
		}
	}
}
