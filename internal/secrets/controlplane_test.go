package secrets

import (
	"context"
	"strings"
	"testing"
)

type moverFake struct{ v map[string]string }

func (m *moverFake) Resolve(_ context.Context, s, k string) (string, error) {
	if x, ok := m.v[s+"|"+k]; ok {
		return x, nil
	}
	return "", ErrValueNotFound
}
func (m *moverFake) SetValue(_ context.Context, s, k, p string) error { m.v[s+"|"+k] = p; return nil }
func (m *moverFake) DeleteValue(_ context.Context, s, k string) error {
	delete(m.v, s+"|"+k)
	return nil
}

func TestMigrateKey_MovesOnceAndKeepsValue(t *testing.T) {
	ctx := context.Background()
	m := &moverFake{v: map[string]string{"build-records|k": "seed"}}
	nw := ControlPlaneService("build-records")
	if !strings.HasPrefix(nw, "_") {
		t.Fatal("namespace must start with an underscore")
	}
	for i := 0; i < 2; i++ {
		if err := MigrateKey(ctx, m, "build-records", nw, "k", nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	if m.v[nw+"|k"] != "seed" {
		t.Fatal("value not preserved")
	}
	if _, ok := m.v["build-records|k"]; ok {
		t.Fatal("legacy entry should be gone")
	}
}

func TestMigrateKey_NeverOverwritesOrDeletesDifferent(t *testing.T) {
	ctx := context.Background()
	nw := ControlPlaneService("x")
	m := &moverFake{v: map[string]string{"x|k": "appvalue", nw + "|k": "real"}}
	if err := MigrateKey(ctx, m, "x", nw, "k", nil, nil); err != nil {
		t.Fatal(err)
	}
	if m.v[nw+"|k"] != "real" || m.v["x|k"] != "appvalue" {
		t.Fatal("existing values must be untouched")
	}
}

func TestMigrateKey_InvalidLegacyIsLeftAlone(t *testing.T) {
	m := &moverFake{v: map[string]string{"x|k": "notakey"}}
	if err := MigrateKey(context.Background(), m, "x", ControlPlaneService("x"), "k", func(string) bool { return false }, nil); err != nil {
		t.Fatal(err)
	}
	if _, ok := m.v["x|k"]; !ok || len(m.v) != 1 {
		t.Fatal("app-owned value must stay")
	}
}
