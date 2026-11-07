package main

import (
	"context"
	"fmt"
	"io"
	"strings"
)

func runTemplatesSearch(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "templates search", "print the matches as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s templates search <term>... [flags]\n\nLists catalog entries whose id, name, category or slogan contain every term.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	tokenFlag, apiURLFlag, profileFlag, jsonOut, of, exitCode, ok := parseAPIFlags(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, prog, stderr)
	if !ok {
		return exitCode
	}
	terms := strings.Fields(strings.ToLower(strings.Join(fs.Args(), " ")))
	if len(terms) == 0 {
		fs.Usage()
		return exitUsage
	}

	client := apiClientFromFlags(prog, apiURLFlag, tokenFlag, profileFlag, lookupEnv)
	all, err := client.ListServiceTemplates(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("search service templates: %w", err))
	}
	var found []serviceTemplateListItem
	for _, t := range all {
		hay := strings.ToLower(t.ID + " " + t.Name + " " + t.Category + " " + t.Slogan)
		hit := true
		for _, term := range terms {
			if !strings.Contains(hay, term) {
				hit = false
				break
			}
		}
		if hit {
			found = append(found, t)
		}
	}
	return writeScheduledTaskResult(stdout, stderr, of, found, func() { printTemplatesTable(stdout, found) })
}
