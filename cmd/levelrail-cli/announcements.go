package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/glincker/levelrail/internal/apiclient"
)

const announcementsUsage = "Usage:\n  %[1]s announcements ls [--all] [--json]\n  %[1]s announcements read <id>\n  %[1]s announcements dismiss <id>\n  %[1]s announcements snooze <id> --for <duration>\n"

// runAnnouncements dispatches the signed announcement feed commands.
func runAnnouncements(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintf(stderr, announcementsUsage, prog)
		return exitUsage
	}
	switch args[0] {
	case "ls", "list":
		return runAnnouncementsLs(prog, args[1:], stdout, stderr, lookupEnv)
	case "read":
		return runAnnouncementsMark(prog, "read", args[1:], stdout, stderr, lookupEnv)
	case "dismiss":
		return runAnnouncementsMark(prog, "dismiss", args[1:], stdout, stderr, lookupEnv)
	case "snooze":
		return runAnnouncementsMark(prog, "snooze", args[1:], stdout, stderr, lookupEnv)
	}
	_, _ = fmt.Fprintf(stderr, "unknown announcements command %q\n", args[0])
	return exitUsage
}

func runAnnouncementsLs(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, t, a, p, j, o, q := apiFlagSet(prog, "announcements ls", "print the announcements as JSON to stdout and nothing else", stderr)
	all := fs.Bool("all", false, "include dismissed and snoozed announcements")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s announcements ls [--all] [flags]\n\nLists verified announcements for this instance, most urgent first as the server orders them.\n\nFlags:\n", prog)
		fs.PrintDefaults()
	}
	client, jsonOut, of, exitCode, ok := parseListClient(fs, args, apiFlagPtrs{t, a, p, j, o, q}, prog, stderr, lookupEnv)
	if !ok {
		return exitCode
	}
	list, err := client.ListAnnouncements(context.Background())
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("list announcements: %w", err))
	}
	rows := []apiclient.Announcement{}
	now := time.Now()
	for _, it := range list.Items {
		if *all || (!it.Dismissed && (it.SnoozedUntil == nil || !it.SnoozedUntil.After(now))) {
			rows = append(rows, it)
		}
	}
	list.Items = rows
	return writeScheduledTaskResult(stdout, stderr, of, list, func() { printAnnouncements(stdout, list) })
}

func printAnnouncements(w io.Writer, list apiclient.AnnouncementList) {
	if len(list.Items) == 0 {
		_, _ = fmt.Fprintf(w, "No announcements (feed mode %s).\n", list.Status.Mode)
		return
	}
	for _, it := range list.Items {
		state := "unread"
		switch {
		case it.Dismissed:
			state = "dismissed"
		case it.SnoozedUntil != nil:
			state = "snoozed until " + it.SnoozedUntil.Format("2006-01-02 15:04")
		case it.Read:
			state = "read"
		}
		_, _ = fmt.Fprintf(w, "%-10s %s  %s\n", it.Tier, it.IssuedAt.Format("2006-01-02"), it.Title)
		_, _ = fmt.Fprintf(w, "           id %s  expires %s  %s\n", it.ID, it.ExpiresAt.Format("2006-01-02"), state)
	}
}

func runAnnouncementsMark(prog, verb string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	fs, t, a, p, j, o, q := apiFlagSet(prog, "announcements "+verb, "print the result as JSON to stdout and nothing else", stderr)
	var forFlag *string
	if verb == "snooze" {
		forFlag = fs.String("for", "", "how long to hide the announcement, for example 4h (the server caps it)")
	}
	fs.Usage = func() {
		_, _ = fmt.Fprintf(stderr, "Usage:\n  %s announcements %s <id> [flags]\n\nFlags:\n", prog, verb)
		fs.PrintDefaults()
	}
	client, id, jsonOut, of, exitCode, ok := parseSingleArgClient(fs, args, apiFlagPtrs{t, a, p, j, o, q}, stderr, singleArgCmd{prog, "announcements " + verb, "announcement id"}, lookupEnv)
	if !ok {
		return exitCode
	}
	ctx := context.Background()
	var err error
	switch verb {
	case "read":
		err = client.MarkAnnouncementRead(ctx, id)
	case "dismiss":
		err = client.DismissAnnouncement(ctx, id)
	case "snooze":
		if forFlag == nil || *forFlag == "" {
			_, _ = fmt.Fprintf(stderr, "announcements snooze needs --for <duration>, for example --for 4h\n")
			return exitUsage
		}
		if _, perr := time.ParseDuration(*forFlag); perr != nil {
			_, _ = fmt.Fprintf(stderr, "--for must be a duration such as 4h: %v\n", perr)
			return exitUsage
		}
		err = client.SnoozeAnnouncement(ctx, id, *forFlag)
	}
	var apiErr *apiclient.APIError
	if verb == "dismiss" && errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusConflict {
		return reportError(stdout, stderr, jsonOut, errors.New(criticalDismissMessage(ctx, client, id)))
	}
	if err != nil {
		return reportError(stdout, stderr, jsonOut, fmt.Errorf("announcements %s %q: %w", verb, id, err))
	}
	return writeScheduledTaskResult(stdout, stderr, of, map[string]any{"id": id, "action": verb, "ok": true}, func() {
		_, _ = fmt.Fprintf(stdout, "announcement %s: %s\n", id, verb)
	})
}

// criticalDismissMessage names the affected versions when the notice still lists them.
func criticalDismissMessage(ctx context.Context, client *apiclient.Client, id string) string {
	msg := fmt.Sprintf("critical notice %s cannot be dismissed until you upgrade", id)
	list, err := client.ListAnnouncements(ctx)
	if err != nil {
		return msg
	}
	for _, it := range list.Items {
		if it.ID == id && it.AppliesTo != "" {
			return fmt.Sprintf("%s to a version outside %s (snooze it with 'announcements snooze %s --for 4h' meanwhile)", msg, it.AppliesTo, id)
		}
	}
	return msg
}
