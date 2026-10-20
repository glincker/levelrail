package dockerssh

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/GLINCKER/levelrail/internal/sshprovision"
)

// DefaultSocketPath is the Docker socket opened when none is given.
const DefaultSocketPath = "/var/run/docker.sock"

const (
	defaultDialTimeout = 15 * time.Second
	dockerPseudoHost   = "docker.invalid"
	streamlocalChannel = "direct-streamlocal@openssh.com"
)

// ErrBlockedAddress means the SSH target resolved to an address the network
// policy forbids.
var ErrBlockedAddress = errors.New("dockerssh: ssh address is not allowed")

// Config is one SSH login to a Docker host. The secrets are held in memory
// for the life of the session and are never written anywhere.
type Config struct {
	Host string
	Port int
	User string
	// PrivateKey is a PEM private key, Passphrase unlocks it, Password is an
	// alternative or additional login. Wipe zeroes all three.
	PrivateKey []byte
	Passphrase []byte
	Password   []byte
	// Fingerprint is the pinned host key, SHA256:... as ssh-keygen -l prints it.
	Fingerprint string
	// InsecureHostKey skips host key verification, an explicit opt in.
	InsecureHostKey bool
	SocketPath      string
	DialTimeout     time.Duration
	// CheckAddr vets every dialed IP, including after DNS resolution.
	CheckAddr func(netip.Addr) error
}

// Wipe zeroes the credential buffers.
func (c *Config) Wipe() {
	for _, b := range [][]byte{c.PrivateKey, c.Passphrase, c.Password} {
		for i := range b {
			b[i] = 0
		}
	}
}

func (c *Config) addr() string { return net.JoinHostPort(c.Host, strconv.Itoa(c.Port)) }

func (c *Config) timeout() time.Duration {
	if c.DialTimeout > 0 {
		return c.DialTimeout
	}
	return defaultDialTimeout
}

func (c *Config) socket() string {
	if c.SocketPath != "" {
		return c.SocketPath
	}
	return DefaultSocketPath
}

func (c *Config) validate() error {
	switch {
	case c.Host == "" || c.User == "":
		return newErr(KindOther, "an ssh host and user are required", nil)
	case c.Port < 1 || c.Port > 65535:
		return newErr(KindOther, "the ssh port must be from 1 to 65535", nil)
	case len(c.PrivateKey) == 0 && len(c.Password) == 0:
		return newErr(KindAuth, "supply a private key or a password for the ssh login", nil)
	case !strings.HasPrefix(c.socket(), "/") || strings.ContainsAny(c.socket(), "\x00"):
		return newErr(KindSocketPath, "the docker socket path must be absolute", nil)
	}
	return nil
}

func (c *Config) dialer() *net.Dialer {
	d := &net.Dialer{Timeout: c.timeout()}
	if c.CheckAddr != nil {
		d.Control = func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return fmt.Errorf("parse dial address: %w", err)
			}
			ip, err := netip.ParseAddr(host)
			if err != nil {
				return fmt.Errorf("parse dial address: %w", err)
			}
			if err := c.CheckAddr(ip); err != nil {
				return fmt.Errorf("%w: %w", ErrBlockedAddress, err)
			}
			return nil
		}
	}
	return d
}

func (c *Config) authMethods() ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod
	if len(c.PrivateKey) > 0 {
		var signer ssh.Signer
		var err error
		if len(c.Passphrase) > 0 {
			signer, err = ssh.ParsePrivateKeyWithPassphrase(c.PrivateKey, c.Passphrase)
		} else {
			signer, err = ssh.ParsePrivateKey(c.PrivateKey)
		}
		if err != nil {
			return nil, newErr(KindAuth, "the private key could not be read (wrong format or passphrase)", nil)
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}
	if len(c.Password) > 0 {
		pw := string(c.Password)
		methods = append(methods, ssh.Password(pw))
	}
	return methods, nil
}

// HostKey is one host key a server presented.
type HostKey = sshprovision.HostKey

var scanFamilies = [][]string{
	{ssh.KeyAlgoED25519},
	{ssh.KeyAlgoECDSA256, ssh.KeyAlgoECDSA384, ssh.KeyAlgoECDSA521},
	{ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSASHA256, ssh.KeyAlgoRSA},
}

func familyOf(keyType string) []string {
	for _, fam := range scanFamilies {
		for _, a := range fam {
			if a == keyType {
				return fam
			}
		}
	}
	return []string{keyType}
}

var errKeyRecorded = errors.New("dockerssh: host key recorded")

// ScanHostKeys connects and returns every host key the server presents. It
// never authenticates: each handshake stops inside the host key callback,
// before a user name or credential is sent.
func ScanHostKeys(ctx context.Context, cfg Config) ([]HostKey, error) {
	if cfg.Host == "" {
		return nil, newErr(KindOther, "an ssh host is required", nil)
	}
	if cfg.Port == 0 {
		cfg.Port = 22
	}
	var keys []HostKey
	var lastErr error
	seen := map[string]bool{}
	for _, fam := range scanFamilies {
		k, err := scanOne(ctx, cfg, fam)
		if err != nil {
			lastErr = err
			if ctx.Err() != nil {
				break
			}
			continue
		}
		if !seen[k.Fingerprint] {
			seen[k.Fingerprint] = true
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		if lastErr == nil {
			lastErr = errors.New("no host key offered")
		}
		return nil, classifyDial(cfg, lastErr)
	}
	return keys, nil
}

func scanOne(ctx context.Context, cfg Config, algos []string) (HostKey, error) {
	conn, err := cfg.dialer().DialContext(ctx, "tcp", cfg.addr())
	if err != nil {
		return HostKey{}, err
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(cfg.timeout()))
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	var got *HostKey
	sc := &ssh.ClientConfig{
		HostKeyAlgorithms: algos,
		HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
			got = &HostKey{Type: key.Type(), Fingerprint: ssh.FingerprintSHA256(key)}
			return errKeyRecorded
		},
		Timeout: cfg.timeout(),
	}
	_, _, _, err = ssh.NewClientConn(conn, cfg.addr(), sc)
	if got != nil {
		return *got, nil
	}
	if err == nil {
		return HostKey{}, errors.New("handshake completed without a host key check")
	}
	return HostKey{}, err
}

func classifyDial(cfg Config, err error) error {
	switch {
	case errors.Is(err, ErrBlockedAddress):
		return newErr(KindBlockedAddress, err.Error(), err)
	default:
		return newErr(KindUnreachable, fmt.Sprintf("could not reach %s over ssh: %v", cfg.addr(), err), err)
	}
}

func (c *Config) hostKeyCallback(ctx context.Context) (ssh.HostKeyCallback, []string, []string, error) {
	if c.InsecureHostKey {
		return ssh.InsecureIgnoreHostKey(), nil, []string{"host key verification is off for this session"}, nil //nolint:gosec // explicit operator opt out, warned on every surface
	}
	if c.Fingerprint == "" {
		return nil, nil, nil, newErr(KindHostKeyRequired, "a host key fingerprint is required: confirm the server's fingerprint first", nil)
	}
	fp, ok := sshprovision.NormalizeFingerprint(c.Fingerprint)
	if !ok {
		return nil, nil, nil, newErr(KindHostKeyRequired, "the host key fingerprint is malformed, expected SHA256:... as ssh-keygen -l prints it", nil)
	}
	keys, err := ScanHostKeys(ctx, *c)
	if err != nil {
		return nil, nil, nil, err
	}
	for _, k := range keys {
		if k.Fingerprint == fp {
			return sshprovision.PinnedHostKeyCallback(fp), familyOf(k.Type), nil, nil
		}
	}
	mm := &sshprovision.HostKeyMismatchError{Expected: fp, Observed: keys}
	return nil, nil, nil, newErr(KindHostKey, "the server's host key does not match the confirmed fingerprint: "+mm.Error(), mm)
}

// Session is one open SSH connection used only as a transport to the Docker
// socket. It has no shell, no command execution and no other forwarding.
type Session struct {
	client   *ssh.Client
	http     *http.Client
	ro       *ReadOnlyTransport
	socket   string
	warnings []string
	once     sync.Once
}

// Open logs in, opens the Docker socket channel and pings the daemon.
func Open(ctx context.Context, cfg Config) (*Session, error) {
	if cfg.Port == 0 {
		cfg.Port = 22
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	methods, err := cfg.authMethods()
	if err != nil {
		return nil, err
	}
	cb, algos, warns, err := cfg.hostKeyCallback(ctx)
	if err != nil {
		return nil, err
	}
	conn, err := cfg.dialer().DialContext(ctx, "tcp", cfg.addr())
	if err != nil {
		return nil, classifyDial(cfg, err)
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	sc := &ssh.ClientConfig{User: cfg.User, Auth: methods, HostKeyCallback: cb, HostKeyAlgorithms: algos, Timeout: cfg.timeout()}
	c, chans, reqs, err := ssh.NewClientConn(conn, cfg.addr(), sc)
	stop()
	if err != nil {
		_ = conn.Close()
		return nil, classifyHandshake(cfg, err)
	}
	s := &Session{client: ssh.NewClient(c, chans, reqs), socket: cfg.socket(), warnings: warns}
	s.ro = NewReadOnlyTransport(&http.Transport{
		DialContext:           s.dialSocket,
		Proxy:                 nil,
		DisableCompression:    true,
		ResponseHeaderTimeout: 60 * time.Second,
	})
	s.http = &http.Client{Transport: s.ro}
	if err := s.Ping(ctx); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

func classifyHandshake(cfg Config, err error) error {
	var mm *sshprovision.HostKeyMismatchError
	switch {
	case errors.As(err, &mm):
		return newErr(KindHostKey, mm.Error(), err)
	case strings.Contains(err.Error(), "unable to authenticate"):
		return newErr(KindAuth, "the ssh login was refused: wrong key, passphrase, password or user", err)
	default:
		return classifyDial(cfg, err)
	}
}

// Warnings lists caveats of this session, such as a skipped host key check.
func (s *Session) Warnings() []string { return append([]string(nil), s.warnings...) }

// Close ends the SSH connection.
func (s *Session) Close() {
	s.once.Do(func() { _ = s.client.Close() })
}

type streamlocalPayload struct {
	SocketPath string
	Reserved0  string
	Reserved1  uint32
}

func (s *Session) dialSocket(ctx context.Context, _, _ string) (net.Conn, error) {
	type result struct {
		ch   ssh.Channel
		reqs <-chan *ssh.Request
		err  error
	}
	done := make(chan result, 1)
	go func() {
		ch, reqs, err := s.client.OpenChannel(streamlocalChannel, ssh.Marshal(streamlocalPayload{SocketPath: s.socket}))
		done <- result{ch, reqs, err}
	}()
	select {
	case <-ctx.Done():
		go func() {
			if r := <-done; r.err == nil {
				_ = r.ch.Close()
			}
		}()
		return nil, ctx.Err()
	case r := <-done:
		if r.err != nil {
			return nil, classifyChannel(s.socket, r.err)
		}
		go ssh.DiscardRequests(r.reqs)
		return &channelConn{Channel: r.ch}, nil
	}
}

func classifyChannel(socket string, err error) error {
	msg := err.Error()
	low := strings.ToLower(msg)
	switch {
	case strings.Contains(low, "permission denied"):
		return newErr(KindSocketPermission, "the ssh user may not open "+socket+": add it to the docker group on the old host", err)
	case strings.Contains(low, "no such file"), strings.Contains(low, "not found"):
		return newErr(KindSocketPath, "there is no docker socket at "+socket+" on the old host", err)
	case strings.Contains(low, "administratively prohibited"), strings.Contains(low, "prohibited"):
		return newErr(KindSocketForbidden, "the old host's sshd refuses socket forwarding for this login (AllowStreamLocalForwarding or the key's permitopen restriction)", err)
	default:
		return newErr(KindOther, "could not open "+socket+" over ssh: "+msg, err)
	}
}

type channelConn struct{ ssh.Channel }

type chanAddr struct{}

func (chanAddr) Network() string { return "ssh-streamlocal" }
func (chanAddr) String() string  { return "docker-socket" }

func (*channelConn) LocalAddr() net.Addr                { return chanAddr{} }
func (*channelConn) RemoteAddr() net.Addr               { return chanAddr{} }
func (*channelConn) SetDeadline(_ time.Time) error      { return nil }
func (*channelConn) SetReadDeadline(_ time.Time) error  { return nil }
func (*channelConn) SetWriteDeadline(_ time.Time) error { return nil }
