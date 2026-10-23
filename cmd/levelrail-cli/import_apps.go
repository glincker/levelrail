package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const (
	appImportPollEvery   = 2 * time.Second
	appImportStateBuild  = "building"
	appImportMapFlagHelp = "rewrite a source hostname in env values, as OLD=NEW, repeatable"
)

type importAppsFlags struct {
	from, url, token, only, collision, apiURL, profile, session, snapshot, appName string
	tokenStdin, plan, apply, verify, receipt, rollback, exitReport, test           bool
	acceptSuggested, insecure, allowPrivate, allowLoopback                         bool
	jsonOut                                                                        bool
	images                                                                         imageMoveFlags
	ssh                                                                            importSSHFlags
	maps                                                                           mapPairs
	wait                                                                           time.Duration
}

type mapPairs []apiclient.AppImportMapping

func (m *mapPairs) String() string { return "" }

func (m *mapPairs) Set(v string) error {
	from, to, ok := strings.Cut(v, "=")
	if !ok || strings.TrimSpace(from) == "" || strings.TrimSpace(to) == "" {
		return errors.New("a mapping must look like OLD=NEW")
	}
	*m = append(*m, apiclient.AppImportMapping{From: strings.TrimSpace(from), To: strings.TrimSpace(to)})
	return nil
}

func (f importAppsFlags) onlyList() []string {
	var out []string
	for _, o := range strings.Split(f.only, ",") {
		if o = strings.TrimSpace(o); o != "" {
			out = append(out, o)
		}
	}
	return out
}

// runImportApps implements "import apps": plan or stage the applications of
// another platform, then verify, roll back or print the receipt of a session.
func runImportApps(prog string, args []string, stdin io.Reader, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if hasResumeFlag(args) {
		return runMigrateResume(prog, args, stdin, stdout, stderr, lookupEnv)
	}
	var f importAppsFlags
	fs := flag.NewFlagSet(prog+" import apps", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, importAppsUsage(prog)) }
	fs.StringVar(&f.from, "from", "coolify", "source platform: coolify, dokploy, caprover, docker, docker-ssh, compose or dokku")
	fs.StringVar(&f.url, "url", "", "source platform base URL")
	fs.StringVar(&f.snapshot, "snapshot", "", "source file: docker inspect JSON, a compose file or a dokku env dump (- for stdin)")
	fs.StringVar(&f.appName, "app-name", "", "the app a dokku env dump belongs to (--from dokku)")
	fs.StringVar(&f.token, "token", "", "source token (prefer "+envImportSourceToken+" or --token-stdin)")
	fs.BoolVar(&f.tokenStdin, "token-stdin", false, "read the source token from the first line of stdin")
	fs.BoolVar(&f.plan, "plan", false, "print the inventory, verdicts and preflight, create nothing")
	fs.BoolVar(&f.test, "test", false, "connect to the source once and report what it holds, store nothing")
	fs.BoolVar(&f.apply, "apply", false, "stage the selected apps (stopped, unrouted) in a new session")
	fs.StringVar(&f.session, "session", "", "an existing import session id (for --verify, --rollback, --receipt)")
	fs.BoolVar(&f.verify, "verify", false, "build or pull the staged apps and wait for readiness (needs --session)")
	fs.BoolVar(&f.exitReport, "exit-report", false, "print a Coolify session's exit report: outcomes per resource and the cutover checklist (needs --session)")
	fs.BoolVar(&f.rollback, "rollback", false, "remove the apps this session staged (needs --session)")
	fs.BoolVar(&f.receipt, "receipt", false, "print the session receipt as JSON (needs --session)")
	fs.StringVar(&f.only, "only", "", "comma-separated app names or source ids")
	fs.Var(&f.maps, "map", appImportMapFlagHelp)
	fs.BoolVar(&f.acceptSuggested, "accept-suggested-maps", false, "also apply the hostname mappings suggested from databases already moved")
	fs.StringVar(&f.collision, "collision", "suffix", "name collision handling: suffix or skip")
	fs.BoolVar(&f.insecure, "insecure-tls", false, "skip TLS verification of the source")
	fs.BoolVar(&f.allowPrivate, "allow-private", false, "allow a private-network source (control plane also needs "+"APP_IMPORT_ALLOW_PRIVATE_NETWORKS=true)")
	fs.BoolVar(&f.allowLoopback, "allow-loopback", false, "allow a loopback source (control plane also needs "+"APP_IMPORT_ALLOW_LOOPBACK=true)")
	fs.DurationVar(&f.wait, "wait", 15*time.Minute, "how long --verify and --transfer-images wait")
	f.images.register(fs)
	fs.StringVar(&f.ssh.fingerprint, "fingerprint", "", "host key fingerprint to pin for --from docker-ssh, SHA256:... (omit it to list the server's keys)")
	fs.BoolVar(&f.ssh.insecureHostKey, "insecure-host-key", false, "skip host key verification for --from docker-ssh (unsafe, warned on every surface)")
	fs.StringVar(&f.ssh.socket, "ssh-socket", "", "Docker socket path on the old host for --from docker-ssh (default /var/run/docker.sock)")
	fs.BoolVar(&f.ssh.testOnly, "test-connection", false, "with --from docker-ssh: log in and count containers, volumes and images, change nothing")
	fs.BoolVar(&f.ssh.copyVolumes, "copy-volumes", false, "copy a docker-ssh session's volumes through the ssh tunnel, read only on the old host (needs --session)")
	fs.StringVar(&f.apiURL, "api-url", "", "control plane API base URL")
	fs.StringVar(&f.profile, "profile", "", "named credentials profile for the control plane")
	fs.BoolVar(&f.jsonOut, "json", false, "print JSON")
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help" || args[0] == "help") {
		_, _ = fmt.Fprint(stdout, importAppsUsage(prog))
		return exitOK
	}
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	client := apiClientFromFlags(prog, f.apiURL, "", f.profile, lookupEnv)
	ctx := context.Background()
	switch {
	case f.images.any() && f.session == "":
		_, _ = fmt.Fprintf(stderr, "%s: --transfer-images, --images and --cancel-images need --session\n", prog)
		return exitUsage
	case f.ssh.copyVolumes && f.session == "":
		_, _ = fmt.Fprintf(stderr, "%s: --copy-volumes needs --session\n", prog)
		return exitUsage
	case f.ssh.copyVolumes:
		return runImportAppsCopyVolumes(ctx, prog, client, f, stdout, stderr)
	case f.images.any():
		return runImportAppsImages(ctx, prog, client, f, stdin, stdout, stderr, lookupEnv)
	case f.session != "" && f.exitReport:
		rep, err := client.AppImportExitReport(ctx, f.session)
		if err != nil {
			return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("exit report: %w", err))
		}
		return printCoolifyExit(stdout, stderr, prog, f.jsonOut, rep)
	case f.session != "" && (f.verify || f.rollback || f.receipt):
		return runImportAppsSession(ctx, client, f, stdout, stderr)
	case f.from == platformDockerSSH && f.ssh.testOnly:
		return runImportAppsSSH(ctx, prog, client, f, stdin, stdout, stderr, lookupEnv)
	case f.test && f.plan == f.apply:
	case f.plan == f.apply:
		_, _ = fmt.Fprintf(stderr, "%s: choose exactly one of --plan or --apply (or --session with --verify, --rollback or --receipt)\n", prog)
		return exitUsage
	case isFileImportSource(f.from) && f.snapshot == "":
		_, _ = fmt.Fprintf(stderr, "%s: --snapshot FILE is required with --from %s\n", prog, f.from)
		return exitUsage
	case f.from == "dokku" && strings.TrimSpace(f.appName) == "":
		_, _ = fmt.Fprintf(stderr, "%s: --app-name is required with --from dokku\n", prog)
		return exitUsage
	case f.from != platformDockerSSH && !isFileImportSource(f.from) && f.url == "":
		_, _ = fmt.Fprintf(stderr, "%s: --url is required\n", prog)
		return exitUsage
	}
	var token, snapshot string
	var conn *apiclient.AppImportSSH
	srcURL := f.url
	switch {
	case f.from == platformDockerSSH:
		c, code, ok := prepareImportSSH(ctx, prog, client, f, stdin, stdout, stderr, lookupEnv)
		if !ok {
			return code
		}
		conn, srcURL = &c, ""
	case isFileImportSource(f.from):
		var err error
		if snapshot, err = readSnapshot(f.snapshot, stdin); err != nil {
			_, _ = fmt.Fprintf(stderr, "%s: %v\n", prog, err)
			return exitUsage
		}
		srcURL = "file://" + f.from
	default:
		var code int
		var ok bool
		pf := importPlatformFlags{token: f.token, tokenStdin: f.tokenStdin}
		if token, code, ok = resolveSourceToken(prog, pf, stdin, stderr, lookupEnv); !ok {
			return code
		}
	}
	req := apiclient.AppImportRequest{Platform: f.from, URL: srcURL, Token: token, Snapshot: snapshot, SSH: conn, AppName: f.appName, InsecureTLS: f.insecure, AllowPrivate: f.allowPrivate,
		AllowLoopback: f.allowLoopback, Collision: f.collision, Mappings: f.maps, Only: f.onlyList()}
	if f.test {
		return runImportAppsTest(ctx, client, f, req, stdout, stderr)
	}
	if f.plan {
		view, err := client.PlanAppImport(ctx, req)
		if err != nil {
			return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("plan app import: %w", err))
		}
		return printAppImportView(stdout, stderr, f.jsonOut, view, "Plan (nothing was created)")
	}
	return applyImportApps(ctx, prog, client, f, req, stdout, stderr)
}

func applyImportApps(ctx context.Context, prog string, client *Client, f importAppsFlags, req apiclient.AppImportRequest, stdout, stderr io.Writer) int {
	req.Only = nil
	view, err := client.CreateAppImportSession(ctx, req)
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("create import session: %w", err))
	}
	update := apiclient.AppImportPlanUpdate{Collision: f.collision}
	if only := f.onlyList(); len(only) > 0 {
		ids := matchItems(view.Items, only)
		update.Selected = &ids
	}
	maps := append([]apiclient.AppImportMapping(nil), f.maps...)
	if f.acceptSuggested {
		maps = append(maps, view.Suggested...)
	}
	if len(maps) > 0 {
		update.Mappings = &maps
	}
	if view, err = client.PutAppImportPlan(ctx, view.ID, update); err != nil {
		return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("update import plan: %w", err))
	}
	staged, err := client.StageAppImport(ctx, view.ID)
	var apiErr *apiclient.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == 422 {
		if blocked, gerr := client.GetAppImportSession(ctx, view.ID); gerr == nil {
			_ = printAppImportView(stdout, stderr, f.jsonOut, blocked, "Blocked by preflight, nothing was created (session "+view.ID+" kept)")
		}
		return exitCheckFailed
	}
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("stage apps: %w", err))
	}
	code := printAppImportView(stdout, stderr, f.jsonOut, staged, "Staged (stopped and not routed)")
	if !f.jsonOut {
		_, _ = fmt.Fprintf(stdout, "\nSession %s. Next: %s import apps --session %s --verify\n", staged.ID, prog, staged.ID)
	}
	if staged.States["stage-failed"] > 0 {
		return exitCheckFailed
	}
	return code
}

func matchItems(items []apiclient.AppImportItem, only []string) []string {
	var ids []string
	for _, it := range items {
		for _, o := range only {
			if o == it.SourceID || strings.EqualFold(o, it.Name) {
				ids = append(ids, it.SourceID)
			}
		}
	}
	return ids
}

func runImportAppsSession(ctx context.Context, client *Client, f importAppsFlags, stdout, stderr io.Writer) int {
	view, err := client.GetAppImportSession(ctx, f.session)
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("load import session: %w", err))
	}
	items := matchItems(view.Items, f.onlyList())
	switch {
	case f.receipt:
		raw, err := client.AppImportReceipt(ctx, f.session)
		if err != nil {
			return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("load receipt: %w", err))
		}
		_, _ = fmt.Fprintln(stdout, string(raw))
		return exitOK
	case f.rollback:
		view, err = client.RollbackAppImport(ctx, f.session, items)
		if err != nil {
			return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("roll back: %w", err))
		}
		return printAppImportView(stdout, stderr, f.jsonOut, view, "Rolled back (only apps this session created were removed)")
	}
	if view, err = client.VerifyAppImport(ctx, f.session, items); err != nil {
		return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("verify: %w", err))
	}
	deadline := time.Now().Add(f.wait)
	for view.States[appImportStateBuild] > 0 && time.Now().Before(deadline) {
		time.Sleep(appImportPollEvery)
		if view, err = client.GetAppImportSession(ctx, f.session); err != nil {
			return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("poll verification: %w", err))
		}
	}
	code := printAppImportView(stdout, stderr, f.jsonOut, view, "Verification")
	if view.States["verify-failed"] > 0 || view.States[appImportStateBuild] > 0 {
		return exitCheckFailed
	}
	return code
}

func printAppImportView(stdout, stderr io.Writer, jsonOut bool, v apiclient.AppImportView, title string) int {
	if jsonOut {
		if err := writeJSONValue(stdout, v); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return exitNetwork
		}
		return exitOK
	}
	_, _ = fmt.Fprintf(stdout, "%s: %s\n\n", title, v.Platform)
	tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "APP\tPROJECT\tSOURCE\tBUILD\tDOMAINS\tENV\tVOLUMES\tVERDICT\tSTATE")
	for _, it := range v.Items {
		e := it.Entry
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%d+%ds\t%d\t%s\t%s\n", it.Name, strings.Trim(e.Project+"/"+e.Environment, "/"), e.Source, e.MapsTo,
			strings.Join(e.Domains, ","), e.Env.Plain, e.Env.Secret, len(e.Volumes), e.Verdict, it.State)
	}
	_ = tw.Flush()
	for _, it := range v.Items {
		if len(it.Entry.Findings) == 0 && it.Reason == "" {
			continue
		}
		_, _ = fmt.Fprintf(stdout, "\n%s (%s)\n", it.Name, it.Entry.Verdict)
		for _, f := range it.Entry.Findings {
			_, _ = fmt.Fprintf(stdout, "  - %s\n", f.Reason)
			if f.Next != "" {
				_, _ = fmt.Fprintf(stdout, "    next: %s\n", f.Next)
			}
		}
		if it.Reason != "" {
			_, _ = fmt.Fprintf(stdout, "  result: %s\n", it.Reason)
		}
	}
	printAppImportPreflight(stdout, v)
	return exitOK
}

func printAppImportPreflight(w io.Writer, v apiclient.AppImportView) {
	if len(v.Suggested) > 0 {
		_, _ = fmt.Fprintln(w, "\nSuggested hostname mappings (databases already moved), apply with --map or --accept-suggested-maps:")
		for _, m := range v.Suggested {
			_, _ = fmt.Fprintf(w, "  %s -> %s\n", m.From, m.To)
		}
	}
	pre := v.Preflight
	if pre == nil {
		return
	}
	for _, c := range pre.Checks {
		if c.Status == "pass" {
			continue
		}
		_, _ = fmt.Fprintf(w, "\n[%s] %s %s: %s\n", c.Status, c.App, c.ID, c.Detail)
		if c.Fix != "" {
			_, _ = fmt.Fprintf(w, "    fix: %s\n", c.Fix)
		}
	}
	if len(pre.Diff) > 0 {
		_, _ = fmt.Fprintln(w, "\nEnvironment values a mapping rewrites (secrets masked):")
		for _, d := range pre.Diff {
			_, _ = fmt.Fprintf(w, "  %s %s\n    - %s\n    + %s\n", d.App, d.Key, d.Before, d.After)
		}
	}
}

func importAppsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s import apps --from coolify --url URL --token-stdin --plan [--only NAME] [--map OLD=NEW] [--json]
  %[1]s import apps --from docker --snapshot inspect.json --plan
  %[1]s import apps --from compose --snapshot docker-compose.yml --plan
  %[1]s import apps --from docker-ssh --ssh user@host[:port] --ssh-key FILE|- [--fingerprint SHA256:... | --insecure-host-key] [--ssh-socket PATH] [--allow-private] --test-connection|--plan|--apply
  %[1]s import apps --session ID --copy-volumes [--only NAME]
  %[1]s import apps --from dokku --app-name web --snapshot dokku-env.txt --plan
  %[1]s import apps --from coolify --url URL --token-stdin --apply [--only NAME] [--map OLD=NEW] [--accept-suggested-maps]
  %[1]s import apps --session ID --transfer-images --ssh user@host[:port] --ssh-key FILE|--ssh-agent [--only NAME]
  %[1]s import apps --session ID --images|--cancel-images
  %[1]s import apps --session ID --verify|--rollback|--receipt [--only NAME]
  %[1]s import apps --session ID --exit-report
  %[1]s import apps --from dokploy --url URL --token-stdin --test
  %[1]s import apps --resume ID [--retry] [--follow] [--retire [--probe]]

Plans and stages the applications of another platform. The source is only
ever read (GET requests). --apply creates the apps stopped and not routed,
with env and secrets imported encrypted and domains held back. --verify
builds or pulls them, waits for readiness and stops them again. DNS, volume
copies and routing stay with you: see the dashboard's Import apps flow.

--transfer-images moves images built on the source host into this node with
"docker save" over SSH (the only command run there) and checks each loaded
image ID against the snapshot. The key is read from --ssh-key (a file, or -
for stdin) and its passphrase from %[3]s; neither is stored. Run it after
--apply and before --verify. A re-run skips images already verified.

--from docker-ssh needs only an SSH login on the old host: no platform token, no
agent. SSH is used as a transport to the Docker socket and nothing is ever run
there: only read-only Engine API GETs are sent (containers, volumes, images, a
mounted directory's archive, an image save stream). The host key must be pinned
with --fingerprint (omit it to list what the server presents; --insecure-host-key
skips the check and is warned about). The key comes from --ssh-key (a file, or
- for stdin) with its passphrase in %[3]s, or a password in %[4]s; they are held
in memory for the session and never stored. env comes from the running
containers: the old platform's own stored config is not read. After --apply,
--transfer-images and --copy-volumes need no further SSH flags.

The source token is read from %[2]s, or --token-stdin, or --token (warns).
It is sent to this control plane in the request body only and never stored.
`, prog, envImportSourceToken, envImportSSHPassphrase, envImportSSHPassword)
}

func isFileImportSource(from string) bool {
	return from == "docker" || from == "compose" || from == "dokku"
}

// readSnapshot reads a docker inspect JSON file, or stdin when path is "-".
func readSnapshot(path string, stdin io.Reader) (string, error) {
	var b []byte
	var err error
	if path == "-" {
		b, err = io.ReadAll(stdin)
	} else {
		b, err = os.ReadFile(path) //nolint:gosec // path is the operator's own CLI argument
	}
	if err != nil {
		return "", fmt.Errorf("read snapshot: %w", err)
	}
	return string(b), nil
}

func prepareImportSSH(ctx context.Context, prog string, client *Client, f importAppsFlags, stdin io.Reader, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) (apiclient.AppImportSSH, int, bool) {
	conn, ok := buildImportSSH(prog, f, stdin, stderr, lookupEnv)
	if !ok {
		return conn, exitUsage, false
	}
	if code, ok := confirmHostKey(ctx, prog, client, f, conn, stdout, stderr); !ok {
		return conn, code, false
	}
	return conn, exitOK, true
}

func runImportAppsSSH(ctx context.Context, prog string, client *Client, f importAppsFlags, stdin io.Reader, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	conn, code, ok := prepareImportSSH(ctx, prog, client, f, stdin, stdout, stderr, lookupEnv)
	if !ok {
		return code
	}
	return runImportAppsSSHTest(ctx, prog, client, f, conn, stdout, stderr)
}

func runImportAppsTest(ctx context.Context, client *Client, f importAppsFlags, req apiclient.AppImportRequest, stdout, stderr io.Writer) int {
	res, err := client.TestMigrationSource(ctx, req)
	if err != nil {
		return reportError(stdout, stderr, f.jsonOut, fmt.Errorf("test the source: %w", err))
	}
	code := exitOK
	if !res.OK {
		code = exitCheckFailed
	}
	if f.jsonOut {
		if c := emitJSON(stdout, stderr, res); c != exitOK {
			return c
		}
		return code
	}
	if !res.OK {
		_, _ = fmt.Fprintf(stdout, "Could not use the source (%s): %s\n", res.Failure, res.Message)
		return code
	}
	_, _ = fmt.Fprintf(stdout, "Connected to %s: %d app(s), %d database(s), %d volume(s), %d domain(s), %d cron job(s), %d the importer cannot move\n",
		res.Platform, res.Apps, res.Databases, res.Volumes, res.Domains, res.Crons, res.Unsupported)
	for _, w := range res.Warnings {
		_, _ = fmt.Fprintf(stdout, "  warning: %s\n", w)
	}
	return code
}
