package sourcecache

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/transport"
)

// An SSH repo with no deploy key must fail before the transport, even when an
// ssh-agent is reachable through SSH_AUTH_SOCK.
func TestSSHRepoWithoutAuthNeverReachesTransport(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "agent.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Skipf("unix sockets unavailable: %v", err)
	}
	defer func() { _ = ln.Close() }()
	var agentHits atomic.Int32
	go func() {
		for {
			conn, aerr := ln.Accept()
			if aerr != nil {
				return
			}
			agentHits.Add(1)
			_ = conn.Close()
		}
	}()
	t.Setenv("SSH_AUTH_SOCK", sock)

	for _, repo := range []string{"git@127.0.0.1:acme/app.git", "ssh://git@127.0.0.1:1/acme/app.git"} {
		c := New(Config{Dir: t.TempDir(), Retries: 1, Backoff: time.Millisecond, FetchTimeout: time.Second}, nil)
		var fetched atomic.Int32
		realFetch := c.fetchFn
		c.fetchFn = func(ctx context.Context, r *git.Repository, a transport.AuthMethod) error {
			fetched.Add(1)
			return realFetch(ctx, r, a)
		}
		_, err := c.Checkout(context.Background(), repo, "main", "")
		if !errors.Is(err, ErrSSHAuthRequired) {
			t.Fatalf("%s: err = %v, want ErrSSHAuthRequired", repo, err)
		}
		if fetched.Load() != 0 {
			t.Fatalf("%s: fetch reached the transport", repo)
		}
		if perr := defaultProbe(context.Background(), repo); !errors.Is(perr, ErrSSHAuthRequired) {
			t.Fatalf("%s: probe err = %v", repo, perr)
		}
		if c.Reachable(context.Background(), repo) != true {
			t.Fatalf("%s: auth refusal is not an outage", repo)
		}
	}
	time.Sleep(50 * time.Millisecond)
	if n := agentHits.Load(); n != 0 {
		t.Fatalf("ssh-agent was contacted %d times", n)
	}
}

func TestAuthErrorFailsClosed(t *testing.T) {
	c := New(Config{Dir: t.TempDir(), Retries: 1, Backoff: time.Millisecond, FetchTimeout: time.Second}, nil)
	c.fetchFn = func(context.Context, *git.Repository, transport.AuthMethod) error {
		t.Fatal("fetch must not run after an auth error")
		return nil
	}
	boom := errors.New("deploy key unreadable")
	ctx := WithAuthError(context.Background(), boom)
	if _, err := c.Checkout(ctx, "git@127.0.0.1:acme/app.git", "main", ""); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the auth error", err)
	}
}
