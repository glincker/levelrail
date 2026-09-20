package main

import (
	"fmt"
	"io"
)

// runTags dispatches "tags <verb> [flags]" to one of list/create/delete,
// the CLI counterpart of internal/api/tags.go's own /api/v1/tags routes.
// Attaching/detaching a tag to/from an app lives under "apps tag"/"apps
// untag" instead (apps_tag.go), the same per-app-scoping
// apps_alerts.go's own routes establish for a different child resource.
func runTags(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(stderr, tagsUsage(prog))
		return exitUsage
	}

	switch args[0] {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, tagsUsage(prog))
		return exitOK
	case "list":
		return runTagsList(prog, args[1:], stdout, stderr, lookupEnv)
	case "create":
		return runTagsCreate(prog, args[1:], stdout, stderr, lookupEnv)
	case "delete":
		return runTagsDelete(prog, args[1:], stdout, stderr, lookupEnv)
	case "apps":
		return runTagsApps(prog, args[1:], stdout, stderr, lookupEnv)
	default:
		_, _ = fmt.Fprintf(stderr, "%s: unknown tags subcommand %q\n\n", prog, args[0])
		_, _ = fmt.Fprint(stderr, tagsUsage(prog))
		return exitUsage
	}
}

func tagsUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s tags list [flags]              list every tag
  %[1]s tags create --name NAME [flags]   create a tag
  %[1]s tags delete <id> [flags]          delete a tag (detaches it from every app it was on)
  %[1]s tags apps <id> [flags]            list every app attached to a tag

Tag/untag an individual app with "%[1]s apps tag <name> <tag>" and
"%[1]s apps untag <name> <tag-id>".

Run "%[1]s tags <subcommand> -h" for a subcommand's own flags.
`, prog)
}
