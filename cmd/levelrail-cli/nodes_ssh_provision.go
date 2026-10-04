package main

import (
	"context"
	"fmt"
	"io"
	"os"
)

// runNodesSSHProvision implements "nodes ssh-provision": POST
// /api/v1/nodes/ssh-provision. Adopts a machine the operator already has
// (any VPS, home server, Raspberry Pi) by dialing it over SSH and
// installing the node agent there; poll "nodes ssh-provisions show <id>"
// for progress, the same shape "nodes provision" establishes for the
// cloud-VM path.
func runNodesSSHProvision(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "nodes ssh-provision", "print the new provision as JSON to stdout and nothing else", stderr)
	var host, username, keyFile, passphrase, name, role, controlPlaneAddr string
	var port int
	var usePassword bool
	fs.StringVar(&host, "host", "", "the remote machine's address (required)")
	fs.IntVar(&port, "port", 22, "the remote machine's SSH port")
	fs.StringVar(&username, "user", "", "the SSH username to authenticate as (required)")
	fs.StringVar(&keyFile, "key-file", "", "path to a private key file for key auth (mutually exclusive with --password)")
	fs.StringVar(&passphrase, "passphrase", "", "passphrase for --key-file, if it's encrypted; prompted interactively if omitted and the key needs one is not detected client-side, pass it explicitly only when scripting")
	fs.BoolVar(&usePassword, "password", false, "authenticate with a password instead of a key: prompted with no echo (or read from a non-terminal stdin), never taken as a flag value so it never lands in shell history or the process list")
	fs.StringVar(&name, "name", "", "name for the new node: lowercase letters, digits, hyphens (required)")
	fs.StringVar(&role, "role", "general", "general or build")
	fs.StringVar(&controlPlaneAddr, "control-plane-addr", "", "host:port the adopted machine dials to reach this control plane's agent listener (default: this command's --api-url host, port 9443)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, nodesSSHProvisionUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if host == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--host is required"))
	}
	if username == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--user is required"))
	}
	if name == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--name is required"))
	}
	if keyFile != "" && usePassword {
		return reportError(stdout, stderr, jsonOut, newValidationError("--key-file and --password are mutually exclusive"))
	}
	if keyFile == "" && !usePassword {
		return reportError(stdout, stderr, jsonOut, newValidationError("one of --key-file or --password is required"))
	}
	if controlPlaneAddr == "" {
		controlPlaneAddr = defaultControlPlaneAddr(apiURLFlag, lookupEnv, prog, profileFlag)
	}
	if controlPlaneAddr == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--control-plane-addr could not be derived from --api-url, pass it explicitly"))
	}

	auth := createSSHNodeProvisionAuthRequest{}
	if usePassword {
		password, err := readSSHPassword(stderr)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, err)
		}
		auth.Type, auth.Password = "password", password
	} else {
		key, err := os.ReadFile(keyFile) //nolint:gosec // operator-supplied local path, same trust boundary as every other --file flag in this CLI
		if err != nil {
			return reportError(stdout, stderr, jsonOut, newValidationError("read --key-file %q: %v", keyFile, err))
		}
		auth.Type, auth.PrivateKey, auth.Passphrase = "key", string(key), passphrase
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	provisioned, err := client.CreateSSHNodeProvision(context.Background(), createSSHNodeProvisionRequest{
		Host: host, Port: port, Username: username, Auth: auth,
		Name: name, Role: role, ControlPlaneAddr: controlPlaneAddr,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("create ssh node provision: %w", err))
	}

	if err := renderResult(stdout, of.Format, of.Query, provisioned, func() {
		_, _ = fmt.Fprintf(stdout, "provision %s created, status: %s\n", provisioned.ID, provisioned.Status)
		_, _ = fmt.Fprintf(stdout, "poll with: %s nodes ssh-provisions show %s\n", prog, provisioned.ID)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// readSSHPassword resolves --password's actual value: a piped,
// non-terminal stdin is read directly (one line, trimmed, the same
// "docker login --password-stdin" shape defaultReadNodeProviderToken
// already establishes), a real terminal gets a no-echo prompt instead so
// the password is never displayed either way, and never taken as the
// flag's own value so it can't land in shell history or the process
// list.
var readSSHPassword = defaultReadSSHPassword

func defaultReadSSHPassword(stderr io.Writer) (string, error) {
	return readNodeProviderProtectedValue(stderr, "SSH password: ", "SSH password")
}

func nodesSSHProvisionUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s nodes ssh-provision --host ADDR --user NAME (--key-file PATH | --password) --name NAME [flags]

Adopts a machine you already have (any VPS, home server, Raspberry Pi) by
dialing it over SSH and installing the node agent there: detects the
remote OS and architecture, installs Docker if it's missing (systemd is
required; there is no workaround for a non-systemd host), then installs
and starts the agent as a systemd-managed container with a fresh join
token. Returns immediately with a provision id to poll; the install
itself runs in the background on the control plane.

--password is prompted with no echo (or read from a non-terminal stdin,
one line, trimmed), never taken as a flag value so it never lands in
shell history or the process list. The credential (key or password) is
used once for this install and never stored.

Flags:
  --host string                  the remote machine's address (required)
  --port int                     the remote machine's SSH port (default 22)
  --user string                  the SSH username to authenticate as (required)
  --key-file string              path to a private key file for key auth (mutually exclusive with --password)
  --passphrase string            passphrase for --key-file, if it's encrypted
  --password                     authenticate with a password instead of a key, prompted with no echo
  --name string                  name for the new node: lowercase letters, digits, hyphens (required)
  --role string                  general or build (default general)
  --control-plane-addr string    host:port the adopted machine dials to reach this control plane's agent listener (default: --api-url's host, port 9443)
  --token string                 API token (default: %[2]s env var, then the credentials file)
  --api-url string               control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string                named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                            print the new provision as JSON to stdout, nothing else
  --output string                  output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string                   JMESPath expression to filter the result before printing
  -h, --help                       show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
