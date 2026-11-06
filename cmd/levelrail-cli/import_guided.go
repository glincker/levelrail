package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/configtranslate"
)

// secretPathHints mark file names that usually hold credentials rather than app config.
var secretPathHints = []string{"secret", "credential", "vault", "keystore", "password", "id_rsa", ".pem", ".p12", "token"}

type guidedImportFlags struct {
	name, envFile, timeout string
	port                   int
	domains                stringListFlag
	yes, dryRun, allowPath bool
}

// runImportVercel implements "import vercel [dir]": a guided move of one project off Vercel.
// It never changes DNS and never touches the Vercel account.
func runImportVercel(prog string, args []string, stdin io.Reader, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiURLP, profileP, jsonOutP, outputP, queryP := apiFlagSet(prog, "import vercel", "accepted for consistency, the guided flow prints text", stderr)
	var f guidedImportFlags
	fs.StringVar(&f.name, "name", "", "app name to create or update (default: the directory name)")
	fs.IntVar(&f.port, "port", 0, "container port (default: detected from the framework)")
	fs.StringVar(&f.envFile, "env-file", "", "read environment values from this KEY=VALUE file; without it values come from the shell environment")
	fs.BoolVar(&f.allowPath, "allow-secret-path", false, "allow --env-file when its name looks like a secret store")
	fs.Var(&f.domains, "domain", "domain to attach, repeatable; apex and www are both attached and www redirects to the apex")
	fs.StringVar(&f.timeout, "timeout", defaultDeployUploadWait, "stop following the deploy after this long")
	fs.BoolVar(&f.dryRun, "dry-run", false, "print every step and change nothing")
	fs.BoolVar(&f.yes, "yes", false, "do not ask before the deploy and domain steps")
	fs.BoolVar(&f.yes, "y", false, "shorthand for --yes")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, importVercelUsage(prog)) }
	tokenFlag, apiURLFlag, profileFlag, _, _, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonOutP, outputP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	dir := "."
	if rest := fs.Args(); len(rest) > 1 {
		_, _ = fmt.Fprintf(stderr, "%s: import vercel takes at most one directory\n", prog)
		return exitUsage
	} else if len(rest) == 1 {
		dir = rest[0]
	}
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		_, _ = fmt.Fprintf(stderr, "%s: %q is not a directory\n", prog, dir)
		return exitUsage
	}
	if f.name == "" {
		f.name = dirAppName(dir)
	}
	g := &guidedRun{prog: prog, f: f, dir: dir, in: bufio.NewReader(stdin), out: stdout, errw: stderr, lookupEnv: lookupEnv,
		token: tokenFlag, apiURL: apiURLFlag, profile: profileFlag}
	return g.run()
}

type guidedRun struct {
	prog, dir              string
	f                      guidedImportFlags
	in                     *bufio.Reader
	out, errw              io.Writer
	lookupEnv              func(string) (string, bool)
	token, apiURL, profile string
	files                  map[string][]byte
	envPairs               []string
	step                   int
}

func (g *guidedRun) say(format string, a ...any) { _, _ = fmt.Fprintf(g.out, format+"\n", a...) }

func (g *guidedRun) header(title string) {
	g.step++
	g.say("\nStep %d of 7: %s", g.step, title)
}

// confirm asks on stdin unless --yes or --dry-run; an empty answer means no.
func (g *guidedRun) confirm(q string) bool {
	if g.f.yes || g.f.dryRun {
		return true
	}
	_, _ = fmt.Fprintf(g.out, "  %s [y/N] ", q)
	line, _ := g.in.ReadString('\n')
	ans := strings.ToLower(strings.TrimSpace(line))
	return ans == "y" || ans == "yes"
}

func (g *guidedRun) run() int {
	if g.f.dryRun {
		g.say("Dry run: nothing will be created, deployed or attached. DNS is never changed by this command.")
	}
	if code := g.stepRead(); code != exitOK {
		return code
	}
	if code := g.stepEnv(); code != exitOK {
		return code
	}
	if code := g.stepDeploy(); code != exitOK {
		return code
	}
	apexes := g.apexes()
	if len(apexes) == 0 {
		g.header("Attach domains")
		g.say("  No --domain given, skipped. Re-run with --domain example.com to attach it.")
		g.header("Cutover check")
		g.say("  Skipped, no domain.")
	} else {
		client := apiClientFromFlags(g.prog, g.apiURL, g.token, g.profile, g.lookupEnv)
		g.stepDomains(client, g.f.name, apexes)
		g.stepCutover(client, apexes)
	}
	g.stepTranslate()
	g.stepChecklist(g.f.name, apexes)
	return exitOK
}

func (g *guidedRun) stepRead() int {
	g.header("Read the project")
	g.files = map[string][]byte{}
	for _, n := range []string{"vercel.json", "package.json"} {
		if b, err := os.ReadFile(filepath.Join(g.dir, n)); err == nil { //nolint:gosec // fixed names under the directory the user chose
			g.files[n] = b
		}
	}
	if len(g.files) == 0 {
		g.say("  No vercel.json or package.json in %s.", g.dir)
		return exitUsage
	}
	s := configtranslate.SummarizeVercel(g.files)
	g.say("  Framework: %s", orDefault(s.Framework, "not detected"))
	g.say("  Build command: %s, output directory: %s", orDefault(s.Build, "default"), orDefault(s.Output, "default"))
	g.say("  Rewrites: %d, redirects: %d, headers: %d, crons: %d", s.Rewrites, s.Redirects, s.Headers, s.Crons)
	if len(s.Regions) > 0 {
		g.say("  Regions %s are ignored: Levelrail runs where your server runs.", strings.Join(s.Regions, ", "))
	}
	return exitOK
}

func (g *guidedRun) stepEnv() int {
	g.header("Environment variables")
	s := configtranslate.SummarizeVercel(g.files)
	vals := map[string]string{}
	if g.f.envFile != "" {
		if looksLikeSecretPath(g.f.envFile) && !g.f.allowPath {
			_, _ = fmt.Fprintf(g.errw, "%s: --env-file %q looks like a secret store; pass --allow-secret-path to read it\n", g.prog, filepath.Base(g.f.envFile))
			return exitUsage
		}
		b, err := os.ReadFile(g.f.envFile)
		if err != nil {
			_, _ = fmt.Fprintf(g.errw, "%s: read env file: %v\n", g.prog, err)
			return exitUsage
		}
		vals = parseEnvFile(b)
	}
	keys := map[string]bool{}
	for k := range vals {
		keys[k] = true
	}
	for _, k := range s.EnvKeys {
		keys[k] = true
	}
	if len(keys) == 0 {
		g.say("  None found. Pass --env-file to list and send them.")
		return exitOK
	}
	sorted := make([]string, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	missing := 0
	for _, k := range sorted {
		v, have := vals[k]
		if !have {
			v, have = g.lookupEnv(k)
		}
		state := "value ready"
		if have {
			g.envPairs = append(g.envPairs, k+"="+v)
		} else {
			state, missing = "no value found, set it on the app later", missing+1
		}
		kind := "runtime"
		if configtranslate.IsBuildTimeEnvKey(k) {
			kind = "build time, passed with --env on the upload deploy"
		}
		g.say("  %s  (%s, %s)", k, kind, state)
	}
	g.say("  Values are never printed. %d without a value.", missing)
	return exitOK
}

func (g *guidedRun) stepDeploy() int {
	g.header("Deploy")
	args := []string{"--app", g.f.name, "--timeout", g.f.timeout, "--yes"}
	for _, p := range g.envPairs {
		args = append(args, "--env", p)
	}
	if g.f.port > 0 {
		args = append(args, "--port", strconv.Itoa(g.f.port))
	}
	for _, a := range []struct{ flag, v string }{{"--token", g.token}, {"--api-url", g.apiURL}, {"--profile", g.profile}} {
		if a.v != "" {
			args = append(args, a.flag, a.v)
		}
	}
	if g.f.dryRun {
		g.say("  Would upload %s to app %q (file list below, nothing is sent).", g.dir, g.f.name)
		args = append(args, "--dry-run")
	} else if !g.confirm(fmt.Sprintf("Upload %s and deploy it to app %q?", g.dir, g.f.name)) {
		g.say("  Stopped before the deploy.")
		return exitUsage
	}
	return runDeployUpload(g.prog, append(args, g.dir), g.out, g.errw, g.lookupEnv, g.in)
}

// apexes returns the hosts the user asked for, without a www prefix.
func (g *guidedRun) apexes() []string {
	var out []string
	seen := map[string]bool{}
	for _, d := range g.f.domains {
		d = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(d)), "www.")
		if d != "" && !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	return out
}

func (g *guidedRun) stepDomains(c *Client, app string, apexes []string) {
	g.header("Attach domains")
	ctx := context.Background()
	for _, d := range apexes {
		if g.f.dryRun {
			g.say("  Would attach %s and www.%s to %s and redirect www to the apex. DNS stays untouched.", d, d, app)
			continue
		}
		if !g.confirm(fmt.Sprintf("Attach %s and www.%s to %q (no DNS writes)?", d, d, app)) {
			g.say("  Skipped %s.", d)
			continue
		}
		if _, err := c.EditAppDomains(ctx, app, apiclient.EditDomainsRequest{Add: []string{d, "www." + d}, DNS: "off"}); err != nil {
			g.say("  Could not attach %s: %v", d, err)
			continue
		}
		g.say("  Attached %s and www.%s.", d, d)
		attach := true
		if _, err := c.SetDomainCanonical(ctx, app, d, apiclient.CanonicalRequest{Preset: "www-to-apex", StatusCode: 308, Attach: &attach}); err != nil {
			g.say("  www redirect not applied: %v", err)
		} else {
			g.say("  www.%s redirects to %s (308).", d, d)
		}
	}
}

func (g *guidedRun) stepCutover(c *Client, apexes []string) {
	g.header("Cutover check and DNS records to set at your registrar")
	if g.f.dryRun {
		g.say("  Would run the domain cutover check for each domain and print the exact DNS records.")
		return
	}
	for _, d := range apexes {
		for _, host := range []string{d, "www." + d} {
			res, err := c.DomainCutover(context.Background(), host)
			if err != nil {
				g.say("  %s: check failed: %v", host, err)
				continue
			}
			g.say("  %s: %s (ready to switch: %t)", host, res.Summary, res.ReadyToSwitch)
			for _, r := range res.DNS.Records {
				g.say("    DNS: %s %s -> %s (TTL %d)", r.Type, r.Name, r.Value, r.TTLSeconds)
			}
		}
	}
}

func (g *guidedRun) stepTranslate() {
	g.header("Redirects and headers")
	res := configtranslate.Translate(g.files)
	routes, headers := res.RoutingRules()
	for _, r := range routes {
		g.say("  translated: %s -> %s (%d)", r.From, r.To, r.Status)
	}
	for _, h := range headers {
		g.say("  translated header rule for %s (%d headers)", h.Path, len(h.Values))
	}
	if len(routes)+len(headers) > 0 {
		g.say("  They are in the proposed app spec: run %s import config %s to review it.", g.prog, g.dir)
	}
	left := res.Untranslated()
	for _, f := range left {
		g.say("  not translated (%s): %s %s", f.Code, f.Message, f.Fix)
	}
	if len(routes)+len(headers)+len(left) == 0 {
		g.say("  Nothing to translate.")
	}
}

func (g *guidedRun) stepChecklist(app string, apexes []string) {
	g.header("Final checklist")
	g.say("  1. Open the temporary URL of %q (%s apps status %s) and test it before touching DNS.", app, g.prog, app)
	if len(apexes) > 0 {
		g.say("  2. At your registrar, set the DNS records printed in step 5 for: %s. This command never changes DNS.", strings.Join(apexes, ", "))
	} else {
		g.say("  2. Re-run with --domain to get the DNS records to set at your registrar.")
	}
	g.say("  3. Roll back with: %s apps rollback %s (or point DNS back at Vercel).", g.prog, app)
	g.say("  4. Stays on Vercel: the project, its domains and env vars, analytics and integrations. Nothing was deleted.")
	for _, n := range configtranslate.VercelCutoverChecklist() {
		g.say("  - %s", strings.TrimPrefix(n, "Cutover checklist: "))
	}
}

func looksLikeSecretPath(p string) bool {
	base := strings.ToLower(filepath.Base(p))
	for _, h := range secretPathHints {
		if strings.Contains(base, h) {
			return true
		}
	}
	return false
}

// parseEnvFile reads KEY=VALUE lines, skipping comments and an optional export prefix.
func parseEnvFile(b []byte) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "export "))
		k, v, ok := strings.Cut(line, "=")
		k = strings.TrimSpace(k)
		if !ok || k == "" || strings.HasPrefix(k, "#") {
			continue
		}
		v = strings.TrimSpace(v)
		if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
			v = v[1 : len(v)-1]
		}
		out[k] = v
	}
	return out
}

func importVercelUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s import vercel [dir] [--name APP] [--port N] [--env-file FILE] [--domain example.com]... [--dry-run] [--yes] [flags]

Moves one project off Vercel in seven steps: read vercel.json and package.json,
list environment variables, deploy the directory, attach domains (apex and www,
www redirected to the apex), run the domain cutover check, translate redirects
and headers, and print a final checklist. Needs only Levelrail credentials.
It never changes DNS and never deletes anything on Vercel. Values are never printed.
NEXT_PUBLIC_ and VITE_ keys are build time and are sent with --env on the upload deploy.

Flags:
  --name string            app name to create or update (default: the directory name)
  --port int               container port (default: detected from the framework)
  --env-file string        read environment values from this KEY=VALUE file; without it values come from the shell environment
  --allow-secret-path      allow --env-file when its name looks like a secret store
  --domain string          domain to attach, repeatable; apex and www are both attached and www redirects to the apex
  --timeout string         stop following the deploy after this long (default %[5]s)
  --dry-run                print every step and change nothing
  --yes, -y                do not ask before the deploy and domain steps
  --token string           API token (default: %[2]s env var, then the credentials file)
  --api-url string         control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string         named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                   accepted for consistency, the guided flow prints text
  --output string          accepted for consistency, the guided flow prints text
  --query string           accepted for consistency, the guided flow prints text
  -h, --help               show this help
`, prog, apiTokenEnvHelp, apiURLEnvHelp, defaultAPIURL, defaultDeployUploadWait)
}
