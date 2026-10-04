package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runAISessions dispatches "ai sessions <verb> [args...]" to one of
// list/get/delete/resolve.
func runAISessions(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, aiSessionsUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, aiSessionsUsage(prog))
		return exitOK
	case "list":
		return runAISessionsList(prog, args[1:], stdout, stderr, lookupEnv)
	case "get":
		return runAISessionsGet(prog, args[1:], stdout, stderr, lookupEnv)
	case "delete":
		return runAISessionsDelete(prog, args[1:], stdout, stderr, lookupEnv)
	case "resolve":
		return runAISessionsResolve(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown ai sessions subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, aiSessionsUsage(prog))
		return exitUsage
	}
}

func aiSessionsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s ai sessions list [flags]
  %[1]s ai sessions get <id> [flags]
  %[1]s ai sessions delete <id> [flags]
  %[1]s ai sessions resolve <id> <confirmation-id> --approve|--reject [flags]

Manages AI assistant chat sessions. "%[1]s ai chat" starts and sends to
one; these subcommands list, inspect, remove, and resolve a pending
confirmation on one.

Run "%[1]s ai sessions <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runAISessionsList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runListCommand(prog, args, stdout, stderr, lookupEnv, listCommandParams[[]apiclient.AIChatSessionSummaryResource]{
		cmdLabel:  "ai sessions list",
		jsonUsage: "print the session list as JSON to stdout and nothing else",
		usageText: fmt.Sprintf("Usage:\n  %s ai sessions list [flags]\n\nLists every chat session, most recently updated first. No message content.\n\nFlags:\n", prog),
		fetch: func(c *Client, ctx context.Context) ([]apiclient.AIChatSessionSummaryResource, error) {
			return c.ListAIChatSessions(ctx)
		},
		errVerb: "list ai chat sessions",
		print:   printAISessionsListHuman,
	})
}

func printAISessionsListHuman(out io.Writer, sessions []apiclient.AIChatSessionSummaryResource) {
	if len(sessions) == 0 {
		_, _ = fmt.Fprintln(out, "no chat sessions yet")
		return
	}
	_, _ = fmt.Fprintf(out, "%-24s %-24s %s\n", "ID", "CREATED", "UPDATED")
	for _, s := range sessions {
		_, _ = fmt.Fprintf(out, "%-24s %-24s %s\n", s.ID, s.CreatedAt.Format("2006-01-02T15:04:05Z"), s.UpdatedAt.Format("2006-01-02T15:04:05Z"))
	}
}

func runAISessionsGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "ai sessions get", "print the session transcript as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s ai sessions get <id> [flags]\n\nShows one session's full transcript.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	id, ok := requireOneArg(fs, stderr, prog, "ai sessions get", "session id")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	session, err := client.GetAIChatSession(context.Background(), id)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get ai chat session %q: %w", id, err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, session, func() { printAISessionHuman(stdout, session) })
}

func printAISessionHuman(out io.Writer, s apiclient.AIChatSessionResource) {
	_, _ = fmt.Fprintf(out, "session: %s\n", s.ID)
	for _, m := range s.Messages {
		_, _ = fmt.Fprintf(out, "\n[%s] %s\n", m.Role, m.Content)
		for _, tc := range m.ToolCalls {
			_, _ = fmt.Fprintf(out, "  tool: %s status=%s\n", tc.Name, tc.Status)
		}
	}
}

func runAISessionsDelete(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "ai sessions delete", `print {"deleted": true} as JSON to stdout on success and nothing else`, stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s ai sessions delete <id> [flags]\n\nDeletes a session and its full message history.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	id, ok := requireOneArg(fs, stderr, prog, "ai sessions delete", "session id")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	if err := client.DeleteAIChatSession(context.Background(), id); err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("delete ai chat session %q: %w", id, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, map[string]bool{"deleted": true}, func() {
		_, _ = fmt.Fprintf(stdout, "session %q deleted\n", id)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func runAISessionsResolve(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "ai sessions resolve", "print one JSON object per stream event instead of text", stderr)
	var approve, reject bool
	fs.BoolVar(&approve, "approve", false, "run the pending tool call")
	fs.BoolVar(&reject, "reject", false, "decline the pending tool call")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, aiSessionsResolveUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, _, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest, ok := requireArgs(fs, stderr, prog, "ai sessions resolve", "a session id and a confirmation id", 2)
	if !ok {
		return exitUsage
	}
	sessionID, confirmationID := rest[0], rest[1]
	if approve == reject {
		_, _ = fmt.Fprintf(stderr, "%s: ai sessions resolve requires exactly one of --approve or --reject\n\n", prog)
		fs.Usage()
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	sawText := false
	err := client.ResolveAIChatConfirmation(ctx, sessionID, confirmationID, approve, func(ev apiclient.AIChatSSEEvent) error {
		if jsonOut {
			if err := writeJSONLine(stdout, ev); err != nil {
				return err
			}
		} else {
			printAIChatEventHuman(stdout, stderr, ev, &sawText)
		}
		if ev.Type == "done" {
			return errAIChatDone
		}
		return nil
	})
	if !jsonOut && sawText {
		_, _ = fmt.Fprintln(stdout)
	}
	if err != nil && !errors.Is(err, errAIChatDone) && !errors.Is(err, context.Canceled) {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("resolve ai chat confirmation %q: %w", confirmationID, err))
	}
	return exitOK
}

func aiSessionsResolveUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s ai sessions resolve <session-id> <confirmation-id> --approve|--reject [flags]

Approves or rejects a mutating tool call the assistant proposed
(reported by "%[1]s ai chat" as "[pending confirmation]"), then streams
the turn's continuation the same way "%[1]s ai chat" does.

Flags:
  --approve                run the pending tool call
  --reject                 decline the pending tool call
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print one JSON object per stream event instead of text
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
