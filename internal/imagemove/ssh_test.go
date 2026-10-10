package imagemove

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"golang.org/x/crypto/ssh"
)

// fakeSSHD answers exec requests with a fixed stdout, stderr and exit code.
type fakeSSHD struct {
	addr     string
	stdout   []byte
	stderr   string
	exit     uint32
	mu       sync.Mutex
	commands []string
}

func newKeyPEM(t *testing.T) ([]byte, ssh.PublicKey) {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(block), signer.PublicKey()
}

func startFakeSSHD(t *testing.T, clientKey ssh.PublicKey, d *fakeSSHD) {
	t.Helper()
	_, hostPriv, _ := ed25519.GenerateKey(rand.Reader)
	hostSigner, err := ssh.NewSignerFromKey(hostPriv)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &ssh.ServerConfig{PublicKeyCallback: func(_ ssh.ConnMetadata, k ssh.PublicKey) (*ssh.Permissions, error) {
		if bytes.Equal(k.Marshal(), clientKey.Marshal()) {
			return &ssh.Permissions{}, nil
		}
		return nil, io.ErrUnexpectedEOF
	}}
	cfg.AddHostKey(hostSigner)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	d.addr = ln.Addr().String()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go d.serve(c, cfg)
		}
	}()
}

func (d *fakeSSHD) serve(c net.Conn, cfg *ssh.ServerConfig) {
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
		for req := range creqs {
			if req.Type != "exec" {
				_ = req.Reply(false, nil)
				continue
			}
			var p struct{ Command string }
			_ = ssh.Unmarshal(req.Payload, &p)
			d.mu.Lock()
			d.commands = append(d.commands, p.Command)
			d.mu.Unlock()
			_ = req.Reply(true, nil)
			_, _ = ch.Write(d.stdout)
			_, _ = ch.Stderr().Write([]byte(d.stderr))
			_, _ = ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{d.exit}))
			_ = ch.Close()
			break
		}
	}
}

func fakeTarget(t *testing.T, addr string) Target {
	t.Helper()
	host, port, _ := net.SplitHostPort(addr)
	p, _ := strconv.Atoi(port)
	return Target{User: "root", Host: host, Port: p}
}

func TestSSHSaverStreamsFixedCommand(t *testing.T) {
	key, pub := newKeyPEM(t)
	d := &fakeSSHD{stdout: []byte("tar-bytes")}
	startFakeSSHD(t, pub, d)
	known := filepath.Join(t.TempDir(), "known_hosts")
	s := &SSHSaver{Target: fakeTarget(t, d.addr), Creds: Credentials{PrivateKey: key}, HostKeys: &HostKeys{Path: known}}
	rc, err := s.Save(context.Background(), "myapp:abc")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(rc)
	if err := rc.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if string(got) != "tar-bytes" {
		t.Fatalf("stdout = %q", got)
	}
	if len(d.commands) != 1 || d.commands[0] != "docker save myapp:abc" {
		t.Fatalf("commands = %q", d.commands)
	}
	info, err := os.Stat(known)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("known hosts = %v, %v", info, err)
	}
	raw, _ := os.ReadFile(known) //nolint:gosec // test temp dir
	if !strings.Contains(string(raw), "ssh-ed25519") {
		t.Fatalf("host key not pinned: %q", raw)
	}
	if _, err := s.Save(context.Background(), "x;id"); err == nil {
		t.Fatal("Save accepted an injected ref")
	}
}

func TestSSHSaverRemoteFailure(t *testing.T) {
	key, pub := newKeyPEM(t)
	d := &fakeSSHD{stderr: "Error response from daemon: reference does not exist", exit: 1}
	startFakeSSHD(t, pub, d)
	s := &SSHSaver{Target: fakeTarget(t, d.addr), Creds: Credentials{PrivateKey: key}, HostKeys: &HostKeys{}}
	rc, err := s.Save(context.Background(), "myapp:abc")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.ReadAll(rc)
	err = rc.Close()
	if err == nil || !strings.Contains(err.Error(), "reference does not exist") {
		t.Fatalf("close err = %v", err)
	}
}

func TestSSHSaverHostKeyChangeRefused(t *testing.T) {
	key, pub := newKeyPEM(t)
	known := filepath.Join(t.TempDir(), "known_hosts")
	first := &fakeSSHD{stdout: []byte("a")}
	startFakeSSHD(t, pub, first)
	tgt := fakeTarget(t, first.addr)
	_, otherHost := newKeyPEM(t)
	line := strings.Replace(string(ssh.MarshalAuthorizedKey(otherHost)), "\n", "", 1)
	if err := os.WriteFile(known, []byte("[127.0.0.1]:"+strconv.Itoa(tgt.Port)+" "+line+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := &SSHSaver{Target: tgt, Creds: Credentials{PrivateKey: key}, HostKeys: &HostKeys{Path: known}}
	if _, err := s.Save(context.Background(), "myapp:abc"); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("err = %v, want a host key change refusal", err)
	}
	if len(first.commands) != 0 {
		t.Fatal("ran a command on a host with a changed key")
	}
}

func TestSSHSaverNeedsCredentials(t *testing.T) {
	s := &SSHSaver{Target: Target{User: "root", Host: "127.0.0.1", Port: 1}, HostKeys: &HostKeys{}}
	if _, err := s.Save(context.Background(), "a:1"); err == nil || !strings.Contains(err.Error(), "private key") {
		t.Fatalf("err = %v", err)
	}
	s.Creds.PrivateKey = []byte("not a key")
	if _, err := s.Save(context.Background(), "a:1"); err == nil || strings.Contains(err.Error(), "not a key") {
		t.Fatalf("err = %v, must not echo key material", err)
	}
}

func TestSSHTransferEndToEnd(t *testing.T) {
	key, pub := newKeyPEM(t)
	d := &fakeSSHD{stdout: bytes.Repeat([]byte("l"), 10000)}
	startFakeSSHD(t, pub, d)
	s := &SSHSaver{Target: fakeTarget(t, d.addr), Creds: Credentials{PrivateKey: key}, HostKeys: &HostKeys{}}
	rt := &fakeRuntime{loadID: srcID}
	res, err := Transfer(context.Background(), s, rt, Request{Ref: "myapp:abc", WantID: srcID})
	if err != nil || !res.Verified || res.Bytes != 10000 || len(rt.loaded) != 10000 {
		t.Fatalf("res = %+v, err = %v, loaded = %d", res, err, len(rt.loaded))
	}
}
