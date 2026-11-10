package gitsshkey

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/transport"
	gitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/go-git/go-git/v5/storage/memory"
)

type recordingTransport struct{ got *transport.Endpoint }

var errRecorded = errors.New("recorded")

func (r *recordingTransport) NewUploadPackSession(ep *transport.Endpoint, _ transport.AuthMethod) (transport.UploadPackSession, error) {
	r.got = ep
	return nil, errRecorded
}

func (r *recordingTransport) NewReceivePackSession(ep *transport.Endpoint, _ transport.AuthMethod) (transport.ReceivePackSession, error) {
	r.got = ep
	return nil, errRecorded
}

func deployAuth(t *testing.T) transport.AuthMethod {
	t.Helper()
	key, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	host, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	auth, err := AuthMethod(key.PrivatePEM, host.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	return auth
}

func TestGuardedTransportRequiresDeployKey(t *testing.T) {
	next := &recordingTransport{}
	g := NewGuardedTransport(GuardOptions{Next: next})
	ep, err := transport.NewEndpoint("ssh://git@203.0.113.10/x.git")
	if err != nil {
		t.Fatal(err)
	}
	for name, auth := range map[string]transport.AuthMethod{
		"none":           nil,
		"password":       &gitssh.Password{User: "git", Password: "pw"},
		"no pinned host": &gitssh.PublicKeys{User: "git"},
		"agent callback": &gitssh.PublicKeysCallback{User: "git"},
	} {
		if _, err := g.NewUploadPackSession(ep, auth); !errors.Is(err, ErrAuthRequired) {
			t.Errorf("%s: err = %v, want ErrAuthRequired", name, err)
		}
	}
	if next.got != nil {
		t.Error("the inner transport was reached without a deploy key")
	}
}

func TestGuardedTransportInternalAddresses(t *testing.T) {
	auth := deployAuth(t)
	cases := []struct {
		name, url    string
		allow        []string
		allowPrivate bool
		wantHost     string
	}{
		{name: "loopback refused", url: "ssh://git@127.0.0.1:6379/x"},
		{name: "private refused", url: "ssh://git@10.0.0.5:6379/x"},
		{name: "scp form refused", url: "git@10.0.0.5:x.git"},
		{name: "loopback allowlisted", url: "ssh://git@127.0.0.1/x", allow: []string{"127.0.0.0/8"}, wantHost: "127.0.0.1"},
		{name: "private allowlisted", url: "ssh://git@10.0.0.5/x", allow: []string{"10.0.0.5"}, wantHost: "10.0.0.5"},
		{name: "private network opt-in", url: "ssh://git@10.0.0.5/x", allowPrivate: true, wantHost: "10.0.0.5"},
		{name: "metadata never", url: "ssh://git@169.254.169.254/x", allow: []string{"169.254.0.0/16"}, allowPrivate: true},
		{name: "public passes", url: "ssh://git@203.0.113.10/x", wantHost: "203.0.113.10"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			next := &recordingTransport{}
			g := NewGuardedTransport(GuardOptions{
				Next:         next,
				Allow:        func() []string { return tc.allow },
				AllowPrivate: func() bool { return tc.allowPrivate },
			})
			ep, err := transport.NewEndpoint(tc.url)
			if err != nil {
				t.Fatal(err)
			}
			_, err = g.NewUploadPackSession(ep, auth)
			if tc.wantHost == "" {
				if !errors.Is(err, ErrBlockedScanTarget) || next.got != nil {
					t.Fatalf("err = %v, reached = %v, want a refusal before dialing", err, next.got != nil)
				}
				return
			}
			if !errors.Is(err, errRecorded) || next.got == nil || next.got.Host != tc.wantHost {
				t.Fatalf("err = %v, endpoint = %+v, want a session to %s", err, next.got, tc.wantHost)
			}
		})
	}
}

// InstallGuard is process-wide: go-git with an ssh URL and no auth must fail
// closed, and an internal host is refused before any TCP connection.
func TestInstallGuardBlocksSSHFetches(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	var hits atomic.Int32
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			hits.Add(1)
			_ = c.Close()
		}
	}()
	allow := []string{}
	InstallGuard(GuardOptions{Allow: func() []string { return allow }})
	url := "ssh://git@" + ln.Addr().String() + "/x.git"
	list := func(auth transport.AuthMethod) error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		r := git.NewRemote(memory.NewStorage(), &config.RemoteConfig{Name: "origin", URLs: []string{url}})
		_, err := r.ListContext(ctx, &git.ListOptions{Auth: auth})
		return err
	}
	if err := list(nil); !errors.Is(err, ErrAuthRequired) {
		t.Errorf("no key: err = %v, want ErrAuthRequired", err)
	}
	auth := deployAuth(t)
	if err := list(auth); !errors.Is(err, ErrBlockedScanTarget) {
		t.Errorf("loopback with the setting off: err = %v, want ErrBlockedScanTarget", err)
	}
	if hits.Load() != 0 {
		t.Fatal("the loopback listener was contacted while refused")
	}
	allow = []string{"127.0.0.1"}
	_ = list(auth)
	if hits.Load() == 0 {
		t.Error("the allowlisted loopback host was not dialed")
	}
}
