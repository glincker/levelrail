package agent

// The control plane's security event log, driven over a real TLS listener
// and a real SQLite store: what it records when agents enroll, connect,
// renew, and when a certificate is refused.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/store"
)

func listEvents(t *testing.T, db *store.DB, nodeID, outcome string) []store.NodeSecurityEvent {
	t.Helper()
	evs, err := db.ListNodeSecurityEvents(context.Background(), nodeID, store.NodeSecurityFilter{Outcome: outcome, Limit: 500})
	if err != nil {
		t.Fatalf("ListNodeSecurityEvents() error = %v", err)
	}
	return evs
}

func findEvent(evs []store.NodeSecurityEvent, kind, reason string) *store.NodeSecurityEvent {
	for i := range evs {
		if evs[i].Kind == kind && (reason == "" || evs[i].Reason == reason) {
			return &evs[i]
		}
	}
	return nil
}

func TestTrustEvents_EnrollRecordsTokenIDPinAndCertNeverTheToken(t *testing.T) {
	for _, tt := range []struct {
		name        string
		pin         bool
		wantOutcome string
		wantReason  string
		wantState   string
	}{
		{"pinned CA verified", true, store.SecurityOutcomeAccepted, store.SecurityReasonPinned, store.CAPinPinned},
		{"no pin warns", false, store.SecurityOutcomeWarning, store.SecurityReasonNotPinned, store.CAPinUnpinned},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db := openLiveTestStore(t)
			ca, _ := GenerateCA()
			ts := startCertTestServer(t, ca, db)
			plaintext := "secret-join-token-" + tt.wantState
			now := time.Now()
			if err := db.SaveNodeJoinToken(context.Background(), store.NodeJoinToken{
				ID: "njt_fixed_" + tt.wantState, TokenHash: hashJoinToken(plaintext), CreatedAt: now, ExpiresAt: now.Add(time.Hour), Purpose: store.NodeJoinTokenPurposeEnroll,
			}); err != nil {
				t.Fatalf("SaveNodeJoinToken() error = %v", err)
			}
			var opts []EnrollOption
			if tt.pin {
				opts = append(opts, WithPinnedCAFingerprint(ca.Fingerprint()))
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			id, err := DialEnroll(ctx, ts.addr, plaintext, "n-"+tt.wantState, opts...)
			if err != nil {
				t.Fatalf("DialEnroll() error = %v", err)
			}

			evs := listEvents(t, db, id.NodeID, "")
			enroll := findEvent(evs, store.SecurityKindEnroll, "")
			if enroll == nil || enroll.Detail.TokenID != "njt_fixed_"+tt.wantState || enroll.SourceIP != "127.0.0.1" {
				t.Fatalf("enroll event = %+v, want token id and source 127.0.0.1", enroll)
			}
			pin := findEvent(evs, store.SecurityKindCAPin, "")
			if pin == nil || pin.Outcome != tt.wantOutcome || pin.Reason != tt.wantReason {
				t.Fatalf("ca_pin event = %+v, want %s/%s", pin, tt.wantOutcome, tt.wantReason)
			}
			issued := findEvent(evs, store.SecurityKindCertIssued, "")
			node, _ := db.GetNode(ctx, id.NodeID)
			if issued == nil || issued.Detail.Serial != node.CertSerial || issued.Detail.Fingerprint != node.CertFingerprint || issued.Detail.NotAfter == 0 || issued.Detail.NotBefore == 0 {
				t.Fatalf("cert_issued event = %+v, want serial, fingerprint and validity of the node's certificate", issued)
			}
			state, err := db.GetNodeTrustState(ctx, id.NodeID)
			if err != nil || state.CAPinState != tt.wantState {
				t.Fatalf("trust state pin = %q (%v), want %q", state.CAPinState, err, tt.wantState)
			}
			raw, _ := json.Marshal(evs)
			if strings.Contains(string(raw), plaintext) || strings.Contains(string(raw), "PRIVATE KEY") {
				t.Fatalf("events leak a token or key: %s", raw)
			}
		})
	}
}

func TestTrustEvents_WrongPinRefusedBeforeAnyEnrollmentEvent(t *testing.T) {
	db := openLiveTestStore(t)
	ca, _ := GenerateCA()
	other, _ := GenerateCA()
	ts := startCertTestServer(t, ca, db)
	mintToken(t, db, "tok-wrong-pin", store.NodeJoinTokenPurposeEnroll, "")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := DialEnroll(ctx, ts.addr, "tok-wrong-pin", "victim", WithPinnedCAFingerprint(other.Fingerprint())); err == nil {
		t.Fatal("DialEnroll() succeeded against a CA that does not match the pin")
	}
	nodes, _ := db.ListNodes(ctx)
	if len(nodes) != 0 {
		t.Fatalf("a refused pin must not create a node, got %d", len(nodes))
	}
	tok, err := db.GetNodeJoinTokenByHash(ctx, hashJoinToken("tok-wrong-pin"))
	if err != nil || tok.UsedAt != nil {
		t.Fatalf("join token must stay unused after a pin mismatch: %+v %v", tok, err)
	}
}

func TestTrustEvents_RefusedJoinTokenIsRecordedWithoutNode(t *testing.T) {
	db := openLiveTestStore(t)
	ca, _ := GenerateCA()
	ts := startCertTestServer(t, ca, db)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := DialEnroll(ctx, ts.addr, "never-minted", "ghost-node"); err == nil {
		t.Fatal("DialEnroll() accepted an unknown token")
	}
	evs := listEvents(t, db, "", "")
	ev := findEvent(evs, store.SecurityKindEnroll, store.SecurityReasonEnrollRefused)
	if ev == nil || ev.Outcome != store.SecurityOutcomeRejected || ev.SourceIP != "127.0.0.1" {
		t.Fatalf("events = %+v, want a rejected enroll from 127.0.0.1", evs)
	}
}

func TestTrustEvents_RejectedConnections(t *testing.T) {
	db := openLiveTestStore(t)
	ca, _ := GenerateCA()
	otherCA, _ := GenerateCA()
	ts := startCertTestServer(t, ca, db)
	id := enrollNode(t, db, ts, "target")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	forge := func(signer *CA, cn string, at time.Time, validFor time.Duration) *Identity {
		keyPEM, csr, err := NewKeyAndCSR(cn)
		if err != nil {
			t.Fatalf("NewKeyAndCSR() error = %v", err)
		}
		issued, err := signer.SignClientCSR(cn, csr, validFor, at)
		if err != nil {
			t.Fatalf("SignClientCSR() error = %v", err)
		}
		return &Identity{NodeID: cn, ClientCertPEM: issued.PEM, ClientKeyPEM: keyPEM, CACertPEM: ca.CertPEM()}
	}

	tests := []struct {
		name       string
		id         *Identity
		wantReason string
		wantNode   string
		revoke     bool
	}{
		{"expired certificate", forge(ca, id.NodeID, time.Now().Add(-48*time.Hour), time.Hour), store.SecurityReasonExpired, id.NodeID, false},
		{"certificate from another CA", forge(otherCA, id.NodeID, time.Now(), time.Hour), store.SecurityReasonUnknownCA, id.NodeID, false},
		{"valid chain, fingerprint not the node's", forge(ca, id.NodeID, time.Now(), time.Hour), store.SecurityReasonFingerprintMismatch, id.NodeID, false},
		{"valid chain, unknown node", forge(ca, "no-such-node", time.Now(), time.Hour), store.SecurityReasonUnknownNode, "", false},
		{"revoked certificate", id, store.SecurityReasonRevoked, id.NodeID, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.revoke {
				if err := db.RevokeNodeCert(ctx, id.NodeID, time.Now()); err != nil {
					t.Fatalf("RevokeNodeCert() error = %v", err)
				}
			}
			if _, err := checkCurrent(t, ts.addr, tt.id); err == nil {
				t.Fatal("the control plane accepted a certificate it must refuse")
			}
			evs := listEvents(t, db, tt.wantNode, store.SecurityOutcomeRejected)
			ev := findEvent(evs, store.SecurityKindConnect, tt.wantReason)
			if ev == nil {
				t.Fatalf("no rejected connect event with reason %q in %+v", tt.wantReason, evs)
			}
			if ev.SourceIP != "127.0.0.1" || ev.Detail.Fingerprint == "" {
				t.Errorf("event = %+v, want source 127.0.0.1 and the presented fingerprint", ev)
			}
			if tt.wantNode == "" && ev.Detail.Subject != "no-such-node" {
				t.Errorf("unknown node subject = %q, want the claimed name", ev.Detail.Subject)
			}
		})
	}
}

func TestTrustEvents_SessionAcceptedMatchesNodeRecord(t *testing.T) {
	db := openLiveTestStore(t)
	ca, _ := GenerateCA()
	ts := startCertTestServer(t, ca, db)
	id := enrollNode(t, db, ts, "session-node")
	sessionCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = RunSession(sessionCtx, ts.addr, id, nil, nil) }()

	node, _ := db.GetNode(context.Background(), id.NodeID)
	eventually(t, "an accepted connect event", func() bool {
		ev := findEvent(listEvents(t, db, id.NodeID, store.SecurityOutcomeAccepted), store.SecurityKindConnect, store.SecurityReasonVerified)
		return ev != nil && ev.Detail.Fingerprint == node.CertFingerprint && ev.Detail.Serial == node.CertSerial
	})
	sum, err := db.SummarizeNodeConnections(context.Background(), id.NodeID, time.Now().Add(-time.Hour))
	if err != nil || sum.Accepted != 1 || sum.Rejected != 0 {
		t.Fatalf("summary = %+v (%v), want one accepted", sum, err)
	}
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestTrustEvents_RejectionFloodIsCoalescedAndBounded(t *testing.T) {
	db := openLiveTestStore(t)
	ca, _ := GenerateCA()
	ts := startCertTestServer(t, ca, db, WithSecurityLog(db, SecurityConfig{PerNodeCap: 40}))
	id := enrollNode(t, db, ts, "flood-target")

	const burst = 3000
	for range burst {
		ts.srv.ObserveRejected(RejectedHandshake{Subject: id.NodeID, SourceIP: "203.0.113.9", Reason: store.SecurityReasonExpired, Fingerprint: "ff"})
	}
	eventually(t, "every repeat to be counted", func() bool {
		total := 0
		for _, e := range listEvents(t, db, id.NodeID, store.SecurityOutcomeRejected) {
			total += e.Count
		}
		return total == burst
	})
	if rows := len(listEvents(t, db, id.NodeID, store.SecurityOutcomeRejected)); rows > 3 {
		t.Fatalf("identical rejections used %d rows, want them merged into at most 3", rows)
	}

	for i := range 200 {
		ts.srv.ObserveRejected(RejectedHandshake{Subject: fmt.Sprintf("attacker-%d", i), SourceIP: fmt.Sprintf("198.51.100.%d", i%250), Reason: store.SecurityReasonUnknownCA, Fingerprint: "ee"})
	}
	time.Sleep(3 * securityBurstGap)
	if rows := len(listEvents(t, db, "", "")); rows > 40 {
		t.Fatalf("hostile distinct rejections left %d rows, want at most the cap 40", rows)
	}
}

func TestTrustEvents_RenewRecordsIssuedCertAndFailures(t *testing.T) {
	db := openLiveTestStore(t)
	ca, _ := GenerateCA()
	ts := startCertTestServer(t, ca, db)
	old := enrollNode(t, db, ts, "renewing")
	file := NewIdentityFile(filepath.Join(t.TempDir(), "identity.json"))
	_ = file.Save(old)
	holder := NewIdentityHolder(old)
	r := NewRenewer(holder, file, CheckIdentityAt(ts.addr), RenewConfig{}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := dialAs(t, ts.addr, old).Renew(ctx, &agentpb.RenewRequest{CsrDer: []byte("not a csr")}); err == nil {
		t.Fatal("Renew() accepted a garbage CSR")
	}
	if findEvent(listEvents(t, db, old.NodeID, store.SecurityOutcomeFailed), store.SecurityKindRenewFailed, store.SecurityReasonSignFailed) == nil {
		t.Fatal("a failed renewal must be recorded")
	}

	if err := r.RenewOnce(ctx, dialAs(t, ts.addr, old)); err != nil {
		t.Fatalf("RenewOnce() error = %v", err)
	}
	node, _ := db.GetNode(ctx, old.NodeID)
	var renewed *store.NodeSecurityEvent
	for _, e := range listEvents(t, db, old.NodeID, store.SecurityOutcomeInfo) {
		if e.Kind == store.SecurityKindCertIssued && e.Reason == "renew" {
			e := e
			renewed = &e
		}
	}
	if renewed == nil || renewed.Detail.Fingerprint != node.CertFingerprint || renewed.Detail.Serial != node.CertSerial {
		t.Fatalf("renew event = %+v, want the renewed certificate's fingerprint and serial", renewed)
	}
}

func TestTrustEvents_RenewalReportAndRenewNow(t *testing.T) {
	db := openLiveTestStore(t)
	ca, _ := GenerateCA()
	ts := startCertTestServer(t, ca, db)
	id := enrollNode(t, db, ts, "report-node")
	file := NewIdentityFile(filepath.Join(t.TempDir(), "identity.json"))
	_ = file.Save(id)
	r := NewRenewer(NewIdentityHolder(id), file, CheckIdentityAt(ts.addr), RenewConfig{}, nil)
	sessionCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = RunSession(sessionCtx, ts.addr, id, nil, nil, WithRenewer(r)) }()

	eventually(t, "the agent's renewal schedule to reach the control plane", func() bool {
		st, err := db.GetNodeTrustState(context.Background(), id.NodeID)
		return err == nil && st.RenewNextAt != nil && st.RenewNextAt.After(time.Now())
	})
	eventually(t, "the session to register", func() bool { return ts.srv.RequestRenew(id.NodeID) == nil })
	eventually(t, "renew-now to rotate the certificate", func() bool {
		n, err := db.GetNode(context.Background(), id.NodeID)
		return err == nil && n.CertGeneration >= 2
	})
	if err := ts.srv.RequestRenew("nobody"); !errors.Is(err, ErrNodeNotConnected) {
		t.Fatalf("RequestRenew(unconnected) = %v, want ErrNodeNotConnected", err)
	}
}
