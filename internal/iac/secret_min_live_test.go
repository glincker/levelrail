package iac

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestLeakedSecretFollowsLiveMinimum(t *testing.T) {
	t.Cleanup(func() { store.SetSecretMatchMinSource(func() int { return 4 }) })
	docs := []byte("password: x")
	secrets := map[string]string{"P": "x"}
	if _, leaked := LeakedSecret(docs, secrets); leaked {
		t.Fatal("a one character secret must be skipped at the default minimum")
	}
	store.SetSecretMatchMinSource(func() int { return 1 })
	if name, leaked := LeakedSecret(docs, secrets); !leaked || name != "P" {
		t.Fatalf("leaked = %v, name = %q, want P", leaked, name)
	}
}
