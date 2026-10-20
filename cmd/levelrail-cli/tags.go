package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
)

// runTags dispatches list/create/delete for /api/v1/tags. Attaching a tag to an app lives under "apps tag".
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
  %[1]s tags delete <name> [flags]        delete a tag by name (detaches it from every app it was on)
  %[1]s tags apps <name> [flags]          list every app attached to a tag, by tag name

Every tag identifier this CLI's flags/arguments take is the tag's name,
never its numeric/opaque ID; commands that need the ID internally
resolve it from the name for you.

Tag/untag an individual app with "%[1]s apps tag <name> <tag>" and
"%[1]s apps untag <name> <tag>".

Run "%[1]s tags <subcommand> -h" for a subcommand's own flags.
`, prog)
}

// resolveTagIDByName looks up a tag's ID by name, so users never have to type IDs by hand.
func resolveTagIDByName(ctx context.Context, client *Client, name string) (string, error) {
	tags, err := client.ListTags(ctx)
	if err != nil {
		return "", err
	}
	for _, t := range tags {
		if t.Name == name {
			return t.ID, nil
		}
	}
	return "", &apiError{StatusCode: http.StatusNotFound, Message: fmt.Sprintf("tag %q not found", name)}
}
