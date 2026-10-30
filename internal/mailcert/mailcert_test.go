package mailcert

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io/fs"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"
)

func genPair(t *testing.T, notAfter time.Time) (certPEM, keyPEM []byte) {
	t.Helper()
	const host = "mail.example.test"
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serial, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
	tpl := &x509.Certificate{
		SerialNumber: serial, Subject: pkix.Name{CommonName: host}, DNSNames: []string{host},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: notAfter,
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &k.PublicKey, k)
	if err != nil {
		t.Fatal(err)
	}
	kd, err := x509.MarshalPKCS8PrivateKey(k)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: kd})
}

func TestValidate(t *testing.T) {
	now := time.Now()
	c, k := genPair(t, now.Add(90*24*time.Hour))
	info, err := Validate(c, k, "mail.example.test", now)
	if err != nil || info.Serial == "" || !info.SelfSigned || info.DaysLeft(now) < 89 {
		t.Fatalf("valid pair: %+v %v", info, err)
	}
	_, otherKey := genPair(t, now.Add(time.Hour))
	expC, expK := genPair(t, now.Add(time.Hour))
	enc := pem.EncodeToMemory(&pem.Block{Type: "ENCRYPTED PRIVATE KEY", Bytes: []byte{1}})
	cases := map[string]struct {
		cert, key []byte
		host      string
		at        time.Time
		want      string
	}{
		"mismatch":     {c, otherKey, "mail.example.test", now, "does not match"},
		"wrong name":   {c, k, "mx.other.test", now, "does not cover"},
		"expired":      {expC, expK, "mail.example.test", now.Add(2 * time.Hour), "expired"},
		"encrypted":    {c, enc, "mail.example.test", now, "encrypted"},
		"key in chain": {append(append([]byte{}, c...), k...), k, "mail.example.test", now, "only CERTIFICATE"},
	}
	for name, tc := range cases {
		_, err := Validate(tc.cert, tc.key, tc.host, tc.at)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want %q", name, err, tc.want)
		}
		if err != nil && strings.Contains(err.Error(), "PRIVATE KEY-----") {
			t.Errorf("%s: error leaks key material", name)
		}
	}
}

func TestWindow(t *testing.T) {
	w, err := ParseWindow("23:00-02:00")
	if err != nil {
		t.Fatal(err)
	}
	at := func(h int) time.Time { return time.Date(2026, 1, 1, h, 30, 0, 0, time.UTC) }
	if !w.Contains(at(23)) || !w.Contains(at(1)) || w.Contains(at(12)) {
		t.Fatal("wrapping window wrong")
	}
	if !(Window{}).Contains(at(12)) {
		t.Fatal("zero window must allow any time")
	}
	if _, err := ParseWindow("nope"); err == nil {
		t.Fatal("bad window accepted")
	}
}

func TestProbeRefusesPublicAndReadsServedCert(t *testing.T) {
	if _, err := Probe(context.Background(), "8.8.8.8", 465, time.Second); !errors.Is(err, ErrNotPrivateTarget) {
		t.Fatalf("public target: %v", err)
	}
	c, k := genPair(t, time.Now().Add(24*time.Hour))
	pair, err := tls.X509KeyPair(c, k)
	if err != nil {
		t.Fatal(err)
	}
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{pair}, MinVersion: tls.VersionTLS12})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.(*tls.Conn).Handshake()
			_ = conn.Close()
		}
	}()
	port := ln.Addr().(*net.TCPAddr).Port
	got, err := Probe(context.Background(), "127.0.0.1", port, 2*time.Second)
	want, _ := Validate(c, k, "mail.example.test", time.Now())
	if err != nil || got.Serial != want.Serial {
		t.Fatalf("probe = %+v, %v", got, err)
	}
}

type fakeVol struct {
	files map[string][]byte
	modes map[string]fs.FileMode
	wide  bool
}

func (v *fakeVol) Read(_ context.Context, p string) ([]byte, bool, error) {
	b, ok := v.files[p]
	return b, ok, nil
}
func (v *fakeVol) Write(_ context.Context, p string, b []byte, private bool) error {
	v.files[p] = b
	v.modes[p] = 0o644
	if private && !v.wide {
		v.modes[p] = 0o600
	}
	return nil
}
func (v *fakeVol) Mode(_ context.Context, p string) (fs.FileMode, error) { return v.modes[p], nil }
func (v *fakeVol) Remove(_ context.Context, p string) error {
	delete(v.files, p)
	delete(v.modes, p)
	return nil
}

type fakeSwitch struct {
	vol               *fakeVol
	activated, revert int
	staleServe        bool
}

func (s *fakeSwitch) Activate(context.Context) error { s.activated++; return nil }
func (s *fakeSwitch) Revert(context.Context) error   { s.revert++; return nil }
func (s *fakeSwitch) Served(context.Context) (Info, error) {
	if s.staleServe {
		return Info{Serial: "old"}, nil
	}
	c := s.vol.files[CertPath]
	b, _ := pem.Decode(c)
	leaf, _ := x509.ParseCertificate(b.Bytes)
	return describe(leaf), nil
}

func TestInstallWritesKey0600KeepsPrevAndRollsBack(t *testing.T) {
	cfg := Config{VerifyTimeout: 10 * time.Millisecond, PollInterval: time.Millisecond}
	oldC, oldK := genPair(t, time.Now().Add(time.Hour))
	newC, newK := genPair(t, time.Now().Add(48*time.Hour))
	want, _ := Validate(newC, newK, "mail.example.test", time.Now())

	vol := &fakeVol{files: map[string][]byte{CertPath: oldC, KeyPath: oldK}, modes: map[string]fs.FileMode{}}
	sw := &fakeSwitch{vol: vol}
	res, err := Install(context.Background(), cfg, vol, sw, newC, newK, want)
	if err != nil || res.Outcome != OutcomeInstalled || res.Served.Serial != want.Serial {
		t.Fatalf("install: %+v %v", res, err)
	}
	if vol.modes[KeyPath] != 0o600 || vol.modes[PrevKeyPath] != 0o600 || string(vol.files[PrevKeyPath]) != string(oldK) {
		t.Fatalf("modes %v", vol.modes)
	}

	sw.staleServe = true
	res, err = Install(context.Background(), cfg, vol, sw, oldC, oldK, Info{Serial: "never"})
	if err == nil || res.Outcome != OutcomeRolledBack || sw.revert != 1 || string(vol.files[CertPath]) != string(newC) {
		t.Fatalf("rollback: %+v %v revert=%d", res, err, sw.revert)
	}

	wide := &fakeVol{files: map[string][]byte{}, modes: map[string]fs.FileMode{}, wide: true}
	if _, err := Install(context.Background(), cfg, wide, &fakeSwitch{vol: wide}, newC, newK, want); !errors.Is(err, ErrKeyModeTooWide) {
		t.Fatalf("wide key: %v", err)
	}
	if _, ok := wide.files[KeyPath]; ok {
		t.Fatal("a key written too wide must be removed")
	}
}
