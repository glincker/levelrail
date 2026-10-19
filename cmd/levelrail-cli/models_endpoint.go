package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// runModelsEndpoint dispatches "models endpoint <verb>" for model servers
// Levelrail does not run (Ollama, LM Studio, an MLX server on a Mac).
func runModelsEndpoint(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, modelsEndpointUsage(prog))
		return exitUsage
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, modelsEndpointUsage(prog))
		return exitOK
	case "add":
		return runModelsEndpointAdd(prog, rest, stdout, stderr, lookupEnv)
	case "test":
		return runModelsEndpointTest(prog, rest, stdout, stderr, lookupEnv)
	case "routing":
		return runModelsEndpointRouting(prog, rest, stdout, stderr, lookupEnv)
	case "credential":
		return runModelsEndpointCredential(prog, rest, stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown models endpoint subcommand %q\n\n", prog, sub)
		_, _ = fmt.Fprint(stderr, modelsEndpointUsage(prog))
		return exitUsage
	}
}

func modelsEndpointUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s models endpoint add --name N --url URL [flags]   register a model server Levelrail does not run
  %[1]s models endpoint test <name> | --url URL          probe an endpoint and list its models
  %[1]s models endpoint routing <name> [flags]           fall back to another model when the endpoint is down or slow
  %[1]s models endpoint credential <name> [flags]        set or remove the endpoint's API key

Delete a registered endpoint with "%[1]s models delete <name>". Keys, usage and limits use the
regular "models keys" and "models usage" commands.
`, prog)
}

func credentialFromEnv(envName string, lookupEnv func(string) (string, bool)) (string, error) {
	if envName == "" {
		return "", nil
	}
	v, _ := lookupEnv(envName)
	v = strings.TrimSpace(v)
	if v == "" {
		return "", newValidationError("%s is empty: export the credential there, it is never taken from a flag", envName)
	}
	return v, nil
}

func runModelsEndpointAdd(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "models endpoint add", "print the model and its one-time API key as JSON to stdout and nothing else", stderr)
	var req apiclient.CreateModelRequest
	ext := apiclient.ExternalEndpointRequest{}
	var credEnv string
	var budget time.Duration
	fs.StringVar(&req.Name, "name", "", "model name used in the gateway hostname (lowercase letters, digits, hyphens)")
	fs.StringVar(&ext.BaseURL, "url", "", "server base URL, for example http://100.64.0.7:11434")
	fs.StringVar(&ext.APIType, "api", "openai", "openai (OpenAI-compatible, LM Studio, MLX, vLLM) or ollama")
	fs.StringVar(&credEnv, "credential-from-env", "", "environment variable holding the server's API key, stored encrypted")
	fs.StringVar(&req.Domain, "domain", "", "hostname for the gateway endpoint (default: a zero-config hostname)")
	fs.StringVar(&ext.FallbackModel, "fallback", "", "another model to use when this endpoint is down or over its latency budget (default: off)")
	fs.StringVar(&ext.FallbackModelRef, "fallback-model-id", "", "model id to send to the fallback (default: the fallback model's own id)")
	fs.DurationVar(&budget, "latency-budget", 0, "with --fallback: time to first byte before falling back, for example 3s (default: no budget)")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s models endpoint add --name N --url URL [--api ollama] [--credential-from-env VAR] [--domain HOST] [--fallback MODEL] [--fallback-model-id ID] [--latency-budget 3s]\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	if req.Name == "" || ext.BaseURL == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--name and --url are required"))
	}
	cred, err := credentialFromEnv(credEnv, lookupEnv)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	ext.Credential, ext.LatencyBudgetMS = cred, budget.Milliseconds()
	req.External = &ext
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	created, err := client.CreateModelDetail(context.Background(), req)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("register endpoint %q: %w", req.Name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, created, func() {
		_, _ = fmt.Fprintf(stdout, "endpoint %q registered\n", created.Name)
		printEndpointHealth(stdout, created.External)
		if created.EndpointURL != "" {
			_, _ = fmt.Fprintf(stdout, "base url:  %s\n", created.EndpointURL)
		}
		_, _ = fmt.Fprintf(stdout, "api key:   %s\n", created.APIKey)
		_, _ = fmt.Fprintln(stdout, "Save the API key now, it is not shown again. Create scoped keys with \"models keys create\".")
	})
}

func printEndpointHealth(out io.Writer, e *apiclient.ExternalEndpoint) {
	if e == nil {
		return
	}
	_, _ = fmt.Fprintf(out, "health:    %s (%d ms)\n", e.Health.State, e.Health.LatencyMS)
	if d := e.Health.Diagnosis; d != nil {
		_, _ = fmt.Fprintf(out, "problem:   %s (%s)\n", d.Message, d.Code)
	}
	if len(e.Models) > 0 {
		_, _ = fmt.Fprintf(out, "models:    %s\n", strings.Join(e.Models, ", "))
	}
}

func runModelsEndpointTest(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "models endpoint test", "print the probe result as JSON to stdout and nothing else", stderr)
	var rawURL, apiType, credEnv string
	fs.StringVar(&rawURL, "url", "", "probe this base URL before registering it (instead of a registered name)")
	fs.StringVar(&apiType, "api", "openai", "with --url: openai or ollama")
	fs.StringVar(&credEnv, "credential-from-env", "", "with --url: environment variable holding the API key")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s models endpoint test <name>\n  %s models endpoint test --url URL [--api ollama] [--credential-from-env VAR]\n\nCalls the server's list-models route and reports health, latency and the models found.\n\nFlags:\n", prog, prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	name := fs.Arg(0)
	if (rawURL == "") == (name == "") {
		return reportError(stdout, stderr, jsonOut, newValidationError("give either a registered endpoint name or --url"))
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	var res apiclient.EndpointTest
	var err error
	if rawURL != "" {
		cred, cerr := credentialFromEnv(credEnv, lookupEnv)
		if cerr != nil {
			return reportError(stdout, stderr, jsonOut, cerr)
		}
		res, err = client.TestModelEndpointInput(context.Background(), apiclient.TestEndpointRequest{BaseURL: rawURL, APIType: apiType, Credential: cred})
	} else {
		res, err = client.TestModelEndpoint(context.Background(), name)
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("test endpoint: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, res, func() {
		_, _ = fmt.Fprintf(stdout, "state:   %s (%d ms)\n", res.State, res.LatencyMS)
		if d := res.Diagnosis; d != nil {
			_, _ = fmt.Fprintf(stdout, "problem: %s (%s)\n", d.Message, d.Code)
		}
		if len(res.Models) > 0 {
			_, _ = fmt.Fprintf(stdout, "models:  %s\n", strings.Join(res.Models, ", "))
		}
	})
}

func runModelsEndpointRouting(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "models endpoint routing", "print the model as JSON to stdout and nothing else", stderr)
	var fallback, ref string
	var budget time.Duration
	off := fs.Bool("off", false, "turn the fallback policy off")
	fs.StringVar(&fallback, "fallback", "", "model to use when the endpoint is down or over its latency budget")
	fs.StringVar(&ref, "fallback-model-id", "", "model id to send to the fallback (default: the fallback model's own id)")
	fs.DurationVar(&budget, "latency-budget", 0, "time to first byte before falling back, for example 3s")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s models endpoint routing <name> --fallback MODEL [--fallback-model-id ID] [--latency-budget 3s]\n  %s models endpoint routing <name> --off\n\nThe policy is off until you set it.\n\nFlags:\n", prog, prog)
		fs.PrintDefaults()
	}
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "models endpoint routing", "model name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	if *off {
		fallback, ref, budget = "", "", 0
	} else if fallback == "" {
		return reportError(stdout, stderr, jsonOut, newValidationError("--fallback is required, or pass --off"))
	}
	out, err := client.SetModelEndpointRouting(context.Background(), name, fallback, ref, budget.Milliseconds())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set routing of endpoint %q: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, out, func() {
		if fallback == "" {
			_, _ = fmt.Fprintf(stdout, "endpoint %q has no fallback\n", name)
			return
		}
		_, _ = fmt.Fprintf(stdout, "endpoint %q falls back to %q (latency budget %s)\n", name, fallback, budget)
	})
}

func runModelsEndpointCredential(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "models endpoint credential", "print the model as JSON to stdout and nothing else", stderr)
	var credEnv string
	clearCred := fs.Bool("clear", false, "remove the stored credential")
	fs.StringVar(&credEnv, "from-env", "", "environment variable holding the new API key")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s models endpoint credential <name> --from-env VAR\n  %s models endpoint credential <name> --clear\n\nFlags:\n", prog, prog)
		fs.PrintDefaults()
	}
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "models endpoint credential", "model name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	if *clearCred == (credEnv != "") {
		return reportError(stdout, stderr, jsonOut, newValidationError("pass exactly one of --from-env or --clear"))
	}
	cred, err := credentialFromEnv(credEnv, lookupEnv)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, err)
	}
	out, err := client.SetModelEndpointCredential(context.Background(), name, cred)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set credential of endpoint %q: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, out, func() {
		_, _ = fmt.Fprintf(stdout, "endpoint %q credential updated\n", name)
		printEndpointHealth(stdout, out.External)
	})
}
