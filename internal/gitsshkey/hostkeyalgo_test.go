package gitsshkey

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"net"
	"slices"
	"strings"
	"testing"
	"time"

	gitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"golang.org/x/crypto/ssh"
)

func mustSigner(t *testing.T, key any) ssh.Signer {
	t.Helper()
	s, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func authorizedLine(s ssh.Signer) string {
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(s.PublicKey())))
}

// startMultiKeyServer serves ssh handshakes with every given host key and
// accepts any client key, so only host key selection decides the outcome.
func startMultiKeyServer(t *testing.T, hostKeys ...ssh.Signer) string {
	t.Helper()
	cfg := &ssh.ServerConfig{
		PublicKeyCallback: func(ssh.ConnMetadata, ssh.PublicKey) (*ssh.Permissions, error) { return nil, nil },
	}
	for _, k := range hostKeys {
		cfg.AddHostKey(k)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
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
			go func(c net.Conn) {
				defer func() { _ = c.Close() }()
				sc, chans, reqs, err := ssh.NewServerConn(c, cfg)
				if err != nil {
					return
				}
				defer func() { _ = sc.Close() }()
				go ssh.DiscardRequests(reqs)
				for ch := range chans {
					_ = ch.Reject(ssh.Prohibited, "test server")
				}
			}(c)
		}
	}()
	return ln.Addr().String()
}

func dialWithPin(t *testing.T, addr, pin string) error {
	t.Helper()
	pair, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	auth, err := AuthMethod(pair.PrivatePEM, pin)
	if err != nil {
		return err
	}
	cfg, err := auth.(*gitssh.PublicKeys).ClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Timeout = 5 * time.Second
	c, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		return err
	}
	return c.Close()
}

func TestPinnedHostKeyTypeSelectsServerKey(t *testing.T) {
	_, edPriv, _ := ed25519.GenerateKey(rand.Reader)
	ecPriv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ed, ec := mustSigner(t, edPriv), mustSigner(t, ecPriv)
	addr := startMultiKeyServer(t, ec, ed)

	if err := dialWithPin(t, addr, authorizedLine(ed)); err != nil {
		t.Fatalf("ed25519 pin against a server also offering ECDSA: %v", err)
	}
	if err := dialWithPin(t, addr, authorizedLine(ec)); err != nil {
		t.Fatalf("ECDSA pin: %v", err)
	}

	rsaPriv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	if err := dialWithPin(t, addr, authorizedLine(mustSigner(t, rsaPriv))); err == nil {
		t.Fatal("a pin of a type the server does not offer connected")
	}
	_, otherEd, _ := ed25519.GenerateKey(rand.Reader)
	err = dialWithPin(t, addr, authorizedLine(mustSigner(t, otherEd)))
	if err == nil || !strings.Contains(err.Error(), "host key mismatch") {
		t.Fatalf("a different ed25519 key: err = %v, want host key mismatch", err)
	}
}

func TestHostKeyAlgorithmsFollowPinnedTypes(t *testing.T) {
	_, edPriv, _ := ed25519.GenerateKey(rand.Reader)
	rsaPriv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pin := authorizedLine(mustSigner(t, edPriv)) + "\n" + authorizedLine(mustSigner(t, rsaPriv))
	got, err := HostKeyAlgorithms(pin)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{ssh.KeyAlgoED25519, ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSASHA256}
	if !slices.Equal(got, want) {
		t.Fatalf("algorithms = %v, want %v (never SHA-1 ssh-rsa)", got, want)
	}
	if _, err := HostKeyAlgorithms(""); err == nil {
		t.Fatal("an empty pin produced an algorithm list")
	}
}
