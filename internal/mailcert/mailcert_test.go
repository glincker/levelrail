package mailcert

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
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
	files      map[string][]byte
	modes      map[string]fs.FileMode
	wide       bool
	failWrite  map[string]bool
	failRemove map[string]bool
}

var errInjected = errors.New("injected")

func (v *fakeVol) Read(_ context.Context, p string) ([]byte, bool, error) {
	b, ok := v.files[p]
	return b, ok, nil
}
func (v *fakeVol) Write(_ context.Context, p string, b []byte, private bool) error {
	if v.failWrite[p] {
		return errInjected
	}
	v.files[p] = b
	v.modes[p] = 0o644
	if private && !v.wide {
		v.modes[p] = 0o600
	}
	return nil
}
func (v *fakeVol) Mode(_ context.Context, p string) (fs.FileMode, error) { return v.modes[p], nil }
func (v *fakeVol) Remove(_ context.Context, p string) error {
	if v.failRemove[p] {
		return errInjected
	}
	delete(v.files, p)
	delete(v.modes, p)
	return nil
}

type fakeSwitch struct {
	vol               *fakeVol
	activated, revert int
	staleServe        bool
	revertCtxDone     bool
}

func (s *fakeSwitch) Activate(context.Context) error { s.activated++; return nil }
func (s *fakeSwitch) Revert(ctx context.Context) error {
	s.revert++
	s.revertCtxDone = ctx.Err() != nil
	return nil
}
func (s *fakeSwitch) Served(context.Context) (Info, error) {
	if s.staleServe {
		return Info{Serial: "old"}, nil
	}
	return describeFile(s.vol.files[CertPath]), nil
}

func describeFile(c []byte) Info {
	b, _ := pem.Decode(c)
	leaf, _ := x509.ParseCertificate(b.Bytes)
	return describe(leaf)
}

func TestInstallWritesKey0600KeepsPrevAndRollsBack(t *testing.T) {
	cfg := testCfg()
	oldC, oldK := genPair(t, time.Now().Add(time.Hour))
	newC, newK := genPair(t, time.Now().Add(48*time.Hour))
	want, _ := Validate(newC, newK, "mail.example.test", time.Now())

	vol := &fakeVol{files: map[string][]byte{CertPath: oldC, KeyPath: oldK}, modes: map[string]fs.FileMode{}}
	sw := &fakeSwitch{vol: vol}
	res, err := Install(context.Background(), cfg, vol, sw, newC, newK, want, Origin{})
	if err != nil || res.Outcome != OutcomeInstalled || res.Served.Serial != want.Serial {
		t.Fatalf("install: %+v %v", res, err)
	}
	if vol.modes[KeyPath] != 0o600 || vol.modes[PrevKeyPath] != 0o600 || string(vol.files[PrevKeyPath]) != string(oldK) {
		t.Fatalf("modes %v", vol.modes)
	}

	sw.staleServe = true
	res, err = Install(context.Background(), cfg, vol, sw, oldC, oldK, Info{Serial: "never"}, Origin{})
	if err == nil || res.Outcome != OutcomeRolledBack || sw.revert != 1 || string(vol.files[CertPath]) != string(newC) {
		t.Fatalf("rollback: %+v %v revert=%d", res, err, sw.revert)
	}

	wide := &fakeVol{files: map[string][]byte{}, modes: map[string]fs.FileMode{}, wide: true}
	if _, err := Install(context.Background(), cfg, wide, &fakeSwitch{vol: wide}, newC, newK, want, Origin{}); !errors.Is(err, ErrKeyModeTooWide) {
		t.Fatalf("wide key: %v", err)
	}
	if _, ok := wide.files[KeyPath]; ok {
		t.Fatal("a key written too wide must be removed")
	}
}

func testCfg() Config {
	return Config{VerifyTimeout: 10 * time.Millisecond, PollInterval: time.Millisecond, RollbackTimeout: time.Second, InstallLease: time.Minute, MaxPEMBytes: 64 << 10}
}

func newVol(files map[string][]byte) *fakeVol {
	return &fakeVol{files: files, modes: map[string]fs.FileMode{}, failWrite: map[string]bool{}, failRemove: map[string]bool{}}
}

func TestInstallStagingAndJournal(t *testing.T) {
	cfg := testCfg()
	oldC, oldK := genPair(t, time.Now().Add(time.Hour))
	newC, newK := genPair(t, time.Now().Add(48*time.Hour))
	want, _ := Validate(newC, newK, "mail.example.test", time.Now())

	t.Run("journal blocks a second install", func(t *testing.T) {
		vol := newVol(map[string][]byte{JournalPath: []byte(`{"phase":"swapping"}`)})
		if _, err := Install(context.Background(), cfg, vol, &fakeSwitch{vol: vol}, newC, newK, want, Origin{}); !errors.Is(err, ErrInstallInProgress) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("staging failure leaves the live pair untouched", func(t *testing.T) {
		vol := newVol(map[string][]byte{CertPath: oldC, KeyPath: oldK})
		vol.failWrite[NewCertPath] = true
		sw := &fakeSwitch{vol: vol}
		res, err := Install(context.Background(), cfg, vol, sw, newC, newK, want, Origin{})
		if err == nil || res.Reason != "stage" || sw.activated != 0 || string(vol.files[KeyPath]) != string(oldK) {
			t.Fatalf("res %+v err %v", res, err)
		}
		if _, left := vol.files[JournalPath]; left {
			t.Fatal("journal left after a clean abort")
		}
	})
	t.Run("old agent delete failure is fatal", func(t *testing.T) {
		vol := newVol(map[string][]byte{})
		vol.wide, vol.failRemove[NewKeyPath] = true, true
		if _, err := Install(context.Background(), cfg, vol, &fakeSwitch{vol: vol}, newC, newK, want, Origin{}); !errors.Is(err, ErrKeyRemoveFailed) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("rollback failure is its own outcome and keeps the journal", func(t *testing.T) {
		vol := newVol(map[string][]byte{CertPath: oldC, KeyPath: oldK})
		sw := &fakeSwitch{vol: vol, staleServe: true}
		vol.failWrite[KeyPath] = false
		res, err := Install(context.Background(), cfg, &restoreFails{fakeVol: vol}, sw, newC, newK, want, Origin{})
		if err == nil || res.Outcome != OutcomeRollbackFailed {
			t.Fatalf("res %+v err %v", res, err)
		}
		if _, ok := vol.files[JournalPath]; !ok {
			t.Fatal("journal must stay after a failed rollback")
		}
	})
	t.Run("rollback runs on a fresh context", func(t *testing.T) {
		vol := newVol(map[string][]byte{CertPath: oldC, KeyPath: oldK})
		sw := &fakeSwitch{vol: vol, staleServe: true}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
		defer cancel()
		res, _ := Install(ctx, Config{VerifyTimeout: time.Second, PollInterval: time.Millisecond, RollbackTimeout: time.Second, InstallLease: time.Minute}, vol, sw, newC, newK, want, Origin{})
		if res.Outcome != OutcomeRolledBack || sw.revertCtxDone || string(vol.files[KeyPath]) != string(oldK) {
			t.Fatalf("res %+v revert ctx done %v", res, sw.revertCtxDone)
		}
	})
	t.Run("served check needs the fingerprint", func(t *testing.T) {
		vol := newVol(map[string][]byte{})
		sameSerial := want
		sameSerial.SHA256 = "other"
		res, _ := Install(context.Background(), cfg, vol, &fakeSwitch{vol: vol}, newC, newK, sameSerial, Origin{})
		if res.Outcome != OutcomeRolledBack {
			t.Fatalf("a serial match alone must not count as served: %+v", res)
		}
	})
}

// restoreFails fails every write of the live key after the first one, so the restore cannot land.
type restoreFails struct {
	*fakeVol
	keyWrites int
}

func (r *restoreFails) Write(ctx context.Context, p string, b []byte, private bool) error {
	if p == KeyPath {
		r.keyWrites++
		if r.keyWrites > 1 {
			return errInjected
		}
	}
	return r.fakeVol.Write(ctx, p, b, private)
}

func TestRecoverInterruptedInstall(t *testing.T) {
	cfg := testCfg()
	oldC, oldK := genPair(t, time.Now().Add(time.Hour))
	newC, newK := genPair(t, time.Now().Add(48*time.Hour))
	want, _ := Validate(newC, newK, "mail.example.test", time.Now())
	j := Journal{Phase: PhaseSwapping, StartedAt: time.Now().Add(-time.Hour), NewSHA256: want.SHA256, HadPrev: true}
	if !j.Stale(cfg, time.Now()) {
		t.Fatal("an hour old journal must be stale")
	}

	// Crash between the key write and the certificate write: new key, old certificate.
	vol := newVol(map[string][]byte{CertPath: oldC, KeyPath: newK, PrevCertPath: oldC, PrevKeyPath: oldK, NewKeyPath: newK, JournalPath: []byte("{}")})
	sw := &fakeSwitch{vol: vol}
	res, err := Recover(context.Background(), cfg, vol, sw, j)
	if err != nil || res.Outcome != OutcomeRolledBack || string(vol.files[KeyPath]) != string(oldK) || sw.revert != 1 {
		t.Fatalf("recover: %+v %v", res, err)
	}
	for _, p := range []string{JournalPath, NewKeyPath} {
		if _, ok := vol.files[p]; ok {
			t.Fatalf("%s left after recovery", p)
		}
	}

	served := newVol(map[string][]byte{CertPath: newC, KeyPath: newK, JournalPath: []byte("{}")})
	ssw := &fakeSwitch{vol: served}
	res, err = Recover(context.Background(), cfg, served, ssw, j)
	if err != nil || res.Outcome != OutcomeInstalled || ssw.revert != 0 || string(served.files[KeyPath]) != string(newK) {
		t.Fatalf("served pair must be kept: %+v %v", res, err)
	}
}

func TestValidateStrengthChainAndUsage(t *testing.T) {
	now := time.Now()
	const host = "mail.example.test"
	mk := func(pub, signer crypto.PrivateKey, parent *x509.Certificate, tpl *x509.Certificate) []byte {
		t.Helper()
		if parent == nil {
			parent = tpl
		}
		der, err := x509.CreateCertificate(rand.Reader, tpl, parent, pub.(crypto.Signer).Public(), signer)
		if err != nil {
			t.Fatal(err)
		}
		return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	}
	keyPEM := func(k crypto.PrivateKey) []byte {
		kd, err := x509.MarshalPKCS8PrivateKey(k)
		if err != nil {
			t.Fatal(err)
		}
		return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: kd})
	}
	leafTpl := func(eku ...x509.ExtKeyUsage) *x509.Certificate {
		return &x509.Certificate{SerialNumber: big.NewInt(now.UnixNano()), Subject: pkix.Name{CommonName: host}, DNSNames: []string{host},
			NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), ExtKeyUsage: eku}
	}
	weak, err := rsa.GenerateKey(rand.Reader, 1024) //nolint:gosec // the weak key is what the test must reject
	if err != nil {
		t.Fatal(err)
	}
	p224, _ := ecdsa.GenerateKey(elliptic.P224(), rand.Reader)
	good, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	caTpl := &x509.Certificate{SerialNumber: big.NewInt(7), Subject: pkix.Name{CommonName: "test ca"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	caPEM := mk(caKey, caKey, nil, caTpl)
	caCert, _ := x509.ParseCertificate(func() []byte { b, _ := pem.Decode(caPEM); return b.Bytes }())
	leafPEM := mk(good, caKey, caCert, leafTpl(x509.ExtKeyUsageServerAuth))

	if _, err := Validate(append(append([]byte{}, leafPEM...), caPEM...), keyPEM(good), host, now); err != nil {
		t.Fatalf("ordered chain: %v", err)
	}
	cases := map[string]struct {
		cert, key []byte
		want      string
	}{
		"rsa 1024":     {mk(weak, weak, nil, leafTpl()), keyPEM(weak), "at least 2048"},
		"p224":         {mk(p224, p224, nil, leafTpl()), keyPEM(p224), "P-256 or larger"},
		"client only":  {mk(good, good, nil, leafTpl(x509.ExtKeyUsageClientAuth)), keyPEM(good), "server authentication"},
		"out of order": {append(append([]byte{}, leafPEM...), mk(p224, p224, nil, leafTpl())...), keyPEM(good), "out of order"},
	}
	for name, tc := range cases {
		if _, err := Validate(tc.cert, tc.key, host, now); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want %q", name, err, tc.want)
		}
	}
	cfg := Config{MaxPEMBytes: 10}
	if err := cfg.CheckSize(leafPEM, []byte("k")); !errors.Is(err, ErrPEMTooLarge) {
		t.Fatalf("size cap: %v", err)
	}
}

func TestProtectedPathsAndBackupExcludes(t *testing.T) {
	for _, p := range []string{CertPath, KeyPath, PrevCertPath, PrevKeyPath, NewKeyPath, JournalPath, "CERTS/Managed.KEY", "certs/.upload-abc"} {
		if !IsProtectedPath(p) {
			t.Errorf("%s must be protected", p)
		}
	}
	for _, p := range []string{"certs/selfsigned.pem", "mox.conf", "other/managed.key", "certs"} {
		if IsProtectedPath(p) {
			t.Errorf("%s must not be protected", p)
		}
	}
	if len(BackupExcludes(TemplateID, ConfigVolume)) == 0 || BackupExcludes("other", ConfigVolume) != nil || BackupExcludes(TemplateID, "data") != nil {
		t.Fatal("backup excludes apply to the mail config volume only")
	}
}
