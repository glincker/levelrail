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
	n := agentNames{name: p.AgentName, display: p.DisplayName}
	if _, err := p.run(ctx, client, n.dataDirCmd(), onLine); err != nil {
		return fmt.Errorf("prepare agent data directory: %w", err)
	}

	if err := p.writeRemoteFile(ctx, client, n.envPath(), renderAgentEnvFile(n, params), "600"); err != nil {
		return fmt.Errorf("write agent environment file: %w", err)
	}
	onLine("wrote agent environment file")

	agentImage := params.AgentImage
	if agentImage == "" {
		agentImage = n.defaultImage()
	}
	unit := renderAgentUnit(n, agentImage, meshRunFlags(params.MeshEnabled))
	if err := p.writeRemoteFile(ctx, client, n.unitPath(), unit, "644"); err != nil {
		return fmt.Errorf("write agent systemd unit: %w", err)
	}
	onLine("wrote agent systemd unit")

	if _, err := p.run(ctx, client, "systemctl daemon-reload && systemctl enable --now "+n.name, onLine); err != nil {
		return fmt.Errorf("start the agent service: %w", err)
	}

	active, actErr := p.run(ctx, client, "sleep 2 && systemctl is-active "+n.name, nil)
	if actErr != nil || strings.TrimSpace(active) != "active" {
		diag, _ := p.run(ctx, client, "journalctl -u "+n.name+" --no-pager -n 50", nil)
		return fmt.Errorf("the agent service did not stay active after starting: %s", lastLines(diag, 20))
	}
	return nil
}

// meshRunFlags are the extra docker run flags the agent needs for WireGuard.
func meshRunFlags(enabled bool) string {
	if !enabled {
		return ""
	}
	return " -e APP_MESH_ENABLED --cap-add NET_ADMIN --device /dev/net/tun"
}

func renderAgentEnvFile(n agentNames, p InstallParams) string {
	var b strings.Builder
	fmt.Fprintf(&b, "APP_CONTROL_PLANE_ADDR=%s\n", p.ControlPlaneAddr)
	fmt.Fprintf(&b, "APP_JOIN_TOKEN=%s\n", p.JoinToken)
	fmt.Fprintf(&b, "APP_NODE_NAME=%s\n", p.NodeName)
	if p.CAFingerprint != "" {
		fmt.Fprintf(&b, "APP_CA_FINGERPRINT=%s\n", p.CAFingerprint)
	}
	fmt.Fprintf(&b, "APP_AGENT_IDENTITY_FILE=%s/identity.json\n", n.dataDir())
	if p.MeshEnabled {
		b.WriteString("APP_MESH_ENABLED=1\n")
	}
	return b.String()
}

// agentNames derives every remote unit, path and container name from one brand-supplied stem.
type agentNames struct{ name, display string }

func (n agentNames) dataDir() string { return "/var/lib/" + n.name + "-data" }
func (n agentNames) envPath() string { return "/etc/" + n.name + ".env" }
func (n agentNames) unitPath() string {
	return "/etc/systemd/system/" + n.name + ".service"
}

// defaultImage is the fallback for a caller that leaves InstallParams.AgentImage empty.
func (n agentNames) defaultImage() string { return "ghcr.io/glincker/" + n.name + ":edge" }

// dataDirCmd creates the identity directory root-owned: the agent container runs as root.
func (n agentNames) dataDirCmd() string {
	d := n.dataDir()
	return "mkdir -p " + d + " && chown root:root " + d + " && chmod 700 " + d
}

func renderAgentUnit(n agentNames, image, meshFlags string) string {
	return fmt.Sprintf(agentUnitTemplate, image, meshFlags, n.name, n.envPath(), n.dataDir(), n.display)
}

const agentUnitTemplate = `[Unit]
Description=%[6]s node agent
After=network-online.target docker.service
Requires=docker.service
Wants=network-online.target

[Service]
EnvironmentFile=%[4]s
ExecStartPre=-/usr/bin/docker rm -f %[3]s
ExecStartPre=/usr/bin/docker pull %[1]s
ExecStart=/usr/bin/docker run --rm --name %[3]s --user 0:0 --network host -v /var/run/docker.sock:/var/run/docker.sock -v %[5]s:%[5]s -e APP_CONTROL_PLANE_ADDR -e APP_JOIN_TOKEN -e APP_CA_FINGERPRINT -e APP_NODE_NAME -e APP_AGENT_IDENTITY_FILE%[2]s %[1]s
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
`
