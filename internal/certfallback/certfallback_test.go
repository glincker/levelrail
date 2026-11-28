package certfallback

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/store"
)

type memStore struct {
	mu sync.Mutex
	m  map[string][]byte
}

func (s *memStore) ListCertStorageKeys(_ context.Context, prefix string, _ bool) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for k := range s.m {
		if strings.HasPrefix(k, prefix) {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out, nil
}

func (s *memStore) GetCertStorageValue(_ context.Context, key string) (*store.CertStorageValue, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.m[key]
	if !ok {
		return nil, store.ErrCertStorageKeyNotFound
	}
	return &store.CertStorageValue{Value: v}, nil
}

func (s *memStore) DeleteCertStorageValue(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k := range s.m {
		if k == key || strings.HasPrefix(k, key+"/") {
			delete(s.m, k)
		}
	}
	return nil
}

func (s *memStore) has(prefix string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k := range s.m {
		if strings.HasPrefix(k, prefix) {
			return true
		}
	}
	return false
}

// seed stores a self-signed leaf for domain under issuer key segment seg.
func (s *memStore) seed(t *testing.T, seg, domain string) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: domain}, DNSNames: []string{domain},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatal(err)
	}
	if s.m == nil {
		s.m = map[string][]byte{}
	}
	dir := "certificates/" + seg + "/" + domain
	s.m[dir+"/"+domain+".crt"] = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	s.m[dir+"/"+domain+".key"] = []byte("key")
}

func TestDropRemovesOnlyTheFallback(t *testing.T) {
	s := &memStore{}
	s.seed(t, "local", "a.example.dev")
	s.seed(t, "acme-v02.api.letsencrypt.org-directory", "b.example.dev")
	n, err := Drop(context.Background(), s, "A.example.dev")
	if err != nil || n != 1 {
		t.Fatalf("Drop = %d, %v; want 1, nil", n, err)
	}
	if s.has("certificates/local/a.example.dev") {
		t.Error("fallback certificate for a.example.dev was not dropped")
	}
	n, _ = Drop(context.Background(), s, "b.example.dev")
	if n != 0 || !s.has("certificates/acme-v02.api.letsencrypt.org-directory/b.example.dev") {
		t.Errorf("public-CA certificate must survive, dropped=%d", n)
	}
}

func TestRetrierPicksOnlyResolvingFallbackDomains(t *testing.T) {
	s := &memStore{}
	s.seed(t, "local", "up.example.dev")
	s.seed(t, "local", "away.example.dev")
	s.seed(t, "acme-v02.api.letsencrypt.org-directory", "good.example.dev")
	var audited []string
	nudges := 0
	r := &Retrier{
		Store: s,
		Candidates: func(context.Context) []string {
			return []string{"up.example.dev", "away.example.dev", "good.example.dev"}
		},
		Resolves:   func(_ context.Context, d string) bool { return d != "away.example.dev" },
		MaxRetries: func(context.Context) int { return 2 },
		Audit:      func(_ context.Context, d, _ string) { audited = append(audited, d) },
		Nudge:      func() { nudges++ },
	}
	got, err := r.Pass(context.Background())
	if err != nil || len(got) != 1 || got[0] != "up.example.dev" {
		t.Fatalf("Pass = %v, %v; want [up.example.dev]", got, err)
	}
	if len(audited) != 1 || nudges != 1 {
		t.Errorf("audited=%v nudges=%d", audited, nudges)
	}
	if !s.has("certificates/local/away.example.dev") || !s.has("certificates/acme-v02.api.letsencrypt.org-directory/good.example.dev") {
		t.Error("a non-resolving fallback or a public certificate was removed")
	}
}

func TestRetrierStopsAtLimit(t *testing.T) {
	s := &memStore{}
	r := &Retrier{
		Store:      s,
		Candidates: func(context.Context) []string { return []string{"up.example.dev"} },
		Resolves:   func(context.Context, string) bool { return true },
		MaxRetries: func(context.Context) int { return 1 },
	}
	s.seed(t, "local", "up.example.dev")
	if got, _ := r.Pass(context.Background()); len(got) != 1 {
		t.Fatalf("first pass dropped %v", got)
	}
	s.seed(t, "local", "up.example.dev") // CA failed again, fallback is back
	if got, _ := r.Pass(context.Background()); len(got) != 0 {
		t.Fatalf("second pass dropped %v, want none past the limit", got)
	}
}
