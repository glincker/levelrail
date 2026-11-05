package main

import (
	"context"
	"fmt"
	"io"
)

// runAppsProjectsExport implements "apps projects export <name>": every app in a
// project as one plain compose file, an env template and a volumes manifest.
func runAppsProjectsExport(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps projects export", "print the written file names as JSON", stderr)
	var out string
	var includeSecrets, force bool
	fs.StringVar(&out, "o", "", "directory to write the files into, or - for a tar stream on stdout (default ./<name>-export)")
	fs.BoolVar(&includeSecrets, "include-secrets", false, "also write a .env with stored secret values (needs write:sensitive, audited)")
	fs.BoolVar(&force, "force", false, "replace an existing .env in the output directory")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s apps projects export <name> [-o DIR|-] [--include-secrets] [--force] [flags]\n\nWrites docker-compose.yml, .env.example, volumes.json and README.md. Secrets are\nplaceholders unless --include-secrets is set; anything not exportable is listed at\nthe top of docker-compose.yml. See docs/leave-anytime.md.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	rest := fs.Args()
	if len(rest) != 1 {
		_, _ = fmt.Fprintf(stderr, "%s: apps projects export requires exactly one project name\n\n", prog)
		fs.Usage()
		return exitUsage
	}
	name := rest[0]
	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	// The team gate looks {id} up as an ID only, so resolve a name first.
	id := resolveProjectID(client, name)
	data, err := client.ExportProject(context.Background(), id, includeSecrets)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("export project %q: %w", name, err))
	}
	if out == "-" {
		if _, err := stdout.Write(data); err != nil {
			return reportError(stdout, stderr, jsonOut, err)
		}
		return exitOK
	}
	if out == "" {
		out = name + "-export"
	}
	written, err := extractExport(data, out, force)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("write export: %w", err))
	}
	if err := renderResult(stdout, of.Format, of.Query, written, func() {
		for _, f := range written {
			_, _ = fmt.Fprintln(stdout, f)
		}
		_, _ = fmt.Fprintf(stdout, "next: cd %s && cp .env.example .env && docker compose up -d\n", out)
	}); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

// resolveProjectID maps a unique project name to its ID, and passes anything
// else through so an ID or an unlisted project still reaches the server.
func resolveProjectID(client *Client, ref string) string {
	list, err := client.ListProjects(context.Background())
	if err != nil {
		return ref
	}
	id := ""
	for _, p := range list {
		if p.ID == ref {
			return ref
		}
		if p.Name == ref {
			if id != "" {
				return ref
			}
			id = p.ID
		}
	}
	if id == "" {
		return ref
	}
	return id
}
