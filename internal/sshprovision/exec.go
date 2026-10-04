package sshprovision

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
)

// run executes cmd in a fresh session (an ssh.Session runs exactly one
// command, x/crypto/ssh's own constraint), streaming each combined
// stdout/stderr line to onLine as it arrives, and returns the full
// combined output. ctx cancellation kills the session so a stuck remote
// command can't hang Provision forever.
func (p *Provisioner) run(ctx context.Context, client *ssh.Client, cmd string, onLine func(string)) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("open session: %w", err)
	}
	defer func() { _ = session.Close() }()

	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = session.Close()
		case <-done:
		}
	}()

	return runStreamed(session, cmd, onLine)
}

// runStreamed starts cmd on session, forwarding each stdout/stderr line
// to onLine (which may be nil) as it arrives, and returns the full
// combined output. Both streams are scanned concurrently since a
// misbehaving command that only ever writes to one of them must not
// block behind an idle read of the other.
func runStreamed(session *ssh.Session, cmd string, onLine func(string)) (string, error) {
	stdout, err := session.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("open stdout: %w", err)
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("open stderr: %w", err)
	}

	var mu sync.Mutex
	var out strings.Builder
	var wg sync.WaitGroup
	scan := func(r io.Reader) {
		defer wg.Done()
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		for sc.Scan() {
			line := sc.Text()
			mu.Lock()
			out.WriteString(line)
			out.WriteByte('\n')
			mu.Unlock()
			if onLine != nil {
				onLine(line)
			}
		}
	}
	wg.Add(2)
	go scan(stdout)
	go scan(stderr)

	if err := session.Start(cmd); err != nil {
		return "", fmt.Errorf("start command: %w", err)
	}
	wg.Wait()
	waitErr := session.Wait()
	mu.Lock()
	full := out.String()
	mu.Unlock()
	if waitErr != nil {
		return full, fmt.Errorf("%w (output: %s)", waitErr, lastLines(full, 10))
	}
	return full, nil
}

// writeRemoteFile writes content to path on the remote host with mode,
// over the session's stdin rather than as a shell-embedded literal: a
// join token in content never appears as a command-line argument or in
// any log this package writes, only inside the session's own stdin
// stream. `install`'s single-argument copy (source /dev/stdin) sets the
// permission bits atomically at creation, so root-only content is never
// briefly world-readable between a create and a later chmod.
func (p *Provisioner) writeRemoteFile(ctx context.Context, client *ssh.Client, path, content, mode string) error {
	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("open session: %w", err)
	}
	defer func() { _ = session.Close() }()

	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = session.Close()
		case <-done:
		}
	}()

	stdin, err := session.StdinPipe()
	if err != nil {
		return fmt.Errorf("open stdin: %w", err)
	}
	cmd := fmt.Sprintf("install -m %s /dev/stdin %s", mode, path)
	if err := session.Start(cmd); err != nil {
		return fmt.Errorf("start write of %s: %w", path, err)
	}
	if _, err := io.WriteString(stdin, content); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := stdin.Close(); err != nil {
		return fmt.Errorf("close stdin for %s: %w", path, err)
	}
	if err := session.Wait(); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// detect gathers DetectedHost by running a handful of read-only probes:
// nothing here changes the remote host.
func (p *Provisioner) detect(ctx context.Context, client *ssh.Client) (DetectedHost, error) {
	var host DetectedHost

	kernel, err := p.run(ctx, client, "uname -s", nil)
	if err != nil {
		return host, fmt.Errorf("run uname -s: %w", err)
	}
	host.OS = strings.ToLower(strings.TrimSpace(kernel))

	machine, err := p.run(ctx, client, "uname -m", nil)
	if err != nil {
		return host, fmt.Errorf("run uname -m: %w", err)
	}
	host.Arch = normalizeArch(strings.TrimSpace(machine))

	osRelease, _ := p.run(ctx, client, "cat /etc/os-release 2>/dev/null || true", nil)
	host.Distro = parsePrettyName(osRelease)

	if _, err := p.run(ctx, client, "command -v systemctl >/dev/null 2>&1", nil); err == nil {
		host.SystemdPresent = true
	}
	if _, err := p.run(ctx, client, "command -v docker >/dev/null 2>&1", nil); err == nil {
		host.DockerPresent = true
	}
	return host, nil
}

// normalizeArch mirrors install.sh's own detect_arch: the two CPU
// architectures this platform publishes agent images for. Empty means
// unsupported.
func normalizeArch(raw string) string {
	switch raw {
	case "x86_64", "amd64":
		return "amd64"
	case "aarch64", "arm64":
		return "arm64"
	default:
		return ""
	}
}

// parsePrettyName reads /etc/os-release's PRETTY_NAME line, best effort.
// "unknown" if the file was missing, empty, or has no such line.
func parsePrettyName(osRelease string) string {
	for _, line := range strings.Split(osRelease, "\n") {
		if v, ok := strings.CutPrefix(line, "PRETTY_NAME="); ok {
			return strings.Trim(strings.TrimSpace(v), `"`)
		}
	}
	return "unknown"
}

// lastLines returns at most n trailing non-empty lines of s, for a
// compact diagnostic tail rather than dumping a whole install log into
// one error message.
func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// installAgent runs the same steps provision.RenderCloudInit's runcmd
// applies on a fresh VM (identity directory, env file, systemd unit,
// enable+start), directly over SSH instead of through a cloud-init
// document, then confirms the service actually came up so a failure
// (bad image pull, a resource limit) is reported here rather than
// silently left for the enrollment poll to eventually time out on.
func (p *Provisioner) installAgent(ctx context.Context, client *ssh.Client, params InstallParams, onLine func(string)) error {
	if _, err := p.run(ctx, client, "mkdir -p /var/lib/levelrail-agent-data && chown 65532:65532 /var/lib/levelrail-agent-data", onLine); err != nil {
		return fmt.Errorf("prepare agent data directory: %w", err)
	}

	if err := p.writeRemoteFile(ctx, client, agentEnvPath, renderAgentEnvFile(params), "600"); err != nil {
		return fmt.Errorf("write agent environment file: %w", err)
	}
	onLine("wrote agent environment file")

	agentImage := params.AgentImage
	if agentImage == "" {
		agentImage = defaultAgentImage
	}
	unit := fmt.Sprintf(agentUnitTemplate, agentImage)
	if err := p.writeRemoteFile(ctx, client, agentUnitPath, unit, "644"); err != nil {
		return fmt.Errorf("write agent systemd unit: %w", err)
	}
	onLine("wrote agent systemd unit")

	if _, err := p.run(ctx, client, "systemctl daemon-reload && systemctl enable --now levelrail-agent", onLine); err != nil {
		return fmt.Errorf("start the agent service: %w", err)
	}

	active, actErr := p.run(ctx, client, "sleep 2 && systemctl is-active levelrail-agent", nil)
	if actErr != nil || strings.TrimSpace(active) != "active" {
		diag, _ := p.run(ctx, client, "journalctl -u levelrail-agent --no-pager -n 50", nil)
		return fmt.Errorf("the agent service did not stay active after starting: %s", lastLines(diag, 20))
	}
	return nil
}

func renderAgentEnvFile(p InstallParams) string {
	var b strings.Builder
	fmt.Fprintf(&b, "APP_CONTROL_PLANE_ADDR=%s\n", p.ControlPlaneAddr)
	fmt.Fprintf(&b, "APP_JOIN_TOKEN=%s\n", p.JoinToken)
	fmt.Fprintf(&b, "APP_NODE_NAME=%s\n", p.NodeName)
	if p.CAFingerprint != "" {
		fmt.Fprintf(&b, "APP_CA_FINGERPRINT=%s\n", p.CAFingerprint)
	}
	b.WriteString("APP_AGENT_IDENTITY_FILE=/var/lib/levelrail-agent-data/identity.json\n")
	return b.String()
}

const (
	agentEnvPath  = "/etc/levelrail-agent.env"
	agentUnitPath = "/etc/systemd/system/levelrail-agent.service"
	// defaultAgentImage mirrors provision.defaultAgentImageRepo (internal/
	// provision/cloudinit.go) at the "edge" tag (internal/provision's own
	// fallback for a non-tagged-release build): that package is cloud-VM
	// creation, a different feature this package must not import from
	// (see this package's doc comment), so the repo is duplicated here
	// rather than shared. internal/api resolves a real release tag and
	// passes it through InstallParams.AgentImage in production; this
	// constant is only the fallback for a caller that leaves it empty.
	defaultAgentImage = "ghcr.io/glincker/levelrail-agent:edge"
)

const agentUnitTemplate = `[Unit]
Description=Levelrail node agent
After=network-online.target docker.service
Requires=docker.service
Wants=network-online.target

[Service]
EnvironmentFile=/etc/levelrail-agent.env
ExecStartPre=-/usr/bin/docker rm -f levelrail-agent
ExecStartPre=/usr/bin/docker pull %[1]s
ExecStart=/usr/bin/docker run --rm --name levelrail-agent --network host -v /var/run/docker.sock:/var/run/docker.sock -v /var/lib/levelrail-agent-data:/var/lib/levelrail-agent-data -e APP_CONTROL_PLANE_ADDR -e APP_JOIN_TOKEN -e APP_CA_FINGERPRINT -e APP_NODE_NAME -e APP_AGENT_IDENTITY_FILE %[1]s
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
`
