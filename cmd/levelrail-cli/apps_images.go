package main

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"
)

// runAppsImages implements "apps images <name>": GET
// /api/v1/apps/{name}/images, every locally-present tag under name's
// current image's repo, newest first. Always succeeds with an empty
// list rather than an error when nothing is found
// (internal/api/images.go's handleListImages own doc comment), so an
// empty result here means "nothing to suggest," not a failure.
func runAppsImages(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP := apiFlagSet(prog, "apps images", "print image tags as a JSON array to stdout and nothing else", stderr)
	fs.Usage = func() { _, _ = fmt.Fprint(stderr, appsImagesUsage(prog)) }

	client, name, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{tokenFlagP, apiURLFlagP, profileFlagP, jsonOutP, outputFlagP, queryFlagP}, stderr, singleArgCmd{prog, "apps images", "app name"}, lookupEnv)
	if !ok {
		return exitCode
	}

	images, err := client.ListAppImages(context.Background(), name)
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list images for app %q: %w", name, err))
	}

	if err := renderResult(stdout, of.Format, of.Query, images, func() { printAppImagesTable(stdout, images) }); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return exitCodeForError(err)
	}
	return exitOK
}

func printAppImagesTable(out io.Writer, images []imageResource) {
	if len(images) == 0 {
		_, _ = fmt.Fprintln(out, "no images")
		return
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "TAG\tCREATED")
	for _, img := range images {
		_, _ = fmt.Fprintf(tw, "%s\t%s\n", img.Tag, img.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))
	}
	_ = tw.Flush()
}

func appsImagesUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps images <name> [flags]

Lists every locally-present tag under <name>'s current image's repo,
newest first: the same suggestions the web dashboard's deploy trigger
form offers, useful for finding an exact tag to pass to "%[1]s apps
deploy" or "%[1]s apps rollback".

Flags:
  --token string          API token (default: %[2]s env var, then the credentials file)
  --api-url string       control plane base URL (default: %[3]s env var, then %[4]s)
  --profile string       named credentials profile to read (overrides APP_PROFILE, default "default")
  --json                    print image tags as a JSON array to stdout, nothing else
  --output string          output format: json, table, or text (default table; --json is shorthand for --output json)
  --query string           JMESPath expression to filter the result before printing
  -h, --help               show this help
`, prog, envAPIToken, envAPIURL, defaultAPIURL)
}
