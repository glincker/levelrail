package backup

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"io"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

const testSFTPPass = "s3cret-test-pass"

func newTestSigner(t *testing.T) ssh.Signer {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	s, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// startSFTPServer runs an in-process SSH server with the sftp subsystem and returns host:port.
func startSFTPServer(t *testing.T, hostSigner ssh.Signer) string {
	t.Helper()
	cfg := &ssh.ServerConfig{PasswordCallback: func(c ssh.ConnMetadata, pw []byte) (*ssh.Permissions, error) {
		if c.User() == "bob" && string(pw) == testSFTPPass {
			return nil, nil
		}
		return nil, errors.New("denied")
	}}
	cfg.AddHostKey(hostSigner)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			nc, err := ln.Accept()
			if err != nil {
				return
			}
			go serveSFTPConn(nc, cfg)
		}
	}()
	return ln.Addr().String()
}

func serveSFTPConn(nc net.Conn, cfg *ssh.ServerConfig) {
	_, chans, reqs, err := ssh.NewServerConn(nc, cfg)
	if err != nil {
		_ = nc.Close()
		return
	}
	go ssh.DiscardRequests(reqs)
	for nch := range chans {
		if nch.ChannelType() != "session" {
			_ = nch.Reject(ssh.UnknownChannelType, "no")
			continue
		}
		ch, creqs, err := nch.Accept()
		if err != nil {
			continue
		}
		go func() {
			for r := range creqs {
				ok := r.Type == "subsystem" && len(r.Payload) > 4 && string(r.Payload[4:]) == "sftp"
				_ = r.Reply(ok, nil)
				if ok {
					srv, err := sftp.NewServer(ch)
					if err == nil {
						_ = srv.Serve()
					}
					_ = ch.Close()
				}
			}
		}()
	}
}

func testDest(addr string, signer ssh.Signer, base string) Destination {
	return Destination{
		Endpoint: SFTPEndpointScheme + addr, Region: signer.PublicKey().Type() + " " + ssh.FingerprintSHA256(signer.PublicKey()),
		Bucket: base, AccessKeyID: "bob", SecretAccessKey: testSFTPPass,
	}
}

func allowLoopbackSFTP(t *testing.T) {
	t.Helper()
	set := defaultSFTPSettings()
	set.AllowPrivate, set.Idle, set.Connect = true, 5*time.Second, 5*time.Second
	SetSFTPSource(func() SFTPSettings { return set })
	t.Cleanup(func() { SetSFTPSource(defaultSFTPSettings) })
}

func TestSFTPRoundTripAndProbe(t *testing.T) {
	allowLoopbackSFTP(t)
	signer := newTestSigner(t)
	addr := startSFTPServer(t, signer)
	base := t.TempDir()
	c, err := newSFTP(testDest(addr, signer, base))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := c.Upload(ctx, "db/2026/a.dump", strings.NewReader("hello")); err != nil {
		t.Fatalf("upload: %v", err)
	}
	if _, err := os.Stat(filepath.Join(base, "db/2026/a.dump.partial")); err == nil {
		t.Fatal("temporary file left behind")
	}
	rc, err := c.Download(ctx, "db/2026/a.dump")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(rc)
	_ = rc.Close()
	if string(got) != "hello" {
		t.Fatalf("got %q", got)
	}
	objs, err := c.List(ctx, "db/")
	if err != nil || len(objs) != 1 || objs[0].Key != "db/2026/a.dump" || objs[0].Size != 5 {
		t.Fatalf("list = %+v, %v", objs, err)
	}
	if err := c.Delete(ctx, "db/2026/a.dump"); err != nil {
		t.Fatal(err)
	}
	if err := c.Delete(ctx, "db/2026/a.dump"); err != nil {
		t.Fatalf("deleting a missing object must succeed: %v", err)
	}
	rep, err := c.Probe(ctx)
	if err != nil || !rep.OK {
		t.Fatalf("probe = %+v, %v", rep, err)
	}
	if sftpActive.Load() != 0 {
		t.Fatalf("connection slots leaked: %d", sftpActive.Load())
	}
}

func TestSFTPRefusesChangedHostKey(t *testing.T) {
	allowLoopbackSFTP(t)
	served, other := newTestSigner(t), newTestSigner(t)
	addr := startSFTPServer(t, served)
	c, err := newSFTP(testDest(addr, other, t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	err = c.Upload(context.Background(), "x", strings.NewReader("data"))
	if !errors.Is(err, ErrSFTPHostKeyMismatch) {
		t.Fatalf("want host key mismatch, got %v", err)
	}
	if strings.Contains(err.Error(), testSFTPPass) {
		t.Fatal("error leaks the password")
	}
}

func TestSFTPNeedsPinnedKeyAndRejectsBadPassword(t *testing.T) {
	d := Destination{Endpoint: SFTPEndpointScheme + "127.0.0.1:22", AccessKeyID: "bob", SecretAccessKey: "x"}
	if _, err := newSFTP(d); !errors.Is(err, ErrSFTPNoHostKey) {
		t.Fatalf("unpinned target must be refused, got %v", err)
	}
	allowLoopbackSFTP(t)
	signer := newTestSigner(t)
	addr := startSFTPServer(t, signer)
	d = testDest(addr, signer, t.TempDir())
	d.SecretAccessKey = "wrong"
	c, _ := newSFTP(d)
	err := c.Upload(context.Background(), "x", strings.NewReader("d"))
	if err == nil || strings.Contains(err.Error(), "wrong") {
		t.Fatalf("want auth failure without the password in it, got %v", err)
	}
}

func TestSFTPScanSendsNoCredentialsAndMatchesPin(t *testing.T) {
	allowLoopbackSFTP(t)
	signer := newTestSigner(t)
	addr := startSFTPServer(t, signer)
	key, err := ScanSFTPHostKey(context.Background(), addr)
	if err != nil {
		t.Fatal(err)
	}
	want := signer.PublicKey().Type() + " " + ssh.FingerprintSHA256(signer.PublicKey())
	if key != want {
		t.Fatalf("scan = %q, want %q", key, want)
	}
}

func TestSFTPPathTraversalRefused(t *testing.T) {
	s := &sftpSession{base: "/srv/backups"}
	for _, k := range []string{"../etc/passwd", "a/../../x", "/abs", "", "a\x00b", `a\b`} {
		if _, err := s.remote(k); err == nil {
			t.Errorf("key %q accepted", k)
		}
	}
	if got, err := s.remote("a/b.dump"); err != nil || got != "/srv/backups/a/b.dump" {
		t.Fatalf("got %q, %v", got, err)
	}
	if _, err := SFTPBasePath("a/../../b"); err == nil {
		t.Fatal("base path with .. accepted")
	}
}

func TestSFTPPrivateAddressesRefusedByDefault(t *testing.T) {
	SetSFTPSource(defaultSFTPSettings)
	if err := ValidateSFTPHost(context.Background(), "127.0.0.1:22"); err == nil {
		t.Fatal("loopback must be refused without the opt-in")
	}
	if err := ValidateSFTPHost(context.Background(), "169.254.169.254"); err == nil {
		t.Fatal("metadata address must be refused")
	}
	if _, err := ScanSFTPHostKey(context.Background(), "127.0.0.1:1"); err == nil {
		t.Fatal("dial to loopback must be refused")
	}
}

func TestParseSFTPHostKey(t *testing.T) {
	s := newTestSigner(t)
	good := s.PublicKey().Type() + " " + ssh.FingerprintSHA256(s.PublicKey())
	if _, _, err := ParseSFTPHostKey(good); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", "ssh-ed25519", "ssh-ed25519 MD5:aa", "ssh-ed25519 SHA256:short"} {
		if _, _, err := ParseSFTPHostKey(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

// endlessHandlers answers every directory read with entries forever.
type endlessHandlers struct{}

type fakeInfo struct{ name string }

func (f fakeInfo) Name() string     { return f.name }
func (fakeInfo) Size() int64        { return 1 }
func (fakeInfo) Mode() os.FileMode  { return 0o644 }
func (fakeInfo) ModTime() time.Time { return time.Time{} }
func (fakeInfo) IsDir() bool        { return false }
func (fakeInfo) Sys() any           { return nil }

type endlessLister struct{}

func (endlessLister) ListAt(ls []os.FileInfo, _ int64) (int, error) {
	for i := range ls {
		ls[i] = fakeInfo{name: strings.Repeat("e", 40)}
	}
	return len(ls), nil
}

func (endlessHandlers) Fileread(*sftp.Request) (io.ReaderAt, error)  { return nil, os.ErrNotExist }
func (endlessHandlers) Filewrite(*sftp.Request) (io.WriterAt, error) { return nil, os.ErrPermission }
func (endlessHandlers) Filecmd(*sftp.Request) error                  { return os.ErrPermission }
func (endlessHandlers) Filelist(*sftp.Request) (sftp.ListerAt, error) {
	return endlessLister{}, nil
}

func startEndlessServer(t *testing.T, hostSigner ssh.Signer) string {
	t.Helper()
	cfg := &ssh.ServerConfig{PasswordCallback: func(ssh.ConnMetadata, []byte) (*ssh.Permissions, error) { return nil, nil }}
	cfg.AddHostKey(hostSigner)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			nc, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				_, chans, reqs, err := ssh.NewServerConn(nc, cfg)
				if err != nil {
					return
				}
				go ssh.DiscardRequests(reqs)
				for nch := range chans {
					ch, creqs, err := nch.Accept()
					if err != nil {
						continue
					}
					go func() {
						for r := range creqs {
							_ = r.Reply(r.Type == "subsystem", nil)
							if r.Type == "subsystem" {
								h := endlessHandlers{}
								_ = sftp.NewRequestServer(ch, sftp.Handlers{FileGet: h, FilePut: h, FileCmd: h, FileList: h}).Serve()
								_ = ch.Close()
							}
						}
					}()
				}
			}()
		}
	}()
	return ln.Addr().String()
}

func TestSFTPListIsBoundedAgainstEndlessEntries(t *testing.T) {
	set := defaultSFTPSettings()
	set.AllowPrivate, set.Idle, set.Connect, set.MaxEntries = true, 20*time.Second, 5*time.Second, 200
	SetSFTPSource(func() SFTPSettings { return set })
	t.Cleanup(func() { SetSFTPSource(defaultSFTPSettings) })
	signer := newTestSigner(t)
	addr := startEndlessServer(t, signer)
	c, _ := newSFTP(testDest(addr, signer, "/"))
	start := time.Now()
	_, err := c.List(context.Background(), "")
	if err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("want a too large error, got %v", err)
	}
	if time.Since(start) > 10*time.Second {
		t.Fatalf("listing was not cut off promptly: %s", time.Since(start))
	}
	if sftpActive.Load() != 0 {
		t.Fatalf("slots leaked: %d", sftpActive.Load())
	}
}

func TestSFTPSessionClosesSafelyWhenContextAlreadyCancelled(t *testing.T) {
	allowLoopbackSFTP(t)
	signer := newTestSigner(t)
	addr := startSFTPServer(t, signer)
	for i := 0; i < 20; i++ {
		conn, err := ssh.Dial("tcp", addr, &ssh.ClientConfig{
			User: "bob", Auth: []ssh.AuthMethod{ssh.Password(testSFTPPass)}, HostKeyCallback: ssh.FixedHostKey(signer.PublicKey()),
		})
		if err != nil {
			t.Fatal(err)
		}
		sftpActive.Add(1)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		s, err := buildSession(ctx, conn, ".", sftpSettings())
		if err == nil {
			_ = s.Close()
		}
	}
	deadline := time.Now().Add(3 * time.Second)
	for sftpActive.Load() != 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if sftpActive.Load() != 0 {
		t.Fatalf("slots leaked: %d", sftpActive.Load())
	}
}

func TestAlwaysBlockedAddresses(t *testing.T) {
	for _, a := range []string{"169.254.169.254", "::ffff:169.254.169.254", "64:ff9b::a9fe:a9fe", "2002:a9fe:a9fe::1", "100.100.100.200", "fe80::1", "0.0.0.0", "::", "fd00:ec2::254", "64:ff9b::6464:64c8"} {
		if !alwaysBlockedWebDAV(netip.MustParseAddr(a)) {
			t.Errorf("%s must stay blocked", a)
		}
	}
	for _, a := range []string{"8.8.8.8", "10.1.2.3", "127.0.0.1", "64:ff9b::808:808"} {
		if alwaysBlockedWebDAV(netip.MustParseAddr(a)) {
			t.Errorf("%s is private or public, not always blocked", a)
		}
	}
}

func TestSFTPKeyLengthCapped(t *testing.T) {
	s := &sftpSession{base: "/srv"}
	if _, err := s.remote(strings.Repeat("a", sftpMaxKeyLen+1)); err == nil {
		t.Fatal("overlong key accepted")
	}
}
