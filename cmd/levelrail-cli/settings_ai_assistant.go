package main

import (
	"context"
	"fmt"
	"io"
)

// runSettingsAIAssistant dispatches "settings ai-assistant <verb> [flags]"
// for the BYOK assistant (internal/api/ai_settings.go, ai_providers.go).
func runSettingsAIAssistant(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, settingsAIAssistantUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, settingsAIAssistantUsage(prog))
		return exitOK
	case "get":
		return runSettingsAIAssistantGet(prog, args[1:], stdout, stderr, lookupEnv)
	case "providers":
		return runSettingsAIProviders(prog, args[1:], stdout, stderr, lookupEnv)
	case "use":
		return runSettingsAIUse(prog, args[1:], stdout, stderr, lookupEnv)
	case "fallback":
		return runSettingsAIFallbackSet(prog, args[1:], stdout, stderr, lookupEnv)
	case "usage":
		return runSettingsAIUsage(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown settings ai-assistant subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, settingsAIAssistantUsage(prog))
		return exitUsage
	}
}

func settingsAIAssistantUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s settings ai-assistant get [flags]
  %[1]s settings ai-assistant providers list|set|test|remove|models [flags]
  %[1]s settings ai-assistant use <provider> <model> [flags]
  %[1]s settings ai-assistant fallback set [provider:model ...] [flags]
  %[1]s settings ai-assistant usage [--days N] [flags]

Configures the BYOK (bring your own key) AI assistant. "providers" manages
keys for every supported provider; "use" and "fallback" pick the models. Keys are stored via envelope encryption and never shown back.

Run "%[1]s settings ai-assistant <subcommand> -h" for a subcommand's own flags.
`, prog)
}

func runSettingsAIAssistantGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runListCommand(prog, args, stdout, stderr, lookupEnv, listCommandParams[aiAssistantSettingsResource]{
		cmdLabel:  "settings ai-assistant get",
		jsonUsage: "print the ai-assistant settings as JSON to stdout and nothing else",
		usageText: fmt.Sprintf("Usage:\n  %s settings ai-assistant get [flags]\n\nShows the current AI assistant settings. The key is never returned, only\nwhether one is stored.\n\nFlags:\n", prog),
		fetch: func(c *Client, ctx context.Context) (aiAssistantSettingsResource, error) {
			return c.GetAIAssistantSettings(ctx)
		},
		errVerb: "get ai-assistant settings",
		print:   printAIAssistantSettingsHuman,
	})
}

func printAIAssistantSettingsHuman(out io.Writer, s aiAssistantSettingsResource) {
	_, _ = fmt.Fprintf(out, "configured: %v\n", s.Configured)
	if s.Active != nil {
		_, _ = fmt.Fprintf(out, "active:     %s:%s\n", s.Active.Provider, s.Active.Model)
	}
	for i, f := range s.Fallback {
		_, _ = fmt.Fprintf(out, "fallback %d:  %s:%s\n", i+1, f.Provider, f.Model)
	}
	if s.Limits.MonthlyTokenBudget > 0 && s.TokenUsageMont != nil {
		_, _ = fmt.Fprintf(out, "budget:     %d of %d tokens this month\n", *s.TokenUsageMont, s.Limits.MonthlyTokenBudget)
	}
}
