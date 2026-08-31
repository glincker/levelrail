package alerting

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeCertSource is an in-memory CertSource, the same hand-written-fake
// pattern every other package in this codebase uses instead of a
// mocking framework.
type fakeCertSource struct {
	certs map[string][]byte // storage key -> PEM bytes
}

func newFakeCertSource() *fakeCertSource {
	return &fakeCertSource{certs: make(map[string][]byte)}
}

func (f *fakeCertSource) seed(t *testing.T, domain string, notBefore, notAfter time.Time) {
	t.Helper()
	key := "certificates/internal/" + domain + "/" + domain + ".crt"
	f.certs[key] = genCertPEM(t, domain, notBefore, notAfter)
}

func (f *fakeCertSource) ListCertStorageKeys(_ context.Context, prefix string, _ bool) ([]string, error) {
	var out []string
	for k := range f.certs {
		if strings.HasPrefix(k, prefix) {
			out = append(out, k)
		}
	}
	return out, nil
}

func (f *fakeCertSource) GetCertStorageValue(_ context.Context, key string) (*store.CertStorageValue, error) {
	v, ok := f.certs[key]
	if !ok {
		return nil, store.ErrCertStorageKeyNotFound
	}
	return &store.CertStorageValue{Value: v}, nil
}

// genCertPEM generates a real, self-signed leaf certificate PEM for
// domain with the given notBefore/notAfter, matching
// internal/api/certificates_test.go's own seedCert helper.
func genCertPEM(t *testing.T, domain string, notBefore, notAfter time.Time) []byte {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: domain},
		DNSNames:     []string{domain},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		Issuer:       pkix.Name{CommonName: "Test CA"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

// fakeCertExpiryObservationStore is an in-memory
// CertExpiryObservationStore.
type fakeCertExpiryObservationStore struct {
	mu   sync.Mutex
	rows map[string]CertExpiryObservation // ruleID + "|" + domain -> row
}

func newFakeCertExpiryObservationStore() *fakeCertExpiryObservationStore {
	return &fakeCertExpiryObservationStore{rows: make(map[string]CertExpiryObservation)}
}

func (f *fakeCertExpiryObservationStore) UpsertCertExpiryObservation(_ context.Context, o CertExpiryObservation) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rows[o.RuleID+"|"+o.Domain] = o
	return nil
}

func (f *fakeCertExpiryObservationStore) ListCertExpiryObservations(_ context.Context, ruleID string) ([]CertExpiryObservation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []CertExpiryObservation
	for _, o := range f.rows {
		if o.RuleID == ruleID {
			out = append(out, o)
		}
	}
	return out, nil
}

func TestCertExpiryStatus(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	const window = 14 * 24 * time.Hour

	tests := []struct {
		name     string
		notAfter time.Time
		want     string
	}{
		{"far in the future", now.Add(60 * 24 * time.Hour), "healthy"},
		{"just inside the warning window", now.Add(13 * 24 * time.Hour), "expiring_soon"},
		{"exactly at the warning window boundary", now.Add(window), "healthy"},
		{"already past", now.Add(-1 * time.Hour), "expired"},
		{"expires this instant", now, "expired"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CertExpiryStatus(tt.notAfter, now, window); got != tt.want {
				t.Errorf("CertExpiryStatus(%v, %v, %v) = %q, want %q", tt.notAfter, now, window, got, tt.want)
			}
		})
	}
}

func TestEvaluateCertExpiry_NoCertificates_NotFiring(t *testing.T) {
	certs := newFakeCertSource()
	snapshots := newFakeCertExpiryObservationStore()
	now := time.Now()
	r := Rule{ID: "r1", Name: "cert watch", Kind: KindCertExpiry, Enabled: true}

	next, notices, err := EvaluateCertExpiry(context.Background(), certs, snapshots, r, 0, 0, now, nil)
	if err != nil {
		t.Fatalf("EvaluateCertExpiry() error = %v", err)
	}
	if next.Firing {
		t.Error("Firing = true, want false with no certificates on record")
	}
	if len(notices) != 0 {
		t.Errorf("notices = %v, want none", notices)
	}
}

func TestEvaluateCertExpiry_HealthyCert_DoesNotFire(t *testing.T) {
	certs := newFakeCertSource()
	now := time.Now()
	certs.seed(t, "healthy.example.com", now.Add(-24*time.Hour), now.Add(60*24*time.Hour))
	snapshots := newFakeCertExpiryObservationStore()
	r := Rule{ID: "r1", Name: "cert watch", Kind: KindCertExpiry, Enabled: true}

	next, notices, err := EvaluateCertExpiry(context.Background(), certs, snapshots, r, 14*24*time.Hour, 0, now, nil)
	if err != nil {
		t.Fatalf("EvaluateCertExpiry() error = %v", err)
	}
	if next.Firing {
		t.Error("Firing = true, want false for a healthy certificate")
	}
	if len(notices) != 0 {
		t.Errorf("notices = %v, want none for a healthy certificate", notices)
	}
	if next.LastValue == nil || *next.LastValue < 59 {
		t.Errorf("LastValue = %v, want roughly 60 (days until expiry)", next.LastValue)
	}
}

func TestEvaluateCertExpiry_ExpiringSoon_FiresImmediately(t *testing.T) {
	certs := newFakeCertSource()
	now := time.Now()
	// 13 days out is inside a 14-day warning window: should fire on the
	// very first evaluation, no ForDuration debounce (unlike a threshold
	// rule, expiry status only moves monotonically with real time).
	certs.seed(t, "soon.example.com", now.Add(-24*time.Hour), now.Add(13*24*time.Hour))
	snapshots := newFakeCertExpiryObservationStore()
	r := Rule{ID: "r1", Name: "cert watch", Kind: KindCertExpiry, Enabled: true}

	next, notices, err := EvaluateCertExpiry(context.Background(), certs, snapshots, r, 14*24*time.Hour, 0, now, nil)
	if err != nil {
		t.Fatalf("EvaluateCertExpiry() error = %v", err)
	}
	if !next.Firing {
		t.Error("Firing = false, want true for a certificate inside the warning window")
	}
	if len(notices) != 1 || !strings.Contains(notices[0], "soon.example.com") {
		t.Errorf("notices = %v, want exactly one mentioning soon.example.com", notices)
	}
	if strings.Contains(notices[0], "stalled") {
		t.Errorf("notices[0] = %q, should not claim a stalled renewal on the very first observation", notices[0])
	}
}

func TestEvaluateCertExpiry_ExactlyAtWindowBoundary_DoesNotFireEarly(t *testing.T) {
	certs := newFakeCertSource()
	// X.509 (ASN.1 UTCTime) only encodes second precision, so notAfter
	// must be truncated the same way before comparing, or the round
	// trip through genCertPEM below loses sub-second precision and
	// flips a value meant to land exactly on the boundary.
	now := time.Now().Truncate(time.Second)
	window := 14 * 24 * time.Hour
	// Exactly at the boundary is still "healthy" (CertExpiryStatus's own
	// exclusive-boundary contract): the rule must not fire early.
	certs.seed(t, "boundary.example.com", now.Add(-24*time.Hour), now.Add(window))
	snapshots := newFakeCertExpiryObservationStore()
	r := Rule{ID: "r1", Name: "cert watch", Kind: KindCertExpiry, Enabled: true}

	next, notices, err := EvaluateCertExpiry(context.Background(), certs, snapshots, r, window, 0, now, nil)
	if err != nil {
		t.Fatalf("EvaluateCertExpiry() error = %v", err)
	}
	if next.Firing {
		t.Error("Firing = true, want false exactly at the warning window boundary")
	}
	if len(notices) != 0 {
		t.Errorf("notices = %v, want none exactly at the boundary", notices)
	}
}

func TestEvaluateCertExpiry_ResolvesOnceRenewed(t *testing.T) {
	certs := newFakeCertSource()
	now := time.Now()
	certs.seed(t, "renews.example.com", now.Add(-24*time.Hour), now.Add(5*24*time.Hour))
	snapshots := newFakeCertExpiryObservationStore()
	r := Rule{ID: "r1", Name: "cert watch", Kind: KindCertExpiry, Enabled: true}

	next, _, err := EvaluateCertExpiry(context.Background(), certs, snapshots, r, 14*24*time.Hour, 0, now, nil)
	if err != nil {
		t.Fatalf("EvaluateCertExpiry() error = %v", err)
	}
	if !next.Firing {
		t.Fatal("Firing = false, want true before the renewal")
	}

	// A real renewal replaces the stored certificate with a fresh one,
	// far from expiry again.
	certs.certs["certificates/internal/renews.example.com/renews.example.com.crt"] =
		genCertPEM(t, "renews.example.com", now, now.Add(90*24*time.Hour))

	resolved, notices, err := EvaluateCertExpiry(context.Background(), certs, snapshots, next, 14*24*time.Hour, 0, now.Add(time.Minute), nil)
	if err != nil {
		t.Fatalf("EvaluateCertExpiry() second call error = %v", err)
	}
	if resolved.Firing {
		t.Error("Firing = true, want false once the certificate has actually been renewed")
	}
	if len(notices) != 0 {
		t.Errorf("notices = %v, want none once resolved", notices)
	}
}

func TestEvaluateCertExpiry_StalledRenewal_FlaggedAfterThreshold(t *testing.T) {
	certs := newFakeCertSource()
	now := time.Now()
	certs.seed(t, "stuck.example.com", now.Add(-80*24*time.Hour), now.Add(5*24*time.Hour))
	snapshots := newFakeCertExpiryObservationStore()
	r := Rule{ID: "r1", Name: "cert watch", Kind: KindCertExpiry, Enabled: true}
	stalledThreshold := time.Hour

	// First observation: a fresh episode, never flagged as stalled even
	// though it's already inside the warning window.
	next, notices, err := EvaluateCertExpiry(context.Background(), certs, snapshots, r, 14*24*time.Hour, stalledThreshold, now, nil)
	if err != nil {
		t.Fatalf("first EvaluateCertExpiry() error = %v", err)
	}
	if len(notices) != 1 || strings.Contains(notices[0], "stalled") {
		t.Fatalf("first notices = %v, want exactly one, not yet flagged as stalled", notices)
	}

	// Same NotAfter, elapsed time still under the threshold: still not
	// stalled.
	next, notices, err = EvaluateCertExpiry(context.Background(), certs, snapshots, next, 14*24*time.Hour, stalledThreshold, now.Add(30*time.Minute), nil)
	if err != nil {
		t.Fatalf("second EvaluateCertExpiry() error = %v", err)
	}
	if len(notices) != 1 || strings.Contains(notices[0], "stalled") {
		t.Fatalf("second notices = %v, want exactly one, still not stalled before the threshold elapses", notices)
	}

	// Same NotAfter, elapsed time now past stalledThreshold: this is the
	// stronger "renewal appears stalled" signal.
	_, notices, err = EvaluateCertExpiry(context.Background(), certs, snapshots, next, 14*24*time.Hour, stalledThreshold, now.Add(2*time.Hour), nil)
	if err != nil {
		t.Fatalf("third EvaluateCertExpiry() error = %v", err)
	}
	if len(notices) != 1 || !strings.Contains(notices[0], "stalled") {
		t.Fatalf("third notices = %v, want exactly one flagged as stalled once the threshold has elapsed with no NotAfter movement", notices)
	}
}

func TestEngine_Tick_CertExpiryFires_NotifiesOnceThenDoesNotDoubleFire(t *testing.T) {
	certs := newFakeCertSource()
	now := time.Now()
	certs.seed(t, "soon.example.com", now.Add(-24*time.Hour), now.Add(5*24*time.Hour))

	r := Rule{ID: "r1", Name: "cert watch", Kind: KindCertExpiry, Enabled: true}
	rules := newFakeRuleStore(r)
	spy := &spyNotifier{}
	engine := NewEngine(rules, nil, nil, nil, certs, nil, 14*24*time.Hour, time.Hour, nil, 0, func(Rule) Notifier { return spy }, nil)

	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("first Tick() error = %v", err)
	}
	if calls := spy.calls(); len(calls) != 1 {
		t.Fatalf("after first Tick, notify calls = %d, want 1", len(calls))
	} else if calls[0].Resolved {
		t.Error("first notification should not be a resolved event")
	} else if len(calls[0].CertNotices) != 1 {
		t.Errorf("CertNotices = %v, want exactly one entry", calls[0].CertNotices)
	}

	// Certificate status hasn't changed: a second tick must not
	// re-notify (Engine only dispatches on a firing/resolved
	// transition, never on every tick a rule stays in the same state).
	if err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("second Tick() error = %v", err)
	}
	if calls := spy.calls(); len(calls) != 1 {
		t.Errorf("after second Tick, notify calls = %d, want still 1 (no double-fire)", len(calls))
	}
	if got := rules.get("r1"); !got.Firing {
		t.Error("rule state Firing = false, want true to persist across ticks")
	}
}
