package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

const aiUsageDefaultDays = 30

// runSettingsAIProviders dispatches "settings ai-assistant providers <verb>".
func runSettingsAIProviders(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, settingsAIProvidersUsage(prog))
		return exitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, settingsAIProvidersUsage(prog))
		return exitOK
	case "list":
		return runListCommand(prog, args[1:], stdout, stderr, lookupEnv, listCommandParams[[]apiclient.AIProviderResource]{
			cmdLabel:  "settings ai-assistant providers list",
			jsonUsage: "print the providers as JSON to stdout and nothing else",
			usageText: fmt.Sprintf("Usage:\n  %s settings ai-assistant providers list [flags]\n\nLists every AI provider with whether a key is saved and its last test status.\n\nFlags:\n", prog),
			fetch: func(c *Client, ctx context.Context) ([]apiclient.AIProviderResource, error) {
				return c.ListAIProviders(ctx)
			},
			errVerb: "list ai providers",
			print:   printAIProviders,
		})
	case "set":
		return runSettingsAIProviderSet(prog, args[1:], stdout, stderr, lookupEnv)
	case "test":
		return runAIProviderIDCommand(prog, "test", args[1:], stdout, stderr, lookupEnv, func(c *Client, ctx context.Context, id string) (any, func(io.Writer), error) {
			res, err := c.TestAIProvider(ctx, id)
			return res, func(w io.Writer) {
				_, _ = fmt.Fprintf(w, "ok:      %v\nstatus:  %s\nlatency: %d ms\n", res.OK, res.Status, res.LatencyMS)
				if res.StatusDetail != "" {
					_, _ = fmt.Fprintf(w, "detail:  %s\n", res.StatusDetail)
				}
			}, err
		})
	case "remove":
		return runAIProviderIDCommand(prog, "remove", args[1:], stdout, stderr, lookupEnv, func(c *Client, ctx context.Context, id string) (any, func(io.Writer), error) {
			res, err := c.DeleteAIProvider(ctx, id)
			return res, func(w io.Writer) { _, _ = fmt.Fprintf(w, "removed the key for %s\n", id) }, err
		})
	case "models":
		return runAIProviderIDCommand(prog, "models", args[1:], stdout, stderr, lookupEnv, func(c *Client, ctx context.Context, id string) (any, func(io.Writer), error) {
			res, err := c.ListAIProviderModels(ctx, id)
			return res, func(w io.Writer) { printAIModels(w, res) }, err
		})
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown settings ai-assistant providers subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, settingsAIProvidersUsage(prog))
		return exitUsage
	}
}

func settingsAIProvidersUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s settings ai-assistant providers list [flags]
  %[1]s settings ai-assistant providers set <id> [--api-key KEY | --api-key-env VAR] [--base-url URL] [--default-model ID] [flags]
  %[1]s settings ai-assistant providers test <id> [flags]
  %[1]s settings ai-assistant providers remove <id> [flags]
  %[1]s settings ai-assistant providers models <id> [flags]

Keys are checked with the provider before they are stored and are never
shown back. Prefer --api-key-env so the key stays out of shell history.
`, prog)
}

func runSettingsAIProviderSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "settings ai-assistant providers set", "print the saved provider as JSON to stdout and nothing else", stderr)
	var apiKey, apiKeyEnv, baseURL, defaultModel string
	fs.StringVar(&apiKey, "api-key", "", "provider API key")
	fs.StringVar(&apiKeyEnv, "api-key-env", "", "name of an environment variable holding the API key")
	fs.StringVar(&baseURL, "base-url", "", "endpoint URL (only for providers that allow one, such as ollama or openai_compatible)")
	fs.StringVar(&defaultModel, "default-model", "", "model id to prefer for this provider")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s settings ai-assistant providers set <id> [flags]\n\nValidates and stores a provider key, base URL and default model.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if fs.NArg() != 1 {
		_, _ = fmt.Fprintf(stderr, "%s: settings ai-assistant providers set requires a provider id\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	if apiKeyEnv != "" {
		v, found := lookupEnv(apiKeyEnv)
		if !found || v == "" {
			_, _ = fmt.Fprintf(stderr, "%s: environment variable %s is not set\n", prog, apiKeyEnv)
			return exitUsage
		}
		apiKey = v
	}
	if apiKey == "" && baseURL == "" && defaultModel == "" {
		_, _ = fmt.Fprintf(stderr, "%s: settings ai-assistant providers set needs --api-key, --api-key-env, --base-url or --default-model\n", prog)
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.SetAIProvider(context.Background(), fs.Arg(0), apiclient.SetAIProviderRequest{APIKey: apiKey, BaseURL: baseURL, DefaultModel: defaultModel})
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set ai provider: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
		printAIProviders(stdout, []apiclient.AIProviderResource{res})
	})
}

// runAIProviderIDCommand runs the shared "<verb> <id>" shape of test/remove/models.
func runAIProviderIDCommand(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool),
	call func(*Client, context.Context, string) (any, func(io.Writer), error)) int {
	label := "settings ai-assistant providers " + verb
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, label, "print the result as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s %s <id> [flags]\n\nFlags:\n", prog, label)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if fs.NArg() != 1 {
		_, _ = fmt.Fprintf(stderr, "%s: %s requires a provider id\n\n", prog, label)
		fs.Usage()
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	value, plain, err := call(client, context.Background(), fs.Arg(0))
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s: %w", label, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, value, func() { plain(stdout) })
}

func printAIProviders(out io.Writer, list []apiclient.AIProviderResource) {
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tNAME\tKEY\tSTATUS\tBASE URL")
	for _, p := range list {
		key := "no"
		if p.HasKey {
			key = "yes"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", p.ID, p.Label, key, p.Status, p.BaseURL)
	}
	_ = tw.Flush()
}

func printAIModels(out io.Writer, list []apiclient.AIModelResource) {
	if len(list) == 0 {
		_, _ = fmt.Fprintln(out, "the provider returned no models; you can still type a model id by hand")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "ID\tCONTEXT\tTOOLS")
	for _, m := range list {
		ctxWin, tools := "-", "-"
		if m.ContextWindow > 0 {
			ctxWin = fmt.Sprint(m.ContextWindow)
		}
		if m.SupportsTools != nil {
			tools = fmt.Sprint(*m.SupportsTools)
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", m.ID, ctxWin, tools)
	}
	_ = tw.Flush()
}

// runSettingsAIUse handles "settings ai-assistant use <provider> <model>".
func runSettingsAIUse(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	return runAIChoiceCommand(prog, "use", args, stdout, stderr, lookupEnv, func(refs []apiclient.AIModelRef) (apiclient.AIChoiceRequest, string) {
		if len(refs) != 1 {
			return apiclient.AIChoiceRequest{}, "use takes <provider> <model>"
		}
		return apiclient.AIChoiceRequest{Active: &refs[0]}, ""
	})
}

// runSettingsAIFallbackSet handles "settings ai-assistant fallback set <provider:model>...".
func runSettingsAIFallbackSet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || args[0] != "set" {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s settings ai-assistant fallback set [provider:model ...] [flags]\n\nNo entries clears the chain.\n", prog)
		return exitUsage
	}
	return runAIChoiceCommand(prog, "fallback set", args[1:], stdout, stderr, lookupEnv, func(refs []apiclient.AIModelRef) (apiclient.AIChoiceRequest, string) {
		return apiclient.AIChoiceRequest{Fallback: &refs}, ""
	})
}

// runAIChoiceCommand parses positional pairs ("provider model" for use,
// "provider:model" entries for fallback) and sends the choice.
func runAIChoiceCommand(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool),
	build func([]apiclient.AIModelRef) (apiclient.AIChoiceRequest, string)) int {
	label := "settings ai-assistant " + verb
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, label, "print the updated settings as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s %s <provider> <model> | <provider:model>...\n\nFlags:\n", prog, label)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	var refs []apiclient.AIModelRef
	pos := fs.Args()
	if verb == "use" {
		if len(pos) != 2 {
			_, _ = fmt.Fprintf(stderr, "%s: %s takes <provider> <model>\n", prog, label)
			return exitUsage
		}
		refs = []apiclient.AIModelRef{{Provider: pos[0], Model: pos[1]}}
	} else {
		refs = []apiclient.AIModelRef{}
		for _, a := range pos {
			p, m, found := strings.Cut(a, ":")
			if !found || p == "" || m == "" {
				_, _ = fmt.Fprintf(stderr, "%s: %q is not provider:model\n", prog, a)
				return exitUsage
			}
			refs = append(refs, apiclient.AIModelRef{Provider: p, Model: m})
		}
	}
	req, msg := build(refs)
	if msg != "" {
		_, _ = fmt.Fprintf(stderr, "%s: %s\n", prog, msg)
		return exitUsage
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.SetAIChoice(context.Background(), req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("%s: %w", label, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() { printAIAssistantSettingsHuman(stdout, res) })
}

// runSettingsAIUsage handles "settings ai-assistant usage [--days N]".
func runSettingsAIUsage(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "settings ai-assistant usage", "print the usage as JSON to stdout and nothing else", stderr)
	days := fs.Int("days", aiUsageDefaultDays, "number of days to report (1 to 365)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s settings ai-assistant usage [--days N] [flags]\n\nToken usage per day, provider and model.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	res, err := client.GetAIUsage(context.Background(), *days)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get ai usage: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() { printAIUsage(stdout, res) })
}

func printAIUsage(out io.Writer, u apiclient.AIUsageResource) {
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "DATE\tPROVIDER\tMODEL\tPROMPT\tCOMPLETION\tREQUESTS\tERRORS")
	for _, d := range u.Days {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%d\t%d\t%d\n", d.Date, d.Provider, d.Model, d.PromptTokens, d.CompletionTokens, d.Requests, d.Errors)
	}
	_, _ = fmt.Fprintf(tw, "total\t\t\t%d\t%d\t%d\t%d\n", u.Totals.PromptTokens, u.Totals.CompletionTokens, u.Totals.Requests, u.Totals.Errors)
	_ = tw.Flush()
}
