package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/staticroutes"
)

func runStaticRoutes(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, staticRoutesUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, staticRoutesUsage(prog))
		return exitOK
	case "show":
		return runStaticRoutesShow(prog, args[1:], stdout, stderr, lookupEnv)
	case "test":
		return runStaticRoutesTest(prog, args[1:], stdout, stderr, lookupEnv)
	case "lint":
		return runStaticRoutesLint(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown static-sites routes subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, staticRoutesUsage(prog))
		return exitUsage
	}
}

func staticRoutesUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s static-sites routes show <app> [flags]
  %[1]s static-sites routes test <app> <path> [--method M] [--request-query Q] [flags]
  %[1]s static-sites routes lint <app> [--redirects FILE] [--headers FILE] [flags]

show lists the redirect, rewrite and header rules the live deploy serves, with
the problems found when it was published. test runs one request through them
and says which rule or file answers. lint parses local _redirects and
_headers files without deploying.

Run "%[1]s static-sites routes <subcommand> -h" for its flags.
`, prog)
}

func runStaticRoutesShow(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, "static-sites routes show", "print the rules as JSON to stdout and nothing else", stderr)
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	name, ok := requireOneArg(fs, stderr, prog, "static-sites routes show", "app name")
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.GetStaticRoutes(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get static routes for %q: %w", name, err))
	}
	return renderOrFail(stdout, stderr, of, res, func() { printStaticRoutes(stdout, res.Ruleset, res.Published) })
}

func runStaticRoutesTest(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, "static-sites routes test", "print the decision as JSON to stdout and nothing else", stderr)
	method := fs.String("method", "GET", "HTTP method to test")
	reqQuery := fs.String("request-query", "", "query string of the test request, without the leading ?")
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "static-sites routes test", "an app name and a path", 2)
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	d, err := client.TestStaticRoutes(context.Background(), rest[0], apiclient.StaticRoutesTestRequest{Method: *method, Path: rest[1], Query: *reqQuery})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("test static routes for %q: %w", rest[0], err))
	}
	return renderOrFail(stdout, stderr, of, d, func() {
		_, _ = fmt.Fprintf(stdout, "%s %s -> %d %s\n", *method, rest[1], d.Status, describeDecision(d))
		for _, line := range d.Trace {
			_, _ = fmt.Fprintf(stdout, "  %s\n", line)
		}
	})
}

func runStaticRoutesLint(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, "static-sites routes lint", "print the parse result as JSON to stdout and nothing else", stderr)
	redirects := fs.String("redirects", "", "path of a _redirects file to check")
	headers := fs.String("headers", "", "path of a _headers file to check")
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	name, ok := requireOneArg(fs, stderr, prog, "static-sites routes lint", "app name")
	if !ok {
		return exitUsage
	}
	if *redirects == "" && *headers == "" {
		_, _ = fmt.Fprintf(stderr, "%s: static-sites routes lint needs --redirects or --headers\n", prog)
		return exitUsage
	}
	req := apiclient.StaticRoutesLintRequest{}
	for _, f := range []struct {
		path string
		dst  *string
	}{{*redirects, &req.Redirects}, {*headers, &req.Headers}} {
		if f.path == "" {
			continue
		}
		raw, err := os.ReadFile(f.path)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("read %s: %w", f.path, err))
		}
		*f.dst = string(raw)
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.LintStaticRoutes(context.Background(), name, req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("lint static routes: %w", err))
	}
	code := renderOrFail(stdout, stderr, of, res, func() {
		if res.Error != "" {
			_, _ = fmt.Fprintf(stdout, "error: %s\n", res.Error)
		}
		printStaticRoutes(stdout, res.Ruleset, true)
	})
	if code == exitOK && (res.Error != "" || len(res.Problems) > 0) {
		return exitValidation
	}
	return code
}

func renderOrFail(stdout, stderr io.Writer, of outputFlags, data any, table func()) int {
	if err := renderResult(stdout, of.Format, of.Query, data, table); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func describeDecision(d staticroutes.Decision) string {
	switch d.Kind {
	case staticroutes.KindRedirect:
		return "redirect to " + d.Location + ruleSuffix(d.Rule)
	case staticroutes.KindServe:
		return "serve " + d.File + ruleSuffix(d.Rule)
	}
	if d.File != "" {
		return "serve " + d.File + ruleSuffix(d.Rule)
	}
	return "plain response" + ruleSuffix(d.Rule)
}

func ruleSuffix(rule string) string {
	if rule == "" {
		return ""
	}
	return " (rule " + rule + ")"
}

func printStaticRoutes(out io.Writer, rs staticroutes.Ruleset, published bool) {
	if !published {
		_, _ = fmt.Fprintln(out, "this site was published before routing rules existed; deploy again to enable them")
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "SOURCE\tFROM\tTO\tSTATUS")
	for _, r := range rs.Rules {
		status := fmt.Sprint(r.Status)
		if r.Force {
			status += "!"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", r.Source, r.From, r.To, status)
	}
	_ = tw.Flush()
	for _, h := range rs.Headers {
		names := make([]string, 0, len(h.Set)+len(h.Remove))
		for _, kv := range h.Set {
			names = append(names, kv.Name)
		}
		for _, n := range h.Remove {
			names = append(names, "!"+n)
		}
		_, _ = fmt.Fprintf(out, "headers %s %s: %s\n", h.Source, h.Path, strings.Join(names, ", "))
	}
	o := rs.Options
	_, _ = fmt.Fprintf(out, "options: spa=%t clean_urls=%t trailing_slash=%s not_found=%s\n", o.SPA, o.CleanURLs, valueOr(o.TrailingSlash, "off"), valueOr(o.NotFound, "404.html"))
	for _, p := range rs.Problems {
		_, _ = fmt.Fprintf(out, "%s: %s:%d: %s\n", p.Severity, p.Source, p.Line, p.Message)
	}
}

func valueOr(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
