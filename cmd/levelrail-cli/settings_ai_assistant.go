package main

import (
	"context"
	"fmt"
	"io"
)

// runSettingsAIAssistant dispatches "settings ai-assistant <verb>
// [flags]" to one of get/set/clear: the singleton BYOK AI assistant
// provider/model/key configuration (internal/api/ai_settings.go),
// mirroring runSettingsEmail's own get/set shape plus a clear verb for
// DELETE, the same three-verb shape runVault already uses for its own
// singleton external-credential settings.
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
	case "set":
		return runSettingsAIAssistantSet(prog, args[1:], stdout, stderr, lookupEnv)
	case "clear":
		return runSettingsAIAssistantClear(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown settings ai-assistant subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, settingsAIAssistantUsage(prog))
		return exitUsage
	}
}

func settingsAIAssistantUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s settings ai-assistant get [flags]
  %[1]s settings ai-assistant set --model NAME --api-key KEY [flags]
  %[1]s settings ai-assistant clear [flags]

Configures the BYOK (bring your own key) AI assistant: provider is always
"anthropic" (the only one currently supported server-side); model is an
Anthropic model id, e.g. "claude-sonnet-4-5". The key is stored via
envelope encryption and never shown back; "get" reports only whether one
is configured.

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

func runSettingsAIAssistantSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "settings ai-assistant set", "print the updated ai-assistant settings as JSON to stdout and nothing else", stderr)
	var model, apiKey string
	fs.StringVar(&model, "model", "", "Anthropic model id, e.g. \"claude-sonnet-4-5\" (required)")
	fs.StringVar(&apiKey, "api-key", "", "Anthropic API key (required)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s settings ai-assistant set --model NAME --api-key KEY [flags]\n\nConfigures the AI assistant.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if model == "" {
		_, _ = fmt.Fprintf(stderr, "%s: settings ai-assistant set requires --model\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	if apiKey == "" {
		_, _ = fmt.Fprintf(stderr, "%s: settings ai-assistant set requires --api-key\n\n", prog)
		fs.Usage()
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	settings, err := client.UpdateAIAssistantSettings(context.Background(), updateAIAssistantSettingsRequest{
		Provider: "anthropic",
		Model:    model,
		APIKey:   apiKey,
	})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set ai-assistant settings: %w", err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, settings, func() { printAIAssistantSettingsHuman(stdout, settings) })
}

func runSettingsAIAssistantClear(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "settings ai-assistant clear", "print the reset ai-assistant settings as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s settings ai-assistant clear [flags]\n\nClears the stored key and resets provider/model.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}

	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)

	settings, err := client.DeleteAIAssistantSettings(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("clear ai-assistant settings: %w", err))
	}

	return writeScheduledTaskResult(stdout, stderr, of, settings, func() { printAIAssistantSettingsHuman(stdout, settings) })
}

func printAIAssistantSettingsHuman(out io.Writer, s aiAssistantSettingsResource) {
	_, _ = fmt.Fprintf(out, "configured: %v\n", s.Configured)
	_, _ = fmt.Fprintf(out, "provider:   %s\n", s.Provider)
	_, _ = fmt.Fprintf(out, "model:      %s\n", s.Model)
}
