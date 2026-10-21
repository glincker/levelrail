package migrateagent

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"

	"github.com/GLINCKER/levelrail/internal/agent/migrationpb"
	"github.com/GLINCKER/levelrail/internal/migrationplan"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestParseTokenIsStrict(t *testing.T) {
	secret := strings.Repeat("A", 43)
	cases := []struct {
		name  string
		token string
		ok    bool
	}{
		{"canonical", tokenPrefix + "ms_0123456789abcdef." + secret, true},
		{"long id", tokenPrefix + "ms_" + strings.Repeat("a", 4096) + "." + secret, false},
		{"upper hex id", tokenPrefix + "ms_0123456789ABCDEF." + secret, false},
		{"short id", tokenPrefix + "ms_0123." + secret, false},
		{"id with newline", tokenPrefix + "ms_0123456789abcde\n." + secret, false},
		{"long secret", tokenPrefix + "ms_0123456789abcdef." + secret + "A", false},
		{"secret with space", tokenPrefix + "ms_0123456789abcdef." + strings.Repeat("A", 42) + " ", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, ok := ParseToken(tc.token); ok != tc.ok {
				t.Fatalf("ParseToken ok=%v want %v", ok, tc.ok)
			}
		})
	}
}

func TestEnrollRefusalAuditOnlyForKnownSessions(t *testing.T) {
	f := newFixture(t)
	c := mustCreate(t, f)
	secret := strings.Repeat("A", 43)
	for _, tok := range []string{"junk", tokenPrefix + "ms_" + strings.Repeat("a", 500) + "." + secret, tokenPrefix + "ms_ffffffffffffffff." + secret} {
		_, err := f.enroll(t, tok)
		wantCode(t, err, codes.Unauthenticated)
	}
	if hasAction(auditActions(t, f.db), ActionEnrollRefused) {
		t.Fatalf("refusals without a real session must not be audited: %v", auditActions(t, f.db))
	}
	before := len(auditActions(t, f.db))
	_, err := f.enroll(t, tokenPrefix+c.ID+"."+secret)
	wantCode(t, err, codes.Unauthenticated)
	if got := auditActions(t, f.db); len(got) != before+1 || !hasAction(got, ActionEnrollRefused) {
		t.Fatalf("a wrong secret for a real session must be audited once: %v", got)
	}
}

func TestEnrollRateLimitPerSource(t *testing.T) {
	f := newFixture(t)
	f.svc = NewService(f.db, f.ca, Config{TokenTTL: time.Hour, SessionMax: time.Hour, RequestTimeout: time.Second, EnrollRate: 3, EnrollWindow: time.Minute}, nil, f.clk.now)
	for i := 0; i < 3; i++ {
		_, err := f.enroll(t, "junk")
		wantCode(t, err, codes.Unauthenticated)
	}
	_, err := f.enroll(t, "junk")
	wantCode(t, err, codes.ResourceExhausted)
	f.clk.advance(2 * time.Minute)
	_, err = f.enroll(t, "junk")
	wantCode(t, err, codes.Unauthenticated)
}

func TestIPLimiterSeparatesSources(t *testing.T) {
	now := time.Now()
	l := newIPLimiter(1, time.Minute)
	if !l.allow("10.0.0.1", now) || l.allow("10.0.0.1", now) || !l.allow("10.0.0.2", now) {
		t.Fatal("limits must apply per source address")
	}
	l.allow("10.0.0.3", now.Add(3*time.Minute))
	if len(l.hits) != 1 {
		t.Fatalf("stale windows were not swept: %d", len(l.hits))
	}
}

func TestEnrollRejectsOversizedAgentFields(t *testing.T) {
	f := newFixture(t)
	c := mustCreate(t, f)
	_, err := f.svc.Enroll(context.Background(), &migrationpb.EnrollRequest{JoinToken: c.Token, CsrDer: []byte{1}, Version: strings.Repeat("v", 1000)})
	wantCode(t, err, codes.InvalidArgument)
}

type racyStore struct {
	Store
	gets atomic.Int32
}

func (r *racyStore) GetMigrationAgentSession(ctx context.Context, id string) (store.MigrationAgentSession, error) {
	s, err := r.Store.GetMigrationAgentSession(ctx, id)
	if r.gets.Add(1) > 1 {
		s.State = store.MigrationAgentRevoked
	}
	return s, err
}

type fakeSessionStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s fakeSessionStream) Context() context.Context { return s.ctx }
func (s fakeSessionStream) Send(*migrationpb.ControlMessage) error {
	return nil
}
func (s fakeSessionStream) Recv() (*migrationpb.AgentMessage, error) {
	<-s.ctx.Done()
	return nil, s.ctx.Err()
}

func TestRevokeBetweenAuthenticateAndRegisterClosesStream(t *testing.T) {
	f := newFixture(t)
	_, cert := f.enrolledCert(t)
	f.svc.st = &racyStore{Store: f.svc.st}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ctx = peer.NewContext(ctx, &peer.Peer{AuthInfo: credentials.TLSInfo{State: tls.ConnectionState{PeerCertificates: []*x509.Certificate{cert}}}})
	err := f.svc.Session(fakeSessionStream{ctx: ctx})
	wantCode(t, err, codes.PermissionDenied)
	if ctx.Err() != nil {
		t.Fatal("the stream stayed open until the test timed out")
	}
}

func readyChecklist(t *testing.T) migrationplan.Checklist {
	t.Helper()
	return checklistAt(t, migrationplan.StepImported)
}

func TestAutoAdvanceRefusesOperatorSuppliedNumbers(t *testing.T) {
	cs := matching(2)
	cs[0].TargetFrom = TargetMeasured
	cs[1].TargetFrom = TargetOperator
	for _, ack := range []bool{false, true} {
		c := readyChecklist(t)
		res := AutoAdvance(&c, cs, 0, AdvanceOptions{AckEmpty: ack}, time.Now())
		if res.Advanced || c.States[migrationplan.StepDataVerified].Status == migrationplan.StatusDone {
			t.Fatalf("operator-supplied numbers advanced the step (ack=%v)", ack)
		}
	}
	if OverrideCount(cs) != 1 {
		t.Fatalf("OverrideCount=%d want 1", OverrideCount(cs))
	}
}

func TestCompareMarksOperatorSuppliedTargets(t *testing.T) {
	vols := []AppVolume{{Name: "a", Source: "a"}, {Name: "b", Source: "b"}}
	scan := []StatDoc{{Target: "a", Files: 2, Bytes: 9}, {Target: "b", Files: 2, Bytes: 9}}
	got := Compare(vols, scan, map[string]TargetStat{"a": {Files: 2, Bytes: 9, Operator: true}, "b": {Files: 2, Bytes: 9}})
	if got[0].TargetFrom != TargetOperator || got[1].TargetFrom != TargetMeasured {
		t.Fatalf("target_from not set: %+v", got)
	}
}

func TestAutoAdvanceEmptyVolumes(t *testing.T) {
	empty := Comparison{Name: "e", Status: StatusMatch, TargetFrom: TargetMeasured}
	full := matching(1)[0]
	cases := []struct {
		name     string
		cs       []Comparison
		ack      bool
		advanced bool
	}{
		{"empty without ack blocks", []Comparison{empty}, false, false},
		{"empty with ack advances", []Comparison{empty}, true, true},
		{"mixed without ack blocks", []Comparison{full, empty}, false, false},
		{"non empty needs no ack", []Comparison{full}, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := readyChecklist(t)
			res := AutoAdvance(&c, tc.cs, 0, AdvanceOptions{AckEmpty: tc.ack}, time.Now())
			if res.Advanced != tc.advanced {
				t.Fatalf("advanced=%v reason=%q want %v", res.Advanced, res.Reason, tc.advanced)
			}
			if tc.advanced && tc.ack && EmptyCount(tc.cs) > 0 {
				found := false
				for _, e := range res.Evidence {
					found = found || strings.Contains(e, "acknowledged")
				}
				if !found {
					t.Fatalf("acknowledgement missing from evidence: %v", res.Evidence)
				}
			}
		})
	}
}
