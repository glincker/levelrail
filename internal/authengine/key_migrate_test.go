package authengine

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/glincker/levelrail/internal/secrets"
)

type moverKS struct{ v map[string]string }

func (m *moverKS) Resolve(_ context.Context, s, k string) (string, error) {
	if x, ok := m.v[s+"|"+k]; ok {
		return x, nil
	}
	return "", secrets.ErrValueNotFound
}
func (m *moverKS) SetValue(_ context.Context, s, k, p string) error { m.v[s+"|"+k] = p; return nil }
func (m *moverKS) DeleteValue(_ context.Context, s, k string) error { delete(m.v, s+"|"+k); return nil }

func TestLoadOrCreateKey_MigratesLegacyKey(t *testing.T) {
	ks := &moverKS{v: map[string]string{}}
	want := make([]byte, keyBytes)
	want[0] = 7
	ks.v[legacyKeyService+"|"+keyName] = base64.StdEncoding.EncodeToString(want)
	for i := 0; i < 2; i++ {
		got, err := LoadOrCreateKey(context.Background(), ks, true)
		if err != nil || string(got) != string(want) {
			t.Fatalf("run %d: key changed or failed: %v", i, err)
		}
	}
	if _, ok := ks.v[legacyKeyService+"|"+keyName]; ok {
		t.Fatal("legacy slot should be empty")
	}
	if !strings.HasPrefix(keyService, "_") {
		t.Fatal("key must live in the private namespace")
	}
}
