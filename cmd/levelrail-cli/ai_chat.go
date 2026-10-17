package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// Stop on "done" rather than waiting for the server to close the stream right after it.
var errAIChatDone = errors.New("ai chat turn finished")

// Mutating tool calls are only reported, never auto-approved, matching the dashboard's confirmation gate.
func runAIChat(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "ai chat", "print one JSON object per stream event instead of text", stderr)
	var sessionFlag string
	fs.StringVar(&sessionFlag, "session", "", "an existing session id to continue (default: start a new session)")
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, aiChatUsage(prog)) }

	tokenFlag, apiURLFlag, profileFlag, jsonOut, _, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	message, ok := requireOneArg(fs, stderr, prog, "ai chat", "a message")
	if !ok {
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	sessionID := sessionFlag
	if sessionID == "" {
		created, err := client.CreateAIChatSession(ctx)
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("create ai chat session: %w", err))
		}
		sessionID = created.ID
		_, _ = fmt.Fprintf(stderr, "session %s created (pass --session %s to continue it)\n", sessionID, sessionID)
	}

	sawText := false
	err := client.SendAIChatMessage(ctx, sessionID, message, func(ev apiclient.AIChatSSEEvent) error {
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
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("ai chat: %w", err))
	}
	return exitOK
}

// Text deltas print without added newlines so sentences are not split across chunks.
func printAIChatEventHuman(stdout, stderr io.Writer, ev apiclient.AIChatSSEEvent, sawText *bool) {
	switch ev.Type {
	case "text_delta":
		_, _ = fmt.Fprint(stdout, ev.Text)
		*sawText = true
	case "tool_call_proposed":
		_, _ = fmt.Fprintf(stderr, "\n[pending confirmation] %s (confirmation_id=%s)\n  resolve: %s ai sessions resolve <session> %s --approve|--reject\n", ev.Name, ev.ConfirmationID, "levelrail-cli", ev.ConfirmationID)
	case "tool_result":
		if ev.ActingAs != "" {
			_, _ = fmt.Fprintf(stderr, "\n[tool] %s%s\n", ev.Name, aiToolActor(ev.ActingAs, ev.Abilities))
		}
		if ev.IsError {
			_, _ = fmt.Fprintf(stderr, "\n[tool error] %s: %s\n", ev.Name, string(ev.Result))
		}
	case "done":
		// No output: the loop that called this already owns printing
		// the trailing newline once streaming ends.
	}
}

func aiChatUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s ai chat "<message>" [--session ID] [flags]

Sends one message to the AI assistant and streams its reply to stdout
as it arrives. Without --session, starts a fresh session and prints its
id to stderr; pass that id back with --session to continue the same
conversation. A tool call the model proposes that would change state
(deploy, rollback, restart, env change, and so on) always pauses for a
confirmation (reported to stderr), never runs automatically; approve or
reject it with "%[1]s ai sessions resolve <session> <confirmation-id> --approve".

Flags:
  --session string        an existing session id to continue (default: start a new session)
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print one JSON object per stream event instead of text
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}

// aiToolActor renders who a tool call ran as, empty when unknown.
func aiToolActor(actingAs string, abilities []string) string {
	if actingAs == "" {
		return ""
	}
	out := " acting as " + actingAs
	if len(abilities) > 0 {
		out += " (ability " + strings.Join(abilities, ", ") + ")"
	}
	return out
}
