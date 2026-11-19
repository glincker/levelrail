package build

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"strings"
	"testing"
	"time"
)

func decodeJWT(t *testing.T, tok string, pub *ecdsa.PublicKey) map[string]any {
	t.Helper()
	p := strings.Split(tok, ".")
	if len(p) != 3 {
		t.Fatalf("not a jwt")
	}
	sig, err := base64.RawURLEncoding.DecodeString(p[2])
	if err != nil || len(sig) != 64 {
		t.Fatalf("bad signature encoding")
	}
	sum := sha256.Sum256([]byte(p[0] + "." + p[1]))
	if !ecdsa.Verify(pub, sum[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
		t.Fatal("signature does not verify")
	}
	raw, _ := base64.RawURLEncoding.DecodeString(p[1])
	var c map[string]any
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	return c
}

func accessOf(c map[string]any) []any { a, _ := c["access"].([]any); return a }

// A push-shaped sequence: buildkit asks for push,pull on its repo, then probes a base image repo.
func TestRegistryTokenExchangeScope(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	iss := testIssuer(t, &now)
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	svc := &RegistryTokenService{Issuer: iss, Key: key, Service: "svc", TokenIssuer: "lr", BearerTTL: func() time.Duration { return 90 * time.Second },
		Host: func() string { return "reg.example" }, now: func() time.Time { return now }}
	tok, err := iss.Issue("reg.example/levelrail/app", "b1", time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	jwt, exp, err := svc.Exchange(tok, []string{"repository:levelrail/app:push,pull"})
	if err != nil {
		t.Fatal(err)
	}
	c := decodeJWT(t, jwt, &key.PublicKey)
	if len(accessOf(c)) != 1 || c["aud"] != "svc" || exp.Sub(now) != 90*time.Second {
		t.Fatalf("own repo grant wrong: %v exp %v", c, exp)
	}
	for name, scopes := range map[string][]string{
		"other repo":     {"repository:levelrail/other:push,pull"},
		"pull only":      {"repository:levelrail/app:pull"},
		"base image":     {"repository:library/alpine:pull"},
		"malformed":      {"registry:catalog:*"},
		"prefix sibling": {"repository:levelrail/app2:push,pull"},
	} {
		j, _, err := svc.Exchange(tok, scopes)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if n := len(accessOf(decodeJWT(t, j, &key.PublicKey))); n != 0 {
			t.Errorf("%s: granted %d entries, want none", name, n)
		}
	}
	if _, _, err := svc.Exchange(tok+"x", []string{"repository:levelrail/app:push,pull"}); !errors.Is(err, ErrPushTokenInvalid) {
		t.Errorf("tampered: %v", err)
	}
	iss.Revoke(tok)
	if _, _, err := svc.Exchange(tok, []string{"repository:levelrail/app:push,pull"}); !errors.Is(err, ErrPushTokenRevoked) {
		t.Errorf("revoked: %v", err)
	}
}

func TestMintPullAndTTL(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	svc := &RegistryTokenService{Issuer: testIssuer(t, &now), Key: key, Service: "svc", TokenIssuer: "lr",
		BearerTTL: func() time.Duration { return 90 * time.Second }, Host: func() string { return "reg.example" }, now: func() time.Time { return now }}
	jwt, err := svc.MintPull("levelrail/app")
	if err != nil {
		t.Fatal(err)
	}
	c := decodeJWT(t, jwt, &key.PublicKey)
	acc := accessOf(c)
	if len(acc) != 1 || c["aud"] != "svc" || c["iss"] != "lr" || int64(c["exp"].(float64)) != now.Add(90*time.Second).Unix() {
		t.Fatalf("pull bearer wrong: %v", c)
	}
	if a := acc[0].(map[string]any); a["name"] != "levelrail/app" || len(a["actions"].([]any)) != 1 || a["actions"].([]any)[0] != "pull" {
		t.Fatalf("pull bearer must be pull only: %v", a)
	}
	svc.BearerTTL = nil
	if _, err := svc.MintPull("x"); err == nil {
		t.Error("no ttl source must fail closed")
	}
	tok, _ := svc.Issuer.Issue("reg.example/levelrail/app", "b1", time.Hour)
	if _, _, err := svc.Exchange(tok, []string{"repository:levelrail/app:pull,push"}); err == nil {
		t.Error("no ttl source must fail closed on exchange")
	}
	svc.BearerTTL = func() time.Duration { return 90 * time.Second }
	j, _, err := svc.Exchange(tok, []string{"repository:levelrail/app:pull,push"})
	if err != nil || len(accessOf(decodeJWT(t, j, &key.PublicKey))) != 1 {
		t.Errorf("pull,push order must be granted: %v", err)
	}
}

func TestValidateInsecureHost(t *testing.T) {
	for host, allowCGNAT := range map[string]bool{"10.0.0.4:5000": false, "192.168.1.2": false, "127.0.0.1:5000": false, "localhost:5000": false, "100.64.1.2:5000": true} {
		if err := ValidateInsecureHost(host, allowCGNAT); err != nil {
			t.Errorf("%s rejected: %v", host, err)
		}
	}
	for host, allowCGNAT := range map[string]bool{"203.0.113.9:5000": true, "100.64.1.2:5000": false, "reg.example": false} {
		if err := ValidateInsecureHost(host, allowCGNAT); err == nil {
			t.Errorf("%s accepted", host)
		}
	}
}

func TestCertBundlePEMMatchesKeyID(t *testing.T) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	b, err := CertBundlePEM(key, time.Now(), time.Hour)
	if err != nil || !strings.Contains(string(b), "BEGIN CERTIFICATE") {
		t.Fatalf("bundle: %v", err)
	}
}

func TestCacheRepoGetsTokenScopeForRemoteBuilds(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	iss := testIssuer(t, &now)
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	svc := &RegistryTokenService{Issuer: iss, Key: key, Service: "svc", TokenIssuer: "lr", BearerTTL: func() time.Duration { return 90 * time.Second },
		Host: func() string { return "reg.example" }, now: func() time.Time { return now }}
	tok, err := iss.IssueWithCache("reg.example/levelrail/app", "b1", "reg.example/buildcache/app", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	for _, scope := range []string{"repository:buildcache/app:pull", "repository:buildcache/app:push,pull"} {
		jwt, _, err := svc.Exchange(tok, []string{scope})
		if err != nil {
			t.Fatal(err)
		}
		if len(accessOf(decodeJWT(t, jwt, &key.PublicKey))) != 1 {
			t.Fatalf("%s: cache scope was not granted", scope)
		}
	}
	jwt, _, err := svc.Exchange(tok, []string{"repository:buildcache/other:push,pull", "repository:levelrail/app:pull"})
	if err != nil || len(accessOf(decodeJWT(t, jwt, &key.PublicKey))) != 0 {
		t.Fatal("cache scope leaked to another repository or pull-only job repo was granted")
	}
	cat, err := svc.MintCatalog()
	if err != nil || len(accessOf(decodeJWT(t, cat, &key.PublicKey))) != 1 {
		t.Fatal("catalog token should carry one access entry")
	}
}

func TestCacheAuthProviderMatchesOnlyCacheRepo(t *testing.T) {
	f := pushAuthConfigWithCache("reg.example", "", "buildcache/app", "tok")
	if f("reg.example", []string{"repository:buildcache/app:pull"}).Password != "tok" {
		t.Fatal("cache pull should get the token")
	}
	if f("reg.example", []string{"repository:library/alpine:pull"}).Password != "" {
		t.Fatal("other repositories must not get the token")
	}
	if f("other.example", []string{"repository:buildcache/app:pull"}).Password != "" {
		t.Fatal("other hosts must not get the token")
	}
}
