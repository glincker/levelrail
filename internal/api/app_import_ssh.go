package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/dockerssh"
	"github.com/GLINCKER/levelrail/internal/imagemove"
	"github.com/GLINCKER/levelrail/internal/platformimport"
	"github.com/GLINCKER/levelrail/internal/sshprovision"
)

const (
	envSSHTimeout       = "APP_MIGRATE_SSH_TIMEOUT"
	defaultSSHTimeout   = 2 * time.Minute
	redactedSecret      = "[redacted]"
	sshInsecureWarning  = "Host key verification is off for this session: anyone between this server and the old host could read what it sends. Confirm the fingerprint instead."
	sshReadOnlyOverview = "read only: lists containers, volumes and images"
)

// secretBytes holds a credential from a request body. It never marshals,
// prints or logs its value.
type secretBytes []byte

func (s *secretBytes) UnmarshalJSON(b []byte) error {
	var str string
	if err := json.Unmarshal(b, &str); err != nil {
		return err
	}
	*s = []byte(str)
	return nil
}

func (secretBytes) MarshalJSON() ([]byte, error) { return []byte(`"` + redactedSecret + `"`), nil }

func (secretBytes) String() string { return redactedSecret }

func (secretBytes) GoString() string { return redactedSecret }

// sshConnection is the SSH login of a docker-ssh source. The secrets live
// only in memory for the session and are zeroed when it is forgotten.
type sshConnection struct {
	Host            string      `json:"host"`
	Port            int         `json:"port,omitempty"`
	User            string      `json:"user"`
	PrivateKey      secretBytes `json:"private_key,omitempty"`
	Passphrase      secretBytes `json:"passphrase,omitempty"`
	Password        secretBytes `json:"password,omitempty"`
	Fingerprint     string      `json:"host_key_fingerprint,omitempty"`
	InsecureHostKey bool        `json:"insecure_host_key,omitempty"`
	SocketPath      string      `json:"socket_path,omitempty"`
}

func (c *sshConnection) wipe() {
	if c == nil {
		return
	}
	for _, b := range []secretBytes{c.PrivateKey, c.Passphrase, c.Password} {
		for i := range b {
			b[i] = 0
		}
	}
}

func (c *sshConnection) target() (imagemove.Target, error) {
	if c == nil {
		return imagemove.Target{}, errors.New("ssh connection details are required for the docker-ssh source")
	}
	return imagemove.ParseTarget(strings.TrimSpace(c.User)+"@"+strings.TrimSpace(c.Host), c.Port)
}

func (c *sshConnection) config(policy platformimport.NetworkPolicy) (dockerssh.Config, error) {
	t, err := c.target()
	if err != nil {
		return dockerssh.Config{}, err
	}
	return dockerssh.Config{
		Host: t.Host, Port: t.Port, User: t.User,
		PrivateKey: c.PrivateKey, Passphrase: c.Passphrase, Password: c.Password,
		Fingerprint: strings.TrimSpace(c.Fingerprint), InsecureHostKey: c.InsecureHostKey,
		SocketPath: strings.TrimSpace(c.SocketPath), CheckAddr: policy.Check,
		DialTimeout: envDuration(envSSHTimeout, defaultSSHTimeout),
	}, nil
}

func (r *platformImportRequest) wipeSecrets() { r.SSH.wipe() }

// normalizeSSH validates a docker-ssh request and derives its source URL so
// the session is identified by the SSH target, never by a credential.
func (r *platformImportRequest) normalizeSSH() error {
	if r.Platform != string(platformimport.DockerSSH) {
		return nil
	}
	t, err := r.SSH.target()
	if err != nil {
		return err
	}
	r.URL = "ssh://" + t.Addr()
	return nil
}

type sshReader struct {
	cfg dockerssh.Config
}

func (s sshReader) InspectAll(ctx context.Context) ([]byte, error) {
	sess, err := dockerssh.Open(ctx, s.cfg)
	if err != nil {
		return nil, err
	}
	defer sess.Close()
	return sess.InspectAll(ctx)
}

// openDockerSSHSource builds the read-only source for a docker-ssh request.
func (rt *Router) openDockerSSHSource(w http.ResponseWriter, req platformImportRequest) (platformimport.Source, bool) {
	policy, err := platformImportPolicy(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return nil, false
	}
	cfg, err := req.SSH.config(policy)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return nil, false
	}
	if cfg.Fingerprint == "" && !cfg.InsecureHostKey {
		writeError(w, http.StatusBadRequest, "host_key_fingerprint is required: read the server's fingerprint first and confirm it (or set insecure_host_key)")
		return nil, false
	}
	if cfg.InsecureHostKey {
		rt.logger.Warn("api: docker-ssh import: host key verification disabled for this session", slog.String("host", cfg.Host))
	}
	return platformimport.DockerSSHSource{Reader: rt.appImportLive.inspectReaderFor(cfg)}, true
}

type sshProbeRequest struct {
	SSH           *sshConnection `json:"ssh"`
	AllowPrivate  bool           `json:"allow_private,omitempty"`
	AllowLoopback bool           `json:"allow_loopback,omitempty"`
}

type sshHostKey struct {
	Type        string `json:"type"`
	Fingerprint string `json:"fingerprint"`
}

type sshProbeResult struct {
	OK       bool                `json:"ok"`
	Target   string              `json:"target,omitempty"`
	Kind     string              `json:"kind,omitempty"`
	Message  string              `json:"message,omitempty"`
	Reads    string              `json:"reads,omitempty"`
	Overview *dockerssh.Overview `json:"overview,omitempty"`
	Keys     []sshHostKey        `json:"host_keys,omitempty"`
	Observed []sshHostKey        `json:"observed_host_keys,omitempty"`
	Warnings []string            `json:"warnings,omitempty"`
}

func (rt *Router) decodeSSHProbe(w http.ResponseWriter, r *http.Request) (sshProbeRequest, dockerssh.Config, bool) {
	var req sshProbeRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxPlatformImportBody)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return req, dockerssh.Config{}, false
	}
	policy, err := platformImportPolicy(platformImportRequest{AllowPrivate: req.AllowPrivate, AllowLoopback: req.AllowLoopback})
	if err != nil {
		req.SSH.wipe()
		writeError(w, http.StatusBadRequest, err.Error())
		return req, dockerssh.Config{}, false
	}
	cfg, err := req.SSH.config(policy)
	if err != nil {
		req.SSH.wipe()
		writeError(w, http.StatusBadRequest, err.Error())
		return req, dockerssh.Config{}, false
	}
	return req, cfg, true
}

func sshFailure(target string, err error) sshProbeResult {
	res := sshProbeResult{Target: target, Kind: dockerssh.KindOf(err), Message: err.Error()}
	var mm *sshprovision.HostKeyMismatchError
	if errors.As(err, &mm) {
		for _, k := range mm.Observed {
			res.Observed = append(res.Observed, sshHostKey{Type: k.Type, Fingerprint: k.Fingerprint})
		}
	}
	return res
}

// handleSSHHostKeys handles POST /api/v1/migration/apps/ssh/host-keys: it
// reads the host keys the server presents without logging in.
func (rt *Router) handleSSHHostKeys(w http.ResponseWriter, r *http.Request) {
	req, cfg, ok := rt.decodeSSHProbe(w, r)
	if !ok {
		return
	}
	defer req.SSH.wipe()
	ctx, cancel := context.WithTimeout(r.Context(), cfg.DialTimeout*4)
	defer cancel()
	target := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	keys, err := dockerssh.ScanHostKeys(ctx, cfg)
	if err != nil {
		writeJSON(w, http.StatusOK, sshFailure(target, err))
		return
	}
	res := sshProbeResult{OK: true, Target: target}
	for _, k := range keys {
		res.Keys = append(res.Keys, sshHostKey{Type: k.Type, Fingerprint: k.Fingerprint})
	}
	writeJSON(w, http.StatusOK, res)
}

// handleSSHTest handles POST /api/v1/migration/apps/ssh/test: it logs in,
// opens the Docker socket and counts what an import could read.
func (rt *Router) handleSSHTest(w http.ResponseWriter, r *http.Request) {
	req, cfg, ok := rt.decodeSSHProbe(w, r)
	if !ok {
		return
	}
	defer req.SSH.wipe()
	target := fmt.Sprintf("%s@%s:%d", cfg.User, cfg.Host, cfg.Port)
	ctx, cancel := context.WithTimeout(r.Context(), cfg.DialTimeout*4)
	defer cancel()
	if cfg.Fingerprint == "" && !cfg.InsecureHostKey {
		writeJSON(w, http.StatusOK, sshProbeResult{Target: target, Kind: dockerssh.KindHostKeyRequired, Message: "confirm the server's host key fingerprint first"})
		return
	}
	sess, err := dockerssh.Open(ctx, cfg)
	if err != nil {
		rt.logger.Info("api: docker-ssh test failed", slog.String("target", target), slog.String("kind", dockerssh.KindOf(err)))
		writeJSON(w, http.StatusOK, sshFailure(target, err))
		return
	}
	defer sess.Close()
	ov, err := sess.Probe(ctx)
	if err != nil {
		writeJSON(w, http.StatusOK, sshFailure(target, err))
		return
	}
	res := sshProbeResult{OK: true, Target: target, Reads: sshReadOnlyOverview, Overview: &ov, Warnings: sess.Warnings()}
	if cfg.InsecureHostKey {
		res.Warnings = append(res.Warnings, sshInsecureWarning)
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *appImportState) inspectReaderFor(cfg dockerssh.Config) platformimport.InspectReader {
	s.mu.Lock()
	f := s.newInspectReader
	s.mu.Unlock()
	if f != nil {
		return f(cfg)
	}
	return sshReader{cfg: cfg}
}

// tunnelSaverFactory builds the image source for a docker-ssh session.
type tunnelSaverFactory func(cfg dockerssh.Config) imagemove.Saver

func defaultTunnelSaver(cfg dockerssh.Config) imagemove.Saver { return dockerssh.Saver{Cfg: cfg} }
