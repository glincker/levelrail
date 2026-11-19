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
	svc := &RegistryTokenService{Issuer: iss, Key: key, Service: "svc", TokenIssuer: "lr", MaxTTL: 10 * time.Minute,
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
	if len(accessOf(c)) != 1 || c["aud"] != "svc" || exp.Sub(now) != 10*time.Minute {
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
	if err := ValidateInsecureHost("203.0.113.9:5000"); err == nil {
		t.Error("public insecure host accepted")
	}
	if err := ValidateInsecureHost("10.0.0.4:5000"); err != nil {
		t.Errorf("private host rejected: %v", err)
	}
}
