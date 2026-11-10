package gitsshkey

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func TestDeployKeyRoundTrip(t *testing.T) {
	pair, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(pair.PublicKey, "ssh-ed25519 ") || strings.Contains(pair.PublicKey, "PRIVATE") {
		t.Fatalf("public key shape: %q", pair.PublicKey)
	}
	derived, err := PublicKeyOf(pair.PrivatePEM)
	if err != nil || derived != pair.PublicKey {
		t.Fatalf("derived public key mismatch: %v", err)
	}

	hostPair, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AuthMethod(pair.PrivatePEM, ""); err == nil {
		t.Fatal("empty host pin must be refused")
	}
	auth, err := AuthMethod(pair.PrivatePEM, hostPair.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(auth.String(), "PRIVATE") || strings.Contains(auth.String(), string(pair.PrivatePEM)) {
		t.Fatal("auth description leaks the private key")
	}

	cb, err := HostKeyCallback(hostPair.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	pinned, _, _, _, _ := ssh.ParseAuthorizedKey([]byte(hostPair.PublicKey))
	other, _, _, _, _ := ssh.ParseAuthorizedKey([]byte(pair.PublicKey))
	addr := &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 22}
	if err := cb("h", addr, pinned); err != nil {
		t.Fatalf("pinned key rejected: %v", err)
	}
	if err := cb("h", addr, other); err == nil {
		t.Fatal("unpinned key accepted")
	}
}

func TestHost(t *testing.T) {
	cases := map[string]string{
		"git@github.com:acme/app.git":        "github.com:22",
		"git@git.example.com:2200:a/b.git":   "git.example.com:2200",
		"ssh://git@git.example.com:2222/a/b": "git.example.com:2222",
		"ssh://git@git.example.com/a/b":      "git.example.com:22",
		"  git@github.com:acme/app.git\r\n":  "github.com:22",
	}
	for in, want := range cases {
		got, err := Host(in)
		if err != nil || got != want {
			t.Errorf("Host(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{
		"https://github.com/a/b", "file:///etc", "plain",
		"git@-oProxyCommand=x:a/b", "ssh://git@-oProxyCommand=x/a/b", "ssh://git@ho st/a/b",
	} {
		if _, err := Host(bad); err == nil {
			t.Errorf("Host(%q) accepted", bad)
		}
	}
	if IsSSHURL("https://github.com/a/b") || IsSSHURL("file:///etc") || IsSSHURL("plain") {
		t.Error("non-ssh url reported as ssh")
	}
	if !IsSSHURL("git@-oProxyCommand=x:a/b") {
		t.Error("malformed ssh url must still count as ssh so it fails closed")
	}
}

func TestCheckScanAddr(t *testing.T) {
	blocked := []string{"127.0.0.1", "::1", "10.1.2.3", "172.16.0.9", "192.168.1.1", "fd00::1", "169.254.169.254", "fe80::1", "0.0.0.0", "100.64.0.1", "::ffff:127.0.0.1", "224.0.0.1"}
	for _, in := range blocked {
		if err := checkScanAddr(netip.MustParseAddr(in), nil); !errors.Is(err, ErrBlockedScanTarget) {
			t.Errorf("%s not blocked: %v", in, err)
		}
	}
	if err := checkScanAddr(netip.MustParseAddr("140.82.112.3"), nil); err != nil {
		t.Errorf("public address blocked: %v", err)
	}
	allow := []string{"10.1.0.0/16", "192.168.1.1"}
	for _, in := range []string{"10.1.2.3", "192.168.1.1"} {
		if err := checkScanAddr(netip.MustParseAddr(in), allow); err != nil {
			t.Errorf("%s should be allowed: %v", in, err)
		}
	}
	if err := checkScanAddr(netip.MustParseAddr("10.2.0.1"), allow); err == nil {
		t.Error("address outside the allowlist was permitted")
	}
	if err := checkScanAddr(netip.MustParseAddr("169.254.169.254"), []string{"169.254.0.0/16"}); err == nil {
		t.Error("metadata address must never be allowable")
	}
}

func TestScanControlRefusesInternal(t *testing.T) {
	ctrl := scanControl(nil)
	if err := ctrl("tcp4", "127.0.0.1:22", nil); !errors.Is(err, ErrBlockedScanTarget) {
		t.Fatalf("loopback dial allowed: %v", err)
	}
	if err := ctrl("tcp4", "203.0.113.5:22", nil); err != nil {
		t.Fatalf("public dial refused: %v", err)
	}
	if _, err := ScanHostKey(context.Background(), "127.0.0.1:1", time.Second, nil); !errors.Is(err, ErrBlockedScanTarget) {
		t.Fatalf("ScanHostKey reached loopback: %v", err)
	}
}

func TestHostKeyCallbackRejectsAnyBadLine(t *testing.T) {
	good, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := HostKeyCallback(good.PublicKey + "\nnot a key"); err == nil {
		t.Fatal("a pin with an invalid later line must be refused")
	}
}

// Only the ssh transport may reach the deploy key path; git and file URLs must not count as ssh.
func TestIsSSHURL(t *testing.T) {
	cases := map[string]bool{
		"ssh://git@github.com/org/repo.git": true,
		"git@github.com:org/repo.git":       true,
		"git://github.com/org/repo.git":     false,
		"file:///srv/repo":                  false,
		"https://github.com/org/repo.git":   false,
	}
	for repo, want := range cases {
		if got := IsSSHURL(repo); got != want {
			t.Errorf("IsSSHURL(%q) = %v, want %v", repo, got, want)
		}
	}
}
