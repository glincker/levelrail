package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func runModelsList(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "models list", "print models as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s models list [flags]\n\nLists every AI model with its status.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	list, err := client.ListModels(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list models: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, list, func() { printModelsTable(stdout, list) })
}

func runModelsGet(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "models get", "print the model as JSON to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s models get <name> [flags]\n\nShows one model, its status and its OpenAI-compatible base URL.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "models get", "model name"}, lookupEnv)
	if !ok {
		return exitCode
	}
	m, err := client.GetModel(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("get model %q: %w", name, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, m, func() { printModelHuman(stdout, m) })
}

func runModelsGPUs(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "models gpus", "print GPU nodes as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s models gpus [flags]\n\nLists nodes that report an NVIDIA GPU: driver, VRAM, usage and whether\nDocker has the nvidia runtime (with the fix when it does not).\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	nodes, err := client.ListGPUNodes(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list gpu nodes: %w", err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, nodes, func() { printGPUNodesTable(stdout, nodes) })
}

func modelNodeLabel(nodeID string) string {
	if nodeID == "" {
		return "local"
	}
	return nodeID
}

func printModelsTable(out io.Writer, list []apiclient.ModelResource) {
	if len(list) == 0 {
		_, _ = fmt.Fprintln(out, "no models deployed")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tENGINE\tMODEL\tNODE\tSTATUS\tDETAIL")
	for _, m := range list {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", m.Name, m.Engine, m.Model, modelNodeLabel(m.NodeID), m.Status.Reason, m.Status.Message)
	}
	_ = tw.Flush()
}

func printModelHuman(out io.Writer, m apiclient.ModelResource) {
	_, _ = fmt.Fprintf(out, "name:        %s\n", m.Name)
	_, _ = fmt.Fprintf(out, "engine:      %s\n", m.Engine)
	_, _ = fmt.Fprintf(out, "model:       %s\n", m.Model)
	_, _ = fmt.Fprintf(out, "node:        %s\n", modelNodeLabel(m.NodeID))
	_, _ = fmt.Fprintf(out, "status:      %s (ready=%t)\n", m.Status.Reason, m.Status.Ready)
	if m.Status.Message != "" {
		_, _ = fmt.Fprintf(out, "detail:      %s\n", m.Status.Message)
	}
	if m.EndpointURL != "" {
		_, _ = fmt.Fprintf(out, "base url:    %s\n", m.EndpointURL)
	}
	_, _ = fmt.Fprintf(out, "api key:     %s... (shown once at deploy time)\n", m.APIKeyPrefix)
}

func printGPUNodesTable(out io.Writer, nodes []apiclient.GPUNodeResource) {
	if len(nodes) == 0 {
		_, _ = fmt.Fprintln(out, "no node has reported an NVIDIA GPU")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NODE\tGPUS\tVRAM USED/TOTAL\tDRIVER\tRUNTIME\tMODELS")
	for _, n := range nodes {
		runtime := "ok"
		if n.Present && !n.RuntimeInstalled {
			runtime = "nvidia runtime missing"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%d\t%d/%d MiB\t%s\t%s\t%d\n", n.Name, n.GPUCount, n.UsedVRAMMiB, n.TotalVRAMMiB, n.DriverVersion, runtime, n.ModelCount)
	}
	_ = tw.Flush()
	for _, n := range nodes {
		if n.Hint != "" {
			_, _ = fmt.Fprintf(out, "\n%s: %s\n", n.Name, n.Hint)
		}
	}
}
