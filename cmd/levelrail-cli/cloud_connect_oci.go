package main

import (
	"bufio"
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/GLINCKER/levelrail/internal/cloudconn"
	"github.com/GLINCKER/levelrail/internal/secretsource"
)

var (
	connectNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)
	ociFPRe       = regexp.MustCompile(`([0-9a-fA-F]{2}:){15}[0-9a-fA-F]{2}`)
	// IAM verbs and tenancy scope never belong in the connection's own policy.
	forbiddenPolicyRe = regexp.MustCompile(`(?i)\bin tenancy\b|\b(users|groups|policies|compartments|dynamic-groups|all-resources|identity-providers)\b`)
)

type ociConnectOpts struct {
	profile, region, tenancy, compartmentName, parent, name string
	dryRun, yes, paste                                      bool
	userOCID, fingerprint, compartmentOCID, keyEnv          string
	apiURL, lrProfile                                       string
}

// ociIDs holds what each step created; empty means not created yet.
type ociIDs struct {
	compartment, group, user, policy, fingerprint string
	member                                        bool
}

type ociStep struct {
	title   string
	args    []string
	capture string // OCID kind to read from the output, "fingerprint", or ""
}

func runCloudConnectOCI(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), d ociConnectDeps) int {
	var o ociConnectOpts
	fs := flag.NewFlagSet(prog+" cloud connect oci", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&o.profile, "profile", "DEFAULT", "oci CLI profile holding your browser session")
	fs.StringVar(&o.region, "region", "", "region, default the profile's region")
	fs.StringVar(&o.tenancy, "tenancy", "", "tenancy OCID, default asked from oci itself")
	fs.StringVar(&o.compartmentName, "compartment-name", "levelrail", "compartment to create for Levelrail")
	fs.StringVar(&o.parent, "parent-compartment", "", "parent compartment OCID, default the tenancy root")
	fs.StringVar(&o.name, "name", "oci", "Levelrail connection name, also used in the group, user and policy names")
	fs.BoolVar(&o.dryRun, "dry-run", true, "print the plan and run nothing")
	fs.BoolVar(&o.yes, "yes", false, "run the plan")
	fs.BoolVar(&o.paste, "paste", false, "store a scoped credential you already made")
	fs.StringVar(&o.userOCID, "user-ocid", "", "--paste: the dedicated user OCID")
	fs.StringVar(&o.fingerprint, "fingerprint", "", "--paste: the API key fingerprint")
	fs.StringVar(&o.compartmentOCID, "compartment-ocid", "", "--paste: the compartment the user's policy covers")
	fs.StringVar(&o.keyEnv, "key-env", "", "--paste: name of an environment variable holding the PEM key (stdin otherwise)")
	fs.StringVar(&o.apiURL, "api-url", "", "Levelrail API URL, default from the CLI profile")
	fs.StringVar(&o.lrProfile, "levelrail-profile", "", "Levelrail CLI profile used to reach the API")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, cloudConnectUsage(prog)+"\nFlags:\n"); fs.PrintDefaults() }
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	dryExplicit := false
	fs.Visit(func(f *flag.Flag) { dryExplicit = dryExplicit || f.Name == "dry-run" })
	if o.yes && !dryExplicit {
		o.dryRun = false
	}
	if !connectNameRe.MatchString(o.name) {
		_, _ = fmt.Fprintf(stderr, "%s: --name must be lowercase letters, digits and dashes\n", prog)
		return exitValidation
	}
	if d.lookup == nil {
		d.lookup = lookupEnv
	}
	ctx := context.Background()
	if o.paste {
		return runOCIPaste(ctx, prog, o, stdout, stderr, d)
	}
	return runOCIBootstrap(ctx, prog, o, stdout, stderr, d)
}

func ociAuthArgs(o ociConnectOpts) []string {
	a := []string{"--auth", "security_token", "--profile", o.profile}
	if o.region != "" {
		a = append(a, "--region", o.region)
	}
	return a
}

func orPlaceholder(v, ph string) string {
	if v == "" {
		return ph
	}
	return v
}

func ociResourceName(o ociConnectOpts) string { return "levelrail-" + o.name }

// ociPolicy builds the statements and refuses anything tenancy wide or touching IAM.
func ociPolicy(o ociConnectOpts, compartmentOCID string) ([]string, error) {
	var st []string
	var err error
	if compartmentOCID == "" {
		st, err = cloudconn.PolicyStatements(o.compartmentName, ociResourceName(o))
		for i := range st {
			st[i] = strings.Replace(st[i], "in compartment "+o.compartmentName, "in compartment id <compartment-ocid>", 1)
		}
	} else {
		st, err = cloudconn.PolicyStatements(compartmentOCID, ociResourceName(o))
	}
	if err != nil {
		return nil, err
	}
	for _, s := range st {
		if forbiddenPolicyRe.MatchString(s) || !strings.Contains(s, " in compartment ") {
			return nil, fmt.Errorf("refusing a policy statement that is not compartment scoped: %q", s)
		}
	}
	return st, nil
}

// ociSteps is shared by the printed plan and the real run so both always match.
func ociSteps(o ociConnectOpts, ids ociIDs, pubKeyPath string) ([]ociStep, error) {
	auth := ociAuthArgs(o)
	rn := ociResourceName(o)
	grp := orPlaceholder(ids.group, "<group-ocid>")
	usr := orPlaceholder(ids.user, "<user-ocid>")
	policy, err := ociPolicy(o, ids.compartment)
	if err != nil {
		return nil, err
	}
	var buf strings.Builder
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(policy)
	stJSON := strings.TrimSpace(buf.String())
	with := func(a ...string) []string { return append(a, auth...) }
	return []ociStep{
		{"create the compartment " + o.compartmentName, with("iam", "compartment", "create", "--compartment-id", o.parent, "--name", o.compartmentName, "--description", "Resources managed by Levelrail", "--wait-for-state", "ACTIVE"), "compartment"},
		{"create the group " + rn, with("iam", "group", "create", "--name", rn, "--description", "Levelrail connection "+o.name), "group"},
		{"create the API only user " + rn, with("iam", "user", "create", "--name", rn, "--description", "Levelrail connection "+o.name), "user"},
		{"turn off console password and other credential types for the user", with("iam", "user", "update-user-capabilities", "--user-id", usr, "--can-use-console-password", "false", "--can-use-auth-tokens", "false", "--can-use-customer-secret-keys", "false", "--can-use-smtp-credentials", "false", "--can-use-api-keys", "true"), ""},
		{"add the user to the group", with("iam", "group", "add-user", "--group-id", grp, "--user-id", usr), "member"},
		{"create the policy " + rn + " scoped to the compartment only", with("iam", "policy", "create", "--compartment-id", o.parent, "--name", rn, "--description", "Levelrail connection "+o.name, "--statements", stJSON), "policy"},
		{"upload the public half of a key pair made on this machine", with("iam", "user", "api-key", "upload", "--user-id", usr, "--key-file", orPlaceholder(pubKeyPath, "<local-public-key.pem>")), "fingerprint"},
	}, nil
}

func ociCleanup(o ociConnectOpts, ids ociIDs) []string {
	a := " " + strings.Join(ociAuthArgs(o), " ")
	var out []string
	if ids.fingerprint != "" {
		out = append(out, "oci iam user api-key delete --user-id "+ids.user+" --fingerprint "+ids.fingerprint+" --force"+a)
	}
	if ids.policy != "" {
		out = append(out, "oci iam policy delete --policy-id "+ids.policy+" --force"+a)
	}
	if ids.member {
		out = append(out, "oci iam group remove-user --group-id "+ids.group+" --user-id "+ids.user+" --force"+a)
	}
	if ids.user != "" {
		out = append(out, "oci iam user delete --user-id "+ids.user+" --force"+a)
	}
	if ids.group != "" {
		out = append(out, "oci iam group delete --group-id "+ids.group+" --force"+a)
	}
	if ids.compartment != "" {
		out = append(out, "oci iam compartment delete --compartment-id "+ids.compartment+" --force"+a)
	}
	return out
}

func printCleanup(w io.Writer, o ociConnectOpts, ids ociIDs) {
	cmds := ociCleanup(o, ids)
	if len(cmds) == 0 {
		_, _ = fmt.Fprintln(w, "Nothing was created.")
		return
	}
	_, _ = fmt.Fprintln(w, "Already created (nothing is deleted for you). To remove it, run:")
	for _, c := range cmds {
		_, _ = fmt.Fprintln(w, "  "+c)
	}
}

func firstOCID(kind string, out []byte) string {
	re := regexp.MustCompile(`ocid1\.` + regexp.QuoteMeta(kind) + `\.[a-z0-9]+\.[a-z0-9-]*\.[a-z0-9]+`)
	return re.FindString(string(out))
}

func runOCIBootstrap(ctx context.Context, prog string, o ociConnectOpts, stdout, stderr io.Writer, d ociConnectDeps) int {
	bin, err := d.lookPath("oci")
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: the oci CLI was not found on PATH; install it, run `oci session authenticate`, or use --paste\n", prog)
		return exitValidation
	}
	red := secretsource.NewRedactor()
	run := func(args []string) ([]byte, error) {
		cctx, cancel := context.WithTimeout(ctx, ociCommandTimeout())
		defer cancel()
		out, err := d.runner.Run(cctx, bin, args)
		if err != nil {
			err = errors.New(redactOutput(red, err.Error()))
		}
		return out, err
	}
	if _, err := run(append([]string{"iam", "region", "list"}, ociAuthArgs(o)...)); err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: the oci session for profile %s is missing or expired: %v\nRun: oci session authenticate --profile-name %s, then try again.\n", prog, o.profile, err, o.profile)
		return exitValidation
	}
	if o.tenancy == "" {
		out, err := run(append([]string{"iam", "availability-domain", "list"}, ociAuthArgs(o)...))
		o.tenancy = firstOCID("tenancy", out)
		if err != nil || o.tenancy == "" {
			_, _ = fmt.Fprintf(stderr, "%s: could not read the tenancy OCID from oci; pass --tenancy\n", prog)
			return exitValidation
		}
	}
	if o.parent == "" {
		o.parent = o.tenancy
	}
	if o.region == "" {
		o.region = ociProfileRegion(o.profile)
	}
	steps, err := ociSteps(o, ociIDs{}, "")
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", prog, err)
		return exitValidation
	}
	_, _ = fmt.Fprintf(stdout, "Plan for connection %q (profile %s, region %s):\n", o.name, o.profile, orPlaceholder(o.region, "profile default"))
	for i, s := range steps {
		_, _ = fmt.Fprintf(stdout, "%d. %s\n   oci %s\n", i+1, s.title, shellJoin(s.args))
	}
	_, _ = fmt.Fprintf(stdout, "%d. send tenancy, region, compartment, user, fingerprint and the private key to Levelrail over the authenticated API, then wipe the local copy\n", len(steps)+1)
	if o.dryRun || !o.yes {
		_, _ = fmt.Fprintln(stdout, "\nDry run: nothing was created. Review the plan, then rerun with --yes.")
		return exitOK
	}
	sink := d.sink(o.apiURL, o.lrProfile)
	if err := sink.Ready(ctx); err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\nNothing was created.\n", prog, err)
		return exitValidation
	}
	return executeOCIPlan(ctx, prog, o, stdout, stderr, d, run, red, sink)
}

func executeOCIPlan(ctx context.Context, prog string, o ociConnectOpts, stdout, stderr io.Writer, d ociConnectDeps, run func([]string) ([]byte, error), red *secretsource.Redactor, sink ociCredentialSink) int {
	key, err := d.genKey()
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: generating the key pair failed\n", prog)
		return exitValidation
	}
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	red.Add(privPEM)
	defer wipe(privPEM)
	// Only the public half touches disk; the private key stays in memory and is wiped.
	dir, err := os.MkdirTemp(d.tempBase, "levelrail-oci-")
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: could not create a private temp dir\n", prog)
		return exitValidation
	}
	defer func() { _ = os.RemoveAll(dir) }()
	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return exitValidation
	}
	pubPath := filepath.Join(dir, "public.pem")
	if err := os.WriteFile(pubPath, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER}), 0o600); err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: could not write the public key\n", prog)
		return exitValidation
	}
	var ids ociIDs
	fail := func(step string, err error) int {
		_, _ = fmt.Fprintf(stderr, "%s: %s failed: %v\n", prog, step, err)
		printCleanup(stderr, o, ids)
		return exitAPIError
	}
	for i := 0; ; i++ {
		steps, err := ociSteps(o, ids, pubPath)
		if err != nil {
			return fail("building the plan", err)
		}
		if i >= len(steps) {
			break
		}
		s := steps[i]
		_, _ = fmt.Fprintf(stdout, "[%d/%d] %s\n", i+1, len(steps), s.title)
		out, err := run(s.args)
		if err != nil {
			return fail(s.title, err)
		}
		if err := ociCapture(&ids, s.capture, out); err != nil {
			return fail(s.title, err)
		}
	}
	cred := cloudconn.OracleCredential{TenancyOCID: o.tenancy, CompartmentOCID: ids.compartment, Region: o.region,
		Secret: cloudconn.NewOracleSecret(ids.user, ids.fingerprint, privPEM)}
	if err := cloudconn.ValidateOracle(cred); err != nil {
		return fail("checking the new credential", errors.New(redactOutput(red, err.Error())))
	}
	if err := sink.Store(ctx, o.name, cred); err != nil {
		return fail("storing the connection in Levelrail", errors.New(redactOutput(red, err.Error())))
	}
	printOCINextSteps(stdout, prog, o.name)
	return exitOK
}

func ociCapture(ids *ociIDs, kind string, out []byte) error {
	switch kind {
	case "":
		return nil
	case "member":
		ids.member = true
		return nil
	case "fingerprint":
		ids.fingerprint = ociFPRe.FindString(string(out))
		if ids.fingerprint == "" {
			return errors.New("oci returned no key fingerprint")
		}
		return nil
	}
	id := firstOCID(kind, out)
	if id == "" {
		return fmt.Errorf("oci returned no %s OCID", kind)
	}
	switch kind {
	case "compartment":
		ids.compartment = id
	case "group":
		ids.group = id
	case "user":
		ids.user = id
	case "policy":
		ids.policy = id
	}
	return nil
}

func runOCIPaste(ctx context.Context, prog string, o ociConnectOpts, stdout, stderr io.Writer, d ociConnectDeps) int {
	if o.compartmentOCID == "" || o.compartmentOCID == o.tenancy || strings.HasPrefix(o.compartmentOCID, "ocid1.tenancy.") {
		_, _ = fmt.Fprintf(stderr, "%s: --compartment-ocid must be a dedicated compartment; the tenancy root is never accepted\n", prog)
		return exitValidation
	}
	var keyPEM []byte
	if o.keyEnv != "" {
		v, ok := d.lookup(o.keyEnv)
		if !ok || v == "" {
			_, _ = fmt.Fprintf(stderr, "%s: environment variable %s is empty\n", prog, o.keyEnv)
			return exitValidation
		}
		keyPEM = []byte(v)
	} else {
		_, _ = fmt.Fprintln(stderr, "Paste the PEM private key, then press Enter after the END line:")
		keyPEM = readPEM(d.stdin)
	}
	defer wipe(keyPEM)
	cred := cloudconn.OracleCredential{TenancyOCID: o.tenancy, CompartmentOCID: o.compartmentOCID, Region: o.region,
		Secret: cloudconn.NewOracleSecret(o.userOCID, o.fingerprint, keyPEM)}
	if err := cloudconn.ValidateOracle(cred); err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", prog, err)
		return exitValidation
	}
	if o.dryRun && !o.yes {
		_, _ = fmt.Fprintln(stdout, "The credential is valid offline. Dry run: nothing was sent. Rerun with --yes to store it.")
		return exitOK
	}
	sink := d.sink(o.apiURL, o.lrProfile)
	if err := sink.Ready(ctx); err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", prog, err)
		return exitValidation
	}
	if err := sink.Store(ctx, o.name, cred); err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: storing the connection failed: %v\n", prog, err)
		return exitAPIError
	}
	printOCINextSteps(stdout, prog, o.name)
	return exitOK
}

func readPEM(r io.Reader) []byte {
	var b []byte
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		b = append(b, sc.Bytes()...)
		b = append(b, '\n')
		if strings.HasPrefix(sc.Text(), "-----END ") {
			break
		}
	}
	return b
}

func wipe(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

func printOCINextSteps(w io.Writer, prog, name string) {
	_, _ = fmt.Fprintf(w, "\nConnected. The local key copy was wiped.\nNext:\n  %[1]s cloud connection test %[2]s\n  %[1]s cloud connection list\nGuide: docs page iac/cloud-connect\n", prog, name)
}

// ociProfileRegion reads only the region key of one profile; nothing else in the file is kept.
func ociProfileRegion(profile string) string {
	path := os.Getenv("OCI_CLI_CONFIG_FILE")
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		path = filepath.Join(home, ".oci", "config")
	}
	f, err := os.Open(path) //nolint:gosec // the user's own oci config path
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	in := false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "[") {
			in = line == "["+profile+"]"
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok && in && strings.TrimSpace(k) == "region" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func shellJoin(args []string) string {
	out := make([]string, len(args))
	for i, a := range args {
		if a == "" || strings.ContainsAny(a, " \"'[]<>$") {
			out[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
		} else {
			out[i] = a
		}
	}
	return strings.Join(out, " ")
}
