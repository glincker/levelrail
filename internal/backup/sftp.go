package backup

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/netguard"
)

// SFTPEndpointScheme prefixes the stored endpoint of an SFTP target; the rest is host:port.
const SFTPEndpointScheme = store.SFTPEndpointScheme

// sftpFingerprintPrefix is the only fingerprint format accepted for pinned host keys.
const sftpFingerprintPrefix = "SHA256:"

// Errors callers can match on.
var (
	ErrSFTPBadEndpoint     = errors.New("backup: sftp endpoint must be host or host:port")
	ErrSFTPBadHostKey      = errors.New("backup: sftp host key must look like \"ssh-ed25519 SHA256:...\"")
	ErrSFTPHostKeyMismatch = errors.New("backup: sftp host key does not match the pinned key")
	ErrSFTPNoHostKey       = errors.New("backup: sftp target has no pinned host key")
	errSFTPScanDone        = errors.New("host key captured")
)

// SFTPSettings are the live SFTP tunables; the API layer feeds them from runtime settings.
type SFTPSettings struct {
	AllowPrivate           bool
	Connect, Idle          time.Duration
	MaxEntries, MaxDepth   int
	MaxConns, MaxPerClient int
}

func defaultSFTPSettings() SFTPSettings {
	return SFTPSettings{Connect: 15 * time.Second, Idle: time.Minute, MaxEntries: 20000, MaxDepth: 8, MaxConns: 4, MaxPerClient: 8}
}

var sftpSource atomic.Pointer[func() SFTPSettings]

// SetSFTPSource makes SFTP targets follow live settings; unset, the safe defaults apply.
func SetSFTPSource(f func() SFTPSettings) { sftpSource.Store(&f) }

func sftpSettings() SFTPSettings {
	if f := sftpSource.Load(); f != nil {
		return (*f)()
	}
	return defaultSFTPSettings()
}

// SFTPAllowPrivate reports whether SFTP targets may reach private networks.
func SFTPAllowPrivate() bool { return sftpSettings().AllowPrivate }

// IsSFTP reports whether the destination is an SFTP server.
func (d Destination) IsSFTP() bool { return strings.HasPrefix(d.Endpoint, SFTPEndpointScheme) }

// SFTPHostPort splits a stored or submitted endpoint into host and numeric port (default 22).
func SFTPHostPort(endpoint string) (string, string, error) {
	raw := strings.TrimPrefix(endpoint, SFTPEndpointScheme)
	if raw == "" || strings.ContainsAny(raw, "/@ \t\x00") {
		return "", "", ErrSFTPBadEndpoint
	}
	host, port, err := net.SplitHostPort(raw)
	if err != nil {
		host, port = strings.Trim(raw, "[]"), "22"
	}
	n, perr := strconv.Atoi(port)
	if host == "" || perr != nil || n < 1 || n > 65535 {
		return "", "", ErrSFTPBadEndpoint
	}
	return host, port, nil
}

// ValidateSFTPHost refuses internal destinations unless the dedicated opt-in is on.
func ValidateSFTPHost(ctx context.Context, endpoint string) error {
	host, _, err := SFTPHostPort(endpoint)
	if err != nil {
		return err
	}
	return validateEndpointHost(ctx, strings.ToLower(host), SFTPAllowPrivate(), "SFTP")
}

// SFTPBasePath normalizes the remote base directory: relative paths start in the login directory.
func SFTPBasePath(dir string) (string, error) {
	if strings.ContainsRune(dir, 0) || strings.Contains(dir, "\\") {
		return "", errors.New("remote path contains invalid characters")
	}
	for _, s := range strings.Split(dir, "/") {
		if s == ".." {
			return "", errors.New("remote path must not contain '..'")
		}
	}
	return path.Clean(dir), nil
}

// ParseSFTPHostKey splits a pinned key "<type> SHA256:<base64>" and checks its shape.
func ParseSFTPHostKey(s string) (algo, fp string, err error) {
	f := strings.Fields(s)
	if len(f) != 2 || !strings.HasPrefix(f[1], sftpFingerprintPrefix) {
		return "", "", ErrSFTPBadHostKey
	}
	raw, derr := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(f[1], sftpFingerprintPrefix))
	if derr != nil || len(raw) != 32 || strings.ContainsAny(f[0], "\x00,") {
		return "", "", ErrSFTPBadHostKey
	}
	return f[0], f[1], nil
}

// hostKeyAlgorithms pins the negotiated key type so a server cannot answer with another one.
func hostKeyAlgorithms(keyType string) []string {
	if keyType == ssh.KeyAlgoRSA {
		return []string{ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSASHA256}
	}
	return []string{keyType}
}

// sftpDialControl refuses blocked addresses at connect time, after name resolution.
func sftpDialControl(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("backup: sftp: parse dial address %q: %w", address, err)
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return fmt.Errorf("backup: sftp: parse dial address %q: %w", address, err)
	}
	if alwaysBlockedWebDAV(ip) {
		return fmt.Errorf("%w: %s (metadata and link-local addresses are never allowed)", netguard.ErrBlockedAddress, ip)
	}
	if !SFTPAllowPrivate() && netguard.IsBlocked(ip) {
		return fmt.Errorf("%w: %s (enable the SFTP private network setting to allow it)", netguard.ErrBlockedAddress, ip)
	}
	return nil
}

// sftpActive counts open connections to bound simultaneous SFTP connections across all targets.
var sftpActive atomic.Int64

func acquireSFTPSlot(ctx context.Context, limit int) error {
	for {
		n := sftpActive.Load()
		if n < int64(limit) && sftpActive.CompareAndSwap(n, n+1) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// sshDial opens the TCP and SSH layers; the callback decides whether the host key is accepted.
func sshDial(ctx context.Context, endpoint string, cfg *ssh.ClientConfig, set SFTPSettings) (*ssh.Client, error) {
	host, port, err := SFTPHostPort(endpoint)
	if err != nil {
		return nil, err
	}
	d := &net.Dialer{Timeout: set.Connect, KeepAlive: 30 * time.Second, Control: sftpDialControl}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(host, port))
	if err != nil {
		return nil, fmt.Errorf("backup: sftp connect: %w", err)
	}
	// The deadline covers the handshake only; transfers use the idle timer.
	_ = conn.SetDeadline(time.Now().Add(set.Connect))
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	c, chans, reqs, err := ssh.NewClientConn(conn, net.JoinHostPort(host, port), cfg)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	_ = conn.SetDeadline(time.Time{})
	return ssh.NewClient(c, chans, reqs), nil
}

// ScanSFTPHostKey fetches the key a server presents without authenticating, so a user can
// review and explicitly pin it. No credentials are ever sent.
func ScanSFTPHostKey(ctx context.Context, endpoint string) (hostKey string, err error) {
	var captured ssh.PublicKey
	cfg := &ssh.ClientConfig{
		User: "levelrail-hostkey-scan",
		HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
			captured = key
			return errSFTPScanDone
		},
	}
	set := sftpSettings()
	if err := acquireSFTPSlot(ctx, set.MaxConns); err != nil {
		return "", fmt.Errorf("backup: sftp host key scan: %w", err)
	}
	defer sftpActive.Add(-1)
	if _, derr := sshDial(ctx, endpoint, cfg, set); captured == nil {
		if derr == nil {
			derr = errors.New("server presented no host key")
		}
		return "", fmt.Errorf("backup: sftp host key scan: %w", derr)
	}
	return captured.Type() + " " + ssh.FingerprintSHA256(captured), nil
}

// pinnedCallback accepts only the key whose type and SHA-256 fingerprint were pinned.
func pinnedCallback(algo, fp string) ssh.HostKeyCallback {
	return func(_ string, _ net.Addr, key ssh.PublicKey) error {
		got := ssh.FingerprintSHA256(key)
		if key.Type() == algo && subtle.ConstantTimeCompare([]byte(got), []byte(fp)) == 1 {
			return nil
		}
		return fmt.Errorf("%w: pinned %s %s, server presented %s %s; refusing the connection (re-verify the server and pin its new key to continue)",
			ErrSFTPHostKeyMismatch, algo, fp, key.Type(), got)
	}
}

// ValidateSFTPPrivateKey checks a key (and its passphrase when encrypted) parses, without echoing it.
func ValidateSFTPPrivateKey(key, passphrase string) error {
	_, err := parseSFTPKey(key, passphrase)
	return err
}

func parseSFTPKey(key, passphrase string) (ssh.Signer, error) {
	var signer ssh.Signer
	var err error
	if passphrase != "" {
		signer, err = ssh.ParsePrivateKeyWithPassphrase([]byte(key), []byte(passphrase))
	} else {
		signer, err = ssh.ParsePrivateKey([]byte(key))
	}
	var missing *ssh.PassphraseMissingError
	if errors.As(err, &missing) {
		return nil, errors.New("sftp private key is encrypted: provide its passphrase")
	}
	if err != nil {
		return nil, errors.New("sftp private key could not be read (wrong passphrase or unsupported format)")
	}
	return signer, nil
}

func sftpAuth(dest Destination) ([]ssh.AuthMethod, error) {
	if dest.PrivateKey != "" {
		signer, err := parseSFTPKey(dest.PrivateKey, dest.KeyPassphrase)
		if err != nil {
			return nil, fmt.Errorf("backup: %w", err)
		}
		return []ssh.AuthMethod{ssh.PublicKeys(signer)}, nil
	}
	if dest.SecretAccessKey == "" {
		return nil, errors.New("backup: sftp target has no password or private key")
	}
	return []ssh.AuthMethod{ssh.Password(dest.SecretAccessKey)}, nil
}

// sftpEntryBytes bounds the wire bytes one listed entry may account for, so a server that
// streams entries cannot grow memory past MaxEntries worth of data.
const sftpEntryBytes = 1024

// sftpMaxKeyLen and sftpMaxPathLen cap what a remote object name may be.
const (
	sftpMaxKeyLen  = 1024
	sftpMaxPathLen = 4096
)

// sftpSession is one authenticated connection; every call re-arms an idle timer that closes it on a stall.
type sftpSession struct {
	sc   *sftp.Client
	ssh  *ssh.Client
	sess *ssh.Session
	base string
	set  SFTPSettings

	// mu guards timer, stop and closed, which are set after the session may already be closing.
	mu     sync.Mutex
	timer  *time.Timer
	stop   func() bool
	closed bool
	once   sync.Once

	// budget is the remaining wire bytes a listing may read; negative means unlimited.
	budget   atomic.Int64
	overflow atomic.Bool
}

func (s *sftpSession) arm() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.timer != nil && !s.closed {
		s.timer.Reset(s.set.Idle)
	}
}

// Close releases the connection and its concurrency slot; it is safe to call twice and concurrently.
func (s *sftpSession) Close() error {
	s.once.Do(func() {
		s.mu.Lock()
		s.closed = true
		if s.timer != nil {
			s.timer.Stop()
		}
		stop := s.stop
		s.mu.Unlock()
		if stop != nil {
			stop()
		}
		if s.sc != nil {
			_ = s.sc.Close()
		}
		_ = s.sess.Close()
		_ = s.ssh.Close()
		sftpActive.Add(-1)
	})
	return nil
}

// sftpBudgetReader fails the transfer once a listing has read more than its byte budget.
type sftpBudgetReader struct {
	r io.Reader
	s *sftpSession
}

func (b sftpBudgetReader) Read(p []byte) (int, error) {
	n, err := b.r.Read(p)
	if b.s.budget.Load() >= 0 && b.s.budget.Add(-int64(n)) < 0 {
		b.s.overflow.Store(true)
		_ = b.s.ssh.Close()
		return n, errors.New("backup: sftp listing too large")
	}
	return n, err
}

// buildSession wraps an authenticated connection. The context watcher is registered last, after
// the session is complete, because it may fire immediately. It owns one connection slot.
func buildSession(ctx context.Context, conn *ssh.Client, base string, set SFTPSettings) (*sftpSession, error) {
	fail := func(err error) (*sftpSession, error) {
		_ = conn.Close()
		sftpActive.Add(-1)
		return nil, err
	}
	sess, err := conn.NewSession()
	if err != nil {
		return fail(fmt.Errorf("backup: sftp session: %w", err))
	}
	out, err1 := sess.StdoutPipe()
	in, err2 := sess.StdinPipe()
	if err := errors.Join(err1, err2); err != nil {
		return fail(fmt.Errorf("backup: sftp session: %w", err))
	}
	if err := sess.RequestSubsystem("sftp"); err != nil {
		return fail(fmt.Errorf("backup: sftp subsystem: %w", err))
	}
	s := &sftpSession{ssh: conn, sess: sess, base: base, set: set}
	s.budget.Store(-1)
	sc, err := sftp.NewClientPipe(sftpBudgetReader{r: out, s: s}, in, sftp.MaxConcurrentRequestsPerFile(set.MaxPerClient))
	if err != nil {
		return fail(fmt.Errorf("backup: sftp subsystem: %w", err))
	}
	s.sc = sc
	s.mu.Lock()
	s.timer = time.AfterFunc(set.Idle, func() { _ = s.Close() })
	s.mu.Unlock()
	stop := context.AfterFunc(ctx, func() { _ = s.Close() })
	s.mu.Lock()
	s.stop = stop
	s.mu.Unlock()
	return s, nil
}

func openSFTP(ctx context.Context, dest Destination) (*sftpSession, error) {
	algo, fp, err := ParseSFTPHostKey(dest.Region)
	if err != nil {
		return nil, ErrSFTPNoHostKey
	}
	base, err := SFTPBasePath(dest.Bucket)
	if err != nil {
		return nil, fmt.Errorf("backup: sftp: %w", err)
	}
	auth, err := sftpAuth(dest)
	if err != nil {
		return nil, err
	}
	set := sftpSettings()
	if err := acquireSFTPSlot(ctx, set.MaxConns); err != nil {
		return nil, err
	}
	cfg := &ssh.ClientConfig{
		User: dest.AccessKeyID, Auth: auth,
		HostKeyCallback: pinnedCallback(algo, fp), HostKeyAlgorithms: hostKeyAlgorithms(algo),
	}
	conn, err := sshDial(ctx, dest.Endpoint, cfg, set)
	if err != nil {
		sftpActive.Add(-1)
		return nil, err
	}
	return buildSession(ctx, conn, base, set)
}

// remote joins key under the base and proves the result never leaves it.
func (s *sftpSession) remote(key string) (string, error) {
	if key == "" || len(key) > sftpMaxKeyLen || strings.ContainsAny(key, "\x00\\") || strings.HasPrefix(key, "/") {
		return "", errors.New("backup: sftp key is empty or invalid")
	}
	for _, seg := range strings.Split(key, "/") {
		if seg == ".." {
			return "", errors.New("backup: sftp key must not contain '..'")
		}
	}
	full := path.Join(s.base, key)
	if len(full) > sftpMaxPathLen {
		return "", errors.New("backup: sftp key is too long")
	}
	if s.base != "." && full != s.base && !strings.HasPrefix(full, strings.TrimSuffix(s.base, "/")+"/") {
		return "", errors.New("backup: sftp key escapes the base path")
	}
	return full, nil
}

type idleWriter struct {
	w io.Writer
	s *sftpSession
}

func (i idleWriter) Write(p []byte) (int, error) { i.s.arm(); return i.w.Write(p) }

type idleReadCloser struct {
	f *sftp.File
	s *sftpSession
}

func (i idleReadCloser) Read(p []byte) (int, error) { i.s.arm(); return i.f.Read(p) }
func (i idleReadCloser) Close() error               { _ = i.f.Close(); return i.s.Close() }

// sftpClient adapts a Destination to the object operations the backup code needs.
type sftpClient struct{ dest Destination }

func newSFTP(dest Destination) (*sftpClient, error) {
	if _, _, err := SFTPHostPort(dest.Endpoint); err != nil {
		return nil, err
	}
	if _, _, err := ParseSFTPHostKey(dest.Region); err != nil {
		return nil, ErrSFTPNoHostKey
	}
	return &sftpClient{dest: dest}, nil
}

// Upload writes under a temporary name and renames it so a half written object never appears under key.
func (c *sftpClient) Upload(ctx context.Context, key string, r io.Reader) error {
	s, err := openSFTP(ctx, c.dest)
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()
	full, err := s.remote(key)
	if err != nil {
		return err
	}
	s.arm()
	if err := s.sc.MkdirAll(path.Dir(full)); err != nil {
		return fmt.Errorf("backup: sftp mkdir: %w", err)
	}
	tmp := full + ".partial"
	f, err := s.sc.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return fmt.Errorf("backup: sftp create: %w", err)
	}
	_ = f.Chmod(0o600)
	if _, err := io.Copy(idleWriter{w: f, s: s}, r); err != nil {
		_ = f.Close()
		s.cleanup(tmp)
		return fmt.Errorf("backup: sftp upload: %w", err)
	}
	if err := f.Close(); err != nil {
		s.cleanup(tmp)
		return fmt.Errorf("backup: sftp upload: %w", err)
	}
	s.arm()
	if err := s.rename(tmp, full); err != nil {
		s.cleanup(tmp)
		return fmt.Errorf("backup: sftp rename: %w", err)
	}
	return nil
}

func (s *sftpSession) rename(from, to string) error {
	if _, ok := s.sc.HasExtension("posix-rename@openssh.com"); ok {
		return s.sc.PosixRename(from, to)
	}
	_ = s.sc.Remove(to)
	return s.sc.Rename(from, to)
}

func (s *sftpSession) cleanup(p string) { s.arm(); _ = s.sc.Remove(p) }

// Download opens key; closing the reader ends the connection.
func (c *sftpClient) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	s, err := openSFTP(ctx, c.dest)
	if err != nil {
		return nil, err
	}
	full, err := s.remote(key)
	if err != nil {
		_ = s.Close()
		return nil, err
	}
	s.arm()
	f, err := s.sc.Open(full)
	if err != nil {
		_ = s.Close()
		return nil, fmt.Errorf("backup: sftp open: %w", err)
	}
	return idleReadCloser{f: f, s: s}, nil
}

// Delete removes key; an already missing object counts as deleted.
func (c *sftpClient) Delete(ctx context.Context, key string) error {
	s, err := openSFTP(ctx, c.dest)
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()
	full, err := s.remote(key)
	if err != nil {
		return err
	}
	s.arm()
	if err := s.sc.Remove(full); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("backup: sftp delete: %w", err)
	}
	return nil
}

// List returns every regular file under prefix with keys relative to the base path.
func (c *sftpClient) List(ctx context.Context, prefix string) ([]RemoteObject, error) {
	s, err := openSFTP(ctx, c.dest)
	if err != nil {
		return nil, err
	}
	defer func() { _ = s.Close() }()
	dir, namePrefix := path.Split(prefix)
	budget := s.set.MaxEntries
	s.budget.Store(int64(s.set.MaxEntries) * sftpEntryBytes)
	var out []RemoteObject
	var walk func(rel string, depth int) error
	walk = func(rel string, depth int) error {
		full := s.base
		if rel != "" {
			var err error
			if full, err = s.remote(rel); err != nil {
				return err
			}
		}
		s.arm()
		entries, err := s.sc.ReadDir(full)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			if s.overflow.Load() {
				return errors.New("backup: sftp listing too large")
			}
			return fmt.Errorf("backup: sftp list: %w", err)
		}
		for _, e := range entries {
			if budget--; budget < 0 {
				return errors.New("backup: sftp listing too large")
			}
			if e.Mode()&os.ModeSymlink != 0 || e.Name() == "." || e.Name() == ".." {
				continue
			}
			key := path.Join(rel, e.Name())
			switch {
			case e.IsDir():
				if depth < s.set.MaxDepth {
					if err := walk(key, depth+1); err != nil {
						return err
					}
				}
			case strings.HasPrefix(key, dir+namePrefix) && !strings.HasSuffix(key, ".partial"):
				out = append(out, RemoteObject{Key: key, Size: e.Size()})
			}
		}
		return nil
	}
	if err := walk(strings.Trim(dir, "/"), 0); err != nil {
		return nil, err
	}
	return out, nil
}

// Probe writes, reads back and deletes a tiny object, timing the round trip; free space is
// reported only when the server offers the statvfs extension.
func (c *sftpClient) Probe(ctx context.Context) (TestReport, error) {
	start := time.Now()
	key := ".levelrail-probe-" + strconv.FormatInt(start.UnixNano(), 36)
	if err := c.Upload(ctx, key, strings.NewReader("ok")); err != nil {
		return TestReport{}, err
	}
	rc, err := c.Download(ctx, key)
	if err != nil {
		_ = c.Delete(ctx, key)
		return TestReport{}, err
	}
	got, readErr := io.ReadAll(io.LimitReader(rc, 64))
	_ = rc.Close()
	if readErr != nil || string(got) != "ok" {
		_ = c.Delete(ctx, key)
		return TestReport{}, errors.New("backup: sftp test: object read back did not match what was written")
	}
	if err := c.Delete(ctx, key); err != nil {
		return TestReport{}, err
	}
	rep := TestReport{OK: true, LatencyMs: time.Since(start).Milliseconds()}
	if s, err := openSFTP(ctx, c.dest); err == nil {
		defer func() { _ = s.Close() }()
		if _, ok := s.sc.HasExtension("statvfs@openssh.com"); ok {
			if st, err := s.sc.StatVFS(s.base); err == nil && st.Frsize > 0 {
				free := int64(st.Bavail * st.Frsize) //nolint:gosec // bounded by a real disk size
				rep.FreeBytes = &free
			}
		}
	}
	return rep, nil
}
