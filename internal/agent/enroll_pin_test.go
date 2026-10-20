package agent

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func startEnrollTestServer(t *testing.T) (addr, token string, ca *CA) {
	t.Helper()
	db := openLiveTestStore(t)
	ca, err := GenerateCA()
	if err != nil {
		t.Fatalf("GenerateCA() error = %v", err)
	}
	listener, _ := startTestAgentServer(t, ca, db, NewRegistry())
	token = "enroll-pin-test-token"
	now := time.Now()
	if err := db.SaveNodeJoinToken(context.Background(), store.NodeJoinToken{
		ID: "njt_pin", TokenHash: hashJoinToken(token), CreatedAt: now, ExpiresAt: now.Add(time.Hour),
	}); err != nil {
		t.Fatalf("SaveNodeJoinToken() error = %v", err)
	}
	return listener.Addr().String(), token, ca
}

func TestDialEnroll_PinnedCA_Succeeds(t *testing.T) {
	addr, token, ca := startEnrollTestServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Uppercase with colons, the way fingerprints are often pasted.
	pasted := strings.ToUpper(ca.Fingerprint()[:2]) + ":" + strings.ToUpper(ca.Fingerprint()[2:])
	id, err := DialEnroll(ctx, addr, token, "pinned-node", WithPinnedCAFingerprint(pasted))
	if err != nil {
		t.Fatalf("DialEnroll() error = %v", err)
	}
	if id.NodeID == "" || len(id.CACertPEM) == 0 {
		t.Fatalf("DialEnroll() returned an incomplete identity: %+v", id)
	}
}

func TestDialEnroll_WrongPinnedCA_RefusesBeforeSendingToken(t *testing.T) {
	addr, token, _ := startEnrollTestServer(t)
	other, err := GenerateCA()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := DialEnroll(ctx, addr, token, "mitm-node", WithPinnedCAFingerprint(other.Fingerprint())); err == nil {
		t.Fatal("DialEnroll() error = nil, want a refusal for a control plane whose CA does not match the pin")
	}
	// The token must still be unused: the handshake failed before Enroll ran.
	ctx2, cancel2 := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel2()
	if _, err := DialEnroll(ctx2, addr, token, "real-node"); err != nil {
		t.Fatalf("DialEnroll() with the same token after a refused pin: %v, want success (token must not be consumed)", err)
	}
}

func TestDialEnroll_NoPin_StillEnrolls(t *testing.T) {
	addr, token, _ := startEnrollTestServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := DialEnroll(ctx, addr, token, "tofu-node"); err != nil {
		t.Fatalf("DialEnroll() error = %v", err)
	}
}

func TestVerifyPinnedChain_RejectsLeafNotSignedByPinnedCA(t *testing.T) {
	ca, err := GenerateCA()
	if err != nil {
		t.Fatal(err)
	}
	attacker, err := GenerateCA()
	if err != nil {
		t.Fatal(err)
	}
	leafPEM, _, err := attacker.IssueServerCert([]string{"127.0.0.1"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(leafPEM)
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	// The attacker forwards the real CA alongside its own leaf.
	err = verifyPinnedChain([]*x509.Certificate{leaf, ca.cert}, ca.Fingerprint())
	if !errors.Is(err, ErrCAFingerprintMismatch) {
		t.Fatalf("verifyPinnedChain() err = %v, want ErrCAFingerprintMismatch", err)
	}
}
