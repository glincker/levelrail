package main

import (
	"fmt"
	"io"
)

// runModels dispatches "models <verb>" for AI model resources on GPU nodes.
func runModels(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, modelsUsage(prog))
		return exitUsage
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, modelsUsage(prog))
		return exitOK
	case "list":
		return runModelsList(prog, rest, stdout, stderr, lookupEnv)
	case "get":
		return runModelsGet(prog, rest, stdout, stderr, lookupEnv)
	case "deploy":
		return runModelsDeploy(prog, rest, stdout, stderr, lookupEnv)
	case "logs":
		return runModelsLogs(prog, rest, stdout, stderr, lookupEnv)
	case "delete":
		return runModelsDelete(prog, rest, stdout, stderr, lookupEnv)
	case "restart":
		return runModelsRestart(prog, rest, stdout, stderr, lookupEnv)
	case "rotate-key":
		return runModelsRotateKey(prog, rest, stdout, stderr, lookupEnv)
	case "keys":
		return runModelsKeys(prog, rest, stdout, stderr, lookupEnv)
	case "usage":
		return runModelsUsage(prog, rest, stdout, stderr, lookupEnv)
	case "gpus":
		return runModelsGPUs(prog, rest, stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown models subcommand %q\n\n", prog, sub)
		_, _ = fmt.Fprint(stderr, modelsUsage(prog))
		return exitUsage
	}
}

func modelsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s models list [flags]                 list AI models
  %[1]s models get <name> [flags]           show one model, its status and endpoint
  %[1]s models deploy [flags]               deploy a model on a GPU node (prints its API key once)
  %[1]s models logs <name> [flags]          search or follow a model engine's logs
  %[1]s models delete <name> [flags]        remove a model (its downloaded weights volume is kept)
  %[1]s models restart <name> [flags]       recreate the engine container
  %[1]s models rotate-key <name> [flags]    issue a new API key (prints it once)
  %[1]s models keys list|create|revoke|rotate   named API keys with limits, expiry and rotation grace
  %[1]s models usage <name> [flags]         gateway requests, tokens, errors and latency per key
  %[1]s models gpus [flags]                 list GPU nodes with VRAM and usage

Run "%[1]s models <subcommand> -h" for a subcommand's own flags.
`, prog)
}
