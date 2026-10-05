// Package sshprovision adopts a machine the operator already has (any
// VPS, home server, Raspberry Pi) as a node by dialing into it over SSH
// and installing the node agent there. This is a different feature from
// internal/provision (which creates a brand-new VM at a cloud provider's
// API and drives it through cloud-init on first boot): this package never
// imports that one, and duplicates the small agent systemd-unit/env-file
// shell snippet CloudInitParams.agentImage/RenderCloudInit render for a
// fresh VM, adapted to run as direct SSH commands against a machine that
// already exists instead of a cloud-init document a fresh VM processes on
// first boot.
package sshprovision

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"

	"golang.org/x/crypto/ssh"
)

// AuthType selects how Credentials authenticates to the remote host.
type AuthType string

// The two credential shapes Provision accepts, matching this feature's
// "SSH or VPS information" ask: a private key, or a password.
const (
	AuthKey      AuthType = "key"
	AuthPassword AuthType = "password"
)

// Credentials dials and authenticates to one remote host for the
// duration of a single Provision call. The caller must discard it once
// Provision returns: nothing in this package persists it anywhere.
type Credentials struct {
	Host     string
	Port     int // 0 defaults to 22
	Username string
	Auth     AuthType
	// PrivateKey is a PEM-encoded private key, required when Auth is AuthKey.
	PrivateKey string
	// Passphrase decrypts PrivateKey when it's encrypted. Optional.
	Passphrase string
	// Password authenticates when Auth is AuthPassword.
	Password string
}

func (c Credentials) addr() string {
	port := c.Port
	if port == 0 {
		port = 22
	}
	return net.JoinHostPort(c.Host, strconv.Itoa(port))
}

func (c Credentials) authMethod() (ssh.AuthMethod, error) {
	switch c.Auth {
	case AuthKey:
		if c.PrivateKey == "" {
			return nil, errors.New("key auth requires a private key")
		}
		var signer ssh.Signer
		var err error
		if c.Passphrase != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase([]byte(c.PrivateKey), []byte(c.Passphrase))
		} else {
			signer, err = ssh.ParsePrivateKey([]byte(c.PrivateKey))
		}
		if err != nil {
			return nil, fmt.Errorf("parse private key: %w", err)
		}
		return ssh.PublicKeys(signer), nil
	case AuthPassword:
		if c.Password == "" {
			return nil, errors.New("password auth requires a password")
		}
		return ssh.Password(c.Password), nil
	default:
		return nil, fmt.Errorf("unknown auth type %q, want %q or %q", c.Auth, AuthKey, AuthPassword)
	}
}

// InstallParams is what the remote host needs to enroll as a node agent:
// the same information provision.CloudInitParams carries for a freshly
// created VM, minus the fields that only make sense for a cloud-init
// document (this package's own doc comment explains why the two aren't
// shared).
type InstallParams struct {
	ControlPlaneAddr string
	JoinToken        string
	CAFingerprint    string
	NodeName         string
	// AgentImage overrides defaultAgentImage (repo:tag). Empty uses the
	// default, the caller is expected to resolve a real release tag the
	// same way provision.CloudInitParams.agentImage does (internal/api
	// wires this from internal/version.Version).
	AgentImage string
	// MeshEnabled gives the agent container the TUN device and NET_ADMIN
	// its WireGuard device needs; off keeps the original unprivileged run.
	MeshEnabled bool
}

// DetectedHost is what Provision learns about the remote machine before
// it changes anything: shown to the operator so this doesn't feel like a
// black box.
type DetectedHost struct {
	// OS is the lowercased `uname -s` output, e.g. "linux". Provision
	// fails before installing anything if this isn't "linux".
	OS string
	// Arch is "amd64" or "arm64", normalized from `uname -m`. Empty means
	// an unsupported architecture, and Provision fails before installing
	// anything.
	Arch string
	// Distro is /etc/os-release's PRETTY_NAME, best effort. "unknown" if
	// the file is missing or unreadable.
	Distro         string
	DockerPresent  bool
	SystemdPresent bool
}

// Step is one stage of Provision, reported through onEvent as it starts,
// streams command output, and finishes.
type Step string

// The stages Provision reports through onEvent, in order. Not every
// install reaches StepDocker's own install sub-events: DockerPresent
// already true skips straight past them.
const (
	StepConnect Step = "connect"
	StepDetect  Step = "detect"
	StepPrereqs Step = "prereqs"
	StepDocker  Step = "docker"
	StepAgent   Step = "agent"
	StepDone    Step = "done"
)

// Event is one line of progress Provision reports through onEvent.
// Message is always safe to log and show to an operator: Provision never
// puts a credential or the join token in a Message.
type Event struct {
	Step    Step
	Message string
	// Err is set on the terminal event for a failed step; Provision
	// returns the same error.
	Err error
}

// defaultDialTimeout bounds the initial TCP connect and SSH handshake,
// separately from ctx: a slow but reachable host still gets ctx's own
// (longer) budget for the install itself.
const defaultDialTimeout = 15 * time.Second

// Provisioner adopts an existing machine over SSH: connects, detects its
// OS/arch, ensures Docker and systemd, then installs and starts the node
// agent as a systemd-managed container.
type Provisioner struct {
	// DialTimeout bounds the initial TCP+SSH handshake. Zero uses
	// defaultDialTimeout.
	DialTimeout time.Duration
}

// New returns a Provisioner with default settings.
func New() *Provisioner { return &Provisioner{} }

// Provision connects to creds, detects the remote host, and installs the
// node agent there so it enrolls with params.ControlPlaneAddr using
// params.JoinToken. It reports progress through onEvent (which may be
// nil) and returns what it detected about the host even on failure,
// where that's known before the failure occurred.
//
// Provision returns once the agent's systemd unit reports active; it
// does not wait for the agent to actually dial the control plane and
// enroll; that is the caller's job to poll for separately (the same
// "server is up" vs. "node has enrolled" split internal/provision's own
// node-provision flow already draws between the provider API and the
// node registry).
func (p *Provisioner) Provision(ctx context.Context, creds Credentials, params InstallParams, onEvent func(Event)) (DetectedHost, error) {
	emit := func(step Step, msg string) {
		if onEvent != nil {
			onEvent(Event{Step: step, Message: msg})
		}
	}
	fail := func(step Step, err error) (DetectedHost, error) {
		if onEvent != nil {
			onEvent(Event{Step: step, Message: err.Error(), Err: err})
		}
		return DetectedHost{}, err
	}

	if params.ControlPlaneAddr == "" || params.JoinToken == "" || params.NodeName == "" {
		return fail(StepConnect, errors.New("sshprovision: control plane address, join token and node name are required"))
	}

	emit(StepConnect, fmt.Sprintf("connecting to %s@%s", creds.Username, creds.addr()))
	client, err := p.dial(ctx, creds)
	if err != nil {
		return fail(StepConnect, err)
	}
	defer func() { _ = client.Close() }()
	emit(StepConnect, "connected")

	emit(StepDetect, "detecting operating system")
	host, err := p.detect(ctx, client)
	if err != nil {
		return fail(StepDetect, fmt.Errorf("detect the remote host: %w", err))
	}
	emit(StepDetect, fmt.Sprintf("detected %s (kernel: %s, arch: %s)", host.Distro, host.OS, ifEmpty(host.Arch, "unknown")))
	if host.OS != "linux" {
		return fail(StepDetect, fmt.Errorf("sshprovision: unsupported OS %q, only Linux is supported", host.OS))
	}
	if host.Arch == "" {
		return fail(StepDetect, errors.New("sshprovision: unsupported CPU architecture, only amd64 (x86_64) and arm64 (aarch64) are supported"))
	}

	emit(StepPrereqs, "checking prerequisites")
	if !host.SystemdPresent {
		return host, fail0(fail, StepPrereqs, errors.New("sshprovision: this host has no systemd; only systemd-based Linux distributions are supported"))
	}

	if host.DockerPresent {
		emit(StepDocker, "Docker already present")
	} else {
		emit(StepDocker, "Docker not found, installing via get.docker.com")
		if _, err := p.run(ctx, client, "curl -fsSL https://get.docker.com | sh", func(line string) { emit(StepDocker, line) }); err != nil {
			return host, fail0(fail, StepDocker, fmt.Errorf("install Docker: %w", err))
		}
		if _, err := p.run(ctx, client, "systemctl enable --now docker", func(line string) { emit(StepDocker, line) }); err != nil {
			return host, fail0(fail, StepDocker, fmt.Errorf("start Docker: %w", err))
		}
		emit(StepDocker, "Docker installed and running")
	}

	emit(StepAgent, "installing the node agent")
	if err := p.installAgent(ctx, client, params, func(line string) { emit(StepAgent, line) }); err != nil {
		return host, fail0(fail, StepAgent, err)
	}

	emit(StepDone, "agent service is active, waiting for it to enroll")
	return host, nil
}

// fail0 reports err through fail (for its side effect: emitting the
// terminal event) and returns err, letting a caller that already has a
// DetectedHost worth keeping return it alongside the error instead of
// fail's own zero-value DetectedHost.
func fail0(fail func(Step, error) (DetectedHost, error), step Step, err error) error {
	_, _ = fail(step, err)
	return err
}

func ifEmpty(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

func (p *Provisioner) dial(ctx context.Context, creds Credentials) (*ssh.Client, error) {
	auth, err := creds.authMethod()
	if err != nil {
		return nil, fmt.Errorf("sshprovision: build auth method: %w", err)
	}
	timeout := p.DialTimeout
	if timeout <= 0 {
		timeout = defaultDialTimeout
	}
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", creds.addr())
	if err != nil {
		return nil, fmt.Errorf("sshprovision: dial %s: %w", creds.addr(), err)
	}
	config := &ssh.ClientConfig{
		User: creds.Username,
		Auth: []ssh.AuthMethod{auth},
		// No known_hosts store exists for an operator's own arbitrary
		// machine (no prior trust-on-first-use pinning to check against):
		// a documented, known limitation, see docs/node-provisioning.md's
		// SSH section, not an oversight.
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec // documented known limitation above, no host key store exists yet for an arbitrary operator machine
		Timeout:         timeout,
	}
	sshConn, chans, reqs, err := ssh.NewClientConn(conn, creds.addr(), config)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("sshprovision: authenticate to %s: %w", creds.addr(), err)
	}
	return ssh.NewClient(sshConn, chans, reqs), nil
}
