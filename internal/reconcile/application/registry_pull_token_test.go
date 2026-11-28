package application

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

func TestResolveRegistryAuthBearerOnlyInTokenMode(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	svc := &build.RegistryTokenService{Key: key, Service: "s", TokenIssuer: "lr",
		BearerTTL: func() time.Duration { return time.Minute }, Host: func() string { return "reg.example" }}
	creds := &fakeRegistryCredentialStore{credentials: map[string]store.RegistryCredential{
		"rc1": {ID: "rc1", RegistryHost: "reg.example", Username: "u"},
	}}
	secrets := newFakeSecretResolver(map[string]string{"registry-credential/rc1/password": "pw"}) //nolint:gosec // fake fixture
	active := false
	c := New("web", &fakeStore{}, newFakeRuntime(0), WithRegistryCredentials(creds), WithSecretResolver(secrets),
		WithRegistryPullTokens(svc.MintPullForImage, func() bool { return active }))
	ctx := context.Background()

	auth, err := c.resolveRegistryAuth(ctx, "rc1", "reg.example/team/web:1")
	if err != nil || auth.Bearer != "" || auth.Username != "u" || auth.Password != "pw" {
		t.Fatalf("htpasswd mode must stay basic: %+v err=%v", auth, err)
	}
	active = true
	auth, err = c.resolveRegistryAuth(ctx, "rc1", "reg.example/team/web:1")
	if err != nil || auth.Bearer == "" || auth.Username != "" || auth.Password != "" {
		t.Fatalf("token mode must give a bearer only: %+v err=%v", auth, err)
	}
	auth, err = c.resolveRegistryAuth(ctx, "rc1", "other.example/team/web:1")
	if err != nil || auth.Bearer != "" || auth.Password != "pw" {
		t.Fatalf("a foreign host keeps the stored login: %+v err=%v", auth, err)
	}
}
