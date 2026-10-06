package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

type moveEnvCall func(ctx context.Context, client *Client, name string, req apiclient.MoveEnvironmentRequest) (apiclient.MoveEnvironmentResult, error)

func runAppsMoveEnv(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runMoveEnv(prog, "apps", args, stdout, stderr, lookupEnv, os.Stdin, func(ctx context.Context, c *Client, name string, req apiclient.MoveEnvironmentRequest) (apiclient.MoveEnvironmentResult, error) {
		return c.MoveAppEnvironment(ctx, name, req)
	})
}

func runDatabasesMoveEnv(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runMoveEnv(prog, "databases", args, stdout, stderr, lookupEnv, os.Stdin, func(ctx context.Context, c *Client, name string, req apiclient.MoveEnvironmentRequest) (apiclient.MoveEnvironmentResult, error) {
		return c.MoveDatabaseEnvironment(ctx, name, req)
	})
}

// runMoveEnv implements "apps|databases move-env <name> <environment-id>".
// Moving into or out of a protected environment needs --confirm (or an
// interactive "yes") and then waits for a second person's approval.
func runMoveEnv(prog, noun string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool), stdin io.Reader, call moveEnvCall) int {
	label := noun + " move-env"
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, label, "print the result as JSON to stdout and nothing else", stderr)
	confirm := fs.Bool("confirm", false, "confirm a move into or out of a protected environment, skipping the prompt")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s %s <name> <environment-id> [flags]\n\nMoves it to another environment. Pass an empty id (\"\") to untag it. A move\ninto or out of a protected environment needs --confirm and then waits for\na different user to approve it (%s deploy-approvals).\n\nFlags:\n", prog, label, prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, label, "a name and an environment id", 2)
	if !ok {
		return exitUsage
	}
	name, envID := rest[0], rest[1]
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx := context.Background()

	res, err := call(ctx, client, name, apiclient.MoveEnvironmentRequest{EnvironmentID: envID, Confirm: *confirm})
	if apiErr, protected := protectedEnvironmentError(err); protected && !*confirm {
		yes, cerr := resolveProtectedEnvironmentConfirmation(apiErr.Message, stdin, stderr)
		if cerr != nil {
			return reportError(stdout, stderr, jsonOut, cerr)
		}
		if yes {
			res, err = call(ctx, client, name, apiclient.MoveEnvironmentRequest{EnvironmentID: envID, Confirm: true})
		}
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("move %s %q to environment %q: %w", noun, name, envID, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
		if res.PendingApproval != nil {
			_, _ = fmt.Fprintf(stdout, "move of %q is pending approval (id %s): a different, sufficiently privileged user must approve it\ncheck status: %s deploy-approvals get %s\n", name, res.PendingApproval.ID, prog, res.PendingApproval.ID)
			return
		}
		_, _ = fmt.Fprintf(stdout, "%s %q moved to environment %q\n", noun, name, envID)
	})
}
