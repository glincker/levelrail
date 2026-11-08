package gitsshkey

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/go-git/go-git/v5/plumbing/transport"
	gitclient "github.com/go-git/go-git/v5/plumbing/transport/client"
	gitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
)

// ErrAuthRequired is returned for an ssh session with no deploy key, so the
// transport never falls back to the host's ssh-agent or known_hosts.
var ErrAuthRequired = errors.New("gitsshkey: ssh repository needs a deploy key with a pinned host key")

const defaultResolveTimeout = 10 * time.Second

// GuardOptions configures the guarded ssh transport. Allow and AllowPrivate
// are read on every session so a settings change applies without a restart.
type GuardOptions struct {
	// Allow lists internal IPs or CIDRs a fetch may reach, like ScanHostKey's.
	Allow func() []string
	// AllowPrivate opens loopback, private and CGNAT ranges; metadata stays refused.
	AllowPrivate   func() bool
	ResolveTimeout time.Duration
	// Next is the real ssh transport; tests replace it to observe sessions.
	Next transport.Transport
}

type guardedTransport struct{ o GuardOptions }

// NewGuardedTransport wraps go-git's ssh transport: it requires an explicit
// deploy key with a pinned host key and dials only a checked, pinned address.
func NewGuardedTransport(o GuardOptions) transport.Transport {
	if o.Next == nil {
		o.Next = gitssh.DefaultClient
	}
	if o.ResolveTimeout <= 0 {
		o.ResolveTimeout = defaultResolveTimeout
	}
	return &guardedTransport{o: o}
}

// InstallGuard makes every go-git ssh fetch in the process go through the
// guarded transport, and removes the agent and ssh_config fallbacks.
func InstallGuard(o GuardOptions) {
	gitssh.DefaultAuthBuilder = func(string) (gitssh.AuthMethod, error) { return nil, ErrAuthRequired }
	gitssh.DefaultSSHConfig = nil
	gitclient.InstallProtocol("ssh", NewGuardedTransport(o))
}

func (g *guardedTransport) NewUploadPackSession(ep *transport.Endpoint, auth transport.AuthMethod) (transport.UploadPackSession, error) {
	pinned, err := g.pin(ep, auth)
	if err != nil {
		return nil, err
	}
	return g.o.Next.NewUploadPackSession(pinned, auth)
}

func (g *guardedTransport) NewReceivePackSession(ep *transport.Endpoint, auth transport.AuthMethod) (transport.ReceivePackSession, error) {
	pinned, err := g.pin(ep, auth)
	if err != nil {
		return nil, err
	}
	return g.o.Next.NewReceivePackSession(pinned, auth)
}

// pin checks auth, resolves the host once and returns an endpoint naming the
// checked IP, so a later lookup cannot rebind the dial to an internal address.
func (g *guardedTransport) pin(ep *transport.Endpoint, auth transport.AuthMethod) (*transport.Endpoint, error) {
	pk, ok := auth.(*gitssh.PublicKeys)
	if !ok || pk == nil || pk.HostKeyCallback == nil {
		return nil, ErrAuthRequired
	}
	if ep == nil || ep.Host == "" {
		return nil, errors.New("gitsshkey: ssh url has no host")
	}
	ctx, cancel := context.WithTimeout(context.Background(), g.o.ResolveTimeout)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupNetIP(ctx, "ip", ep.Host)
	if err != nil {
		return nil, fmt.Errorf("gitsshkey: resolve %s: %w", ep.Host, err)
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("gitsshkey: resolve %s: no addresses", ep.Host)
	}
	allow := g.allow()
	for _, a := range addrs {
		if err := checkScanAddr(a, allow); err != nil {
			return nil, err
		}
	}
	pinned := *ep
	pinned.Host = addrs[0].Unmap().String()
	return &pinned, nil
}

func (g *guardedTransport) allow() []string {
	var allow []string
	if g.o.Allow != nil {
		allow = append(allow, g.o.Allow()...)
	}
	if g.o.AllowPrivate != nil && g.o.AllowPrivate() {
		// checkScanAddr still refuses metadata, multicast and unspecified first.
		allow = append(allow, "0.0.0.0/0", "::/0")
	}
	return allow
}
