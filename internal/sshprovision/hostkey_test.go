package sshprovision

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"net"
	"strings"
	"sync/atomic"
	"testing"

	"golang.org/x/crypto/ssh"
)

// countingServer is a bare x/crypto/ssh server that records every
// authentication attempt of any method, including "none".
type countingServer struct {
	addr         string
	fingerprints map[string]string
	authAttempts atomic.Int64
}

func newCountingServer(t *testing.T, withRSA bool) *countingServer {
	t.Helper()
	cs := &countingServer{fingerprints: map[string]string{}}
	config := &ssh.ServerConfig{
		PasswordCallback: func(ssh.ConnMetadata, []byte) (*ssh.Permissions, error) {
			cs.authAttempts.Add(1)
			return nil, errors.New("denied")
		},
		PublicKeyCallback: func(ssh.ConnMetadata, ssh.PublicKey) (*ssh.Permissions, error) {
			cs.authAttempts.Add(1)
			return nil, errors.New("denied")
		},
		KeyboardInteractiveCallback: func(ssh.ConnMetadata, ssh.KeyboardInteractiveChallenge) (*ssh.Permissions, error) {
			cs.authAttempts.Add(1)
			return nil, errors.New("denied")
		},
		AuthLogCallback: func(ssh.ConnMetadata, string, error) { cs.authAttempts.Add(1) },
	}
	ed := newHostSigner(t)
	config.AddHostKey(ed)
	cs.fingerprints[ed.PublicKey().Type()] = ssh.FingerprintSHA256(ed.PublicKey())
	if withRSA {
		priv, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatalf("generate rsa host key: %v", err)
		}
		signer, err := ssh.NewSignerFromKey(priv)
		if err != nil {
			t.Fatalf("rsa signer: %v", err)
		}
		config.AddHostKey(signer)
		cs.fingerprints[signer.PublicKey().Type()] = ssh.FingerprintSHA256(signer.PublicKey())
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = conn.Close() }()
				sc, chans, reqs, err := ssh.NewServerConn(conn, config)
				if err != nil {
					return
				}
				go ssh.DiscardRequests(reqs)
				for ch := range chans {
					_ = ch.Reject(ssh.Prohibited, "test server")
				}
				_ = sc.Close()
			}()
		}
	}()
	cs.addr = ln.Addr().String()
	return cs
}

func TestNormalizeFingerprint(t *testing.T) {
	valid := "SHA256:" + strings.Repeat("a", 43)
	tests := []struct {
		name, in, want string
		ok             bool
	}{
		{"canonical", valid, valid, true},
		{"surrounding space", "  " + valid + "\n", valid, true},
		{"missing prefix", strings.Repeat("a", 43), valid, true},
		{"padded", valid + "=", valid, true},
		{"md5 style", "MD5:aa:bb", "SHA256:MD5:aa:bb", false},
		{"too short", "SHA256:abc", "SHA256:abc", false},
		{"empty", "", "SHA256:", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := NormalizeFingerprint(tc.in)
			if got != tc.want || ok != tc.ok {
				t.Fatalf("NormalizeFingerprint(%q) = %q, %v; want %q, %v", tc.in, got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestScanHostKeysNeverAuthenticates(t *testing.T) {
	cs := newCountingServer(t, true)
	h, p := hostPort(t, cs.addr)

	keys, err := New("acme-agent", "Acme").ScanHostKeys(context.Background(), h, p)
	if err != nil {
		t.Fatalf("ScanHostKeys: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("expected ed25519 and rsa keys, got %+v", keys)
	}
	for _, k := range keys {
		if cs.fingerprints[k.Type] != k.Fingerprint {
			t.Fatalf("key %s fingerprint %s, server has %s", k.Type, k.Fingerprint, cs.fingerprints[k.Type])
		}
	}
	if n := cs.authAttempts.Load(); n != 0 {
		t.Fatalf("scan made %d authentication attempts, want 0", n)
	}
}

func TestProvisionHostKeyPolicyRefusesBeforeAuth(t *testing.T) {
	wrong := "SHA256:" + strings.Repeat("B", 43)
	tests := []struct {
		name  string
		fp    string
		check func(t *testing.T, err error, cs *countingServer)
	}{
		{
			name: "missing fingerprint",
			check: func(t *testing.T, err error, _ *countingServer) {
				if !errors.Is(err, ErrHostKeyRequired) {
					t.Fatalf("want ErrHostKeyRequired, got %v", err)
				}
			},
		},
		{
			name: "malformed fingerprint",
			fp:   "not-a-fingerprint",
			check: func(t *testing.T, err error, _ *countingServer) {
				if err == nil || !strings.Contains(err.Error(), "malformed host key fingerprint") {
					t.Fatalf("want malformed error, got %v", err)
				}
			},
		},
		{
			name: "mismatch names observed key",
			fp:   wrong,
			check: func(t *testing.T, err error, cs *countingServer) {
				var mm *HostKeyMismatchError
				if !errors.As(err, &mm) {
					t.Fatalf("want HostKeyMismatchError, got %T %v", err, err)
				}
				if !strings.Contains(err.Error(), cs.fingerprints[ssh.KeyAlgoED25519]) || !strings.Contains(err.Error(), wrong) {
					t.Fatalf("error should name expected and observed fingerprints: %v", err)
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cs := newCountingServer(t, false)
			h, p := hostPort(t, cs.addr)
			_, err := New("acme-agent", "Acme").Provision(context.Background(), Credentials{
				Host: h, Port: p, Username: "root", Auth: AuthPassword, Password: "s3cret",
				HostKeyFingerprint: tc.fp,
			}, testParams(), nil)
			tc.check(t, err, cs)
			if n := cs.authAttempts.Load(); n != 0 {
				t.Fatalf("credentials reached the server: %d auth attempts", n)
			}
		})
	}
}

func TestProvisionHostKeyPolicyAccepts(t *testing.T) {
	tests := []struct {
		name     string
		fp       func(h *fakeHost) string
		insecure bool
	}{
		{"pinned match", func(h *fakeHost) string { return h.fingerprint }, false},
		{"pinned match without prefix", func(h *fakeHost) string { return strings.TrimPrefix(h.fingerprint, "SHA256:") }, false},
		{"explicit insecure skip", func(*fakeHost) string { return "" }, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			addr, host := newFakeHost(t, "s3cret", nil)
			h, p := hostPort(t, addr)
			_, err := New("acme-agent", "Acme").Provision(context.Background(), Credentials{
				Host: h, Port: p, Username: "root", Auth: AuthPassword, Password: "s3cret",
				HostKeyFingerprint: tc.fp(host), InsecureSkipHostKeyCheck: tc.insecure,
			}, testParams(), nil)
			if err != nil {
				t.Fatalf("Provision: %v", err)
			}
		})
	}
}

func TestPinnedHostKeyCallbackSelectsPinnedKeyType(t *testing.T) {
	cs := newCountingServer(t, true)
	h, p := hostPort(t, cs.addr)
	prov := New("acme-agent", "Acme")
	cb, algos, err := prov.hostKeyConfig(context.Background(), Credentials{
		Host: h, Port: p, HostKeyFingerprint: cs.fingerprints[ssh.KeyAlgoRSA],
	})
	if err != nil {
		t.Fatalf("hostKeyConfig: %v", err)
	}
	if cb == nil || len(algos) == 0 || algos[0] != ssh.KeyAlgoRSASHA512 {
		t.Fatalf("expected rsa algorithms for an rsa pin, got %v", algos)
	}
}
