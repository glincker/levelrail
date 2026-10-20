package main

import (
	"flag"
	"fmt"
	"io"
)

// dbAccessCall is the parsed shape every databases users/access/network
// subcommand shares: a client, the positional arguments and the output flags.
type dbAccessCall struct {
	client  *Client
	rest    []string
	jsonOut bool
	of      outputFlags
	stdout  io.Writer
	stderr  io.Writer
}

// parseDBAccess parses one subcommand's flags. setup registers its own flags
// before parsing; wantArgs is how many positional arguments it takes. ok is
// false when the caller should return code (help, bad flags or usage).
func parseDBAccess(prog, label, usage string, args []string, wantArgs int, argLabel string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), setup func(fs *flag.FlagSet)) (call dbAccessCall, code int, ok bool) {
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, label, "print the result as JSON to stdout and nothing else", stderr)
	if setup != nil {
		setup(fs)
	}
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s %s\n\nFlags:\n", prog, usage)
		fs.PrintDefaults()
	}
	token, apiURL, profile, jsonOut, of, exitCode, parsed := parseAPIFlags(fs, args, apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !parsed {
		return dbAccessCall{}, exitCode, false
	}
	rest, argsOK := requireArgs(fs, stderr, prog, label, argLabel, wantArgs)
	if !argsOK {
		return dbAccessCall{}, exitUsage, false
	}
	return dbAccessCall{
		client: apiClientFromFlags(prog, apiURL, token, profile, lookupEnv), rest: rest,
		jsonOut: jsonOut, of: of, stdout: stdout, stderr: stderr,
	}, exitOK, true
}

func (c dbAccessCall) fail(err error) int { return reportError(c.stdout, c.stderr, c.jsonOut, err) }

func (c dbAccessCall) result(value any, plain func()) int {
	return writeScheduledTaskResult(c.stdout, c.stderr, c.of, value, plain)
}
