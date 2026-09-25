package main

import (
	"flag"
	"fmt"
	"io"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// cliCall is the parsed common state of one API-backed subcommand.
type cliCall struct {
	client  *apiclient.Client
	fs      *flag.FlagSet
	jsonOut bool
	of      outputFlags
	stdout  io.Writer
	stderr  io.Writer
}

// parseCLICall registers the shared API flags plus bind's own, parses args,
// and builds the client. ok=false means the caller returns code as is.
func parseCLICall(prog, label, usage string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), bind func(*flag.FlagSet)) (c cliCall, code int, ok bool) {
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, label, "print the result as JSON to stdout and nothing else", stderr)
	if bind != nil {
		bind(fs)
	}
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, usage) }
	token, apiURL, profile, jsonOut, of, code, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return cliCall{}, code, false
	}
	return cliCall{
		client: apiClientFromFlags(prog, apiURL, token, profile, lookupEnv), fs: fs, jsonOut: jsonOut, of: of, stdout: stdout, stderr: stderr,
	}, exitOK, true
}

func (c cliCall) fail(err error) int { return reportError(c.stdout, c.stderr, c.jsonOut, err) }

func (c cliCall) invalid(format string, a ...any) int {
	return c.fail(newValidationError(format, a...))
}

func (c cliCall) render(data any, table func()) int {
	if err := renderResult(c.stdout, c.of.Format, c.of.Query, data, table); err != nil {
		_, _ = fmt.Fprintln(c.stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

const commonFlagsHelp = `
Common flags:
  --token string     API token (default: the credentials file or the API token env var)
  --api-url string   control plane base URL
  --profile string   named credentials profile
  --json             print the result as JSON and nothing else
  --output string    json, table, or text
  --query string     JMESPath expression applied to the result
`
