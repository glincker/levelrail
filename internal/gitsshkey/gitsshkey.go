// Package gitsshkey holds the SSH deploy key path for private git repos:
// key generation, strict pinned host key checking, and the go-git auth
// method. Nothing here shells out or writes the private key to disk.
package gitsshkey

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/go-git/go-git/v5/plumbing/transport"
	gitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"golang.org/x/crypto/ssh"
)

// Pair is a freshly generated deploy key. PrivatePEM must go straight into
// the secrets store and never into a response or log line.
type Pair struct {
	PrivatePEM []byte
	PublicKey  string
}

// Generate makes a new ed25519 deploy key. The public half is in
// authorized_keys format, ready to paste into the repository's deploy keys.
func Generate() (Pair, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return Pair{}, fmt.Errorf("gitsshkey: generate key: %w", err)
	}
	block, err := ssh.MarshalPrivateKey(priv, "levelrail-deploy-key")
	if err != nil {
		return Pair{}, fmt.Errorf("gitsshkey: marshal private key: %w", err)
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		return Pair{}, fmt.Errorf("gitsshkey: marshal public key: %w", err)
	}
	return Pair{
		PrivatePEM: pem.EncodeToMemory(block),
		PublicKey:  strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub))) + " levelrail-deploy-key",
	}, nil
}

// PublicKeyOf derives the shareable public key from a stored private key.
func PublicKeyOf(privatePEM []byte) (string, error) {
	signer, err := ssh.ParsePrivateKey(privatePEM)
	if err != nil {
		return "", errors.New("gitsshkey: stored private key is not readable")
	}
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey()))) + " levelrail-deploy-key", nil
}

// Fingerprint returns the SHA256 fingerprint of an authorized_keys line.
func Fingerprint(authorizedKey string) (string, error) {
	pub, _, _, _, err := ssh.ParseAuthorizedKey([]byte(authorizedKey))
	if err != nil {
		return "", errors.New("gitsshkey: not a valid public key")
	}
	return ssh.FingerprintSHA256(pub), nil
}

// IsSSHURL reports whether go-git would use its ssh transport for repoURL.
// It is deliberately broader than Host's validation so a malformed ssh URL
// still counts as ssh and cannot slip past a fail-closed auth check.
func IsSSHURL(repoURL string) bool {
	ep, err := transport.NewEndpoint(strings.TrimSpace(repoURL))
	return err == nil && ep.Protocol == "ssh"
}

// Host returns the host:port an SSH repo URL dials, defaulting to port 22.
// It uses go-git's own endpoint parser so the scanner and the cloner always
// agree on the target.
func Host(repoURL string) (string, error) {
	raw := strings.TrimSpace(repoURL)
	ep, err := transport.NewEndpoint(raw)
	if err != nil || ep.Protocol != "ssh" {
		return "", errors.New("gitsshkey: not an ssh url")
	}
	host := ep.Host
	if host == "" || strings.HasPrefix(host, "-") || strings.ContainsAny(host, " \t\r\n/\\@") {
		return "", errors.New("gitsshkey: invalid ssh host")
	}
	port := ep.Port
	if port == 0 {
		port = 22
	}
	return net.JoinHostPort(host, strconv.Itoa(port)), nil
}

// HostKeyCallback accepts only the pinned key(s). An empty pin is an error,
// never a fallback to accepting anything.
func HostKeyCallback(pinned string) (ssh.HostKeyCallback, error) {
	var keys []ssh.PublicKey
	for _, line := range strings.Split(pinned, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		pub, _, _, _, err := ssh.ParseAuthorizedKey([]byte(line))
		if err != nil {
			return nil, errors.New("gitsshkey: pinned host key is not a valid public key")
		}
		keys = append(keys, pub)
	}
	if len(keys) == 0 {
		return nil, errors.New("gitsshkey: no pinned host key")
	}
	return func(hostname string, _ net.Addr, key ssh.PublicKey) error {
		for _, k := range keys {
			if k.Type() == key.Type() && string(k.Marshal()) == string(key.Marshal()) {
				return nil
			}
		}
		return fmt.Errorf("gitsshkey: host key mismatch for %s (got %s)", hostname, ssh.FingerprintSHA256(key))
	}, nil
}

// AuthMethod builds the go-git auth for a clone: the deploy key held in
// memory plus the pinned host key check. User is "git" for every major forge.
func AuthMethod(privatePEM []byte, pinnedHostKey string) (transport.AuthMethod, error) {
	signer, err := ssh.ParsePrivateKey(privatePEM)
	if err != nil {
		return nil, errors.New("gitsshkey: stored private key is not readable")
	}
	cb, err := HostKeyCallback(pinnedHostKey)
	if err != nil {
		return nil, err
	}
	return &gitssh.PublicKeys{
		User:                  "git",
		Signer:                signer,
		HostKeyCallbackHelper: gitssh.HostKeyCallbackHelper{HostKeyCallback: cb},
	}, nil
}

var errCaptured = errors.New("gitsshkey: host key captured")

// ErrBlockedScanTarget marks a host key scan refused because the resolved
// address is internal and not on the operator allowlist.
var ErrBlockedScanTarget = errors.New("gitsshkey: scan target is an internal address")

var cgnat = netip.MustParsePrefix("100.64.0.0/10")

// checkScanAddr refuses loopback, link-local (including cloud metadata),
// private, CGNAT, multicast and unspecified addresses unless allow lists a
// CIDR or IP covering it. Metadata, multicast and unspecified are never allowable.
func checkScanAddr(addr netip.Addr, allow []string) error {
	addr = addr.Unmap()
	if !addr.IsValid() || addr.IsUnspecified() || addr.IsMulticast() || addr.IsLinkLocalMulticast() {
		return fmt.Errorf("%w: %s", ErrBlockedScanTarget, addr)
	}
	if addr.IsLinkLocalUnicast() {
		return fmt.Errorf("%w: %s is a link-local or metadata address", ErrBlockedScanTarget, addr)
	}
	if !addr.IsLoopback() && !addr.IsPrivate() && !cgnat.Contains(addr) {
		return nil
	}
	for _, entry := range allow {
		entry = strings.TrimSpace(entry)
		if p, err := netip.ParsePrefix(entry); err == nil && p.Contains(addr) {
			return nil
		}
		if ip, err := netip.ParseAddr(entry); err == nil && ip.Unmap() == addr {
			return nil
		}
	}
	return fmt.Errorf("%w: %s (list it in the git.ssh_scan_allow_internal setting to permit)", ErrBlockedScanTarget, addr)
}

// scanControl runs after DNS resolution on every dial, so a name that
// resolves or rebinds to an internal address is refused too.
func scanControl(allow []string) func(network, address string, c syscall.RawConn) error {
	return func(_, address string, _ syscall.RawConn) error {
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return err
		}
		ip, err := netip.ParseAddr(host)
		if err != nil {
			return err
		}
		return checkScanAddr(ip, allow)
	}
}

// ScanHostKey fetches the server's host key for an explicit trust on first
// use. The caller records the returned key and shows its fingerprint. allow
// lists internal CIDRs or IPs the operator permits scanning.
func ScanHostKey(ctx context.Context, hostPort string, timeout time.Duration, allow []string) (string, error) {
	d := net.Dialer{Timeout: timeout, Control: scanControl(allow)}
	conn, err := d.DialContext(ctx, "tcp", hostPort)
	if err != nil {
		return "", fmt.Errorf("gitsshkey: dial %s: %w", hostPort, err)
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(timeout))
	var got ssh.PublicKey
	cfg := &ssh.ClientConfig{
		User: "git",
		HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
			got = key
			return errCaptured
		},
	}
	_, _, _, err = ssh.NewClientConn(conn, hostPort, cfg)
	if got == nil {
		return "", fmt.Errorf("gitsshkey: read host key from %s: %w", hostPort, err)
	}
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(got))), nil
}
