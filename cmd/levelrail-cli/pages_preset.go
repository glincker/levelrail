package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
)

const pagesPresetUsage = "Usage:\n  %[1]s pages preset list [flags]\n  %[1]s pages preset show <name> [flags]\n  %[1]s pages preset set <name> <preset|none> [flags]\n"

func runPagesPreset(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 || (args[0] != "list" && args[0] != "show" && args[0] != "set") {
		_, _ = fmt.Fprintf(stderr, pagesPresetUsage, prog)
		return exitUsage
	}
	sub := args[0]
	fs, tokenP, apiURLP, profileP, jsonP, outputP, queryP := apiFlagSet(prog, "pages preset "+sub, "print the result as JSON to stdout and nothing else", stderr)
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args[1:], apiFlagPtrs{tokenP, apiURLP, profileP, jsonP, outputP, queryP}, prog, stderr)
	if !ok {
		return exitCode
	}
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	rest := fs.Args()
	switch sub {
	case "list":
		res, err := client.ListPagesPresets(context.Background())
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("list presets: %w", err))
		}
		return renderOrFail(stdout, stderr, of, res, func() {
			tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "ID\tNAME\tDESCRIPTION")
			for _, p := range res {
				_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", p.ID, p.Name, p.Description)
			}
			_ = tw.Flush()
		})
	case "show":
		if len(rest) != 1 {
			_, _ = fmt.Fprintf(stderr, pagesPresetUsage, prog)
			return exitUsage
		}
		res, err := client.GetStaticSitePreset(context.Background(), rest[0])
		if err != nil {
			return reportError(stdout, stderr, jsonOut, fmt.Errorf("get preset of %q: %w", rest[0], err))
		}
		return renderOrFail(stdout, stderr, of, res, func() { _, _ = fmt.Fprintln(stdout, presetLabel(res.Preset)) })
	}
	if len(rest) != 2 {
		_, _ = fmt.Fprintf(stderr, pagesPresetUsage, prog)
		return exitUsage
	}
	want := rest[1]
	if want == "none" {
		want = ""
	}
	res, err := client.SetStaticSitePreset(context.Background(), rest[0], want)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("set preset of %q: %w", rest[0], err))
	}
	return renderOrFail(stdout, stderr, of, res, func() { _, _ = fmt.Fprintf(stdout, "%s preset: %s\n", res.Name, presetLabel(res.Preset)) })
}

func presetLabel(id string) string {
	if id == "" {
		return "none"
	}
	return id
}
