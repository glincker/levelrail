package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const (
	envImportSSHPassword = "APP_IMPORT_SSH_PASSWORD" //nolint:gosec // env var name, not a credential
	platformDockerSSH    = "docker-ssh"
	volumeStateCopied    = "copied"
	volumeStateSkipped   = "skipped"
)

type importSSHFlags struct {
	fingerprint, socket       string
	insecureHostKey, testOnly bool
	copyVolumes               bool
}

func splitSSHTarget(target string, port int) (user, host string, outPort int, err error) {
	user, rest, ok := strings.Cut(strings.TrimSpace(target), "@")
	if !ok || user == "" || rest == "" {
		return "", "", 0, errors.New("--ssh must look like user@host or user@host:port")
	}
	host, outPort = rest, port
	if strings.HasPrefix(rest, "[") || strings.Count(rest, ":") == 1 {
		h, p, serr := net.SplitHostPort(rest)
		if serr != nil {
			return "", "", 0, fmt.Errorf("--ssh host: %w", serr)
		}
		n, perr := strconv.Atoi(p)
		if perr != nil {
			return "", "", 0, errors.New("--ssh port must be a number")
		}
		host, outPort = h, n
	}
	return user, host, outPort, nil
}

func buildImportSSH(prog string, f importAppsFlags, stdin io.Reader, stderr io.Writer, lookupEnv func(string) (string, bool)) (apiclient.AppImportSSH, bool) {
	var out apiclient.AppImportSSH
	if f.images.ssh == "" {
		_, _ = fmt.Fprintf(stderr, "%s: --ssh user@host is required with --from %s\n", prog, platformDockerSSH)
		return out, false
	}
	user, host, port, err := splitSSHTarget(f.images.ssh, f.images.port)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", prog, err)
		return out, false
	}
	key, err := readSSHKey(f.images.key, stdin)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", prog, err)
		return out, false
	}
	password, _ := lookupEnv(envImportSSHPassword)
	if key == "" && password == "" {
		_, _ = fmt.Fprintf(stderr, "%s: supply --ssh-key FILE (or - for stdin), or set %s\n", prog, envImportSSHPassword)
		return out, false
	}
	passphrase, _ := lookupEnv(envImportSSHPassphrase)
	return apiclient.AppImportSSH{Host: host, Port: port, User: user, PrivateKey: key, Passphrase: passphrase, Password: password,
		Fingerprint: strings.TrimSpace(f.ssh.fingerprint), InsecureHostKey: f.ssh.insecureHostKey, SocketPath: f.ssh.socket}, true
}

// confirmHostKey makes the operator pin the old host's fingerprint: with none
// given it prints what the server presents and stops.
func confirmHostKey(ctx context.Context, prog string, client *Client, f importAppsFlags, conn apiclient.AppImportSSH, stdout, stderr io.Writer) (int, bool) {
	if conn.Fingerprint != "" || conn.InsecureHostKey {
		if conn.InsecureHostKey {
			_, _ = fmt.Fprintln(stderr, "warning: host key verification is off for this session, anything between here and the old host could read the traffic")
		}
		return exitOK, true
	}
	res, err := client.SSHHostKeys(ctx, apiclient.AppImportSSHProbe{SSH: apiclient.AppImportSSH{Host: conn.Host, Port: conn.Port, User: conn.User},
		AllowPrivate: f.allowPrivate, AllowLoopback: f.allowLoopback})
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("read host key: %w", err)), false
	}
	if !res.OK {
		_, _ = fmt.Fprintf(stderr, "%s: could not read the host key of %s (%s): %s\n", prog, res.Target, res.Kind, res.Message)
		return exitNetwork, false
	}
	_, _ = fmt.Fprintf(stderr, "%s: the old host presents these host keys. Compare one with what the host itself prints, then re-run with --fingerprint:\n", prog)
	for _, k := range res.Keys {
		_, _ = fmt.Fprintf(stderr, "  %s  %s\n", k.Type, k.Fingerprint)
	}
	_, _ = fmt.Fprintln(stderr, "  on the old host: ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub")
	return exitUsage, false
}

func runImportAppsSSHTest(ctx context.Context, prog string, client *Client, f importAppsFlags, conn apiclient.AppImportSSH, stdout, stderr io.Writer) int {
	res, err := client.TestAppImportSSH(ctx, apiclient.AppImportSSHProbe{SSH: conn, AllowPrivate: f.allowPrivate, AllowLoopback: f.allowLoopback})
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("test ssh connection: %w", err))
	}
	if f.jsonOut {
		if err := writeJSONValue(stdout, res); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
	} else if res.OK && res.Overview != nil {
		o := res.Overview
		_, _ = fmt.Fprintf(stdout, "Connected to %s (%s).\nDocker %s on %s: %d containers (%d running), %d volumes, %d images.\n",
			res.Target, res.Reads, o.DockerVersion, o.OS, o.Containers, o.Running, o.Volumes, o.Images)
		for _, w := range res.Warnings {
			_, _ = fmt.Fprintf(stdout, "warning: %s\n", w)
		}
	} else {
		_, _ = fmt.Fprintf(stderr, "%s: connection failed (%s): %s\n", prog, res.Kind, res.Message)
		if hint := sshFailureHint(res.Kind); hint != "" {
			_, _ = fmt.Fprintf(stderr, "fix: %s\n", hint)
		}
	}
	if !res.OK {
		return exitCheckFailed
	}
	return exitOK
}

func sshFailureHint(kind string) string {
	switch kind {
	case "auth_failed":
		return "check the key file, its passphrase (" + envImportSSHPassphrase + ") and the user name"
	case "host_key_mismatch":
		return "the host key changed or the fingerprint is wrong: confirm it again before continuing"
	case "socket_permission":
		return "add the ssh user to the docker group on the old host, then log in again"
	case "socket_path":
		return "pass the right socket with --ssh-socket (rootless Docker uses /run/user/UID/docker.sock)"
	case "socket_forbidden":
		return "sshd on the old host blocks socket forwarding: allow AllowStreamLocalForwarding for this login"
	case "blocked_address":
		return "private addresses need --allow-private and APP_IMPORT_ALLOW_PRIVATE_NETWORKS=true on the control plane"
	}
	return ""
}

func runImportAppsCopyVolumes(ctx context.Context, prog string, client *Client, f importAppsFlags, stdout, stderr io.Writer) int {
	v, err := client.StartAppImportVolumeCopy(ctx, f.session, f.onlyList())
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("start volume copy: %w", err))
	}
	deadline := time.Now().Add(f.wait)
	for v.Running && time.Now().Before(deadline) {
		time.Sleep(appImportPollEvery)
		if v, err = client.AppImportVolumeCopyStatus(ctx, f.session); err != nil {
			return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("poll volume copy: %w", err))
		}
	}
	if f.jsonOut {
		if err := writeJSONValue(stdout, v); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
	} else {
		for _, r := range v.Results {
			_, _ = fmt.Fprintf(stdout, "%s %s -> %s: %s", r.App, r.ContainerPath, r.Volume, r.State)
			if r.State == volumeStateCopied {
				_, _ = fmt.Fprintf(stdout, " (%d files, %s)", r.Files, humanBytes(r.Bytes))
			}
			_, _ = fmt.Fprintln(stdout)
			for _, msg := range []string{r.Error, r.Fallback} {
				if msg != "" {
					_, _ = fmt.Fprintf(stdout, "  %s\n", msg)
				}
			}
			for _, w := range r.Warnings {
				_, _ = fmt.Fprintf(stdout, "  left out: %s\n", w)
			}
		}
	}
	if v.Running {
		return exitCheckFailed
	}
	for _, r := range v.Results {
		if r.State != volumeStateCopied && r.State != volumeStateSkipped {
			return exitCheckFailed
		}
	}
	if !f.jsonOut {
		_, _ = fmt.Fprintf(stdout, "\nRun it again right before the DNS switch to catch the last changes. Next: %s import apps --session %s --verify\n", prog, f.session)
	}
	return exitOK
}
