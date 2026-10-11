package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

const policyJSONUsage = "print the result as JSON to stdout and nothing else"

// policyInvocation is one parsed "domains <kind> <verb> <app> <domain> [value]" call.
type policyInvocation struct {
	client  *Client
	app     string
	domain  string
	extra   []string
	jsonOut bool
	of      outputFlags
}

type policyCmdSpec struct {
	label    string
	argsHelp string
	summary  string
	extra    int
	define   func(fs *flag.FlagSet)
}

// parsePolicyCall parses flags plus <app> <domain> and spec.extra trailing values.
func parsePolicyCall(prog string, args []string, stderr io.Writer, lookupEnv func(string) (string, bool), spec policyCmdSpec) (policyInvocation, int, bool) {
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, spec.label, policyJSONUsage, stderr)
	if spec.define != nil {
		spec.define(fs)
	}
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s %s %s [flags]\n\n%s\n\nFlags:\n", prog, spec.label, spec.argsHelp, spec.summary)
		fs.PrintDefaults()
	}
	token, apiURL, profile, jsonOut, of, code, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return policyInvocation{}, code, false
	}
	rest, ok := requireArgs(fs, stderr, prog, spec.label, spec.argsHelp, 2+spec.extra)
	if !ok {
		return policyInvocation{}, exitUsage, false
	}
	return policyInvocation{
		client: apiClientFromFlags(prog, apiURL, token, profile, lookupEnv),
		app:    rest[0], domain: rest[1], extra: rest[2:], jsonOut: jsonOut, of: of,
	}, exitOK, true
}

// policyVerb is one "domains <kind> <verb>" handler.
type policyVerb func(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int

// dispatchPolicy routes args[0] to verbs, printing usage on help or an unknown verb.
func dispatchPolicy(prog, kind string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), verbs func(args []string) policyVerb, usage string) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, usage)
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, usage)
		return exitOK
	}
	verb := verbs(args)
	if verb == nil {
		_, _ = fmt.Fprintf(stderr, "%s: unknown domains %s subcommand %q\n\n", prog, kind, args[0])
		_, _ = fmt.Fprint(stderr, usage)
		return exitUsage
	}
	return verb(prog, args[1:], stdout, stderr, lookupEnv)
}

// readStrictJSON decodes a policy file, refusing unknown fields so a typo
// fails here rather than being silently dropped.
func readStrictJSON(path string, dst any) error {
	data, err := os.ReadFile(path) //nolint:gosec // operator-supplied local path, the CLI's documented input
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}

func splitCSVUpper(s string) []string {
	out := splitCSV(s)
	for i := range out {
		out[i] = strings.ToUpper(out[i])
	}
	return out
}

func splitCSVInts(s string) ([]int, error) {
	var out []int
	for _, p := range splitCSV(s) {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, fmt.Errorf("%q is not a number", p)
		}
		out = append(out, n)
	}
	return out, nil
}

func usageError(stderr io.Writer, prog, msg string) int {
	_, _ = fmt.Fprintf(stderr, "%s: %s\n", prog, msg)
	return exitUsage
}

func bg() context.Context { return context.Background() }

// Purge scopes, matching internal/ingress without pulling Caddy into the CLI.
const (
	purgeScopeURL    = "url"
	purgeScopePrefix = "prefix"
	purgeScopeAll    = "all"
)

func readFileString(path string) (string, error) {
	data, err := os.ReadFile(path) //nolint:gosec // operator-supplied local path, the CLI's documented input
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return string(data), nil
}

func domainsPoliciesUsageLines(prog string) string {
	return fmt.Sprintf(`  %[1]s domains headers <verb> <app> <domain>      request and response header rules and presets
  %[1]s domains forwarders <verb> <app> <domain>   path rules that forward to another app, a URL, or redirect
  %[1]s domains geo <verb> <app> <domain>          allow or deny visitors by country
  %[1]s domains cache <verb> <app> <domain>        ingress response cache rules, purge and stats
  %[1]s domains redirects <verb> <app> <domain>    force HTTPS, www and apex, aliases, trailing slash
  %[1]s domains ports <verb> <app> <domain>        the app's raw TCP/UDP ports and who may reach them
`, prog)
}
