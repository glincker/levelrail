package dockerssh

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

type fakeHost struct {
	t           *testing.T
	addr        string
	port        int
	fingerprint string
	userKey     []byte
	socket      string

	mu       sync.Mutex
	requests []string
	sessions int
	others   []string
}

func (h *fakeHost) record(r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.requests = append(h.requests, r.Method+" "+r.URL.RequestURI())
}

func tarOf(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	_ = tw.WriteHeader(&tar.Header{Name: "data/", Typeflag: tar.TypeDir, Mode: 0o755})
	for n, c := range files {
		_ = tw.WriteHeader(&tar.Header{Name: "data/" + n, Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(c))})
		_, _ = tw.Write([]byte(c))
	}
	_ = tw.Close()
	return buf.Bytes()
}

func (h *fakeHost) handler() http.Handler {
	mux := http.NewServeMux()
	write := func(w http.ResponseWriter, body string) { _, _ = io.WriteString(w, body) }
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		h.record(r)
		switch r.URL.Path {
		case "/_ping":
			write(w, "OK")
		case "/version":
			write(w, `{"Version":"27.3.1","Os":"linux"}`)
		case "/info":
			write(w, `{"Swarm":{"LocalNodeState":"inactive"}}`)
		case "/containers/json":
			write(w, `[{"Id":"abc123","State":"running"},{"Id":"def456","State":"exited"}]`)
		case "/containers/abc123/json":
			write(w, `{"Id":"abc123","Name":"/web","Config":{"Image":"web:1"},"Mounts":[{"Destination":"/data"}]}`)
		case "/containers/def456/json":
			write(w, `{"Id":"def456","Name":"/old","Config":{"Image":"old:1"},"Mounts":[]}`)
		case "/containers/abc123/archive":
			_, _ = w.Write(tarOf(h.t, map[string]string{"a.txt": "hello"}))
		case "/volumes":
			write(w, `{"Volumes":[{"Name":"v1"},{"Name":"v2"}]}`)
		case "/images/json":
			write(w, `[{"Id":"sha256:1"}]`)
		case "/images/web:1/get":
			write(w, "IMAGE-TAR")
		default:
			http.NotFound(w, r)
		}
	})
	return mux
}

type oneConnListener struct {
	conn net.Conn
	once sync.Once
	done chan struct{}
}

func (l *oneConnListener) Accept() (net.Conn, error) {
	var c net.Conn
	l.once.Do(func() { c = l.conn })
	if c != nil {
		return c, nil
	}
	<-l.done
	return nil, errors.New("closed")
}
func (l *oneConnListener) Close() error   { return nil }
func (l *oneConnListener) Addr() net.Addr { return chanAddr{} }

func startFakeHost(t *testing.T) *fakeHost {
	t.Helper()
	h := &fakeHost{t: t, socket: DefaultSocketPath}
	_, hostPriv, _ := ed25519.GenerateKey(rand.Reader)
	hostSigner, err := ssh.NewSignerFromKey(hostPriv)
	if err != nil {
		t.Fatal(err)
	}
	h.fingerprint = ssh.FingerprintSHA256(hostSigner.PublicKey())
	_, userPriv, _ := ed25519.GenerateKey(rand.Reader)
	userSigner, _ := ssh.NewSignerFromKey(userPriv)
	h.userKey = marshalPEM(t, userPriv)
	cfg := &ssh.ServerConfig{PublicKeyCallback: func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
		if bytes.Equal(key.Marshal(), userSigner.PublicKey().Marshal()) {
			return nil, nil
		}
		return nil, errors.New("unknown key")
	}}
	cfg.AddHostKey(hostSigner)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	h.addr = ln.Addr().String()
	h.port = ln.Addr().(*net.TCPAddr).Port
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go h.serve(c, cfg)
		}
	}()
	return h
}

func (h *fakeHost) serve(c net.Conn, cfg *ssh.ServerConfig) {
	sc, chans, reqs, err := ssh.NewServerConn(c, cfg)
	if err != nil {
		return
	}
	go ssh.DiscardRequests(reqs)
	for nc := range chans {
		switch nc.ChannelType() {
		case streamlocalChannel:
			var p streamlocalPayload
			if err := ssh.Unmarshal(nc.ExtraData(), &p); err != nil || p.SocketPath != h.socket {
				_ = nc.Reject(ssh.ConnectionFailed, "connect failed (No such file or directory)")
				continue
			}
			ch, creq, err := nc.Accept()
			if err != nil {
				continue
			}
			go ssh.DiscardRequests(creq)
			l := &oneConnListener{conn: &channelConn{Channel: ch}, done: make(chan struct{})}
			srv := &http.Server{Handler: h.handler(), ReadHeaderTimeout: 5 * time.Second}
			go func() { _ = srv.Serve(l) }()
		case "session":
			h.mu.Lock()
			h.sessions++
			h.mu.Unlock()
			_ = nc.Reject(ssh.Prohibited, "no sessions")
		default:
			h.mu.Lock()
			h.others = append(h.others, nc.ChannelType())
			h.mu.Unlock()
			_ = nc.Reject(ssh.UnknownChannelType, "no")
		}
	}
	_ = sc.Close()
}

func (h *fakeHost) cfg() Config {
	return Config{Host: "127.0.0.1", Port: h.port, User: "root", PrivateKey: h.userKey, Fingerprint: h.fingerprint, DialTimeout: 5 * time.Second}
}

func TestSessionOnlyAllowlistedGETsReachTheHost(t *testing.T) {
	h := startFakeHost(t)
	ctx := context.Background()
	s, err := Open(ctx, h.cfg())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	ov, err := s.Probe(ctx)
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if ov.DockerVersion != "27.3.1" || ov.Containers != 2 || ov.Running != 1 || ov.Volumes != 2 || ov.Images != 1 {
		t.Fatalf("overview = %+v", ov)
	}
	raw, err := s.InspectAll(ctx)
	if err != nil || !strings.Contains(string(raw), `"web:1"`) || !strings.HasPrefix(string(raw), "[") {
		t.Fatalf("InspectAll = %s, %v", raw, err)
	}
	rc, err := s.ImageSave(ctx, "web:1")
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := io.ReadAll(rc); string(b) != "IMAGE-TAR" {
		t.Fatalf("image stream = %q", b)
	}
	_ = rc.Close()
	ar, err := s.ContainerArchive(ctx, "abc123", "/data")
	if err != nil {
		t.Fatalf("ContainerArchive: %v", err)
	}
	dst := t.TempDir()
	if _, err := ExtractArchive(ar, dst, Limits{}); err != nil {
		t.Fatalf("extract: %v", err)
	}
	_ = ar.Close()
	if _, err := s.ContainerArchive(ctx, "abc123", "/etc"); !errors.Is(err, ErrRequestForbidden) {
		t.Fatalf("archive of a non mount path: %v", err)
	}

	for _, m := range []string{"POST", "PUT", "DELETE"} {
		req, _ := http.NewRequestWithContext(ctx, m, "http://"+dockerPseudoHost+"/containers/abc123/exec", strings.NewReader("{}"))
		if _, err := s.http.Do(req); !errors.Is(err, ErrRequestForbidden) {
			t.Fatalf("%s exec: %v", m, err)
		}
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", "http://"+dockerPseudoHost+"/containers/abc123/logs", nil)
	if _, err := s.http.Do(req); !errors.Is(err, ErrRequestForbidden) {
		t.Fatalf("logs: %v", err)
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if h.sessions != 0 || len(h.others) != 0 {
		t.Fatalf("the host saw %d session channels and other channels %v", h.sessions, h.others)
	}
	ok := regexpAllowed()
	for _, r := range h.requests {
		if !strings.HasPrefix(r, "GET ") || !ok(strings.TrimPrefix(r, "GET ")) {
			t.Errorf("unexpected request reached the host: %s", r)
		}
	}
	if len(h.requests) == 0 {
		t.Fatal("no request recorded")
	}
}

func regexpAllowed() func(string) bool {
	tr := NewReadOnlyTransport(nil)
	tr.learn("abc123", []byte(`{"Id":"abc123","Mounts":[{"Destination":"/data"}]}`))
	return func(uri string) bool {
		r, err := http.NewRequest(http.MethodGet, "http://x"+uri, nil)
		return err == nil && tr.Check(r) == nil
	}
}

func TestOpenFailureKinds(t *testing.T) {
	h := startFakeHost(t)
	ctx := context.Background()
	cases := []struct {
		name string
		mod  func(*Config)
		want string
	}{
		{"fingerprint required", func(c *Config) { c.Fingerprint = "" }, KindHostKeyRequired},
		{"host key mismatch", func(c *Config) { c.Fingerprint = "SHA256:" + strings.Repeat("A", 43) }, KindHostKey},
		{"wrong key", func(c *Config) {
			_, k, _ := ed25519.GenerateKey(rand.Reader)
			c.PrivateKey = marshalPEM(t, k)
		}, KindAuth},
		{"unreadable key", func(c *Config) { c.PrivateKey = []byte("nope") }, KindAuth},
		{"no credential", func(c *Config) { c.PrivateKey = nil }, KindAuth},
		{"socket path wrong", func(c *Config) { c.SocketPath = "/run/other.sock" }, KindSocketPath},
		{"unreachable", func(c *Config) { c.Port = 1 }, KindUnreachable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := h.cfg()
			tc.mod(&cfg)
			_, err := Open(ctx, cfg)
			if err == nil {
				t.Fatal("Open succeeded")
			}
			if got := KindOf(err); got != tc.want {
				t.Fatalf("kind = %s (%v), want %s", got, err, tc.want)
			}
		})
	}
}

func TestInsecureHostKeyIsWarned(t *testing.T) {
	h := startFakeHost(t)
	cfg := h.cfg()
	cfg.Fingerprint, cfg.InsecureHostKey = "", true
	s, err := Open(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if len(s.Warnings()) == 0 {
		t.Fatal("skipping the host key check carried no warning")
	}
}

func TestBlockedAddressIsRefused(t *testing.T) {
	h := startFakeHost(t)
	cfg := h.cfg()
	cfg.CheckAddr = func(a netip.Addr) error {
		if a.IsLoopback() {
			return errors.New("loopback")
		}
		return nil
	}
	_, err := Open(context.Background(), cfg)
	if KindOf(err) != KindBlockedAddress {
		t.Fatalf("kind = %s (%v), want %s", KindOf(err), err, KindBlockedAddress)
	}
	if _, err := ScanHostKeys(context.Background(), cfg); KindOf(err) != KindBlockedAddress {
		t.Fatalf("scan kind = %s (%v)", KindOf(err), err)
	}
}

func marshalPEM(t *testing.T, k ed25519.PrivateKey) []byte {
	t.Helper()
	blk, err := ssh.MarshalPrivateKey(k, "")
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(blk)
}

func TestScanHostKeysReturnsFingerprintWithoutLogin(t *testing.T) {
	h := startFakeHost(t)
	keys, err := ScanHostKeys(context.Background(), Config{Host: "127.0.0.1", Port: h.port, DialTimeout: 5 * time.Second})
	if err != nil || len(keys) != 1 || keys[0].Fingerprint != h.fingerprint {
		t.Fatalf("keys = %+v, %v", keys, err)
	}
}

func TestWipeZeroesSecrets(t *testing.T) {
	c := Config{PrivateKey: []byte("key"), Passphrase: []byte("pp"), Password: []byte("pw")}
	c.Wipe()
	for _, b := range [][]byte{c.PrivateKey, c.Passphrase, c.Password} {
		if !bytes.Equal(b, make([]byte, len(b))) {
			t.Fatalf("secret not zeroed: %v", b)
		}
	}
}
