package main

import (
	"fmt"
	"io"
)

// runAI dispatches "ai <verb> [args...]" to one of chat/sessions: the
// CLI surface for the in-app AI assistant chat feature
// (internal/api/ai_chat.go), gated behind the same ai-chat experimental
// flag as the dashboard panel and "settings ai-assistant" (see
// experimental_gate.go). A BYOK provider/model/key must also already be
// configured via "settings ai-assistant set", independent of the flag.
func runAI(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, aiUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, aiUsage(prog))
		return exitOK
	case "chat":
		return runAIChat(prog, args[1:], stdout, stderr, lookupEnv)
	case "sessions":
		return runAISessions(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown ai subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, aiUsage(prog))
		return exitUsage
	}
}

func aiUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s ai chat "<message>" [--session ID] [flags]
  %[1]s ai sessions list|get|delete|resolve [flags]

Talks to the in-app AI assistant (read-and-suggest only: it never
deploys, rolls back, or restarts anything without an explicit approval,
see "%[1]s ai sessions resolve -h"). Requires the ai-chat experimental
flag (see "%[1]s settings ai-assistant -h") and a configured provider/
model/key ("%[1]s settings ai-assistant set").

Run "%[1]s ai <subcommand> -h" for a subcommand's own flags.
`, prog)
}
