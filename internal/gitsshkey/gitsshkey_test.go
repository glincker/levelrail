package gitsshkey

import (
	"net"
	"strings"
	"testing"

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
		"ssh://git@git.example.com:2222/a/b": "git.example.com:2222",
		"ssh://git@git.example.com/a/b":      "git.example.com:22",
	}
	for in, want := range cases {
		got, err := Host(in)
		if err != nil || got != want {
			t.Errorf("Host(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"https://github.com/a/b", "git@-oProxyCommand=x:a/b", "file:///etc", "plain"} {
		if IsSSHURL(bad) {
			t.Errorf("IsSSHURL(%q) = true", bad)
		}
	}
}
