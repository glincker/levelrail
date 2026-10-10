package imagemove

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

const (
	defaultDialTimeout = 15 * time.Second
	stderrTailBytes    = 2048
)

// Saver streams `docker save` of one image from the source host.
type Saver interface {
	Save(ctx context.Context, ref string) (io.ReadCloser, error)
}

// Credentials authenticate to the source host. They are held in memory only.
type Credentials struct {
	PrivateKey []byte
	Passphrase []byte
	// UseAgent signs with the control plane process's SSH agent (SSH_AUTH_SOCK).
	UseAgent bool
}

// HostKeys pins each source host's key on first use, the in-process
// equivalent of StrictHostKeyChecking=accept-new. Path "" pins in memory.
type HostKeys struct {
	Path string
	mu   sync.Mutex
	mem  map[string]string
}

// Callback checks key against the pinned one, recording it when unknown.
func (h *HostKeys) Callback(hostname string, remote net.Addr, key ssh.PublicKey) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.Path == "" {
		return h.checkMemory(knownhosts.Normalize(hostname), key)
	}
	if err := os.MkdirAll(filepath.Dir(h.Path), 0o700); err != nil {
		return fmt.Errorf("imagemove: known hosts dir: %w", err)
	}
	f, err := os.OpenFile(h.Path, os.O_CREATE|os.O_RDONLY, 0o600)
	if err != nil {
		return fmt.Errorf("imagemove: known hosts file: %w", err)
	}
	_ = f.Close()
	check, err := knownhosts.New(h.Path)
	if err != nil {
		return fmt.Errorf("imagemove: read known hosts: %w", err)
	}
	err = check(hostname, remote, key)
	var keyErr *knownhosts.KeyError
	switch {
	case err == nil:
		return nil
	case errors.As(err, &keyErr) && len(keyErr.Want) == 0:
		return appendKnownHost(h.Path, hostname, key)
	case errors.As(err, &keyErr):
		return fmt.Errorf("the host key of %s changed since it was first trusted; remove its line from %s if the change is expected", hostname, h.Path)
	}
	return fmt.Errorf("imagemove: check host key: %w", err)
}

func (h *HostKeys) checkMemory(host string, key ssh.PublicKey) error {
	if h.mem == nil {
		h.mem = map[string]string{}
	}
	got := string(key.Marshal())
	if want, ok := h.mem[host]; ok && want != got {
		return fmt.Errorf("the host key of %s changed since it was first trusted", host)
	}
	h.mem[host] = got
	return nil
}

func appendKnownHost(path, hostname string, key ssh.PublicKey) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600) //nolint:gosec // path is the known hosts file in the control plane data dir, never request input
	if err != nil {
		return fmt.Errorf("imagemove: open known hosts: %w", err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString(knownhosts.Line([]string{knownhosts.Normalize(hostname)}, key) + "\n"); err != nil {
		return fmt.Errorf("imagemove: record host key: %w", err)
	}
	return nil
}

// SSHSaver runs the fixed save command over an in-process SSH client: no
// shell or ssh binary runs on this side, and no prompt can ever block.
type SSHSaver struct {
	Target      Target
	Creds       Credentials
	HostKeys    *HostKeys
	DialTimeout time.Duration
}

func (s *SSHSaver) auth() ([]ssh.AuthMethod, func(), error) {
	var methods []ssh.AuthMethod
	cleanup := func() {}
	if len(s.Creds.PrivateKey) > 0 {
		var signer ssh.Signer
		var err error
		if len(s.Creds.Passphrase) > 0 {
			signer, err = ssh.ParsePrivateKeyWithPassphrase(s.Creds.PrivateKey, s.Creds.Passphrase)
		} else {
			signer, err = ssh.ParsePrivateKey(s.Creds.PrivateKey)
		}
		if err != nil {
			return nil, cleanup, errors.New("the private key could not be read (wrong format or passphrase)")
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}
	if s.Creds.UseAgent {
		sock := os.Getenv("SSH_AUTH_SOCK")
		if sock == "" {
			return nil, cleanup, errors.New("no SSH agent is available to the control plane (SSH_AUTH_SOCK is not set)")
		}
		conn, err := net.Dial("unix", sock) //nolint:gosec // socket comes from this process's own SSH_AUTH_SOCK, not request input
		if err != nil {
			return nil, cleanup, fmt.Errorf("connect to the SSH agent: %w", err)
		}
		cleanup = func() { _ = conn.Close() }
		methods = append(methods, ssh.PublicKeysCallback(agent.NewClient(conn).Signers))
	}
	if len(methods) == 0 {
		return nil, cleanup, errors.New("supply a private key or use the SSH agent")
	}
	return methods, cleanup, nil
}

// Save starts `docker save ref` on the source. Close waits for the remote
// command and reports its failure with the tail of its stderr.
func (s *SSHSaver) Save(ctx context.Context, ref string) (io.ReadCloser, error) {
	cmd, err := SaveCommand(ref)
	if err != nil {
		return nil, err
	}
	if s.HostKeys == nil {
		return nil, errors.New("imagemove: no host key store")
	}
	methods, cleanup, err := s.auth()
	if err != nil {
		return nil, err
	}
	timeout := s.DialTimeout
	if timeout <= 0 {
		timeout = defaultDialTimeout
	}
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", s.Target.Addr())
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("connect to %s: %w", s.Target.Addr(), err)
	}
	cfg := &ssh.ClientConfig{User: s.Target.User, Auth: methods, HostKeyCallback: s.HostKeys.Callback, Timeout: timeout}
	sc, chans, reqs, err := ssh.NewClientConn(conn, s.Target.Addr(), cfg)
	cleanup()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("ssh login to %s: %w", s.Target.String(), err)
	}
	client := ssh.NewClient(sc, chans, reqs)
	sess, err := client.NewSession()
	if err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("open ssh session: %w", err)
	}
	out, err := sess.StdoutPipe()
	if err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("ssh stdout: %w", err)
	}
	tail := &tailBuffer{max: stderrTailBytes}
	sess.Stderr = tail
	if err := sess.Start(cmd); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("start %q on the source: %w", cmd, err)
	}
	st := &sshStream{r: out, sess: sess, client: client, tail: tail, done: make(chan struct{})}
	go func() {
		select {
		case <-ctx.Done():
			_ = client.Close()
		case <-st.done:
		}
	}()
	return st, nil
}

type sshStream struct {
	r      io.Reader
	sess   *ssh.Session
	client *ssh.Client
	tail   *tailBuffer
	eof    bool
	once   sync.Once
	done   chan struct{}
	err    error
}

func (s *sshStream) Read(p []byte) (int, error) {
	n, err := s.r.Read(p)
	if errors.Is(err, io.EOF) {
		s.eof = true
	}
	return n, err
}

func (s *sshStream) Close() error {
	s.once.Do(func() {
		defer close(s.done)
		if !s.eof {
			_ = s.client.Close()
		}
		werr := s.sess.Wait()
		_ = s.client.Close()
		if werr != nil && s.eof {
			s.err = remoteError(werr, s.tail.String())
		}
	})
	return s.err
}

func remoteError(err error, stderr string) error {
	stderr = strings.TrimSpace(stderr)
	if stderr == "" {
		return fmt.Errorf("docker save on the source failed: %w", err)
	}
	return fmt.Errorf("docker save on the source failed: %s", stderr)
}

// tailBuffer keeps the last max bytes written to it.
type tailBuffer struct {
	mu  sync.Mutex
	max int
	buf []byte
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > t.max {
		t.buf = t.buf[len(t.buf)-t.max:]
	}
	return len(p), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.buf)
}
