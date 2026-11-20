package buildrecord

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"strings"
	"testing"
	"time"
)

type memStore struct {
	recs   map[string]Signed
	linked map[string]string
}

func newMem() *memStore { return &memStore{recs: map[string]Signed{}, linked: map[string]string{}} }

func (m *memStore) SaveRecord(_ context.Context, id, _ string, s Signed) error {
	m.recs[id] = s
	return nil
}
func (m *memStore) GetRecord(_ context.Context, id string) (Signed, error) {
	r, ok := m.recs[id]
	if !ok {
		return Signed{}, errors.New("missing")
	}
	return r, nil
}
func (m *memStore) LinkAttempt(_ context.Context, id, a string) error { m.linked[id] = a; return nil }

func testSigner(t *testing.T) *Signer {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return NewSigner(priv)
}

func claims() Claims {
	return Claims{App: "web", Environment: "prod", Repo: "reg/web:abc", Digest: "sha256:" + strings.Repeat("a", 64),
		BuilderNodeID: "n1", BuilderTrust: TierPersonal, SourceRef: "abc", IssuedAt: time.Unix(1700000000, 0).UTC()}
}

func TestSignVerifyAndTamper(t *testing.T) {
	s := testSigner(t)
	rec, err := s.Sign(claims())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Verify(rec); err != nil {
		t.Fatalf("verify: %v", err)
	}
	bad := rec
	bad.Claims.Digest = "sha256:" + strings.Repeat("b", 64)
	if err := s.Verify(bad); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("tampered digest must fail, got %v", err)
	}
	if err := Verify(testSigner(t).PublicKey(), rec); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("other key must fail, got %v", err)
	}
}

func gateFor(s *Signer, st Store, p Policy) *Gate {
	return &Gate{Signer: s, Store: st, Policy: func(context.Context, string) (Policy, error) { return p, nil }}
}

func TestGatePersonalRules(t *testing.T) {
	s, st := testSigner(t), newMem()
	rec, _ := s.Sign(claims())
	_ = st.SaveRecord(context.Background(), "r1", "web", rec)
	d := claims().Digest

	prod := gateFor(s, st, Policy{EnvKind: "production"})
	if err := prod.CheckBuild(context.Background(), "web", "a1", "r1", d, TierPersonal, "n1"); err == nil {
		t.Fatal("personal builder in production must be refused")
	}
	dev := gateFor(s, st, Policy{EnvKind: "development"})
	if err := dev.CheckBuild(context.Background(), "web", "a1", "r1", d, TierPersonal, "n1"); err != nil {
		t.Fatalf("valid record: %v", err)
	}
	if st.linked["r1"] != "a1" {
		t.Fatal("record was not linked to the attempt")
	}
	if err := dev.CheckBuild(context.Background(), "web", "a1", "r1", "sha256:"+strings.Repeat("c", 64), TierPersonal, "n1"); err == nil {
		t.Fatal("digest not in record must fail")
	}
	if err := dev.CheckBuild(context.Background(), "web", "a1", "", d, TierPersonal, "n1"); err == nil {
		t.Fatal("missing record must fail closed")
	}
	st.recs["r1"] = Signed{Claims: rec.Claims, Signature: "AAAA", KeyID: rec.KeyID}
	if err := dev.CheckBuild(context.Background(), "web", "a1", "r1", d, TierPersonal, "n1"); err == nil {
		t.Fatal("bad signature must fail")
	}
}

func TestGateServerAndTarUnaffected(t *testing.T) {
	g := gateFor(testSigner(t), newMem(), Policy{EnvKind: "production"})
	if err := g.CheckBuild(context.Background(), "web", "a1", "", "", "", ""); err != nil {
		t.Fatalf("tar or local build: %v", err)
	}
	if err := g.CheckBuild(context.Background(), "web", "a1", "", "", TierServer, "n1"); err != nil {
		t.Fatalf("server tier: %v", err)
	}
}

func TestAllowListDecidesAlone(t *testing.T) {
	p := Policy{EnvKind: "production", Allowed: []string{TierManaged}}
	if p.TierAllowed(TierPersonal, "n1") || !p.TierAllowed(TierManaged, "n2") {
		t.Fatal("allow-list must decide alone")
	}
}
