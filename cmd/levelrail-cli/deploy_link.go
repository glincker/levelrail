package main

import (
	"flag"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"
)

const envBrandDocsURL = "APP_BRAND_DOCS_URL"

var (
	deployLinkImageRE = regexp.MustCompile(`^(?:[a-zA-Z0-9](?:[a-zA-Z0-9-]*[a-zA-Z0-9])?(?:\.[a-zA-Z0-9](?:[a-zA-Z0-9-]*[a-zA-Z0-9])?)*(?::[0-9]+)?/)?[a-z0-9]+(?:(?:[._]|__|-+)[a-z0-9]+)*(?:/[a-z0-9]+(?:(?:[._]|__|-+)[a-z0-9]+)*)*(?::[A-Za-z0-9_][A-Za-z0-9._-]{0,127})?(?:@sha256:[a-f0-9]{64})?$`)
	deployLinkNameRE  = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
	deployLinkEnvRE   = regexp.MustCompile(`^[A-Z_][A-Z0-9_]{0,63}$`)
)

const deployLinkMaxEnvKeys = 50

type deployLinkResult struct {
	DirectURL   string `json:"direct_url,omitempty"`
	LauncherURL string `json:"launcher_url,omitempty"`
	Markdown    string `json:"markdown,omitempty"`
}

type deployLinkParams struct {
	Image  string
	Name   string
	Port   int
	Env    []string
	Health string
}

func validateDeployLink(p deployLinkParams) error {
	if len(p.Image) == 0 || len(p.Image) > 255 || !deployLinkImageRE.MatchString(p.Image) {
		return fmt.Errorf("--image %q is not a valid OCI image reference", p.Image)
	}
	if !deployLinkNameRE.MatchString(p.Name) {
		return fmt.Errorf("--name %q must be a DNS label: lowercase letters, digits and hyphens, at most 63 characters", p.Name)
	}
	if p.Port < 1 || p.Port > 65535 {
		return fmt.Errorf("--port %d must be between 1 and 65535", p.Port)
	}
	if len(p.Env) > deployLinkMaxEnvKeys {
		return fmt.Errorf("--env lists %d keys, at most %d are allowed", len(p.Env), deployLinkMaxEnvKeys)
	}
	for _, k := range p.Env {
		if !deployLinkEnvRE.MatchString(k) {
			return fmt.Errorf("--env key %q must be an uppercase identifier such as DATABASE_URL (values are never part of a link)", k)
		}
	}
	if p.Health != "" {
		if len(p.Health) > 256 || !strings.HasPrefix(p.Health, "/") || strings.HasPrefix(p.Health, "//") || strings.ContainsAny(p.Health, " \\\t\r\n") {
			return fmt.Errorf("--health %q must start with a single slash", p.Health)
		}
	}
	return nil
}

// deployLinkQuery builds the query in a fixed parameter order. deploy=1 is
// included only for direct server URLs; the launcher page adds it itself.
func deployLinkQuery(p deployLinkParams, withDeploy bool) string {
	var parts []string
	add := func(k, v string) { parts = append(parts, k+"="+url.QueryEscape(v)) }
	if withDeploy {
		add("deploy", "1")
	}
	add("image", p.Image)
	add("name", p.Name)
	add("port", fmt.Sprint(p.Port))
	if len(p.Env) > 0 {
		add("env", strings.Join(p.Env, ","))
	}
	if p.Health != "" {
		add("health", p.Health)
	}
	return strings.Join(parts, "&")
}

func parseHTTPBase(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
		return nil, fmt.Errorf("%q must be an http or https URL without a username or password", raw)
	}
	return u, nil
}

func runDeployLink(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs := flag.NewFlagSet(prog+" deploy-link", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var p deployLinkParams
	var envList, server, launcher string
	fs.StringVar(&p.Image, "image", "", "image reference, e.g. ghcr.io/you/app:latest (required)")
	fs.StringVar(&p.Name, "name", "", "suggested app name, a DNS label (required)")
	fs.IntVar(&p.Port, "port", 0, "container port the app listens on (required)")
	fs.StringVar(&envList, "env", "", "comma separated env var names to ask for, uppercase (names only, never values)")
	fs.StringVar(&p.Health, "health", "", "health check path, must start with a single slash")
	fs.StringVar(&server, "server", "", "also print the direct URL for this server, e.g. https://levelrail.example.com")
	fs.StringVar(&launcher, "launcher", "", "launcher page URL (default: "+envBrandDocsURL+" env var plus /deploy.html)")
	jsonOutP := fs.Bool("json", false, "print the links as JSON to stdout and nothing else")
	outputFlagP, queryFlagP := bindOutputQueryFlags(fs)
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, deployLinkUsage(prog))
		fs.PrintDefaults()
	}
	_, of, exitCode, ok := parseSessionFlags(fs, args, jsonOutP, outputFlagP, queryFlagP, prog, stderr)
	if !ok {
		return exitCode
	}
	if envList != "" {
		for _, k := range strings.Split(envList, ",") {
			if k = strings.TrimSpace(k); k != "" {
				p.Env = append(p.Env, k)
			}
		}
	}
	if err := validateDeployLink(p); err != nil {
		_, _ = fmt.Fprintf(stderr, "%s: %v\n", prog, err)
		return exitValidation
	}

	var launcherURL *url.URL
	if launcher == "" {
		if docs, ok := lookupEnv(envBrandDocsURL); ok && docs != "" {
			launcher = strings.TrimRight(docs, "/") + "/deploy.html"
		}
	}
	if launcher != "" {
		u, err := parseHTTPBase(launcher)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "%s: --launcher: %v\n", prog, err)
			return exitValidation
		}
		launcherURL = u
	}

	var serverURL *url.URL
	if server != "" {
		u, err := parseHTTPBase(server)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "%s: --server: %v\n", prog, err)
			return exitValidation
		}
		serverURL = u
	}
	if launcherURL == nil && serverURL == nil {
		_, _ = fmt.Fprintf(stderr, "%s: pass --launcher URL or --server URL (or set %s)\n", prog, envBrandDocsURL)
		return exitUsage
	}

	var out deployLinkResult
	if serverURL != nil {
		out.DirectURL = serverURL.Scheme + "://" + serverURL.Host + "/apps/new?" + deployLinkQuery(p, true)
	}
	if launcherURL != nil {
		out.LauncherURL = launcherURL.Scheme + "://" + launcherURL.Host + launcherURL.Path + "?" + deployLinkQuery(p, false)
		badge := launcherURL.Scheme + "://" + launcherURL.Host + "/badges/deploy.svg"
		out.Markdown = fmt.Sprintf("[![Deploy](%s)](%s)", badge, out.LauncherURL)
	}
	if err := renderResult(stdout, of.Format, of.Query, out, func() {
		if out.DirectURL != "" {
			_, _ = fmt.Fprintf(stdout, "Direct link:\n%s\n\n", out.DirectURL)
		}
		if out.LauncherURL != "" {
			_, _ = fmt.Fprintf(stdout, "Launcher link:\n%s\n\nREADME Markdown:\n%s\n", out.LauncherURL, out.Markdown)
		}
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func deployLinkUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s deploy-link --image REF --name NAME --port PORT [--env K1,K2] [--health /path] [--server URL] [--launcher URL]

Prints a "Deploy with" link and the README Markdown for it. A visitor who
clicks it enters their own server once and lands on its create-app form with
these values prefilled, nothing is submitted automatically. Env values are
never part of a link, only the names to ask for.

Flags:
`, prog)
}
