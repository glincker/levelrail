package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runAuthSignInRequests implements "auth sign-in-requests list|reveal".
// Unlike "auth code", list never reveals, so it works with a list-only token.
func runAuthSignInRequests(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, authSignInRequestsUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, authSignInRequestsUsage(prog))
		return exitOK
	case "list":
		return runAuthSignInRequestsList(prog, args[1:], stdout, stderr, lookupEnv)
	case "reveal":
		return runAuthSignInRequestsReveal(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown auth sign-in-requests subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, authSignInRequestsUsage(prog))
		return exitUsage
	}
}

func authSignInRequestsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s auth sign-in-requests list [flags]           waiting sign-in codes and new-browser sign-ins, no code shown
  %[1]s auth sign-in-requests reveal <code-id> [flags] print one waiting sign-in code, once

Listing needs write:sensitive or signin:approve. reveal prints the code to
stdout only; it is never saved. Only use a code if you started that sign-in.

`+signInApproveHelp(prog), prog)
}

func runAuthSignInRequestsList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "auth sign-in-requests list", "print the waiting requests as JSON", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, authSignInRequestsUsage(prog)+"\nFlags:\n")
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if _, ok := requireArgs(fs, stderr, prog, "auth sign-in-requests list", "no arguments", 0); !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	reqs, err := client.ListSignInRequests(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list sign-in requests: %w", err))
	}
	if reqs.Codes == nil {
		reqs.Codes = []apiclient.SignInCode{}
	}
	if reqs.Approvals == nil {
		reqs.Approvals = []apiclient.SignInApproval{}
	}
	return writeScheduledTaskResult(stdout, stderr, of, reqs, func() {
		if len(reqs.Codes) == 0 && len(reqs.Approvals) == 0 {
			_, _ = fmt.Fprintln(stdout, "No sign-in is waiting for you.")
			return
		}
		for _, c := range reqs.Codes {
			hint := "code no longer available"
			if c.Revealable {
				hint = "show with: " + prog + " auth sign-in-requests reveal " + c.ID
			}
			_, _ = fmt.Fprintf(stdout, "code request %s  from %s  %s  expires %s  (%s)\n", c.ID, c.RequesterIP, c.UserAgent,
				c.ExpiresAt.Local().Format(time.Kitchen), hint)
		}
		for _, a := range reqs.Approvals {
			_, _ = fmt.Fprintf(stdout, "new browser %s  from %s  %s  requested %s  (decide with %s auth approve|deny %s)\n",
				a.ID, a.RequesterIP, a.UserAgent, a.CreatedAt.Local().Format(time.Kitchen), prog, a.ID)
		}
	})
}

func runAuthSignInRequestsReveal(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "auth sign-in-requests reveal", "print the code and its expiry as JSON", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprint(stderr, authSignInRequestsUsage(prog)+"\nFlags:\n")
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "auth sign-in-requests reveal", "exactly one code request id", 1)
	if !ok {
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	revealed, err := client.RevealLoginCode(context.Background(), rest[0])
	if err != nil {
		code := reportError(stdout, stderr, jsonOut, fmt.Errorf("reveal sign-in code: %w", err))
		// The server decides who may reveal; say how to get that token, never retry another way.
		var apiErr *apiError
		if !jsonOut && errors.As(err, &apiErr) && (apiErr.StatusCode == http.StatusForbidden || apiErr.StatusCode == http.StatusUnauthorized) {
			_, _ = fmt.Fprint(stderr, "\n"+signInApproveHelp(prog))
		}
		return code
	}
	return writeScheduledTaskResult(stdout, stderr, of, revealed, func() {
		_, _ = fmt.Fprintln(stdout, revealed.Code)
		_, _ = fmt.Fprintf(stderr, "expires %s; only type it into a sign-in you started yourself\n", revealed.ExpiresAt.Local().Format(time.Kitchen))
	})
}
