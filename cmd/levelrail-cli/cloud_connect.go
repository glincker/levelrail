package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/cloudconn"
	"github.com/GLINCKER/levelrail/internal/secretsource"
)

func cloudConnectUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s cloud connect oci [--profile P] [--region R] [--tenancy OCID] [--compartment-name N]
      [--parent-compartment OCID] [--name NAME] [--dry-run=false --yes]
  %[1]s cloud connect oci --paste --name NAME --tenancy OCID --compartment-ocid OCID --region R
      --user-ocid OCID --fingerprint FP [--key-env VAR]

Preview. Uses your local oci CLI session once to create a dedicated compartment, group,
API-only user, a policy scoped to that compartment and a key pair made on this machine.
Only that scoped credential reaches Levelrail. The default is a dry run that prints every
command; --yes runs them. Root or tenancy admin credentials are never accepted.
--json prints the plan or the result as JSON and never prompts; it acts only with --yes.
--paste stores a scoped credential you already made; the key comes from --key-env or stdin.
`, prog)
}

func runCloudConnect(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, cloudConnectUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, cloudConnectUsage(prog))
		return exitOK
	case "oci":
		return runCloudConnectOCI(prog, args[1:], stdout, stderr, lookupEnv, defaultOCIConnectDeps(prog, lookupEnv))
	}
	_, _ = fmt.Fprintf(stderr, "%s: unknown cloud connect provider %q (only oci for now)\n\n", prog, args[0])
	_, _ = fmt.Fprint(stderr, cloudConnectUsage(prog))
	return exitUsage
}

// commandRunner runs one external command; tests swap in a fake so no real cloud is touched.
type commandRunner interface {
	Run(ctx context.Context, name string, args []string) ([]byte, error)
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, name string, args []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // fixed binary from LookPath, args built by this command
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if len(msg) > 600 {
			msg = msg[len(msg)-600:]
		}
		return out.Bytes(), fmt.Errorf("%w: %s", err, msg)
	}
	return out.Bytes(), nil
}

// ociCredentialSink receives the scoped credential; the API one is the only production path.
type ociCredentialSink interface {
	Ready(ctx context.Context) error
	Store(ctx context.Context, name string, cred cloudconn.OracleCredential) (fingerprintPrefix string, err error)
}

type ociConnectDeps struct {
	runner   commandRunner
	lookPath func(string) (string, error)
	sink     func(apiURL, profile string) ociCredentialSink
	stdin    io.Reader
	tempBase string
	genKey   func() (*rsa.PrivateKey, error)
	lookup   func(string) (string, bool)
}

func defaultOCIConnectDeps(prog string, lookupEnv func(string) (string, bool)) ociConnectDeps {
	return ociConnectDeps{
		runner:   execRunner{},
		lookPath: exec.LookPath,
		sink: func(apiURL, profile string) ociCredentialSink {
			return apiOCISink{prog: prog, apiURL: apiURL, profile: profile, lookupEnv: lookupEnv}
		},
		stdin:  os.Stdin,
		genKey: func() (*rsa.PrivateKey, error) { return rsa.GenerateKey(rand.Reader, ociKeyBits()) },
		lookup: lookupEnv,
	}
}

// apiOCISink talks to the control plane with the CLI's normal profile.
type apiOCISink struct {
	prog, apiURL, profile string
	lookupEnv             func(string) (string, bool)
}

func (s apiOCISink) base() string {
	return resolveAPIURL(s.apiURL, s.lookupEnv, s.prog, resolveProfile(s.profile, s.lookupEnv))
}

// Ready runs before any cloud resource is created, so a bad transport stops the flow early.
func (s apiOCISink) Ready(context.Context) error { return secureTransport(s.base()) }

// Store uploads the bundle once; the key copy is wiped when the call returns.
func (s apiOCISink) Store(ctx context.Context, name string, cred cloudconn.OracleCredential) (string, error) {
	base := s.base()
	if err := secureTransport(base); err != nil {
		return "", err
	}
	profile := resolveProfile(s.profile, s.lookupEnv)
	key := cred.Secret.KeyPEM()
	defer clear(key)
	got, err := NewClient(base, resolveToken("", s.lookupEnv, s.prog, profile)).CreateOracleConnection(ctx, apiclient.OracleConnectionRequest{
		Name: name, TenancyOCID: cred.TenancyOCID, Region: cred.Region, CompartmentOCID: cred.CompartmentOCID,
		UserOCID: cred.Secret.UserOCID, Fingerprint: cred.Secret.Fingerprint,
	}, key)
	if err != nil {
		return "", err
	}
	return got.FingerprintPrefix, nil
}

// secureTransport refuses to send a key over plain http unless the API is on this machine.
func secureTransport(base string) error {
	u, err := url.Parse(base)
	if err != nil || u.Host == "" {
		return errors.New("the Levelrail API URL is not valid")
	}
	if u.Scheme == "https" || (u.Scheme == "http" && isLoopbackURL(base)) {
		return nil
	}
	return fmt.Errorf("refusing to send a credential to %s over %s; use https", u.Host, u.Scheme)
}

func ociKeyBits() int {
	if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv("APP_CLOUD_OCI_MIN_RSA_BITS"))); err == nil && v >= 2048 {
		return v
	}
	return 2048
}

func ociCommandTimeout() time.Duration {
	if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv("APP_CLOUD_CONNECT_TIMEOUT_SECONDS"))); err == nil && v > 0 {
		return time.Duration(v) * time.Second
	}
	return 120 * time.Second
}

var jwtLikeRe = regexp.MustCompile(`eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+`)

// redactOutput masks the generated key and anything shaped like a session token.
func redactOutput(r *secretsource.Redactor, s string) string {
	return jwtLikeRe.ReplaceAllString(r.Mask(s), secretsource.Mask)
}
