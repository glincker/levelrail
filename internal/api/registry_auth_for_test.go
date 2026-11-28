package api

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/build"
	"github.com/glincker/levelrail/internal/store"
)

func TestRegistryAuthForBearerOnlyInTokenMode(t *testing.T) {
	setter := &fakeRegistryCredentialSecretsSetter{resolveValue: "pw"}
	rt, db := newTestRouterWithRegistryCredentialSecrets(t, setter)
	ctx := context.Background()
	if err := db.SaveRegistryCredential(ctx, store.RegistryCredential{ID: "rc1", Name: "c", RegistryHost: "reg.example", Username: "u"}); err != nil {
		t.Fatal(err)
	}
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	active := false
	rt.registryToken = &build.RegistryTokenService{Key: key, Service: "s", TokenIssuer: "lr",
		BearerTTL: func() time.Duration { return time.Minute }, Host: func() string { return "reg.example" }}
	rt.registryTokenActive = func() bool { return active }

	auth, err := rt.registryAuthFor(ctx, "rc1", "reg.example/team/web:1")
	if err != nil || auth.Bearer != "" || auth.Username != "u" || auth.Password != "pw" {
		t.Fatalf("htpasswd mode must stay basic: %+v err=%v", auth, err)
	}
	active = true
	auth, err = rt.registryAuthFor(ctx, "rc1", "reg.example/team/web:1")
	if err != nil || auth.Bearer == "" || auth.Username != "" || auth.Password != "" {
		t.Fatalf("token mode must give a bearer only: %+v err=%v", auth, err)
	}
	auth, err = rt.registryAuthFor(ctx, "rc1", "other.example/team/web:1")
	if err != nil || auth.Bearer != "" || auth.Password != "pw" {
		t.Fatalf("a foreign host keeps the stored login: %+v err=%v", auth, err)
	}
}
