package deploy

import (
	"testing"

	"github.com/GLINCKER/levelrail/internal/spec"
)

func TestVaultEnvRefsMapsAndValidatesSecretRefs(t *testing.T) {
	ok, err := vaultEnvRefs(map[string]spec.EnvVar{
		"A": {SecretRef: &spec.SecretRef{Text: "secret://prod/apps/web#a"}},
		"B": {SecretRef: &spec.SecretRef{Conn: "prod", Path: "p", Key: "b", Version: 2}},
		"C": {Vault: &spec.VaultRef{Path: "p", Key: "c"}},
	})
	if err != nil || ok["A"].Conn != "prod" || ok["B"].Version != 2 || ok["C"].Conn != "" {
		t.Fatalf("refs = %+v err = %v", ok, err)
	}
	for _, bad := range []*spec.SecretRef{{Text: "secret://nokey/path"}, {Text: "secret://c/p#k?version=0"}} {
		if _, err := vaultEnvRefs(map[string]spec.EnvVar{"A": {SecretRef: bad}}); err == nil {
			t.Fatalf("%+v must be rejected", bad)
		}
	}
}
