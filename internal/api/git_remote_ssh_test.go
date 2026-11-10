package api

import (
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	gitclient "github.com/go-git/go-git/v5/plumbing/transport/client"
	gitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"golang.org/x/crypto/ssh"

	"github.com/GLINCKER/levelrail/internal/gitsshkey"
	"github.com/GLINCKER/levelrail/internal/store"
)

type mapGitSecrets map[string]string

func (m mapGitSecrets) SetValue(_ context.Context, svc, k, v string) error {
	m[svc+"/"+k] = v
	return nil
}
func (m mapGitSecrets) Resolve(_ context.Context, svc, k string) (string, error) {
	v, ok := m[svc+"/"+k]
	if !ok {
		return "", fmt.Errorf("missing %s", k)
	}
	return v, nil
}
func (m mapGitSecrets) Exists(_ context.Context, svc, k string) (bool, error) {
	_, ok := m[svc+"/"+k]
	return ok, nil
}

func pktLine(s string) string { return fmt.Sprintf("%04x%s", len(s)+4, s) }

// fakeGitSSHServer accepts only deployPub and advertises refs/heads/main at sha.
func fakeGitSSHServer(t *testing.T, deployPub ssh.PublicKey, sha string) (addr string, hostKey string) {
	t.Helper()
	pair, err := gitsshkey.Generate()
	if err != nil {
		t.Fatal(err)
	}
	hostSigner, err := ssh.ParsePrivateKey(pair.PrivatePEM)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &ssh.ServerConfig{PublicKeyCallback: func(_ ssh.ConnMetadata, k ssh.PublicKey) (*ssh.Permissions, error) {
		if string(k.Marshal()) == string(deployPub.Marshal()) {
			return nil, nil
		}
		return nil, fmt.Errorf("unknown key")
	}}
	cfg.AddHostKey(hostSigner)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	adv := pktLine(sha+" HEAD\x00symref=HEAD:refs/heads/main agent=test\n") + pktLine(sha+" refs/heads/main\n") + "0000"
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go serveFakeGit(c, cfg, adv)
		}
	}()
	return ln.Addr().String(), strings.TrimSpace(string(ssh.MarshalAuthorizedKey(hostSigner.PublicKey())))
}

func serveFakeGit(c net.Conn, cfg *ssh.ServerConfig, adv string) {
	defer func() { _ = c.Close() }()
	_ = c.SetDeadline(time.Now().Add(10 * time.Second))
	_, chans, reqs, err := ssh.NewServerConn(c, cfg)
	if err != nil {
		return
	}
	go ssh.DiscardRequests(reqs)
	for nc := range chans {
		ch, creqs, err := nc.Accept()
		if err != nil {
			return
		}
		for r := range creqs {
			if r.Type != "exec" {
				_ = r.Reply(false, nil)
				continue
			}
			_ = r.Reply(true, nil)
			_, _ = io.WriteString(ch, adv)
			_, _ = io.Copy(io.Discard, ch)
			_, _ = ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
			_ = ch.Close()
		}
	}
}

// The stored deploy key and pinned host key are what an ssh ref listing uses,
// through the process-wide ssh guard with loopback allowlisted.
func TestResolveBranchSHAUsesDeployKeyOverSSH(t *testing.T) {
	const sha = "0123456789abcdef0123456789abcdef01234567"
	key, err := gitsshkey.Generate()
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.ParsePrivateKey(key.PrivatePEM)
	if err != nil {
		t.Fatal(err)
	}
	addr, hostKey := fakeGitSSHServer(t, signer.PublicKey(), sha)
	gitsshkey.InstallGuard(gitsshkey.GuardOptions{Allow: func() []string { return []string{"127.0.0.1"} }})
	t.Cleanup(func() { gitclient.InstallProtocol("ssh", gitssh.DefaultClient) })

	secrets := mapGitSecrets{}
	sk := store.GitSourceSecretsKey("web")
	secrets[sk+"/"+gitSSHPrivateKey] = string(key.PrivatePEM)
	secrets[sk+"/"+gitSSHHostKey] = hostKey
	rt := &Router{gitSourceSecrets: secrets}
	url := "ssh://git@" + addr + "/o/r.git"
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	got, err := resolveRemoteBranchSHA(rt.gitAuthCtx(ctx, "web", url), url, "main", "")
	if err != nil || got != sha {
		t.Fatalf("resolve = %q, %v, want %s", got, err, sha)
	}
	other, _ := gitsshkey.Generate()
	secrets[sk+"/"+gitSSHHostKey] = other.PublicKey
	if _, err := resolveRemoteBranchSHA(rt.gitAuthCtx(ctx, "web", url), url, "main", ""); err == nil || !strings.Contains(err.Error(), "host key mismatch") {
		t.Fatalf("wrong pinned host key: err = %v, want a host key mismatch", err)
	}
}
