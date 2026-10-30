package monitor

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/kit/netguard"
)

type memHB struct {
	byID map[string]*HeartbeatState
}

func (h *memHB) SaveHeartbeatToken(_ context.Context, id, hash string, at int64) error {
	h.byID[id] = &HeartbeatState{MonitorID: id, TokenHash: hash, ArmedAt: at}
	return nil
}
func (h *memHB) FindHeartbeat(_ context.Context, hash string) (HeartbeatState, error) {
	for _, v := range h.byID {
		if v.TokenHash == hash {
			return *v, nil
		}
	}
	return HeartbeatState{}, ErrNotFound
}
func (h *memHB) GetHeartbeat(_ context.Context, id string) (HeartbeatState, error) {
	if v, ok := h.byID[id]; ok {
		return *v, nil
	}
	return HeartbeatState{}, ErrNotFound
}
func (h *memHB) RecordHeartbeat(_ context.Context, id, signal string, at int64) error {
	v := h.byID[id]
	switch signal {
	case PingStart:
		v.LastStart = at
	case PingFail:
		v.LastFail = at
	default:
		v.LastOK = at
	}
	return nil
}

type memStore struct {
	ResultSink
	mons map[string]Monitor
}

func (s *memStore) GetMonitor(_ context.Context, id string) (Monitor, error) {
	if m, ok := s.mons[id]; ok {
		return m, nil
	}
	return Monitor{}, ErrNotFound
}
func (s *memStore) SaveMonitor(_ context.Context, m Monitor) error { s.mons[m.ID] = m; return nil }
func (s *memStore) CountMonitors(context.Context) (int, error)     { return len(s.mons), nil }
func (s *memStore) DeleteMonitor(_ context.Context, id string) error {
	delete(s.mons, id)
	return nil
}
func (s *memStore) ListMonitors(context.Context) ([]Monitor, error) { return nil, nil }
func (s *memStore) ListResults(context.Context, string, int64, int) ([]Result, error) {
	return nil, nil
}
func (s *memStore) ListRollups(context.Context, string, int64, int) ([]Rollup, error) {
	return nil, nil
}
func (s *memStore) PruneResults(context.Context, string, int64, int) (int, error) { return 0, nil }
func (s *memStore) PruneRollups(context.Context, string, int64, int) (int, error) { return 0, nil }

type nopSink struct{}

func (nopSink) InsertResults(context.Context, []Result) error { return nil }

func newHBService(t *testing.T, events *[]Transition) (*Service, *time.Time, Monitor) {
	t.Helper()
	st := &memStore{ResultSink: nopSink{}, mons: map[string]Monitor{}}
	svc := New(Options{Config: testConfig(), Store: st, Heartbeats: &memHB{byID: map[string]*HeartbeatState{}},
		Notify: func(tr Transition) { *events = append(*events, tr) }})
	clock := time.Now()
	svc.now = func() time.Time { return clock }
	svc.started = clock
	m, err := svc.Create(context.Background(), CreateInput{Monitor: Monitor{Name: "backup", Kind: KindHeartbeat,
		Interval: time.Hour, Enabled: true, Spec: Spec{GraceSeconds: 600}}})
	if err != nil {
		t.Fatal(err)
	}
	return svc, &clock, m
}

func advance(svc *Service, clock *time.Time, d time.Duration) {
	*clock = clock.Add(d)
	svc.now = func() time.Time { return *clock }
}

func TestHeartbeatLateOnTimeFailStartAndRotation(t *testing.T) {
	var events []Transition
	svc, clock, m := newHBService(t, &events)
	ctx := context.Background()
	if m.PingToken == "" || len(m.PingToken) < 40 {
		t.Fatalf("token should be long and returned once: %q", m.PingToken)
	}
	advance(svc, clock, 59*time.Minute)
	svc.probeHeartbeat(ctx, m)
	if got := svc.State(m.ID); got != StateUp || len(events) != 0 {
		t.Fatalf("on time must stay up, got %s %v", got, events)
	}
	advance(svc, clock, 12*time.Minute) // 71 min since arm, over 60+10
	svc.probeHeartbeat(ctx, m)
	if svc.State(m.ID) != StateDown || len(events) != 1 || events[0].Reason != ReasonHeartbeatMissed {
		t.Fatalf("late must go down once: %v", events)
	}
	if err := svc.Ping(ctx, m.PingToken, PingOK); err != nil {
		t.Fatal(err)
	}
	if svc.State(m.ID) != StateUp || len(events) != 2 || events[1].State != StateUp {
		t.Fatalf("next ping must recover: %v", events)
	}
	if err := svc.Ping(ctx, m.PingToken, PingFail); err != nil {
		t.Fatal(err)
	}
	if svc.State(m.ID) != StateDown || events[2].Reason != ReasonJobFailed {
		t.Fatalf("fail ping must go down at once: %v", events)
	}
	svc.probeHeartbeat(ctx, m)
	if len(events) != 3 {
		t.Fatalf("a failed state must not renotify: %v", events)
	}
	// A start ping alone keeps a late job alive but does not clear a failure.
	advance(svc, clock, 5*time.Minute)
	if err := svc.Ping(ctx, m.PingToken, PingStart); err != nil {
		t.Fatal(err)
	}
	svc.probeHeartbeat(ctx, m)
	if svc.State(m.ID) != StateDown {
		t.Fatal("start must not clear a fail")
	}
	old := m.PingToken
	rot, err := svc.RotateToken(ctx, m.ID)
	if err != nil || rot.PingToken == old {
		t.Fatalf("rotate: %v", err)
	}
	if err := svc.Ping(ctx, old, PingOK); !errors.Is(err, ErrNotFound) {
		t.Fatalf("old token must stop working, got %v", err)
	}
	if err := svc.Ping(ctx, rot.PingToken, PingOK); err != nil {
		t.Fatalf("new token must work: %v", err)
	}
}

func TestHeartbeatUnknownTokenUniform(t *testing.T) {
	var events []Transition
	svc, _, _ := newHBService(t, &events)
	for _, tok := range []string{"", "nope", "x" + string(make([]byte, 0)), "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"} {
		if err := svc.Ping(context.Background(), tok, PingOK); !errors.Is(err, ErrNotFound) {
			t.Fatalf("token %q: want ErrNotFound, got %v", tok, err)
		}
	}
}

func certServer(t *testing.T, notAfter time.Time) (host string, port int, pool *x509.CertPool) {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "127.0.0.1"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: notAfter, IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert := tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() { _ = c.(*tls.Conn).Handshake(); _ = c.Close() }()
		}
	}()
	leaf, _ := x509.ParseCertificate(der)
	pool = x509.NewCertPool()
	pool.AddCert(leaf)
	return "127.0.0.1", ln.Addr().(*net.TCPAddr).Port, pool
}

func TestTLSDaysLeftThresholds(t *testing.T) {
	t.Setenv(netguard.AllowPrivateEnv, "true")
	cases := []struct {
		name       string
		days       int
		ok, degr   bool
		reason     string
		warn, crit int
	}{
		{"healthy", 60, true, false, "", 0, 0},
		{"warning", 14, true, true, ReasonCertWarn, 0, 0},
		{"critical", 3, false, false, ReasonCertCritical, 0, 0},
		{"override keeps 14 days healthy", 14, true, false, "", 10, 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			host, port, pool := certServer(t, time.Now().Add(time.Duration(tc.days)*24*time.Hour+time.Hour))
			c := NewChecker(testConfig())
			c.roots = pool
			m := Normalize(Monitor{ID: "t", Kind: KindTLS, Interval: time.Hour, Timeout: 3 * time.Second,
				Spec: Spec{Host: host, Port: port, WarnDays: tc.warn, CritDays: tc.crit}})
			r := c.Check(context.Background(), m, nil)
			if r.OK != tc.ok || r.Degraded != tc.degr || r.Reason != tc.reason || r.Code != tc.days {
				t.Fatalf("got %+v", r)
			}
		})
	}
}

func TestTLSInvalidChainIsAnErrorState(t *testing.T) {
	t.Setenv(netguard.AllowPrivateEnv, "true")
	host, port, _ := certServer(t, time.Now().Add(90*24*time.Hour))
	c := NewChecker(testConfig()) // no extra roots: self signed cert must fail verification
	m := Normalize(Monitor{ID: "t", Kind: KindTLS, Interval: time.Hour, Spec: Spec{Host: host, Port: port}})
	if r := c.Check(context.Background(), m, nil); r.OK || r.Reason != ReasonCertInvalid {
		t.Fatalf("want cert_invalid, got %+v", r)
	}
	host, port, pool := certServer(t, time.Now().Add(-time.Hour))
	c.roots = pool
	m.Spec.Host, m.Spec.Port = host, port
	if r := c.Check(context.Background(), m, nil); r.OK || r.Reason != ReasonCertExpired {
		t.Fatalf("want cert_expired, got %+v", r)
	}
}
